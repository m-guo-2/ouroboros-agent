package storage

import (
	"fmt"
	"strings"

	"agent/internal/github"
)

// SkillRecord mirrors the skill data shape expected by API handlers.
type SkillRecord struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Enabled     bool     `json:"enabled"`
	Readme      string   `json:"readme"`
	Scripts     []string `json:"scripts,omitempty"`
	References  []string `json:"references,omitempty"`
}

func fromGitHub(d *github.SkillData) *SkillRecord {
	if d == nil {
		return nil
	}
	return &SkillRecord{
		ID: d.ID, Name: d.Name, Description: d.Description,
		Enabled: d.Enabled, Readme: d.Readme,
		Scripts: d.Scripts, References: d.References,
	}
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

// GetSkillsContext compiles a Level 1 metadata index for the given skill IDs.
// All skills use progressive loading — no always/on_demand distinction.
func GetSkillsContext(skillIDs []string) (*SkillContext, error) {
	all := store().GetAll()

	enabledMap := make(map[string]github.SkillData)
	for _, s := range all {
		if s.Enabled {
			enabledMap[s.ID] = s
		}
	}

	ctx := &SkillContext{
		LoadableSkillIDs: make(map[string]bool),
	}

	var lines []string
	for _, id := range skillIDs {
		s, ok := enabledMap[id]
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("- **%s**（id: `%s`）: %s", s.Name, s.ID, s.Description))
		ctx.LoadableSkillIDs[s.ID] = true
	}

	if len(lines) > 0 {
		ctx.SkillsSnippet = fmt.Sprintf(
			"## 可用技能\n\n以下技能可通过 load_skill 加载完整说明，通过 run_script 执行脚本。\n\n%s",
			strings.Join(lines, "\n"),
		)
	}

	return ctx, nil
}

// GetSkillDetail returns a skill's content, scripts list, and reference index for load_skill.
func GetSkillDetail(skillID string) (map[string]interface{}, error) {
	d := store().GetByID(skillID)
	if d == nil || !d.Enabled {
		return nil, fmt.Errorf("skill not found or disabled: %s", skillID)
	}

	result := map[string]interface{}{
		"skill_id": d.ID,
		"name":     d.Name,
		"content":  d.Readme,
	}
	if len(d.Scripts) > 0 {
		result["scripts"] = d.Scripts
	}
	if len(d.References) > 0 {
		result["references"] = d.References
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
