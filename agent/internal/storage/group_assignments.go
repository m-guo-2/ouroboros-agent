package storage

import (
	"database/sql"
	"fmt"
)

func scanGroupAssignment(scan func(...interface{}) error) (GroupAssignment, error) {
	var ga GroupAssignment
	var personaID sql.NullString

	if err := scan(
		&ga.ID, &ga.AgentID, &ga.SessionKey, &ga.GroupName,
		&personaID, &ga.CreatedAt, &ga.UpdatedAt,
	); err != nil {
		return ga, err
	}
	if personaID.Valid {
		ga.PersonaID = &personaID.String
	}
	return ga, nil
}

const groupAssignmentSelectSQL = `SELECT id, agent_id, session_key, group_name, persona_id, created_at, updated_at`

// GetGroupAssignment finds the assignment for a specific agent + session_key.
// Returns nil, nil when no assignment exists.
func GetGroupAssignment(agentID, sessionKey string) (*GroupAssignment, error) {
	row := DB.QueryRow(
		groupAssignmentSelectSQL+` FROM group_persona_assignments WHERE agent_id = ? AND session_key = ?`,
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
	row := DB.QueryRow(groupAssignmentSelectSQL+` FROM group_persona_assignments WHERE id = ?`, id)
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
		groupAssignmentSelectSQL+` FROM group_persona_assignments WHERE agent_id = ? ORDER BY created_at ASC`,
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
	now := nowTimestamp()
	_, err := DB.Exec(
		`INSERT INTO group_persona_assignments (id, agent_id, session_key, group_name, persona_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ga.ID, ga.AgentID, ga.SessionKey, ga.GroupName, nullStr(ga.PersonaID), now, now,
	)
	if err != nil {
		return nil, err
	}
	return GetGroupAssignmentByID(ga.ID)
}

func UpdateGroupAssignment(id string, updates map[string]interface{}) (*GroupAssignment, error) {
	now := nowTimestamp()
	colMap := map[string]string{
		"groupName": "group_name",
		"personaId": "persona_id",
	}
	for key, val := range updates {
		col, ok := colMap[key]
		if !ok {
			continue
		}
		if _, err := DB.Exec(
			fmt.Sprintf("UPDATE group_persona_assignments SET %s = ?, updated_at = ? WHERE id = ?", col),
			val, now, id,
		); err != nil {
			return nil, err
		}
	}
	return GetGroupAssignmentByID(id)
}

func DeleteGroupAssignment(id string) (bool, error) {
	res, err := DB.Exec("DELETE FROM group_persona_assignments WHERE id = ?", id)
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
		SELECT DISTINCT s.session_key, COALESCE(s.channel_name,''), COALESCE(s.source_channel,''), MAX(s.updated_at) as last_active
		FROM agent_sessions s
		WHERE s.agent_id = ?
		  AND s.channel_conversation_id != ''
		  AND s.session_key NOT IN (
		      SELECT ga.session_key FROM group_persona_assignments ga WHERE ga.agent_id = ?
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
