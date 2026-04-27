package storage

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"agent/internal/timeutil"
)

// Persona timestamps are exposed to callers as RFC3339 strings for historical
// reasons; internally we store UTC epoch milliseconds (BIGINT) like everything
// else and convert at the boundary.

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func prefixedID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%x", prefix, b)
}

func msToRFC3339(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// personaCoreSelect pulls the scalar personas row; children are loaded by
// loadPersonaChildren.
const personaCoreSelect = `SELECT p.id, p.agent_id, p.display_name,
	p.system_prompt, p.provider, p.model, p.created_at, p.updated_at,
	(SELECT COUNT(*) FROM group_persona_assignments g
	   WHERE g.persona_id = p.id AND g.deleted_at = 0) AS group_count
	FROM agent_personas p`

// scanPersonaCore scans only the core columns. Nullability is modeled via
// sql.NullString; pointer semantics in Persona preserve "unset" vs "empty".
func scanPersonaCore(scan func(...interface{}) error) (Persona, error) {
	var p Persona
	var systemPrompt, provider, model sql.NullString
	var createdMs, updatedMs int64
	var groupCount int
	if err := scan(
		&p.ID, &p.AgentID, &p.DisplayName,
		&systemPrompt, &provider, &model,
		&createdMs, &updatedMs, &groupCount,
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
	p.CreatedAt = msToRFC3339(createdMs)
	p.UpdatedAt = msToRFC3339(updatedMs)
	p.GroupCount = groupCount
	return p, nil
}

func loadPersonaChildren(q dbQ, p *Persona) error {
	// Skills (ordered). Use pointer-to-slice so "no row" ≠ "empty slice" only
	// when callers explicitly set Skills; here we always populate since the
	// binding table is authoritative.
	var skillIDs []string
	rows, err := q.Query(`SELECT skill_id FROM persona_skill_bindings
		WHERE persona_id = ? AND deleted_at = 0 ORDER BY position ASC, created_at ASC`, p.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		skillIDs = append(skillIDs, id)
	}
	rows.Close()
	if skillIDs == nil {
		skillIDs = []string{}
	}
	p.Skills = &skillIDs

	// SubagentModels
	rows, err = q.Query(`SELECT subagent_key, COALESCE(provider,''), COALESCE(model,'')
		FROM persona_subagent_models WHERE persona_id = ? AND deleted_at = 0`, p.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var k, pr, m string
		if err := rows.Scan(&k, &pr, &m); err != nil {
			rows.Close()
			return err
		}
		if p.SubagentModels == nil {
			p.SubagentModels = map[string]SubagentModelConfig{}
		}
		p.SubagentModels[k] = SubagentModelConfig{Provider: pr, Model: m}
	}
	rows.Close()

	// SubagentSkills
	rows, err = q.Query(`SELECT subagent_key, skill_id FROM persona_subagent_skill_bindings
		WHERE persona_id = ? AND deleted_at = 0 ORDER BY subagent_key, position ASC`, p.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var k, sid string
		if err := rows.Scan(&k, &sid); err != nil {
			rows.Close()
			return err
		}
		if p.SubagentSkills == nil {
			p.SubagentSkills = map[string][]string{}
		}
		p.SubagentSkills[k] = append(p.SubagentSkills[k], sid)
	}
	rows.Close()
	return nil
}

func ListPersonas(agentID string) ([]Persona, error) {
	rows, err := DB.Query(personaCoreSelect+` WHERE p.agent_id = ? AND p.deleted_at = 0 ORDER BY p.created_at ASC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Persona
	for rows.Next() {
		p, err := scanPersonaCore(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := loadPersonaChildren(DB, &out[i]); err != nil {
			return nil, err
		}
	}
	if out == nil {
		out = []Persona{}
	}
	return out, nil
}

func GetPersona(id string) (*Persona, error) {
	row := DB.QueryRow(personaCoreSelect+` WHERE p.id = ? AND p.deleted_at = 0`, id)
	p, err := scanPersonaCore(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := loadPersonaChildren(DB, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func CreatePersona(p Persona) (*Persona, error) {
	if p.ID == "" {
		p.ID = prefixedID("persona")
	}
	now := timeutil.NowMs()

	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO agent_personas
		 (id, agent_id, display_name, system_prompt, provider, model, created_at, updated_at, deleted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		p.ID, p.AgentID, p.DisplayName,
		nullStr(p.SystemPrompt), nullStr(p.Provider), nullStr(p.Model),
		now, now,
	); err != nil {
		return nil, err
	}
	if err := replacePersonaChildren(tx, p.ID, &p, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
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
	now := timeutil.NowMs()

	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for key, val := range updates {
		col, ok := colMap[key]
		if !ok {
			continue
		}
		if _, err := tx.Exec(
			fmt.Sprintf("UPDATE agent_personas SET %s = ?, updated_at = ? WHERE id = ? AND deleted_at = 0", col),
			val, now, id,
		); err != nil {
			return nil, err
		}
	}

	if skills, ok := updates["skills"]; ok {
		ids := toStringSlice(skills)
		if err := replacePersonaSkillBindings(tx, id, ids, now); err != nil {
			return nil, err
		}
	}
	if sm, ok := updates["subagentModels"]; ok {
		m := toSubagentModels(sm)
		if err := replacePersonaSubagentModels(tx, id, m, now); err != nil {
			return nil, err
		}
	}
	if ss, ok := updates["subagentSkills"]; ok {
		m := toSubagentSkills(ss)
		if err := replacePersonaSubagentSkillBindings(tx, id, m, now); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(`UPDATE agent_personas SET updated_at = ? WHERE id = ? AND deleted_at = 0`, now, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
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
	if err := DB.QueryRow(`SELECT COUNT(*) FROM group_persona_assignments
		WHERE persona_id = ? AND deleted_at = 0`, id).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, fmt.Errorf("persona is referenced by %d group(s), unassign them first", count)
	}
	now := timeutil.NowMs()

	tx, err := DB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE agent_personas SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND deleted_at = 0`, now, now, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, tx.Commit()
	}
	if _, err := tx.Exec(`UPDATE persona_skill_bindings SET deleted_at = ?
		WHERE persona_id = ? AND deleted_at = 0`, now, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`UPDATE persona_subagent_models SET deleted_at = ?
		WHERE persona_id = ? AND deleted_at = 0`, now, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`UPDATE persona_subagent_skill_bindings SET deleted_at = ?
		WHERE persona_id = ? AND deleted_at = 0`, now, id); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// --- child-table replacement helpers -------------------------------------

func replacePersonaChildren(tx *sql.Tx, personaID string, p *Persona, now int64) error {
	if p.Skills != nil {
		if err := replacePersonaSkillBindings(tx, personaID, *p.Skills, now); err != nil {
			return err
		}
	}
	if p.SubagentModels != nil {
		if err := replacePersonaSubagentModels(tx, personaID, p.SubagentModels, now); err != nil {
			return err
		}
	}
	if p.SubagentSkills != nil {
		if err := replacePersonaSubagentSkillBindings(tx, personaID, p.SubagentSkills, now); err != nil {
			return err
		}
	}
	return nil
}

func replacePersonaSkillBindings(tx *sql.Tx, personaID string, skillIDs []string, now int64) error {
	if _, err := tx.Exec(`UPDATE persona_skill_bindings SET deleted_at = ?
		WHERE persona_id = ? AND deleted_at = 0`, now, personaID); err != nil {
		return err
	}
	for i, id := range skillIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		rowID := fmt.Sprintf("psb-%s-%s-%d", personaID, id, now)
		if _, err := tx.Exec(
			`INSERT INTO persona_skill_bindings (id, persona_id, skill_id, mode, position, created_at, deleted_at)
			 VALUES (?, ?, ?, '', ?, ?, 0)`,
			rowID, personaID, id, i, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func replacePersonaSubagentModels(tx *sql.Tx, personaID string, models map[string]SubagentModelConfig, now int64) error {
	if _, err := tx.Exec(`UPDATE persona_subagent_models SET deleted_at = ?
		WHERE persona_id = ? AND deleted_at = 0`, now, personaID); err != nil {
		return err
	}
	for k, v := range models {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		rowID := fmt.Sprintf("psm-%s-%s-%d", personaID, k, now)
		if _, err := tx.Exec(
			`INSERT INTO persona_subagent_models (id, persona_id, subagent_key, provider, model, created_at, updated_at, deleted_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
			rowID, personaID, k, v.Provider, v.Model, now, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func replacePersonaSubagentSkillBindings(tx *sql.Tx, personaID string, m map[string][]string, now int64) error {
	if _, err := tx.Exec(`UPDATE persona_subagent_skill_bindings SET deleted_at = ?
		WHERE persona_id = ? AND deleted_at = 0`, now, personaID); err != nil {
		return err
	}
	for k, ids := range m {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		for i, sid := range ids {
			sid = strings.TrimSpace(sid)
			if sid == "" {
				continue
			}
			rowID := fmt.Sprintf("pssb-%s-%s-%s-%d", personaID, k, sid, now)
			if _, err := tx.Exec(
				`INSERT INTO persona_subagent_skill_bindings (id, persona_id, subagent_key, skill_id, position, created_at, deleted_at)
				 VALUES (?, ?, ?, ?, ?, ?, 0)`,
				rowID, personaID, k, sid, i, now,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// nullStr converts a *string to a sql.NullString-compatible value.
func nullStr(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}
