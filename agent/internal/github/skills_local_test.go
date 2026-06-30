package github

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent/internal/config"
)

func TestNewStoreUsesEmbeddedSkillsWhenRepoUnset(t *testing.T) {
	repoRoot := t.TempDir()
	skillDir := filepath.Join(repoRoot, "agent", "data", "skills", "local-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: Local Skill\ndescription: built in\n---\nUse locally.\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	oldStore := DefaultStore
	defer func() { DefaultStore = oldStore }()
	t.Chdir(repoRoot)

	if err := NewStore(config.GitHub{}); err != nil {
		t.Fatalf("new store: %v", err)
	}
	if DefaultStore.sourceDir != "." {
		t.Fatalf("expected embedded source dir '.', got %q", DefaultStore.sourceDir)
	}
	if DefaultStore.basePath != embeddedBasePath {
		t.Fatalf("expected embedded base path %q, got %q", embeddedBasePath, DefaultStore.basePath)
	}
	if err := DefaultStore.LoadCache(); err != nil {
		t.Fatalf("load cache: %v", err)
	}
	got := DefaultStore.GetByID("local-skill")
	if got == nil || got.Name != "Local Skill" || got.Description != "built in" {
		t.Fatalf("unexpected loaded skill: %+v", got)
	}
}

func TestLocalStoreWritesToEmbeddedSource(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "agent", "data", "skills"), 0o755); err != nil {
		t.Fatalf("mkdir embedded skills: %v", err)
	}

	oldStore := DefaultStore
	defer func() { DefaultStore = oldStore }()

	if err := NewStore(config.GitHub{
		SkillsSourceDir: repoRoot,
		SkillsPath:      embeddedBasePath,
		SkillsLocalDir:  filepath.Join(t.TempDir(), "runtime-skills"),
	}); err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := DefaultStore.LoadCache(); err != nil {
		t.Fatalf("load cache: %v", err)
	}

	created, err := DefaultStore.Create(SkillData{
		ID:          "local-writer",
		Name:        "Local Writer",
		Description: "writes local files",
		Enabled:     true,
		Readme:      "Initial body.",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created == nil || created.ID != "local-writer" {
		t.Fatalf("unexpected created skill: %+v", created)
	}

	sourceSkill := filepath.Join(repoRoot, "agent", "data", "skills", "local-writer", "SKILL.md")
	content, err := os.ReadFile(sourceSkill)
	if err != nil {
		t.Fatalf("read created source skill: %v", err)
	}
	if !strings.Contains(string(content), "Initial body.") {
		t.Fatalf("created source skill missing body: %s", content)
	}

	updated, err := DefaultStore.Update("local-writer", map[string]interface{}{
		"description": "updated description",
		"readme":      "Updated body.",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated == nil || updated.Description != "updated description" {
		t.Fatalf("unexpected updated skill: %+v", updated)
	}
	content, err = os.ReadFile(sourceSkill)
	if err != nil {
		t.Fatalf("read updated source skill: %v", err)
	}
	if !strings.Contains(string(content), "Updated body.") {
		t.Fatalf("updated source skill missing body: %s", content)
	}

	if err := DefaultStore.Delete("local-writer"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(sourceSkill)); !os.IsNotExist(err) {
		t.Fatalf("expected source skill dir deleted, stat err=%v", err)
	}
}
