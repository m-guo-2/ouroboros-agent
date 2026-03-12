package storage

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"
)

// newID generates a short random ID suitable for sessions and messages.
func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x%d", b, time.Now().UnixNano()%1e6)
}

func scanSession(row *sql.Row) (*SessionData, error) {
	var sd SessionData
	var agentID, userID, sourceChannel, sessionKey, channelConvID, channelName, workDir, ctx sql.NullString
	err := row.Scan(
		&sd.ID, &sd.Title, &agentID, &userID, &sourceChannel,
		&sessionKey, &channelConvID, &channelName, &workDir,
		&sd.ExecutionStatus, &sd.CreatedAt, &sd.UpdatedAt, &ctx,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sd.AgentID = agentID.String
	sd.UserID = userID.String
	sd.SourceChannel = sourceChannel.String
	sd.SessionKey = sessionKey.String
	sd.ChannelConversationID = channelConvID.String
	sd.ChannelName = channelName.String
	sd.WorkDir = workDir.String
	sd.Context = ctx.String
	return &sd, nil
}

const sessionSelectSQL = `
	SELECT id, title, agent_id, user_id, source_channel, session_key,
	       channel_conversation_id, COALESCE(channel_name,''), work_dir,
	       COALESCE(execution_status,'idle'),
	       COALESCE(created_at,''),
	       COALESCE(updated_at,''),
	       COALESCE(context,'')
	FROM agent_sessions`

// GetSession retrieves a session by its ID.
func GetSession(sessionID string) (*SessionData, error) {
	row := DB.QueryRow(sessionSelectSQL+" WHERE id = ?", sessionID)
	return scanSession(row)
}

// FindSessionByKey finds the most-recent session for a given agent + session key.
func FindSessionByKey(agentID, sessionKey string) (*SessionData, error) {
	row := DB.QueryRow(
		sessionSelectSQL+" WHERE agent_id = ? AND session_key = ? ORDER BY created_at DESC LIMIT 1",
		agentID, sessionKey,
	)
	return scanSession(row)
}

// FindSessionByConversationID finds the most-recent session matching a channelConversationId + agentId.
// Used as legacy fallback for sessions created before session_key was introduced.
func FindSessionByConversationID(channelConversationID, agentID string) (*SessionData, error) {
	row := DB.QueryRow(
		sessionSelectSQL+" WHERE channel_conversation_id = ? AND agent_id = ? ORDER BY created_at DESC LIMIT 1",
		channelConversationID, agentID,
	)
	return scanSession(row)
}

// PatchSession updates a set of named columns directly (channelName, channelConversationId, sessionKey).
func PatchSession(sessionID string, fields map[string]string) error {
	colMap := map[string]string{
		"channelName":           "channel_name",
		"channelConversationId": "channel_conversation_id",
		"sessionKey":            "session_key",
	}
	for key, val := range fields {
		col, ok := colMap[key]
		if !ok {
			continue
		}
		if _, err := DB.Exec(
			fmt.Sprintf("UPDATE agent_sessions SET %s = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", col),
			val, sessionID,
		); err != nil {
			return err
		}
	}
	return nil
}

// CreateSession inserts a new session and returns the created record.
func CreateSession(params map[string]interface{}) (*SessionData, error) {
	id, _ := params["id"].(string)
	if id == "" {
		id = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}
	title, _ := params["title"].(string)
	if title == "" {
		title = "新对话"
	}
	agentID, _ := params["agentId"].(string)
	userID, _ := params["userId"].(string)
	channel, _ := params["channel"].(string)
	sessionKey, _ := params["sessionKey"].(string)
	channelConvID, _ := params["channelConversationId"].(string)
	channelName, _ := params["channelName"].(string)
	workDir, _ := params["workDir"].(string)

	_, err := DB.Exec(
		`INSERT INTO agent_sessions
		 (id, title, agent_id, user_id, source_channel, session_key, channel_conversation_id, channel_name, work_dir, execution_status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'idle')`,
		id, title, agentID, userID, channel, sessionKey, channelConvID, channelName, workDir,
	)
	if err != nil {
		return nil, err
	}
	return GetSession(id)
}

// ListSessions returns sessions filtered by optional agentID/userID/channel, newest first.
func ListSessions(agentID, userID, channel string, limit int) ([]SessionData, error) {
	query := sessionSelectSQL
	var args []interface{}
	var clauses []string
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	if userID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, userID)
	}
	if channel != "" {
		clauses = append(clauses, "source_channel = ?")
		args = append(args, channel)
	}
	if len(clauses) > 0 {
		query += " WHERE " + joinClauses(clauses)
	}
	query += " ORDER BY updated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionData
	for rows.Next() {
		var sd SessionData
		var agentIDn, userIDn, sourceChannel, sessionKey, channelConvID, channelName, workDir, ctx sql.NullString
		if err := rows.Scan(
			&sd.ID, &sd.Title, &agentIDn, &userIDn, &sourceChannel,
			&sessionKey, &channelConvID, &channelName, &workDir,
			&sd.ExecutionStatus, &sd.CreatedAt, &sd.UpdatedAt, &ctx,
		); err != nil {
			return nil, err
		}
		sd.AgentID = agentIDn.String
		sd.UserID = userIDn.String
		sd.SourceChannel = sourceChannel.String
		sd.SessionKey = sessionKey.String
		sd.ChannelConversationID = channelConvID.String
		sd.ChannelName = channelName.String
		sd.WorkDir = workDir.String
		sd.Context = ctx.String
		out = append(out, sd)
	}
	return out, rows.Err()
}

func joinClauses(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " AND "
		}
		result += p
	}
	return result
}

// DeleteSession removes a session row by ID.
func DeleteSession(sessionID string) error {
	_, err := DB.Exec("DELETE FROM agent_sessions WHERE id = ?", sessionID)
	return err
}

// UpdateSession applies a partial update to session fields.
// Supported keys: executionStatus, workDir, context, title, sessionKey.
func UpdateSession(sessionID string, updates map[string]interface{}) error {
	colMap := map[string]string{
		"executionStatus": "execution_status",
		"workDir":         "work_dir",
		"context":         "context",
		"title":           "title",
		"sessionKey":      "session_key",
	}
	for key, val := range updates {
		col, ok := colMap[key]
		if !ok {
			continue
		}
		if _, err := DB.Exec(
			fmt.Sprintf("UPDATE agent_sessions SET %s = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", col),
			fmt.Sprint(val), sessionID,
		); err != nil {
			return err
		}
	}
	return nil
}
