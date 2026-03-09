package storage

import (
	"encoding/json"
	"fmt"
	"strings"

	"agent/internal/github"
	"agent/internal/types"
)

// SkillRecord mirrors the skill data shape expected by API handlers.
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

func fromGitHub(d *github.SkillData) *SkillRecord {
	if d == nil {
		return nil
	}
	return &SkillRecord{
		ID: d.ID, Name: d.Name, Description: d.Description,
		Version: d.Version, Type: d.Type, Enabled: d.Enabled,
		Triggers: d.Triggers, Tools: d.Tools,
		Readme: d.Readme, Metadata: d.Metadata,
	}
}

func toGitHub(s *SkillRecord) github.SkillData {
	return github.SkillData{
		ID: s.ID, Name: s.Name, Description: s.Description,
		Version: s.Version, Type: s.Type, Enabled: s.Enabled,
		Triggers: s.Triggers, Tools: s.Tools,
		Readme: s.Readme, Metadata: s.Metadata,
	}
}

func store() *github.Store {
	return github.DefaultStore
}

// GetAllSkills returns all skills ordered by name.
func GetAllSkills() ([]SkillRecord, error) {
	all := store().GetAll()
	out := make([]SkillRecord, len(all))
	for i := range all {
		out[i] = *fromGitHub(&all[i])
	}
	return out, nil
}

// GetSkillByID returns one skill by ID, or (nil, nil) if not found.
func GetSkillByID(skillID string) (*SkillRecord, error) {
	return fromGitHub(store().GetByID(skillID)), nil
}

// GetSkillByName returns the first skill with the given name.
func GetSkillByName(name string) (*SkillRecord, error) {
	return fromGitHub(store().GetByName(name)), nil
}

// CreateSkill inserts a new skill.
func CreateSkill(s SkillRecord) (*SkillRecord, error) {
	result, err := store().Create(toGitHub(&s))
	if err != nil {
		return nil, err
	}
	return fromGitHub(result), nil
}

// UpdateSkill applies partial updates to a skill.
func UpdateSkill(skillID string, updates map[string]interface{}) (*SkillRecord, error) {
	result, err := store().Update(skillID, updates)
	if err != nil {
		return nil, err
	}
	return fromGitHub(result), nil
}

// DeleteSkill removes a skill by ID. Returns true if deleted.
func DeleteSkill(skillID string) (bool, error) {
	if store().GetByID(skillID) == nil {
		return false, nil
	}
	if err := store().Delete(skillID); err != nil {
		return false, err
	}
	return true, nil
}

// dbSkillTool mirrors the JSON structure stored in skills.tools.
type dbSkillTool struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	InputSchema types.JSONSchema  `json:"inputSchema"`
	Executor    SkillToolExecutor `json:"executor"`
}

// GetSkillsContext compiles enabled skills into an agent-consumable SkillContext.
//
// agentSkills controls which skills are "active" (tools registered + readme in prompt):
//   - non-empty: only skills whose ID is in agentSkills are active
//   - empty/nil: all enabled skills are active (backward compatible)
//
// Skills that are enabled but not active become "deferred": their docs are
// available via load_skill, and a brief index is appended to the system prompt.
func GetSkillsContext(agentID string, agentSkills []string) (*SkillContext, error) {
	all := store().GetAll()

	// Filter to enabled skills only.
	var skills []github.SkillData
	for _, s := range all {
		if s.Enabled {
			skills = append(skills, s)
		}
	}

	activeSet := make(map[string]bool, len(agentSkills))
	for _, id := range agentSkills {
		activeSet[id] = true
	}
	hasFilter := len(activeSet) > 0

	ctx := &SkillContext{
		Tools:         []types.ToolDefinition{},
		ToolExecutors: map[string]SkillToolExecutor{},
		SkillDocs:     map[string]string{},
	}

	var activeSummaries []string
	var actionDocs []string
	var deferredSummaries []string

	for _, s := range skills {
		if s.Readme != "" {
			ctx.SkillDocs[s.ID] = s.Readme
		}

		isActive := !hasFilter || activeSet[s.ID]

		var dbTools []dbSkillTool
		toolsJSON, _ := json.Marshal(s.Tools)
		_ = json.Unmarshal(toolsJSON, &dbTools)

		if isActive {
			toolNames := make([]string, 0, len(dbTools))
			for _, t := range dbTools {
				toolNames = append(toolNames, t.Name)
			}
			toolSuffix := ""
			if len(toolNames) > 0 {
				toolSuffix = fmt.Sprintf(" [工具: %s]", strings.Join(toolNames, ", "))
			}
			activeSummaries = append(activeSummaries, fmt.Sprintf("- **%s**: %s%s", s.Name, s.Description, toolSuffix))

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
		} else {
			deferredSummaries = append(deferredSummaries,
				fmt.Sprintf("- **%s**（id: `%s`）: %s", s.Name, s.ID, s.Description))
		}
	}

	if len(activeSummaries) > 0 {
		ctx.SkillsSnippet = fmt.Sprintf(
			"\n## 你拥有的 Skills\n以下是你已注册的技能，可以根据用户需求主动使用：\n%s",
			strings.Join(activeSummaries, "\n"),
		)
	}
	if len(actionDocs) > 0 {
		ctx.SkillsSnippet += "\n\n## Skill 使用指南\n\n" + strings.Join(actionDocs, "\n\n---\n\n")
	}
	if len(deferredSummaries) > 0 {
		ctx.SkillsSnippet += fmt.Sprintf(
			"\n\n## 可按需加载的扩展技能\n以下技能未默认加载。需要时使用 `load_skill` 工具加载对应技能的文档和工具参考，然后通过已有工具执行操作。\n%s",
			strings.Join(deferredSummaries, "\n"),
		)
	}

	ctx.Tools = append(ctx.Tools, types.ToolDefinition{
		Name:        "load_skill",
		Description: "加载一个扩展技能的完整文档和工具参考。当你需要使用「可按需加载的扩展技能」中的能力时，先调用此工具获取详细说明，再通过已有工具（如 wecom_api）执行具体操作。",
		InputSchema: types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"skill_id": map[string]interface{}{
					"type":        "string",
					"description": "要加载的技能 ID",
				},
			},
			Required: []string{"skill_id"},
		},
	})
	ctx.ToolExecutors["load_skill"] = SkillToolExecutor{Type: "internal", Handler: "load_skill"}

	return ctx, nil
}

// GetSkillDetail returns a skill's readme and tool definitions for load_skill.
func GetSkillDetail(skillID string) (map[string]interface{}, error) {
	d := store().GetByID(skillID)
	if d == nil || !d.Enabled {
		return nil, fmt.Errorf("skill not found or disabled: %s", skillID)
	}

	var toolRefs []map[string]interface{}
	for _, t := range d.Tools {
		if m, ok := t.(map[string]interface{}); ok {
			ref := map[string]interface{}{
				"name":        m["name"],
				"description": m["description"],
			}
			if schema, ok := m["inputSchema"]; ok {
				ref["inputSchema"] = schema
			}
			if exec, ok := m["executor"]; ok {
				ref["executor"] = exec
			}
			toolRefs = append(toolRefs, ref)
		}
	}

	return map[string]interface{}{
		"skill_id":    d.ID,
		"name":        d.Name,
		"description": d.Description,
		"readme":      d.Readme,
		"tools":       toolRefs,
	}, nil
}
