package storage

import (
	"database/sql"
	"fmt"

	"agent/internal/timeutil"
)

// scanGroupAssignment reads a row where created_at / updated_at are stored as
// BIGINT epoch-ms but surfaced to callers as RFC3339 strings for API stability.
func scanGroupAssignment(scan func(...interface{}) error) (GroupAssignment, error) {
	var ga GroupAssignment
	var personaID, sandboxTemplateID sql.NullString
	var createdMs, updatedMs int64

	if err := scan(
		&ga.ID, &ga.AgentID, &ga.SessionKey, &ga.GroupName,
		&personaID, &sandboxTemplateID, &createdMs, &updatedMs,
	); err != nil {
		return ga, err
	}
	if personaID.Valid && personaID.String != "" {
		ga.PersonaID = &personaID.String
	}
	if sandboxTemplateID.Valid && sandboxTemplateID.String != "" {
		ga.SandboxTemplateID = &sandboxTemplateID.String
	}
	ga.CreatedAt = msToRFC3339(createdMs)
	ga.UpdatedAt = msToRFC3339(updatedMs)
	return ga, nil
}

const groupAssignmentSelectSQL = `SELECT id, agent_id, session_key, group_name, persona_id, sandbox_template_id, created_at, updated_at`

// GetGroupAssignment finds the assignment for a specific agent + session_key.
// Returns nil, nil when no assignment exists.
func GetGroupAssignment(agentID, sessionKey string) (*GroupAssignment, error) {
	row := DB.QueryRow(
		groupAssignmentSelectSQL+` FROM group_persona_assignments
		WHERE agent_id = ? AND session_key = ? AND deleted_at = 0`,
		agentID, sessionKey,
	)
	ga, err := scanGroupAssignment(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ga, nil
}

func GetGroupAssignmentByID(id string) (*GroupAssignment, error) {
	row := DB.QueryRow(groupAssignmentSelectSQL+`
		FROM group_persona_assignments WHERE id = ? AND deleted_at = 0`, id)
	ga, err := scanGroupAssignment(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ga, nil
}

func ListGroupAssignments(agentID string) ([]GroupAssignment, error) {
	rows, err := DB.Query(
		groupAssignmentSelectSQL+` FROM group_persona_assignments
		WHERE agent_id = ? AND deleted_at = 0 ORDER BY created_at ASC`,
		agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GroupAssignment
	for rows.Next() {
		ga, err := scanGroupAssignment(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, ga)
	}
	if out == nil {
		out = []GroupAssignment{}
	}
	return out, rows.Err()
}

func CreateGroupAssignment(ga GroupAssignment) (*GroupAssignment, error) {
	if ga.ID == "" {
		ga.ID = prefixedID("ga")
	}
	now := timeutil.NowMs()
	_, err := DB.Exec(
		`INSERT INTO group_persona_assignments
		 (id, agent_id, session_key, group_name, persona_id, sandbox_template_id, created_at, updated_at, deleted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		ga.ID, ga.AgentID, ga.SessionKey, ga.GroupName, nullStr(ga.PersonaID), nullStr(ga.SandboxTemplateID), now, now,
	)
	if err != nil {
		return nil, err
	}
	return GetGroupAssignmentByID(ga.ID)
}

func UpdateGroupAssignment(id string, updates map[string]interface{}) (*GroupAssignment, error) {
	now := timeutil.NowMs()
	colMap := map[string]string{
		"groupName":         "group_name",
		"personaId":         "persona_id",
		"sandboxTemplateId": "sandbox_template_id",
	}
	for key, val := range updates {
		col, ok := colMap[key]
		if !ok {
			continue
		}
		if _, err := DB.Exec(
			fmt.Sprintf("UPDATE group_persona_assignments SET %s = ?, updated_at = ? WHERE id = ? AND deleted_at = 0", col),
			val, now, id,
		); err != nil {
			return nil, err
		}
	}
	return GetGroupAssignmentByID(id)
}

func DeleteGroupAssignment(id string) (bool, error) {
	now := timeutil.NowMs()
	res, err := DB.Exec(`UPDATE group_persona_assignments SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND deleted_at = 0`, now, now, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ListUnconfiguredGroups returns group-chat sessions for an agent that have
// no entry in group_persona_assignments. Private chats (empty channel_conversation_id)
// are excluded.
func ListUnconfiguredGroups(agentID string) ([]UnconfiguredGroup, error) {
	rows, err := DB.Query(`
		SELECT s.session_key,
		       MAX(COALESCE(s.channel_name,'')) AS channel_name,
		       MAX(COALESCE(s.source_channel,'')) AS source_channel,
		       MAX(s.updated_at) AS last_active
		FROM agent_sessions s
		WHERE s.agent_id = ?
		  AND s.channel_conversation_id != ''
		  AND s.deleted_at = 0
		  AND s.session_key NOT IN (
		      SELECT ga.session_key FROM group_persona_assignments ga
		      WHERE ga.agent_id = ? AND ga.deleted_at = 0
		  )
		GROUP BY s.session_key
		ORDER BY last_active DESC`, agentID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UnconfiguredGroup
	for rows.Next() {
		var g UnconfiguredGroup
		if err := rows.Scan(&g.SessionKey, &g.ChannelName, &g.SourceChannel, &g.LastActive); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if out == nil {
		out = []UnconfiguredGroup{}
	}
	return out, rows.Err()
}
