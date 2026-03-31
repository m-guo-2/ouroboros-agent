package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agent/internal/github"
	"agent/internal/timeutil"
)

const skillSelectSQL = `SELECT id, name, COALESCE(description,''), enabled, COALESCE(metadata,'{}'), updated_at FROM skills`

// SkillRuntimeMetadata stores the local runtime copy details for a skill.
type SkillRuntimeMetadata struct {
	BasePath   string   `json:"basePath,omitempty"`
	Scripts    []string `json:"scripts,omitempty"`
	References []string `json:"references,omitempty"`
	SyncedAt   int64    `json:"syncedAt,omitempty"`
	SourceSHA  string   `json:"sourceSha,omitempty"`
}

// SkillRecord mirrors the skill data shape expected by API handlers.
type SkillRecord struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Enabled     bool                  `json:"enabled"`
	Readme      string                `json:"readme"`
	Scripts     []string              `json:"scripts,omitempty"`
	References  []string              `json:"references,omitempty"`
	Metadata    *SkillRuntimeMetadata `json:"metadata,omitempty"`
}

type skillRow struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
	Metadata    SkillRuntimeMetadata
	UpdatedAt   int64
}

func init() {
	github.SetSkillSnapshotWriter(replaceSkillSnapshot)
}

func toGitHub(s *SkillRecord) github.SkillData {
	return github.SkillData{
		ID: s.ID, Name: s.Name, Description: s.Description,
		Enabled: s.Enabled, Readme: s.Readme,
		Scripts: s.Scripts, References: s.References,
	}
}

func store() *github.Store {
	return github.DefaultStore
}

// RefreshSkills forces a re-read of all skills from the GitHub repository.
func RefreshSkills() error {
	return store().Refresh()
}

