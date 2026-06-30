package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent/internal/github"
)

func setupSkillTestDB(t *testing.T) func() {
	t.Helper()
	SetupTestDB(t)
	return func() {}
}

func writeSkillFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "references"), 0o755); err != nil {
		t.Fatalf("mkdir references: %v", err)
	}
	skillDoc := "---\nname: Skill A\ndescription: test skill\n---\n# Usage\n\nUse it carefully.\n"
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(skillDoc), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "run.sh"), []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "references", "guide.md"), []byte("reference body"), 0o644); err != nil {
		t.Fatalf("write reference: %v", err)
	}
}

func TestLocalSkillRuntimeUsesSQLiteAndFiles(t *testing.T) {
	cleanup := setupSkillTestDB(t)
	defer cleanup()

	root := filepath.Join(t.TempDir(), "skill-a")
	writeSkillFixture(t, root)

	err := replaceSkillSnapshot([]github.SkillData{{
		ID:          "skill-a",
		Name:        "Skill A",
		Description: "test skill",
		Enabled:     true,
		BasePath:    root,
		Scripts:     []string{"run.sh"},
		References:  []string{"guide.md"},
		SourceSHA:   "sha-1",
	}})
	if err != nil {
		t.Fatalf("replace snapshot: %v", err)
	}

	previousStore := github.DefaultStore
	github.DefaultStore = nil
	defer func() {
		github.DefaultStore = previousStore
	}()

	ctx, err := GetSkillsContext([]string{"skill-a"})
	if err != nil {
		t.Fatalf("get skills context: %v", err)
	}
	if !strings.Contains(ctx.SkillsSnippet, "Skill A") {
		t.Fatalf("expected skill snippet to include local metadata, got %q", ctx.SkillsSnippet)
	}
	if len(ctx.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics, got %v", ctx.Diagnostics)
	}

	detail, err := GetSkillDetail("skill-a")
	if err != nil {
		t.Fatalf("get skill detail: %v", err)
	}
	content, _ := detail["content"].(string)
	if !strings.Contains(content, "Use it carefully.") {
		t.Fatalf("expected local SKILL.md body, got %q", content)
	}

	ref, err := GetSkillReference("skill-a", "guide.md")
	if err != nil {
		t.Fatalf("get skill reference: %v", err)
	}
	if got, _ := ref["content"].(string); got != "reference body" {
		t.Fatalf("unexpected reference content: %q", got)
	}

	meta, err := GetSkillRuntimeMetadata("skill-a")
	if err != nil {
		t.Fatalf("get runtime metadata: %v", err)
	}
	if meta.BasePath != root {
		t.Fatalf("expected base path %q, got %q", root, meta.BasePath)
	}

	skills, err := GetAllSkills()
	if err != nil {
		t.Fatalf("get all skills: %v", err)
	}
	if len(skills) != 1 || skills[0].ID != "skill-a" {
		t.Fatalf("unexpected skills list: %+v", skills)
	}
}

func TestSeededIcebreakerSkillLoadsFromReadme(t *testing.T) {
	cleanup := setupSkillTestDB(t)
	defer cleanup()

	ctx, err := GetSkillsContext([]string{"icebreaker"})
	if err != nil {
		t.Fatalf("get skills context: %v", err)
	}
	if !strings.Contains(ctx.SkillsSnippet, "破冰引导") {
		t.Fatalf("expected snippet to include icebreaker, got %q", ctx.SkillsSnippet)
	}
	if len(ctx.Diagnostics) != 0 {
		t.Fatalf("expected no diagnostics for seeded readme skill, got %v", ctx.Diagnostics)
	}

	detail, err := GetSkillDetail("icebreaker")
	if err != nil {
		t.Fatalf("get icebreaker detail: %v", err)
	}
	content, _ := detail["content"].(string)
	if !strings.Contains(content, "complete_skill") {
		t.Fatalf("expected icebreaker content to include exit instruction, got %q", content)
	}
}

