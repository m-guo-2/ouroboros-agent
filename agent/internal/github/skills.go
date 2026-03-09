package github

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// SkillData represents a skill stored in the GitHub repo.
type SkillData struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	Type        string                 `json:"type"`
	Enabled     bool                   `json:"enabled"`
	Triggers    []interface{}          `json:"triggers"`
	Tools       []interface{}          `json:"tools"`
	Readme      string                 `json:"readme"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// skillManifest is the JSON structure written to manifest.json (no readme).
type skillManifest struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	Type        string                 `json:"type"`
	Enabled     bool                   `json:"enabled"`
	Triggers    []interface{}          `json:"triggers"`
	Tools       []interface{}          `json:"tools"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type cachedEntry struct {
	SkillData
	manifestSHA string
	readmeSHA   string
}

const defaultBasePath = "skills"

// Store manages skills in a GitHub repository with an in-memory cache.
//
// Repo layout:
//
//	{basePath}/{skill-id}/manifest.json   — all fields except readme
//	{basePath}/{skill-id}/README.md       — readme content
type Store struct {
	client   *Client
	basePath string

	mu    sync.RWMutex
	cache map[string]*cachedEntry
}

// DefaultStore is the package-level singleton, set by InitStore.
var DefaultStore *Store

// InitStore creates the skill store from environment variables and loads the cache.
func InitStore() error {
	client, err := NewClientFromEnv()
	if err != nil {
		return err
	}
	base := os.Getenv("GITHUB_SKILLS_PATH")
	if base == "" {
		base = defaultBasePath
	}
	s := &Store{
		client:   client,
		basePath: strings.TrimSuffix(base, "/"),
		cache:    make(map[string]*cachedEntry),
	}
	if err := s.refresh(); err != nil {
		return fmt.Errorf("initial cache load: %w", err)
	}
	DefaultStore = s
	log.Printf("📦 GitHub skill store initialized: %d skills loaded", len(s.cache))
	return nil
}

func (s *Store) skillDir(id string) string    { return s.basePath + "/" + id }
func (s *Store) manifestPath(id string) string { return s.skillDir(id) + "/manifest.json" }
func (s *Store) readmePath(id string) string   { return s.skillDir(id) + "/README.md" }

// refresh reloads every skill from the GitHub repo into memory.
func (s *Store) refresh() error {
	entries, err := s.client.ListDir(s.basePath)
	if err != nil {
		if IsNotFound(err) {
			s.mu.Lock()
			s.cache = make(map[string]*cachedEntry)
			s.mu.Unlock()
			return nil
		}
		return err
	}

	next := make(map[string]*cachedEntry, len(entries))
	for _, e := range entries {
		if e.Type != "dir" {
			continue
		}
		id := e.Name

		content, sha, err := s.client.GetFileContent(s.manifestPath(id))
		if err != nil {
			log.Printf("⚠️  skip skill %s: %s", id, err)
			continue
		}
		var m skillManifest
		if err := json.Unmarshal([]byte(content), &m); err != nil {
			log.Printf("⚠️  skip skill %s: invalid manifest: %s", id, err)
			continue
		}

		entry := &cachedEntry{
			SkillData: SkillData{
				ID:          m.ID,
				Name:        m.Name,
				Description: m.Description,
				Version:     m.Version,
				Type:        m.Type,
				Enabled:     m.Enabled,
				Triggers:    m.Triggers,
				Tools:       m.Tools,
				Metadata:    m.Metadata,
			},
			manifestSHA: sha,
		}
		if entry.Triggers == nil {
			entry.Triggers = []interface{}{}
		}
		if entry.Tools == nil {
			entry.Tools = []interface{}{}
		}

		readme, rSHA, err := s.client.GetFileContent(s.readmePath(id))
		if err == nil {
			entry.Readme = readme
			entry.readmeSHA = rSHA
		}

		next[id] = entry
	}

	s.mu.Lock()
	s.cache = next
	s.mu.Unlock()
	return nil
}

// GetAll returns a copy of all cached skills.
func (s *Store) GetAll() []SkillData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]SkillData, 0, len(s.cache))
	for _, e := range s.cache {
		out = append(out, e.SkillData)
	}
	return out
}

// GetByID returns a skill by ID, or nil if not found.
func (s *Store) GetByID(id string) *SkillData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.cache[id]; ok {
		d := e.SkillData
		return &d
	}
	return nil
}

// GetByName returns the first skill with the given name, or nil.
func (s *Store) GetByName(name string) *SkillData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.cache {
		if e.Name == name {
			d := e.SkillData
			return &d
		}
	}
	return nil
}

