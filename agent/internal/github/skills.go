package github

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agent/internal/config"
)

// SkillData represents a skill stored in the GitHub repo.
// Standard structure: SKILL.md (frontmatter + body) + scripts/ + references/
type SkillData struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Enabled     bool     `json:"enabled"`
	Readme      string   `json:"readme"`
	Scripts     []string `json:"scripts,omitempty"`
	References  []string `json:"references,omitempty"`
	BasePath    string   `json:"-"` // local disk path, not serialized
	SourceSHA   string   `json:"-"` // source SKILL.md sha for local sync metadata
}

type cachedEntry struct {
	SkillData
	skillMdSHA string
}

const defaultBasePath = "skills"
const embeddedBasePath = "agent/data/skills"

// Store manages skills in either an embedded local repository directory or a
// GitHub repository, with an in-memory cache and local disk mirror for
// scripts/references.
//
// Repo layout:
//
//	{basePath}/{skill-id}/SKILL.md        — standard: frontmatter + body
//	{basePath}/{skill-id}/manifest.json   — legacy: JSON metadata
//	{basePath}/{skill-id}/README.md       — legacy: readme content
//	{basePath}/{skill-id}/scripts/        — executable scripts
//	{basePath}/{skill-id}/references/     — reference documents
type Store struct {
	client    *Client
	basePath  string
	localDir  string // local disk mirror root
	sourceDir string // optional local repository root; bypasses GitHub API when set

	mu    sync.RWMutex
	cache map[string]*cachedEntry
	ready atomic.Bool

	cancel context.CancelFunc
}

// DefaultStore is the package-level singleton, set by NewStore.
var DefaultStore *Store

var skillSnapshotWriter func([]SkillData) error

// SetSkillSnapshotWriter installs a callback that persists the refreshed local
// skill snapshot into the runtime store.
func SetSkillSnapshotWriter(fn func([]SkillData) error) {
	skillSnapshotWriter = fn
}

// NewStore creates the skill store and sets DefaultStore, but does NOT load
// skills. Call Store.LoadCache afterwards (typically in a goroutine) to perform
// the initial fetch.
func NewStore(gh config.GitHub) error {
	base := strings.TrimSuffix(strings.TrimSpace(gh.SkillsPath), "/")
	sourceDir := strings.TrimSuffix(strings.TrimSpace(gh.SkillsSourceDir), "/")
	if sourceDir == "" && strings.TrimSpace(gh.SkillsRepo) == "" {
		if detectedSource, detectedBase, ok := discoverEmbeddedSkills(base); ok {
			sourceDir = detectedSource
			base = detectedBase
		}
	}
	if base == "" {
		base = defaultBasePath
	}
	localDir := gh.SkillsLocalDir
	if localDir == "" {
		localDir = "data/skills"
	}
	var client *Client
	if sourceDir == "" {
		var err error
		client, err = NewClientFromConfig(gh)
		if err != nil {
			return err
		}
	}
	DefaultStore = &Store{
		client:    client,
		basePath:  base,
		localDir:  localDir,
		sourceDir: sourceDir,
		cache:     make(map[string]*cachedEntry),
	}
	return nil
}

// LoadCache performs the initial skill fetch. Safe to call from a goroutine.
// After it returns successfully the store is ready to serve data.
func (s *Store) LoadCache() error {
	if err := s.refresh(); err != nil {
		return fmt.Errorf("initial cache load: %w", err)
	}
	s.ready.Store(true)
	log.Printf("📦 skill store ready: %d skills loaded", len(s.cache))
	return nil
}

// Ready reports whether the initial cache load has completed.
func (s *Store) Ready() bool {
	return s.ready.Load()
}

func (s *Store) skillDir(id string) string    { return s.basePath + "/" + id }
func (s *Store) skillMdPath(id string) string { return s.skillDir(id) + "/SKILL.md" }
func (s *Store) refsDir(id string) string     { return s.skillDir(id) + "/references" }
func (s *Store) scriptsDir(id string) string  { return s.skillDir(id) + "/scripts" }

