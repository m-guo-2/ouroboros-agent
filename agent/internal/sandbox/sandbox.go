package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultExecTimeout = 300 * time.Second
	maxOutputBytes     = 10 * 1024 // 10KB, truncate preserving head+tail
	skillsDirName      = "skills"
)

// Sandbox is an isolated per-session workspace on the local filesystem.
// All file and command operations are confined to RootDir.
type Sandbox struct {
	SessionID string
	RootDir   string
	Template  SandboxTemplate

	env       []string // host environment snapshot, captured at creation
	createdAt time.Time
	lastUsed  time.Time
	mu        sync.Mutex
}

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// SkillBasePath returns the base directory for a skill within the sandbox.
// This is used as BasePath in skillexec.ScriptRequest.
func (s *Sandbox) SkillBasePath(skillID string) string {
	return filepath.Join(s.RootDir, skillsDirName, skillID)
}

// Environ returns the sandbox environment as a key-value map.
// Used to populate ScriptRequest.Env for skill script execution.
func (s *Sandbox) Environ() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.environLocked(nil)
}

// SetEnv persists environment variables for all future commands and scripts
// in this sandbox.
func (s *Sandbox) SetEnv(values map[string]string) error {
	if err := validateEnvMap(values); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	env := s.environLocked(values)
	s.env = envMapToSlice(env)
	s.lastUsed = time.Now()
	return nil
}

// UnsetEnv removes persisted environment variables from future commands and
// scripts in this sandbox.
func (s *Sandbox) UnsetEnv(names []string) error {
	for _, name := range names {
		if !envNamePattern.MatchString(name) {
			return fmt.Errorf("invalid env name: %s", name)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	env := s.environLocked(nil)
	for _, name := range names {
		delete(env, name)
	}
	s.env = envMapToSlice(env)
	s.lastUsed = time.Now()
	return nil
}

func (s *Sandbox) environLocked(overrides map[string]string) map[string]string {
	m := make(map[string]string, len(s.env)+len(overrides))
	for _, entry := range s.env {
		if k, v, ok := strings.Cut(entry, "="); ok {
			m[k] = v
		}
	}
	for k, v := range overrides {
		m[k] = v
	}
	return m
}

// Exec runs a shell command inside the sandbox workspace.
func (s *Sandbox) Exec(ctx context.Context, command string, timeout time.Duration) (string, int, error) {
	return s.ExecWithEnv(ctx, command, timeout, nil)
}

// ExecWithEnv runs a shell command with per-command environment overrides.
// Overrides are not persisted to the sandbox.
func (s *Sandbox) ExecWithEnv(ctx context.Context, command string, timeout time.Duration, env map[string]string) (string, int, error) {
	if err := validateEnvMap(env); err != nil {
		return "", -1, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()

	if timeout <= 0 {
		timeout = defaultExecTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = s.RootDir
	if len(s.env) > 0 || len(env) > 0 {
		cmd.Env = envMapToSlice(s.environLocked(env))
	}

	out, err := cmd.CombinedOutput()
	output := truncateOutput(string(out))

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return output, -1, fmt.Errorf("exec: %w", err)
		}
	}
	return output, exitCode, nil
}

func validateEnvMap(env map[string]string) error {
	for k := range env {
		if !envNamePattern.MatchString(k) {
			return fmt.Errorf("invalid env name: %s", k)
		}
	}
	return nil
}

func envMapToSlice(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]string, 0, len(keys))
	for _, k := range keys {
		result = append(result, k+"="+env[k])
	}
	return result
}

// WriteFile writes content to a path relative to the sandbox root.
func (s *Sandbox) WriteFile(relPath, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()

	absPath, err := s.safePath(relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return os.WriteFile(absPath, []byte(content), 0o644)
}

// ReadFile reads content from a path relative to the sandbox root.
func (s *Sandbox) ReadFile(relPath string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()

	absPath, err := s.safePath(relPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return truncateOutput(string(data)), nil
}

// FileInfo describes a file entry.
type FileInfo struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// ListFiles lists entries at a path relative to the sandbox root.
func (s *Sandbox) ListFiles(relPath string) ([]FileInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastUsed = time.Now()

	absPath, err := s.safePath(relPath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}
	var result []FileInfo
	for _, e := range entries {
		info, _ := e.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		result = append(result, FileInfo{
			Name:  e.Name(),
			IsDir: e.IsDir(),
			Size:  size,
		})
	}
	return result, nil
}

// Cleanup removes the sandbox directory entirely.
func (s *Sandbox) Cleanup() error {
	return os.RemoveAll(s.RootDir)
}

// safePath resolves a relative path and ensures it stays within the sandbox root.
func (s *Sandbox) safePath(relPath string) (string, error) {
	if relPath == "" {
		relPath = "."
	}
	abs := filepath.Join(s.RootDir, filepath.Clean(relPath))
	if abs != s.RootDir && !strings.HasPrefix(abs, s.RootDir+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes sandbox: %s", relPath)
	}
	return abs, nil
}

// SyncSkills copies skill script files from global cache directories into the sandbox.
// basePaths maps skill ID → global base path (from github.Store.GetBasePath).
func (s *Sandbox) SyncSkills(basePaths map[string]string) error {
	for skillID, globalBase := range basePaths {
		srcScripts := filepath.Join(globalBase, "scripts")
		if _, err := os.Stat(srcScripts); os.IsNotExist(err) {
			continue
		}
		dstScripts := filepath.Join(s.SkillBasePath(skillID), "scripts")
		if err := copyDir(srcScripts, dstScripts); err != nil {
			return fmt.Errorf("sync skill %s scripts: %w", skillID, err)
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func truncateOutput(s string) string {
	if len(s) <= maxOutputBytes {
		return strings.TrimSpace(s)
	}
	half := maxOutputBytes / 2
	return strings.TrimSpace(s[:half]) +
		fmt.Sprintf("\n\n... [truncated %d bytes] ...\n\n", len(s)-maxOutputBytes) +
		strings.TrimSpace(s[len(s)-half:])
}