// GetAllSkills returns all locally synchronized skills ordered by name.
func GetAllSkills() ([]SkillRecord, error) {
	rows, err := DB.Query(skillSelectSQL + ` ORDER BY name COLLATE NOCASE ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SkillRecord
	for rows.Next() {
		row, err := scanSkillRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, skillRecordFromRow(row, false))
	}
	if out == nil {
		out = []SkillRecord{}
	}
	return out, rows.Err()
}

// GetSkillByID returns one skill by ID, or (nil, nil) if not found.
func GetSkillByID(skillID string) (*SkillRecord, error) {
	row, err := getSkillRowByID(skillID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record := skillRecordFromRow(*row, false)
	readme, err := readLocalSkillBody(*row)
	if err != nil {
		return nil, err
	}
	record.Readme = readme
	return &record, nil
}

// GetSkillByName returns the first skill with the given name.
func GetSkillByName(name string) (*SkillRecord, error) {
	row := DB.QueryRow(skillSelectSQL+` WHERE name = ? COLLATE NOCASE LIMIT 1`, name)
	parsed, err := scanSkillRow(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record := skillRecordFromRow(parsed, false)
	readme, err := readLocalSkillBody(parsed)
	if err != nil {
		return nil, err
	}
	record.Readme = readme
	return &record, nil
}

// CreateSkill inserts a new skill.
func CreateSkill(s SkillRecord) (*SkillRecord, error) {
	result, err := store().Create(toGitHub(&s))
	if err != nil {
		return nil, err
	}
	return GetSkillByID(result.ID)
}

// UpdateSkill applies partial updates to a skill.
func UpdateSkill(skillID string, updates map[string]interface{}) (*SkillRecord, error) {
	if _, err := store().Update(skillID, updates); err != nil {
		return nil, err
	}
	return GetSkillByID(skillID)
}

// DeleteSkill removes a skill by ID. Returns true if deleted.
func DeleteSkill(skillID string) (bool, error) {
	if _, err := getSkillRowByID(skillID); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if err := store().Delete(skillID); err != nil {
		return false, err
	}
	return true, nil
}

// GetSkillsContext compiles a Level 1 metadata index for the given skill IDs.
// All skills use progressive loading — no always/on_demand distinction.
func GetSkillsContext(skillIDs []string) (*SkillContext, error) {
	ctx := &SkillContext{
		LoadableSkillIDs: make(map[string]bool),
	}
	if len(skillIDs) == 0 {
		return ctx, nil
	}

	orderedSkillIDs, err := SortSkillIDsByName(skillIDs)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, id := range orderedSkillIDs {
		row, err := getSkillRowByID(id)
		if err == sql.ErrNoRows {
			ctx.Diagnostics = append(ctx.Diagnostics, fmt.Sprintf("bound skill %q missing from local store", id))
			continue
		}
		if err != nil {
			return nil, err
		}
		if !row.Enabled {
			ctx.Diagnostics = append(ctx.Diagnostics, fmt.Sprintf("bound skill %q disabled in local store", id))
			continue
		}
		if strings.TrimSpace(row.Metadata.BasePath) == "" {
			ctx.Diagnostics = append(ctx.Diagnostics, fmt.Sprintf("bound skill %q missing local base path metadata", id))
		}
		lines = append(lines, fmt.Sprintf("- **%s**（id: `%s`）: %s", row.Name, row.ID, row.Description))
		ctx.LoadableSkillIDs[row.ID] = true
	}

	if len(lines) > 0 {
		ctx.SkillsSnippet = fmt.Sprintf(
			"## 可用技能\n\n以下技能可通过 load_skill 加载完整说明，通过 run_script 执行脚本。\n\n%s",
			strings.Join(lines, "\n"),
		)
	}
	return ctx, nil
}

// SortSkillIDsByName returns de-duplicated skill IDs ordered by skill name.
// Missing skills fall back to skill ID ordering so runtime behavior stays deterministic.
func SortSkillIDsByName(skillIDs []string) ([]string, error) {
	if len(skillIDs) == 0 {
		return nil, nil
	}

	type skillOrderEntry struct {
		ID       string
		SortName string
	}

	seen := make(map[string]bool, len(skillIDs))
	entries := make([]skillOrderEntry, 0, len(skillIDs))
	for _, skillID := range skillIDs {
		skillID = strings.TrimSpace(skillID)
		if skillID == "" || seen[skillID] {
			continue
		}
		seen[skillID] = true

		sortName := strings.ToLower(skillID)
		row, err := getSkillRowByID(skillID)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == nil {
			if name := strings.TrimSpace(row.Name); name != "" {
				sortName = strings.ToLower(name)
			}
		}
		entries = append(entries, skillOrderEntry{
			ID:       skillID,
			SortName: sortName,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].SortName == entries[j].SortName {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].SortName < entries[j].SortName
	})

	ordered := make([]string, 0, len(entries))
	for _, entry := range entries {
		ordered = append(ordered, entry.ID)
	}
	return ordered, nil
}

// GetSkillDetail returns a skill's content, scripts list, and reference index for load_skill.
func GetSkillDetail(skillID string) (map[string]interface{}, error) {
	row, err := requireEnabledSkill(skillID)
	if err != nil {
		return nil, err
	}
	content, err := readLocalSkillBody(*row)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"skill_id": row.ID,
		"name":     row.Name,
		"content":  content,
	}
	if len(row.Metadata.Scripts) > 0 {
		result["scripts"] = append([]string(nil), row.Metadata.Scripts...)
	}
	if len(row.Metadata.References) > 0 {
		result["references"] = append([]string(nil), row.Metadata.References...)
	}
	return result, nil
}

// GetSkillReference fetches a specific reference file for a skill on demand.
func GetSkillReference(skillID, refName string) (map[string]interface{}, error) {
	row, err := requireEnabledSkill(skillID)
	if err != nil {
		return nil, err
	}
	found := false
	for _, r := range row.Metadata.References {
		if r == refName {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("reference %q not found in local metadata for skill %s; available: %v", refName, skillID, row.Metadata.References)
	}
	path := filepath.Join(row.Metadata.BasePath, "references", refName)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("skill %q local reference %q missing: %w", skillID, refName, err)
	}
	return map[string]interface{}{
		"skill_id":  skillID,
		"reference": refName,
		"content":   string(content),
	}, nil
}

// GetSkillRuntimeMetadata returns the synchronized local runtime metadata.
func GetSkillRuntimeMetadata(skillID string) (*SkillRuntimeMetadata, error) {
	row, err := requireEnabledSkill(skillID)
	if err != nil {
		return nil, err
	}
	meta := row.Metadata
	return &meta, nil
}

func scanSkillRow(scan func(...interface{}) error) (skillRow, error) {
	var row skillRow
	var enabled int
	var metadataJSON string
	if err := scan(&row.ID, &row.Name, &row.Description, &enabled, &metadataJSON, &row.UpdatedAt); err != nil {
		return row, err
	}
	row.Enabled = enabled == 1
	if strings.TrimSpace(metadataJSON) != "" {
		_ = json.Unmarshal([]byte(metadataJSON), &row.Metadata)
	}
	if row.Metadata.Scripts == nil {
		row.Metadata.Scripts = []string{}
	}
	if row.Metadata.References == nil {
		row.Metadata.References = []string{}
	}
	return row, nil
}

func skillRecordFromRow(row skillRow, includeReadme bool) SkillRecord {
	record := SkillRecord{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		Enabled:     row.Enabled,
		Scripts:     append([]string(nil), row.Metadata.Scripts...),
		References:  append([]string(nil), row.Metadata.References...),
		Metadata:    &row.Metadata,
	}
	if !includeReadme {
		record.Readme = ""
	}
	return record
}

func getSkillRowByID(skillID string) (*skillRow, error) {
	row := DB.QueryRow(skillSelectSQL+` WHERE id = ?`, skillID)
	parsed, err := scanSkillRow(row.Scan)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func requireEnabledSkill(skillID string) (*skillRow, error) {
	row, err := getSkillRowByID(skillID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("skill %q missing from local store", skillID)
	}
	if err != nil {
		return nil, err
	}
	if !row.Enabled {
		return nil, fmt.Errorf("skill not found or disabled: %s", skillID)
	}
	if strings.TrimSpace(row.Metadata.BasePath) == "" {
		return nil, fmt.Errorf("skill %q missing local base path metadata", skillID)
	}
	return row, nil
}

func readLocalSkillBody(row skillRow) (string, error) {
	if strings.TrimSpace(row.Metadata.BasePath) == "" {
		return "", fmt.Errorf("skill %q missing local base path metadata", row.ID)
	}
	path := filepath.Join(row.Metadata.BasePath, "SKILL.md")
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("skill %q local SKILL.md missing: %w", row.ID, err)
	}
	_, body := splitFrontmatter(string(content))
	return body, nil
}

func replaceSkillSnapshot(skills []github.SkillData) error {
	if DB == nil {
		return fmt.Errorf("local skill snapshot writer: db not initialized")
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := timeutil.NowMs()
	seen := make(map[string]struct{}, len(skills))
	for _, skill := range skills {
		seen[skill.ID] = struct{}{}
		metaBytes, err := json.Marshal(SkillRuntimeMetadata{
			BasePath:   skill.BasePath,
			Scripts:    append([]string(nil), skill.Scripts...),
			References: append([]string(nil), skill.References...),
			SyncedAt:   now,
			SourceSHA:  skill.SourceSHA,
		})
		if err != nil {
			return err
		}
		enabled := 0
		if skill.Enabled {
			enabled = 1
		}
		if _, err := tx.Exec(`
			INSERT INTO skills (id, name, description, enabled, metadata, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				enabled = excluded.enabled,
				metadata = excluded.metadata,
				updated_at = excluded.updated_at
		`, skill.ID, skill.Name, skill.Description, enabled, string(metaBytes), now, now); err != nil {
			return err
		}
	}

	rows, err := tx.Query(`SELECT id FROM skills`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var staleIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		if _, ok := seen[id]; !ok {
			staleIDs = append(staleIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range staleIDs {
		if _, err := tx.Exec(`DELETE FROM skills WHERE id = ?`, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func splitFrontmatter(content string) (frontmatter, body string) {
	const delim = "---"
	if !strings.HasPrefix(strings.TrimSpace(content), delim) {
		return "", strings.TrimSpace(content)
	}
	trimmed := strings.TrimSpace(content)
	rest := trimmed[len(delim):]
	idx := strings.Index(rest, delim)
	if idx < 0 {
		return "", strings.TrimSpace(content)
	}
	return strings.TrimSpace(rest[:idx]), strings.TrimSpace(rest[idx+len(delim):])
}