func discoverEmbeddedSkills(configuredBase string) (sourceDir, basePath string, ok bool) {
	bases := []string{configuredBase}
	if configuredBase == "" {
		bases = []string{embeddedBasePath}
	}
	for _, base := range bases {
		base = strings.TrimSuffix(strings.TrimSpace(base), "/")
		if base == "" {
			continue
		}
		for _, root := range []string{".", ".."} {
			candidate := filepath.Join(root, base)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return root, base, true
			}
		}
	}
	return "", "", false
}

// refresh reloads every skill from the GitHub repo, syncs files to local disk,
// and updates the in-memory metadata cache.
func (s *Store) refresh() error {
	if s.sourceDir != "" {
		return s.refreshFromLocal()
	}

	entries, err := s.client.ListDir(s.basePath)
	if err != nil {
		if IsNotFound(err) {
			if err := s.pruneLocalSkills(map[string]*cachedEntry{}); err != nil {
				return err
			}
			if skillSnapshotWriter != nil {
				if err := skillSnapshotWriter([]SkillData{}); err != nil {
					return fmt.Errorf("persist empty local skill snapshot: %w", err)
				}
			}
			s.mu.Lock()
			s.cache = make(map[string]*cachedEntry)
			s.mu.Unlock()
			return nil
		}
		return err
	}

	if err := os.MkdirAll(s.localDir, 0o755); err != nil {
		return fmt.Errorf("create local skills dir: %w", err)
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

		localSkillDir := filepath.Join(s.localDir, id)
		if err := s.syncSkillToDisk(id, localSkillDir); err != nil {
			log.Printf("⚠️  disk sync failed for skill %s: %s", id, err)
			continue
		}
		entry.BasePath = localSkillDir

		next[id] = entry
	}

	if err := s.pruneLocalSkills(next); err != nil {
		return err
	}

	snapshot := make([]SkillData, 0, len(next))
	for _, entry := range next {
		snapshot = append(snapshot, entry.SkillData)
	}
	if skillSnapshotWriter != nil {
		if err := skillSnapshotWriter(snapshot); err != nil {
			return fmt.Errorf("persist local skill snapshot: %w", err)
		}
	}

	s.mu.Lock()
	s.cache = next
	s.mu.Unlock()
	return nil
}

func (s *Store) refreshFromLocal() error {
	sourceBase := s.sourcePath()
	entries, err := os.ReadDir(sourceBase)
	if err != nil {
		return fmt.Errorf("read local skills source: %w", err)
	}
	if err := os.MkdirAll(s.localDir, 0o755); err != nil {
		return fmt.Errorf("create local skills dir: %w", err)
	}

	next := make(map[string]*cachedEntry, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		sourceSkillDir := filepath.Join(sourceBase, id)
		entry, err := s.loadLocalSkillEntry(id, sourceSkillDir)
		if err != nil {
			log.Printf("⚠️  skip local skill %s: %s", id, err)
			continue
		}
		localSkillDir := filepath.Join(s.localDir, id)
		if err := copyLocalSkillToDisk(sourceSkillDir, localSkillDir); err != nil {
			log.Printf("⚠️  local disk sync failed for skill %s: %s", id, err)
			continue
		}
		entry.BasePath = localSkillDir
		next[id] = entry
	}

	if err := s.pruneLocalSkills(next); err != nil {
		return err
	}

	snapshot := make([]SkillData, 0, len(next))
	for _, entry := range next {
		snapshot = append(snapshot, entry.SkillData)
	}
	if skillSnapshotWriter != nil {
		if err := skillSnapshotWriter(snapshot); err != nil {
			return fmt.Errorf("persist local skill snapshot: %w", err)
		}
	}

	s.mu.Lock()
	s.cache = next
	s.mu.Unlock()
	return nil
}

func (s *Store) sourcePath(parts ...string) string {
	all := append([]string{s.sourceDir, s.basePath}, parts...)
	return filepath.Join(all...)
}

