package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Manager manages per-session sandboxes.
type Manager struct {
	baseDir   string
	template  SandboxTemplate
	mu        sync.RWMutex
	sandboxes map[string]*Sandbox
}

// NewManager creates a sandbox manager that stores workspaces under baseDir.
func NewManager(baseDir string) *Manager {
	return NewManagerWithTemplate(baseDir, DefaultSandboxTemplate())
}

// NewManagerWithTemplate creates a sandbox manager with an explicit default template.
func NewManagerWithTemplate(baseDir string, template SandboxTemplate) *Manager {
	if strings.TrimSpace(template.ID) == "" {
		template = DefaultSandboxTemplate()
	}
	return &Manager{
		baseDir:   baseDir,
		template:  template,
		sandboxes: make(map[string]*Sandbox),
	}
}

// GetOrCreate returns the existing sandbox for a session, or creates a new one.
// The second return value is true when a new sandbox was created (caller should sync skills).
func (m *Manager) GetOrCreate(sessionID string) (*Sandbox, bool, error) {
	return m.GetOrCreateWithTemplate(sessionID, m.template)
}

// GetOrCreateWithTemplate returns the existing sandbox for a session, or creates
// a new one with the requested template. Existing live sandboxes keep their
// original template for runtime stability.
func (m *Manager) GetOrCreateWithTemplate(sessionID string, template SandboxTemplate) (*Sandbox, bool, error) {
	if strings.TrimSpace(template.ID) == "" {
		template = m.template
	}
	m.mu.RLock()
	if sb, ok := m.sandboxes[sessionID]; ok {
		m.mu.RUnlock()
		return sb, false, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	if sb, ok := m.sandboxes[sessionID]; ok {
		return sb, false, nil
	}

	rootDir := filepath.Join(m.baseDir, sessionID)
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, false, fmt.Errorf("create sandbox dir: %w", err)
	}

	sb := &Sandbox{
		SessionID: sessionID,
		RootDir:   rootDir,
		Template:  template,
		env:       os.Environ(),
		createdAt: time.Now(),
		lastUsed:  time.Now(),
	}
	if err := sb.InstallRuntime(); err != nil {
		return nil, false, fmt.Errorf("install sandbox runtime: %w", err)
	}
	m.sandboxes[sessionID] = sb
	return sb, true, nil
}

// Get returns the sandbox for a session, or nil.
func (m *Manager) Get(sessionID string) *Sandbox {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sandboxes[sessionID]
}

// Destroy removes and cleans up the sandbox for a session.
func (m *Manager) Destroy(sessionID string) error {
	m.mu.Lock()
	sb, ok := m.sandboxes[sessionID]
	if ok {
		delete(m.sandboxes, sessionID)
	}
	m.mu.Unlock()

	if !ok {
		return nil
	}
	return sb.Cleanup()
}

// Shutdown destroys all active sandboxes. Called on graceful shutdown.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	all := make([]*Sandbox, 0, len(m.sandboxes))
	for _, sb := range m.sandboxes {
		all = append(all, sb)
	}
	m.sandboxes = make(map[string]*Sandbox)
	m.mu.Unlock()

	for _, sb := range all {
		_ = sb.Cleanup()
	}
}
