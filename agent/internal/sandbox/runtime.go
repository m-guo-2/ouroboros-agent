package sandbox

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	sandboxruntime "agent/internal/sandbox/runtime"
)

const runtimeDirName = ".ouroboros"

// InstallRuntime installs bundled sandbox runtime helpers into the workspace.
func (s *Sandbox) InstallRuntime() error {
	if err := copyEmbeddedRuntime(sandboxruntime.FS, "files", filepath.Join(s.RootDir, runtimeDirName)); err != nil {
		return err
	}
	template := s.Template
	if strings.TrimSpace(template.ID) == "" {
		template = DefaultSandboxTemplate()
		s.Template = template
	}
	if err := writeTemplateManifest(filepath.Join(s.RootDir, runtimeDirName, "template.json"), template); err != nil {
		return err
	}
	env := map[string]string{
		"OUROBOROS_RUNTIME_DIR": filepath.Join(s.RootDir, runtimeDirName),
		"SANDBOX_TEMPLATE_ID":   template.ID,
		"SANDBOX_CATEGORY":      template.Category,
		"PATH":                  prependPath(filepath.Join(s.RootDir, runtimeDirName, "bin"), os.Getenv("PATH")),
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		env["HOME"] = home
		if strings.TrimSpace(os.Getenv("PYTHONUSERBASE")) == "" {
			env["PYTHONUSERBASE"] = filepath.Join(home, ".local")
		}
	}
	return s.SetEnv(env)
}

func writeTemplateManifest(path string, template SandboxTemplate) error {
	data, err := json.MarshalIndent(template, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal template manifest: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func copyEmbeddedRuntime(source embed.FS, root, dst string) error {
	return fs.WalkDir(source, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := source.ReadFile(path)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if strings.HasPrefix(filepath.ToSlash(rel), "bin/") {
			mode = 0o755
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, mode); err != nil {
			return fmt.Errorf("write runtime file %s: %w", rel, err)
		}
		return nil
	})
}

func prependPath(dir, existing string) string {
	if existing == "" {
		return dir
	}
	return dir + string(os.PathListSeparator) + existing
}