func (s *Store) loadLocalSkillEntry(id, sourceSkillDir string) (*cachedEntry, error) {
	content, err := os.ReadFile(filepath.Join(sourceSkillDir, "SKILL.md"))
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md: %w", err)
	}
	entry := &cachedEntry{}
	entry.skillMdSHA = localContentSHA(content)
	fm, body := splitFrontmatter(string(content))
	entry.SkillData = SkillData{
		ID:        id,
		Enabled:   true,
		Readme:    body,
		SourceSHA: entry.skillMdSHA,
	}
	if fm != "" {
		var meta map[string]interface{}
		if err := json.Unmarshal([]byte(frontmatterToJSON(fm)), &meta); err == nil {
			if v, ok := meta["name"].(string); ok {
				entry.Name = v
			}
			if v, ok := meta["description"].(string); ok {
				entry.Description = v
			}
			if v, ok := meta["enabled"].(bool); ok {
				entry.Enabled = v
			}
		}
	}
	if entry.Name == "" {
		entry.Name = id
	}
	entry.Scripts = listLocalFiles(filepath.Join(sourceSkillDir, "scripts"), true)
	entry.References = listLocalFiles(filepath.Join(sourceSkillDir, "references"), false)
	return entry, nil
}

// syncSkillToDisk writes SKILL.md, scripts/, and references/ to local disk.
func (s *Store) syncSkillToDisk(id, localDir string) error {
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return err
	}

	content, _, err := s.client.GetFileContent(s.skillMdPath(id))
	if err != nil {
		return fmt.Errorf("read SKILL.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		return fmt.Errorf("write SKILL.md: %w", err)
	}

	// Sync scripts/
	if err := s.syncDirToDisk(s.scriptsDir(id), filepath.Join(localDir, "scripts")); err != nil {
		return fmt.Errorf("sync scripts: %w", err)
	}

	// Sync references/
	if err := s.syncDirToDisk(s.refsDir(id), filepath.Join(localDir, "references")); err != nil {
		return fmt.Errorf("sync references: %w", err)
	}

	return nil
}

func copyLocalSkillToDisk(sourceDir, localDir string) error {
	if err := os.RemoveAll(localDir); err != nil {
		return err
	}
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(sourceDir, "SKILL.md"), filepath.Join(localDir, "SKILL.md"), 0o644); err != nil {
		return err
	}
	for _, name := range []string{"scripts", "references"} {
		sourceSubdir := filepath.Join(sourceDir, name)
		if _, err := os.Stat(sourceSubdir); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := copyDir(sourceSubdir, filepath.Join(localDir, name)); err != nil {
			return err
		}
	}
	return nil
}

func copyDir(sourceDir, targetDir string) error {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(sourceDir, entry.Name())
		targetPath := filepath.Join(targetDir, entry.Name())
		if entry.IsDir() {
			if err := copyDir(sourcePath, targetPath); err != nil {
				return err
			}
			continue
		}
		mode := os.FileMode(0o644)
		if strings.Contains(sourceDir, string(filepath.Separator)+"scripts") {
			mode = 0o755
		}
		if err := copyFile(sourcePath, targetPath, mode); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(sourcePath, targetPath string, mode os.FileMode) error {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(targetPath, content, mode)
}

func localContentSHA(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func listLocalFiles(dir string, excludeInternal bool) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if excludeInternal && strings.HasPrefix(name, "_") {
			continue
		}
		names = append(names, name)
	}
	return names
}

// syncDirToDisk downloads all files from a remote directory to a local directory.
func (s *Store) syncDirToDisk(remoteDir, localDir string) error {
	entries, err := s.client.ListDir(remoteDir)
	if err != nil {
		if IsNotFound(err) {
			if rmErr := os.RemoveAll(localDir); rmErr != nil {
				return rmErr
			}
			return nil
		}
		return err
	}
	if err := os.RemoveAll(localDir); err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.Type != "file" {
			continue
		}
		content, _, err := s.client.GetFileContent(remoteDir + "/" + e.Name)
		if err != nil {
			return fmt.Errorf("read %s/%s: %w", remoteDir, e.Name, err)
		}
		localPath := filepath.Join(localDir, e.Name)
		if err := os.WriteFile(localPath, []byte(content), 0o755); err != nil {
			return fmt.Errorf("write %s: %w", localPath, err)
		}
	}
	return nil
}

// loadSkillEntry loads a single skill from its SKILL.md (standard format).
func (s *Store) loadSkillEntry(id string) (*cachedEntry, error) {
	entry := &cachedEntry{}

	if err := s.loadFromSkillMd(id, entry); err != nil {
		return nil, fmt.Errorf("skill %s: %w", id, err)
	}

	if entry.ID == "" {
		entry.ID = id
	}

	entry.Scripts = s.listScripts(id)
	entry.References = s.listReferences(id)

	return entry, nil
}