// Create writes a new skill to the repo and refreshes the cache.
func (s *Store) Create(skill SkillData) (*SkillData, error) {
	if skill.ID == "" {
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		skill.ID = fmt.Sprintf("skill-%x", b)
	}
	defaults(&skill)

	msg := fmt.Sprintf("create skill: %s", skill.Name)
	if err := s.writeFiles(skill, "", "", msg); err != nil {
		return nil, err
	}
	if err := s.refresh(); err != nil {
		return nil, fmt.Errorf("refresh after create: %w", err)
	}
	return s.GetByID(skill.ID), nil
}

// Update applies a partial update to an existing skill and refreshes the cache.
func (s *Store) Update(id string, updates map[string]interface{}) (*SkillData, error) {
	s.mu.RLock()
	entry, ok := s.cache[id]
	if !ok {
		s.mu.RUnlock()
		return nil, fmt.Errorf("skill not found: %s", id)
	}
	skill := entry.SkillData
	mSHA := entry.manifestSHA
	rSHA := entry.readmeSHA
	s.mu.RUnlock()

	readmeChanged := applyUpdates(&skill, updates)

	msg := fmt.Sprintf("update skill: %s", skill.Name)
	if err := s.writeFiles(skill, mSHA, rSHA, msg); err != nil {
		return nil, err
	}
	_ = readmeChanged // writeFiles always writes README if content is non-empty

	if err := s.refresh(); err != nil {
		return nil, fmt.Errorf("refresh after update: %w", err)
	}
	return s.GetByID(id), nil
}

// Delete removes a skill's directory from the repo and refreshes the cache.
func (s *Store) Delete(id string) error {
	entries, err := s.client.ListDir(s.skillDir(id))
	if err != nil {
		if IsNotFound(err) {
			return nil
		}
		return err
	}

	s.mu.RLock()
	name := id
	if e, ok := s.cache[id]; ok {
		name = e.Name
	}
	s.mu.RUnlock()

	for _, e := range entries {
		if e.Type != "file" {
			continue
		}
		msg := fmt.Sprintf("delete skill %s: %s", name, e.Name)
		if err := s.client.DeleteFile(e.Path, msg, e.SHA); err != nil {
			return fmt.Errorf("delete %s: %w", e.Path, err)
		}
	}

	if err := s.refresh(); err != nil {
		return fmt.Errorf("refresh after delete: %w", err)
	}
	return nil
}

// writeFiles writes manifest.json and README.md for a skill.
func (s *Store) writeFiles(skill SkillData, mSHA, rSHA, msg string) error {
	m := skillManifest{
		ID: skill.ID, Name: skill.Name, Description: skill.Description,
		Version: skill.Version, Type: skill.Type, Enabled: skill.Enabled,
		Triggers: skill.Triggers, Tools: skill.Tools, Metadata: skill.Metadata,
	}
	data, _ := json.MarshalIndent(m, "", "  ")

	if err := s.client.PutFile(s.manifestPath(skill.ID), msg, string(data), mSHA); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	if skill.Readme != "" || rSHA != "" {
		if err := s.client.PutFile(s.readmePath(skill.ID), msg, skill.Readme, rSHA); err != nil {
			return fmt.Errorf("write readme: %w", err)
		}
	}
	return nil
}

func defaults(s *SkillData) {
	if s.Version == "" {
		s.Version = "1.0.0"
	}
	if s.Type == "" {
		s.Type = "knowledge"
	}
	if s.Triggers == nil {
		s.Triggers = []interface{}{}
	}
	if s.Tools == nil {
		s.Tools = []interface{}{}
	}
}

func applyUpdates(s *SkillData, updates map[string]interface{}) bool {
	readmeChanged := false
	for key, val := range updates {
		switch key {
		case "name":
			if v, ok := val.(string); ok {
				s.Name = v
			}
		case "description":
			if v, ok := val.(string); ok {
				s.Description = v
			}
		case "version":
			if v, ok := val.(string); ok {
				s.Version = v
			}
		case "type":
			if v, ok := val.(string); ok {
				s.Type = v
			}
		case "readme":
			if v, ok := val.(string); ok {
				s.Readme = v
				readmeChanged = true
			}
		case "enabled":
			if v, ok := val.(bool); ok {
				s.Enabled = v
			}
		case "triggers":
			if v, ok := val.([]interface{}); ok {
				s.Triggers = v
			}
		case "tools":
			if v, ok := val.([]interface{}); ok {
				s.Tools = v
			}
		case "metadata":
			if v, ok := val.(map[string]interface{}); ok {
				s.Metadata = v
			}
		}
	}
	return readmeChanged
}
