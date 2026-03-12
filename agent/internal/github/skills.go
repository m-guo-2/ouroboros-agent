package github

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agent/internal/config"
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
	References  []string               `json:"references,omitempty"`
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
	ready atomic.Bool

	cancel context.CancelFunc
}

// DefaultStore is the package-level singleton, set by NewStore.
var DefaultStore *Store

// NewStore creates the skill store and sets DefaultStore, but does NOT load
// skills from GitHub. Call Store.LoadCache afterwards (typically in a goroutine)
// to perform the initial network fetch.
func NewStore(gh config.GitHub) error {
	client, err := NewClientFromConfig(gh)
	if err != nil {
		return err
	}
	base := gh.SkillsPath
	if base == "" {
		base = defaultBasePath
	}
	DefaultStore = &Store{
		client:   client,
		basePath: strings.TrimSuffix(base, "/"),
		cache:    make(map[string]*cachedEntry),
	}
	return nil
}

// LoadCache performs the initial skill fetch from GitHub. Safe to call from a
// goroutine. After it returns successfully the store is ready to serve data.
func (s *Store) LoadCache() error {
	if err := s.refresh(); err != nil {
		return fmt.Errorf("initial cache load: %w", err)
	}
	s.ready.Store(true)
	log.Printf("📦 GitHub skill store ready: %d skills loaded", len(s.cache))
	return nil
}

// Ready reports whether the initial cache load has completed.
func (s *Store) Ready() bool {
	return s.ready.Load()
}

func (s *Store) skillDir(id string) string    { return s.basePath + "/" + id }
func (s *Store) manifestPath(id string) string { return s.skillDir(id) + "/manifest.json" }
func (s *Store) readmePath(id string) string   { return s.skillDir(id) + "/README.md" }
func (s *Store) skillMdPath(id string) string  { return s.skillDir(id) + "/SKILL.md" }
func (s *Store) refsDir(id string) string      { return s.skillDir(id) + "/references" }

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

		entry, err := s.loadSkillEntry(id)
		if err != nil {
			log.Printf("⚠️  skip skill %s: %s", id, err)
			continue
		}
		next[id] = entry
	}

	s.mu.Lock()
	s.cache = next
	s.mu.Unlock()
	return nil
}

// loadSkillEntry loads a single skill's metadata, readme, and reference index.
// It tries manifest.json first, then falls back to SKILL.md (frontmatter + body).
func (s *Store) loadSkillEntry(id string) (*cachedEntry, error) {
	entry := &cachedEntry{}

	content, sha, err := s.client.GetFileContent(s.manifestPath(id))
	if err == nil {
		var m skillManifest
		if err := json.Unmarshal([]byte(content), &m); err != nil {
			return nil, fmt.Errorf("invalid manifest: %w", err)
		}
		entry.SkillData = SkillData{
			ID: m.ID, Name: m.Name, Description: m.Description,
			Version: m.Version, Type: m.Type, Enabled: m.Enabled,
			Triggers: m.Triggers, Tools: m.Tools, Metadata: m.Metadata,
		}
		entry.manifestSHA = sha

		readme, rSHA, err := s.client.GetFileContent(s.readmePath(id))
		if err == nil {
			entry.Readme = readme
			entry.readmeSHA = rSHA
		}
	} else if IsNotFound(err) {
		if err := s.loadFromSkillMd(id, entry); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	if entry.ID == "" {
		entry.ID = id
	}
	if entry.Triggers == nil {
		entry.Triggers = []interface{}{}
	}
	if entry.Tools == nil {
		entry.Tools = []interface{}{}
	}

	entry.References = s.listReferences(id)

	return entry, nil
}

// loadFromSkillMd parses a SKILL.md file (YAML frontmatter + markdown body)
// as a fallback when manifest.json does not exist.
func (s *Store) loadFromSkillMd(id string, entry *cachedEntry) error {
	content, _, err := s.client.GetFileContent(s.skillMdPath(id))
	if err != nil {
		return fmt.Errorf("no manifest.json or SKILL.md: %w", err)
	}

	fm, body := splitFrontmatter(content)

	entry.SkillData = SkillData{
		ID:      id,
		Version: "1.0.0",
		Type:    "knowledge",
		Enabled: true,
	}
	entry.Readme = body

	if fm != "" {
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(frontmatterToJSON(fm)), &meta); err == nil {
			if v, ok := meta["name"].(string); ok {
				entry.Name = v
			}
			if v, ok := meta["description"].(string); ok {
				entry.Description = v
			}
			if v, ok := meta["type"].(string); ok {
				entry.Type = v
			}
		}
	}
	if entry.Name == "" {
		entry.Name = id
	}
	return nil
}

// listReferences returns the file names in the skill's references/ directory.
// Returns nil if the directory does not exist.
func (s *Store) listReferences(id string) []string {
	entries, err := s.client.ListDir(s.refsDir(id))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.Type == "file" {
			names = append(names, e.Name)
		}
	}
	return names
}

// GetReference fetches a specific reference file's content on demand.
func (s *Store) GetReference(skillID, refName string) (string, error) {
	path := s.refsDir(skillID) + "/" + refName
	content, _, err := s.client.GetFileContent(path)
	if err != nil {
		return "", fmt.Errorf("reference %s/%s: %w", skillID, refName, err)
	}
	return content, nil
}

// splitFrontmatter separates YAML frontmatter (between --- delimiters) from body.
func splitFrontmatter(content string) (frontmatter, body string) {
	const delim = "---"
	if !strings.HasPrefix(strings.TrimSpace(content), delim) {
		return "", content
	}
	trimmed := strings.TrimSpace(content)
	rest := trimmed[len(delim):]
	idx := strings.Index(rest, delim)
	if idx < 0 {
		return "", content
	}
	return strings.TrimSpace(rest[:idx]), strings.TrimSpace(rest[idx+len(delim):])
}

// frontmatterToJSON does a minimal conversion of simple YAML key-value pairs
// to JSON. Handles single-line string values and quoted JSON values.
func frontmatterToJSON(fm string) string {
	lines := strings.Split(fm, "\n")
	pairs := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if val == "" {
			continue
		}
		// If value looks like JSON object/array, use as-is
		if (strings.HasPrefix(val, "{") && strings.HasSuffix(val, "}")) ||
			(strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]")) {
			pairs = append(pairs, fmt.Sprintf("%q: %s", key, val))
		} else {
			pairs = append(pairs, fmt.Sprintf("%q: %q", key, val))
		}
	}
	return "{" + strings.Join(pairs, ", ") + "}"
}

// Refresh reloads skills from the GitHub repo. Safe for concurrent use.
func (s *Store) Refresh() error {
	return s.refresh()
}

// StartSync begins periodic background refresh at the given interval.
// It is a no-op if interval <= 0.
func (s *Store) StartSync(interval time.Duration) {
	if interval <= 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.refresh(); err != nil {
					log.Printf("⚠️  skill sync failed: %s", err)
				} else {
					s.mu.RLock()
					n := len(s.cache)
					s.mu.RUnlock()
					log.Printf("🔄 skill sync complete: %d skills", n)
				}
			}
		}
	}()
	log.Printf("🔄 skill sync started: interval %s", interval)
}

// StopSync stops the background sync goroutine. Safe to call if sync was never started.
func (s *Store) StopSync() {
	if s.cancel != nil {
		s.cancel()
	}
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