// loadFromSkillMd parses a SKILL.md file (YAML frontmatter + markdown body).
// Standard frontmatter: name + description.
func (s *Store) loadFromSkillMd(id string, entry *cachedEntry) error {
	content, sha, err := s.client.GetFileContent(s.skillMdPath(id))
	if err != nil {
		return err
	}

	entry.skillMdSHA = sha
	fm, body := splitFrontmatter(content)

	entry.SkillData = SkillData{
		ID:        id,
		Enabled:   true,
		SourceSHA: sha,
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
			if v, ok := meta["enabled"].(bool); ok {
				entry.Enabled = v
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

// listScripts returns the file names in the skill's scripts/ directory.
// Files starting with "_" are internal helpers and excluded from the list.
func (s *Store) listScripts(id string) []string {
	entries, err := s.client.ListDir(s.scriptsDir(id))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.Type == "file" && !strings.HasPrefix(e.Name, "_") {
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

func (s *Store) pruneLocalSkills(next map[string]*cachedEntry) error {
	entries, err := os.ReadDir(s.localDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ok := next[entry.Name()]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.localDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
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

// Refresh reloads skills from the configured source. Safe for concurrent use.
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

// GetBasePath returns the local disk path for a skill's files.
func (s *Store) GetBasePath(id string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.cache[id]; ok {
		return e.BasePath
	}
	return ""
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

// Create writes a new skill to the source and refreshes the cache.
func (s *Store) Create(skill SkillData) (*SkillData, error) {
	if skill.ID == "" {
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		skill.ID = fmt.Sprintf("skill-%x", b)
	}
	defaults(&skill)

	msg := fmt.Sprintf("create skill: %s", skill.Name)
	if err := s.writeFiles(skill, "", msg); err != nil {
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
	sha := entry.skillMdSHA
	s.mu.RUnlock()

	applyUpdates(&skill, updates)

	msg := fmt.Sprintf("update skill: %s", skill.Name)
	if err := s.writeFiles(skill, sha, msg); err != nil {
		return nil, err
	}

	if err := s.refresh(); err != nil {
		return nil, fmt.Errorf("refresh after update: %w", err)
	}
	return s.GetByID(id), nil
}

// Delete removes a skill's directory from the source and refreshes the cache.
func (s *Store) Delete(id string) error {
	if s.sourceDir != "" {
		if err := os.RemoveAll(s.sourcePath(id)); err != nil {
			return err
		}
		if err := s.refresh(); err != nil {
			return fmt.Errorf("refresh after delete: %w", err)
		}
		return nil
	}

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

// writeFiles writes a standard SKILL.md (YAML frontmatter + markdown body).
func (s *Store) writeFiles(skill SkillData, sha, msg string) error {
	var buf strings.Builder
	buf.WriteString("---\n")
	buf.WriteString("name: " + skill.Name + "\n")
	buf.WriteString("description: " + skill.Description + "\n")
	if !skill.Enabled {
		buf.WriteString("enabled: false\n")
	}
	buf.WriteString("---\n")
	if skill.Readme != "" {
		buf.WriteString(skill.Readme)
		if !strings.HasSuffix(skill.Readme, "\n") {
			buf.WriteString("\n")
		}
	}

	if s.sourceDir != "" {
		localSkillDir := s.sourcePath(skill.ID)
		if err := os.MkdirAll(localSkillDir, 0o755); err != nil {
			return fmt.Errorf("create local skill dir: %w", err)
		}
		if err := os.WriteFile(filepath.Join(localSkillDir, "SKILL.md"), []byte(buf.String()), 0o644); err != nil {
			return fmt.Errorf("write local SKILL.md: %w", err)
		}
		return nil
	}

	if err := s.client.PutFile(s.skillMdPath(skill.ID), msg, buf.String(), sha); err != nil {
		return fmt.Errorf("write SKILL.md: %w", err)
	}
	return nil
}

func defaults(s *SkillData) {
	// No-op: simplified SkillData has no fields requiring defaults
}

func applyUpdates(s *SkillData, updates map[string]interface{}) {
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
		case "readme":
			if v, ok := val.(string); ok {
				s.Readme = v
			}
		case "enabled":
			if v, ok := val.(bool); ok {
				s.Enabled = v
			}
		}
	}
}