func TestSkillDiagnosticsAndMissingLocalFile(t *testing.T) {
	cleanup := setupSkillTestDB(t)
	defer cleanup()

	disabledRoot := filepath.Join(t.TempDir(), "disabled-skill")
	writeSkillFixture(t, disabledRoot)
	missingRoot := filepath.Join(t.TempDir(), "missing-skill")
	if err := os.MkdirAll(missingRoot, 0o755); err != nil {
		t.Fatalf("mkdir missing skill root: %v", err)
	}

	err := replaceSkillSnapshot([]github.SkillData{
		{
			ID:          "disabled-skill",
			Name:        "Disabled",
			Description: "disabled skill",
			Enabled:     false,
			BasePath:    disabledRoot,
		},
		{
			ID:          "missing-file-skill",
			Name:        "Missing File",
			Description: "broken local skill",
			Enabled:     true,
			BasePath:    missingRoot,
		},
	})
	if err != nil {
		t.Fatalf("replace snapshot: %v", err)
	}

	ctx, err := GetSkillsContext([]string{"missing-bound-skill", "disabled-skill"})
	if err != nil {
		t.Fatalf("get skills context: %v", err)
	}
	if len(ctx.Diagnostics) != 2 {
		t.Fatalf("expected 2 diagnostics, got %v", ctx.Diagnostics)
	}

	_, err = GetSkillDetail("missing-file-skill")
	if err == nil || !strings.Contains(err.Error(), "local SKILL.md missing") {
		t.Fatalf("expected explicit missing file error, got %v", err)
	}

	if err := replaceSkillSnapshot(nil); err != nil {
		t.Fatalf("clear snapshot: %v", err)
	}
	skills, err := GetAllSkills()
	if err != nil {
		t.Fatalf("get all skills after clear: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected empty local skill store after clear, got %+v", skills)
	}
}

func TestSortSkillIDsByNameAndSkillsContextUseNameOrder(t *testing.T) {
	cleanup := setupSkillTestDB(t)
	defer cleanup()

	alphaRoot := filepath.Join(t.TempDir(), "alpha-skill")
	writeSkillFixture(t, alphaRoot)
	moonRoot := filepath.Join(t.TempDir(), "moon-skill")
	writeSkillFixture(t, moonRoot)
	zebraRoot := filepath.Join(t.TempDir(), "zebra-skill")
	writeSkillFixture(t, zebraRoot)

	err := replaceSkillSnapshot([]github.SkillData{
		{
			ID:          "skill-z",
			Name:        "Zebra",
			Description: "z skill",
			Enabled:     true,
			BasePath:    zebraRoot,
		},
		{
			ID:          "skill-a",
			Name:        "Alpha",
			Description: "a skill",
			Enabled:     true,
			BasePath:    alphaRoot,
		},
		{
			ID:          "skill-m",
			Name:        "Moon",
			Description: "m skill",
			Enabled:     true,
			BasePath:    moonRoot,
		},
	})
	if err != nil {
		t.Fatalf("replace snapshot: %v", err)
	}

	orderedIDs, err := SortSkillIDsByName([]string{"skill-z", "skill-m", "skill-a"})
	if err != nil {
		t.Fatalf("sort skill ids: %v", err)
	}
	expectedIDs := []string{"skill-a", "skill-m", "skill-z"}
	if strings.Join(orderedIDs, ",") != strings.Join(expectedIDs, ",") {
		t.Fatalf("unexpected skill id order: got %v want %v", orderedIDs, expectedIDs)
	}

	ctx, err := GetSkillsContext([]string{"skill-z", "skill-m", "skill-a"})
	if err != nil {
		t.Fatalf("get skills context: %v", err)
	}

	alphaIdx := strings.Index(ctx.SkillsSnippet, "**Alpha**")
	moonIdx := strings.Index(ctx.SkillsSnippet, "**Moon**")
	zebraIdx := strings.Index(ctx.SkillsSnippet, "**Zebra**")
	if alphaIdx == -1 || moonIdx == -1 || zebraIdx == -1 {
		t.Fatalf("expected ordered skill names in snippet, got %q", ctx.SkillsSnippet)
	}
	if !(alphaIdx < moonIdx && moonIdx < zebraIdx) {
		t.Fatalf("expected snippet sorted by name, got %q", ctx.SkillsSnippet)
	}
}
