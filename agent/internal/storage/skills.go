package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"agent/internal/types"
)

// SkillRecord mirrors the skills table for CRUD operations.
type SkillRecord struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	Type        string                 `json:"type"`
	Enabled     bool                   `json:"enabled"`
	Triggers    []interface{}          `json:"triggers"`
	Tools       []interface{}          `json:"tools"`
	Readme      string                 `json:"readme"`
	Metadata    map[string]interface{} `json:"metadata"`
}

func scanSkillRecord(row interface {
	Scan(...interface{}) error
}) (*SkillRecord, error) {
	var s SkillRecord
	var isEnabled int
	var triggersJSON, toolsJSON, metaJSON string
	err := row.Scan(
		&s.ID, &s.Name, &s.Description, &s.Version, &s.Type, &isEnabled,
		&triggersJSON, &toolsJSON, &s.Readme, &metaJSON,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Enabled = isEnabled == 1
	_ = json.Unmarshal([]byte(triggersJSON), &s.Triggers)
	_ = json.Unmarshal([]byte(toolsJSON), &s.Tools)
	_ = json.Unmarshal([]byte(metaJSON), &s.Metadata)
	if s.Triggers == nil {
		s.Triggers = []interface{}{}
	}
	if s.Tools == nil {
		s.Tools = []interface{}{}
	}
	return &s, nil
}

const skillSelectSQL = `SELECT id, name, COALESCE(description,''), COALESCE(version,'1.0.0'),
	COALESCE(type,'knowledge'), enabled,
	COALESCE(triggers,'[]'), COALESCE(tools,'[]'), COALESCE(readme,''), COALESCE(metadata,'{}')`

// GetAllSkills returns all skills ordered by name.
func GetAllSkills() ([]SkillRecord, error) {
	rows, err := DB.Query(skillSelectSQL + " FROM skills ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SkillRecord
	for rows.Next() {
		s, err := scanSkillRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// GetSkillByID returns one skill by ID, or (nil, nil) if not found.
func GetSkillByID(skillID string) (*SkillRecord, error) {
	row := DB.QueryRow(skillSelectSQL+" FROM skills WHERE id = ?", skillID)
	return scanSkillRecord(row)
}

// GetSkillByName returns the first skill with the given name.
func GetSkillByName(name string) (*SkillRecord, error) {
	row := DB.QueryRow(skillSelectSQL+" FROM skills WHERE name = ? LIMIT 1", name)
	return scanSkillRecord(row)
}

// CreateSkill inserts a new skill. An ID is generated if not provided.
func CreateSkill(s SkillRecord) (*SkillRecord, error) {
	if s.ID == "" {
		b := make([]byte, 6)
		_, _ = rand.Read(b)
		s.ID = fmt.Sprintf("skill-%x", b)
	}
	triggersJSON, _ := json.Marshal(s.Triggers)
	toolsJSON, _ := json.Marshal(s.Tools)
	metaJSON, _ := json.Marshal(s.Metadata)
	isEnabled := 0
	if s.Enabled {
		isEnabled = 1
	}
	_, err := DB.Exec(
		`INSERT INTO skills (id, name, description, version, type, enabled, triggers, tools, readme, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.Description, s.Version, s.Type, isEnabled,
		string(triggersJSON), string(toolsJSON), s.Readme, string(metaJSON),
	)
	if err != nil {
		return nil, err
	}
	return GetSkillByID(s.ID)
}

// UpdateSkill applies partial updates to a skill.
func UpdateSkill(skillID string, updates map[string]interface{}) (*SkillRecord, error) {
	colMap := map[string]string{
		"name": "name", "description": "description", "version": "version",
		"type": "type", "readme": "readme",
	}
	for key, val := range updates {
		col, ok := colMap[key]
		if !ok {
			continue
		}
		if _, err := DB.Exec(
			fmt.Sprintf("UPDATE skills SET %s = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", col),
			val, skillID,
		); err != nil {
			return nil, err
		}
	}
	if v, ok := updates["enabled"]; ok {
		isEnabled := 0
		if b, ok := v.(bool); ok && b {
			isEnabled = 1
		}
		DB.Exec("UPDATE skills SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", isEnabled, skillID)
	}
	for _, key := range []string{"triggers", "tools", "metadata"} {
		if v, ok := updates[key]; ok {
			b, _ := json.Marshal(v)
			DB.Exec(
				fmt.Sprintf("UPDATE skills SET %s = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", key),
				string(b), skillID,
			)
		}
	}
	return GetSkillByID(skillID)
}

// DeleteSkill removes a skill by ID. Returns true if deleted.
func DeleteSkill(skillID string) (bool, error) {
	res, err := DB.Exec("DELETE FROM skills WHERE id = ?", skillID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// dbSkillTool mirrors the JSON structure stored in skills.tools.
type dbSkillTool struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	InputSchema types.JSONSchema  `json:"inputSchema"`
	Executor    SkillToolExecutor `json:"executor"`
}

// dbSkillRow holds raw rows from the skills table.
type dbSkillRow struct {
	ID          string
	Name        string
	Description string
	Type        string
	Enabled     int
	ToolsJSON   string
	Readme      string
}

func getEnabledSkills() ([]dbSkillRow, error) {
	rows, err := DB.Query(
		`SELECT id, name, description, COALESCE(type,'knowledge'), enabled,
		        COALESCE(tools,'[]'), COALESCE(readme,'')
		 FROM skills WHERE enabled = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []dbSkillRow
	for rows.Next() {
		var s dbSkillRow
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Type, &s.Enabled, &s.ToolsJSON, &s.Readme); err != nil {
			return nil, err
		}
		skills = append(skills, s)
	}
	return skills, rows.Err()
}

// GetSkillsContext compiles all enabled skills into an agent-consumable SkillContext.
// This mirrors the skillManager.compileContext() function in skill-manager.ts.
// agentID is accepted for API compatibility but currently all enabled skills are global.
func GetSkillsContext(agentID string) (*SkillContext, error) {
	skills, err := getEnabledSkills()
	if err != nil {
		return nil, fmt.Errorf("get enabled skills: %w", err)
	}

	ctx := &SkillContext{
		Tools:         []types.ToolDefinition{},
		ToolExecutors: map[string]SkillToolExecutor{},
		SkillDocs:     map[string]string{},
	}

	var summaryLines []string
	var actionDocs []string

	for _, s := range skills {
		var dbTools []dbSkillTool
		_ = json.Unmarshal([]byte(s.ToolsJSON), &dbTools)

		toolNames := make([]string, 0, len(dbTools))
		for _, t := range dbTools {
			toolNames = append(toolNames, t.Name)
		}
		toolSuffix := ""
		if len(toolNames) > 0 {
			toolSuffix = fmt.Sprintf(" [工具: %s]", strings.Join(toolNames, ", "))
		}
		summaryLines = append(summaryLines, fmt.Sprintf("- **%s**: %s%s", s.Name, s.Description, toolSuffix))

		if (s.Type == "action" || s.Type == "hybrid") && s.Readme != "" {
			actionDocs = append(actionDocs, fmt.Sprintf("### Skill: %s\n\n%s", s.Name, s.Readme))
		}

		for _, t := range dbTools {
			ctx.Tools = append(ctx.Tools, types.ToolDefinition{
				Name:        t.Name,
				Description: fmt.Sprintf("[Skill: %s] %s", s.Name, t.Description),
				InputSchema: t.InputSchema,
			})
			ctx.ToolExecutors[t.Name] = t.Executor
		}

		if s.Readme != "" {
			ctx.SkillDocs[s.ID] = s.Readme
		}
	}

	if len(skills) > 0 {
		ctx.SystemPromptAddition = fmt.Sprintf(
			"\n## 你拥有的 Skills\n以下是你已注册的技能，可以根据用户需求主动使用：\n%s",
			strings.Join(summaryLines, "\n"),
		)
	}
	if len(actionDocs) > 0 {
		ctx.SystemPromptAddition += "\n\n## Skill 使用指南\n\n" + strings.Join(actionDocs, "\n\n---\n\n")
	}

	// Always include the built-in get_skill_doc tool.
	ctx.Tools = append(ctx.Tools, types.ToolDefinition{
		Name:        "get_skill_doc",
		Description: "查阅指定 skill 的详细文档。当你需要了解某个 skill 的具体用法时使用。",
		InputSchema: types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"skill_name": map[string]interface{}{
					"type":        "string",
					"description": "skill 的 ID（如 'channel-reply'）",
				},
			},
			Required: []string{"skill_name"},
		},
	})
	ctx.ToolExecutors["get_skill_doc"] = SkillToolExecutor{Type: "internal", Handler: "get_skill_doc"}

	return ctx, nil
}
