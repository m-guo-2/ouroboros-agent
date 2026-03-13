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
	References  []string               `json:"references,omitempty"`
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
		Readme: d.Readme, References: d.References, Metadata: d.Metadata,
	}
}

func toGitHub(s *SkillRecord) github.SkillData {
	return github.SkillData{
		ID: s.ID, Name: s.Name, Description: s.Description,
		Version: s.Version, Type: s.Type, Enabled: s.Enabled,
		Triggers: s.Triggers, Tools: s.Tools,
		Readme: s.Readme, References: s.References, Metadata: s.Metadata,
	}
}

func store() *github.Store {
	return github.DefaultStore
}

// RefreshSkills forces a re-read of all skills from the GitHub repository.
func RefreshSkills() error {
	return store().Refresh()
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
// agentSkills controls which skills are bound and how they appear:
//   - mode "always": full readme inlined into SkillsSnippet, tools registered
//   - mode "on_demand": only name/description/id index in SkillsSnippet, no tools
//   - unbound enabled skills: appear in "available on-demand" section, loadable via load_skill
func GetSkillsContext(agentID string, agentSkills []SkillBinding) (*SkillContext, error) {
	all := store().GetAll()

	var enabled []github.SkillData
	for _, s := range all {
		if s.Enabled {
			enabled = append(enabled, s)
		}
	}

	bindingMode := make(map[string]string, len(agentSkills))
	for _, b := range agentSkills {
		bindingMode[b.ID] = b.Mode
	}

	ctx := &SkillContext{
		Tools:         []types.ToolDefinition{},
		ToolExecutors: map[string]SkillToolExecutor{},
		SkillDocs:     map[string]string{},
	}

	var alwaysDocs []string
	var onDemandSummaries []string
	var unboundSummaries []string

	for _, s := range enabled {
		if s.Readme != "" {
			ctx.SkillDocs[s.ID] = s.Readme
		}

		var dbTools []dbSkillTool
		toolsJSON, _ := json.Marshal(s.Tools)
		_ = json.Unmarshal(toolsJSON, &dbTools)

		mode, bound := bindingMode[s.ID]

		switch {
		case bound && mode == "always":
			doc := fmt.Sprintf("### Skill: %s\n%s", s.Name, s.Description)
			if s.Readme != "" {
				doc += "\n\n" + s.Readme
			}
			alwaysDocs = append(alwaysDocs, doc)

			for _, t := range dbTools {
				ctx.Tools = append(ctx.Tools, types.ToolDefinition{
					Name:        t.Name,
					Description: fmt.Sprintf("[Skill: %s] %s", s.Name, t.Description),
					InputSchema: t.InputSchema,
				})
				ctx.ToolExecutors[t.Name] = t.Executor
			}

		case bound && mode == "on_demand":
			onDemandSummaries = append(onDemandSummaries,
				fmt.Sprintf("- **%s**（id: `%s`）: %s", s.Name, s.ID, s.Description))

		default:
			unboundSummaries = append(unboundSummaries,
				fmt.Sprintf("- **%s**（id: `%s`）: %s", s.Name, s.ID, s.Description))
		}
	}

	if len(alwaysDocs) > 0 {
		ctx.SkillsSnippet = "\n## Skills\n\n" + strings.Join(alwaysDocs, "\n\n---\n\n")
	}

	var deferred []string
	deferred = append(deferred, onDemandSummaries...)
	deferred = append(deferred, unboundSummaries...)
	if len(deferred) > 0 {
		ctx.SkillsSnippet += fmt.Sprintf(
			"\n\n## 可按需加载的扩展技能\n以下技能未默认加载。需要时使用 `load_skill` 工具加载对应技能的文档和工具参考，然后通过已有工具执行操作。\n%s",
			strings.Join(deferred, "\n"),
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

	ctx.Tools = append(ctx.Tools, types.ToolDefinition{
		Name:        "load_skill_reference",
		Description: "加载技能的详细 API 参考文档。当 load_skill 返回的 readme 概览不够详细时，使用此工具按需加载具体的参考文件（如完整参数说明、枚举值、调用示例）。",
		InputSchema: types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"skill_id": map[string]interface{}{
					"type":        "string",
					"description": "技能 ID",
				},
				"reference": map[string]interface{}{
					"type":        "string",
					"description": "参考文件名（从 load_skill 返回的 references 列表中选择）",
				},
			},
			Required: []string{"skill_id", "reference"},
		},
	})
	ctx.ToolExecutors["load_skill_reference"] = SkillToolExecutor{Type: "internal", Handler: "load_skill_reference"}

	return ctx, nil
}

// GetSkillDetail returns a skill's readme, tool definitions, and reference index for load_skill.
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

	result := map[string]interface{}{
		"skill_id":    d.ID,
		"name":        d.Name,
		"description": d.Description,
		"readme":      d.Readme,
		"tools":       toolRefs,
	}
	if len(d.References) > 0 {
		result["references"] = d.References
		result["references_hint"] = "如需查看详细 API 参考文档，使用 load_skill_reference 工具加载具体的 reference 文件。"
	}
	return result, nil
}

// GetSkillReference fetches a specific reference file for a skill on demand.
func GetSkillReference(skillID, refName string) (map[string]interface{}, error) {
	d := store().GetByID(skillID)
	if d == nil || !d.Enabled {
		return nil, fmt.Errorf("skill not found or disabled: %s", skillID)
	}

	found := false
	for _, r := range d.References {
		if r == refName {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("reference %q not found in skill %s; available: %v", refName, skillID, d.References)
	}

	content, err := store().GetReference(skillID, refName)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"skill_id":  skillID,
		"reference": refName,
		"content":   content,
	}, nil
}
