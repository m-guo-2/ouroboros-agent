package storage

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func prefixedID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%x", prefix, b)
}

func scanPersona(scan func(...interface{}) error) (Persona, error) {
	var p Persona
	var systemPrompt, provider, model sql.NullString
	var skillsJSON, subModelsJSON, subSkillsJSON sql.NullString
	var groupCount int

	if err := scan(
		&p.ID, &p.AgentID, &p.DisplayName,
		&systemPrompt, &provider, &model,
		&skillsJSON, &subModelsJSON, &subSkillsJSON,
		&groupCount, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return p, err
	}

	if systemPrompt.Valid {
		p.SystemPrompt = &systemPrompt.String
	}
	if provider.Valid {
		p.Provider = &provider.String
	}
	if model.Valid {
		p.Model = &model.String
	}
	if skillsJSON.Valid {
		var ids []string
		if json.Unmarshal([]byte(skillsJSON.String), &ids) == nil {
			p.Skills = &ids
		}
	}
	if subModelsJSON.Valid {
		_ = json.Unmarshal([]byte(subModelsJSON.String), &p.SubagentModels)
	}
	if subSkillsJSON.Valid {
		_ = json.Unmarshal([]byte(subSkillsJSON.String), &p.SubagentSkills)
	}
	p.GroupCount = groupCount
	return p, nil
}

const personaSelectSQL = `SELECT p.id, p.agent_id, p.display_name,
	p.system_prompt, p.provider, p.model,
	p.skills, p.subagent_models, p.subagent_skills,
	(SELECT COUNT(*) FROM group_persona_assignments g WHERE g.persona_id = p.id) AS group_count,
	p.created_at, p.updated_at`

func ListPersonas(agentID string) ([]Persona, error) {
	rows, err := DB.Query(personaSelectSQL+` FROM agent_personas p WHERE p.agent_id = ? ORDER BY p.created_at ASC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Persona
	for rows.Next() {
		p, err := scanPersona(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if out == nil {
		out = []Persona{}
	}
	return out, rows.Err()
}

func GetPersona(id string) (*Persona, error) {
	row := DB.QueryRow(personaSelectSQL+` FROM agent_personas p WHERE p.id = ?`, id)
	p, err := scanPersona(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func CreatePersona(p Persona) (*Persona, error) {
	if p.ID == "" {
		p.ID = prefixedID("persona")
	}
	now := nowTimestamp()

	skillsJSON := nullableJSON(p.Skills)
	subModelsJSON := nullableJSON(p.SubagentModels)
	subSkillsJSON := nullableJSON(p.SubagentSkills)

	_, err := DB.Exec(
		`INSERT INTO agent_personas (id, agent_id, display_name, system_prompt, provider, model, skills, subagent_models, subagent_skills, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.AgentID, p.DisplayName,
		nullStr(p.SystemPrompt), nullStr(p.Provider), nullStr(p.Model),
		skillsJSON, subModelsJSON, subSkillsJSON,
		now, now,
	)
	if err != nil {
		return nil, err
	}
	return GetPersona(p.ID)
}

func UpdatePersona(id string, updates map[string]interface{}) (*Persona, error) {
	colMap := map[string]string{
		"displayName":  "display_name",
		"systemPrompt": "system_prompt",
		"provider":     "provider",
		"model":        "model",
	}
	now := nowTimestamp()

	for key, val := range updates {
		col, ok := colMap[key]
		if !ok {
			continue
		}
		if _, err := DB.Exec(
			fmt.Sprintf("UPDATE agent_personas SET %s = ?, updated_at = ? WHERE id = ?", col),
			val, now, id,
		); err != nil {
			return nil, err
		}
	}

	if skills, exists := updates["skills"]; exists {
		var v interface{}
		if skills == nil {
			v = nil
		} else {
			b, _ := json.Marshal(skills)
			v = string(b)
		}
		DB.Exec("UPDATE agent_personas SET skills = ?, updated_at = ? WHERE id = ?", v, now, id)
	}
	if sm, exists := updates["subagentModels"]; exists {
		var v interface{}
		if sm == nil {
			v = nil
		} else {
			b, _ := json.Marshal(sm)
			v = string(b)
		}
		DB.Exec("UPDATE agent_personas SET subagent_models = ?, updated_at = ? WHERE id = ?", v, now, id)
	}
	if ss, exists := updates["subagentSkills"]; exists {
		var v interface{}
		if ss == nil {
			v = nil
		} else {
			b, _ := json.Marshal(ss)
			v = string(b)
		}
		DB.Exec("UPDATE agent_personas SET subagent_skills = ?, updated_at = ? WHERE id = ?", v, now, id)
	}

	return GetPersona(id)
}

func ClonePersona(srcID, newDisplayName string) (*Persona, error) {
	src, err := GetPersona(srcID)
	if err != nil || src == nil {
		return nil, fmt.Errorf("source persona not found: %s", srcID)
	}
	clone := *src
	clone.ID = prefixedID("persona")
	clone.DisplayName = newDisplayName
	clone.GroupCount = 0
	return CreatePersona(clone)
}

func DeletePersona(id string) (bool, error) {
	var count int
	if err := DB.QueryRow("SELECT COUNT(*) FROM group_persona_assignments WHERE persona_id = ?", id).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, fmt.Errorf("persona is referenced by %d group(s), unassign them first", count)
	}
	res, err := DB.Exec("DELETE FROM agent_personas WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// nullStr converts a *string to a sql.NullString-compatible value.
func nullStr(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

// nullableJSON serializes v to JSON string or returns nil if v is nil.
func nullableJSON(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case *[]string:
		if val == nil {
			return nil
		}
		b, _ := json.Marshal(*val)
		return string(b)
	case map[string]SubagentModelConfig:
		if val == nil {
			return nil
		}
		b, _ := json.Marshal(val)
		return string(b)
	case map[string][]string:
		if val == nil {
			return nil
		}
		b, _ := json.Marshal(val)
		return string(b)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		return string(b)
	}
}
