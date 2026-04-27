package storage

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"

	"agent/internal/timeutil"
)

// newID generates a short random ID suitable for sessions and messages.
func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x%d", b, time.Now().UnixNano()%1e6)
}

func scanSession(row *sql.Row) (*SessionData, error) {
	var sd SessionData
	var agentID, userID, sourceChannel, sessionKey, channelConvID, channelName, workDir, mode, ctx sql.NullString
	err := row.Scan(
		&sd.ID, &sd.Title, &agentID, &userID, &sourceChannel,
		&sessionKey, &channelConvID, &channelName, &workDir,
		&sd.ExecutionStatus, &mode, &sd.EventCursor, &sd.CreatedAt, &sd.UpdatedAt, &ctx,
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
	sd.Mode = mode.String
	if sd.Mode == "" {
		sd.Mode = "normal"
	}
	sd.Context = ctx.String
	return &sd, nil
}

const sessionSelectSQL = `
	SELECT id, title, agent_id, user_id, source_channel, session_key,
	       channel_conversation_id, COALESCE(channel_name,''), work_dir,
	       COALESCE(execution_status,'idle'),
	       COALESCE(mode,'normal'),
	       COALESCE(event_cursor, 0),
	       created_at,
	       updated_at,
	       COALESCE(context,'')
	FROM agent_sessions`

// GetSession retrieves a session by its ID.
func GetSession(sessionID string) (*SessionData, error) {
	row := DB.QueryRow(sessionSelectSQL+" WHERE id = ? AND deleted_at = 0", sessionID)
	return scanSession(row)
}

// FindSessionByKey finds the most-recent session for a given agent + session key.
func FindSessionByKey(agentID, sessionKey string) (*SessionData, error) {
	row := DB.QueryRow(
		sessionSelectSQL+" WHERE agent_id = ? AND session_key = ? AND deleted_at = 0 ORDER BY created_at DESC LIMIT 1",
		agentID, sessionKey,
	)
	return scanSession(row)
}

// FindSessionByConversationID finds the most-recent session matching a channelConversationId + agentId.
// Used as legacy fallback for sessions created before session_key was introduced.
func FindSessionByConversationID(channelConversationID, agentID string) (*SessionData, error) {
	row := DB.QueryRow(
		sessionSelectSQL+" WHERE channel_conversation_id = ? AND agent_id = ? AND deleted_at = 0 ORDER BY created_at DESC LIMIT 1",
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
			fmt.Sprintf("UPDATE agent_sessions SET %s = ?, updated_at = ? WHERE id = ?", col),
			val, timeutil.NowMs(), sessionID,
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

	now := timeutil.NowMs()
	_, err := DB.Exec(
		`INSERT INTO agent_sessions
		 (id, title, agent_id, user_id, source_channel, session_key, channel_conversation_id, channel_name, work_dir, execution_status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'idle', ?, ?)`,
		id, title, agentID, userID, channel, sessionKey, channelConvID, channelName, workDir, now, now,
	)
	if err != nil {
		return nil, err
	}
	return GetSession(id)
}

// ListSessions returns sessions filtered by optional agentID/userID/channel/status/search, newest first.
// When beforeUpdatedAt > 0, only sessions with updated_at < beforeUpdatedAt are returned (cursor pagination).
func ListSessions(agentID, userID, channel, status, search string, limit int, beforeUpdatedAt int64) ([]SessionData, error) {
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
	if status != "" {
		clauses = append(clauses, "execution_status = ?")
		args = append(args, status)
	}
	if search != "" {
		clauses = append(clauses, "(title LIKE ? OR channel_name LIKE ?)")
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern)
	}
	if beforeUpdatedAt > 0 {
		clauses = append(clauses, "updated_at < ?")
		args = append(args, beforeUpdatedAt)
	}
	clauses = append(clauses, "deleted_at = 0")
	query += " WHERE " + joinClauses(clauses)
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
		var agentIDn, userIDn, sourceChannel, sessionKey, channelConvID, channelName, workDir, mode, ctx sql.NullString
		if err := rows.Scan(
			&sd.ID, &sd.Title, &agentIDn, &userIDn, &sourceChannel,
			&sessionKey, &channelConvID, &channelName, &workDir,
			&sd.ExecutionStatus, &mode, &sd.EventCursor, &sd.CreatedAt, &sd.UpdatedAt, &ctx,
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
		sd.Mode = mode.String
		if sd.Mode == "" {
			sd.Mode = "normal"
		}
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

// DeleteSession soft-deletes a session by ID. The messages log is kept for
// auditability; use PurgeSession for irreversible physical removal.
func DeleteSession(sessionID string) error {
	now := timeutil.NowMs()
	_, err := DB.Exec(`UPDATE agent_sessions SET deleted_at = ?, updated_at = ?
		WHERE id = ? AND deleted_at = 0`, now, now, sessionID)
	return err
}

// PurgeSession irreversibly removes a session and all its messages. Only
// invoked by explicit admin tooling.
func PurgeSession(sessionID string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := deleteSessionChildren(tx, sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM agent_sessions WHERE id = ?`, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

func deleteSessionChildren(tx *sql.Tx, sessionID string) error {
	for _, stmt := range []string{
		`DELETE mtc FROM message_tool_calls mtc
		 JOIN messages m ON m.id = mtc.message_id WHERE m.session_id = ?`,
		`DELETE ma FROM message_attachments ma
		 JOIN messages m ON m.id = ma.message_id WHERE m.session_id = ?`,
		`DELETE ccam FROM context_compaction_archived_messages ccam
		 JOIN context_compaction_archives cca ON cca.id = ccam.archive_id
		 WHERE cca.session_id = ?`,
		`DELETE FROM context_compaction_archives WHERE session_id = ?`,
		`DELETE FROM context_compactions WHERE session_id = ?`,
		`DELETE FROM session_events WHERE session_id = ?`,
		`DELETE FROM session_facts WHERE session_id = ?`,
		`DELETE FROM session_active_skills WHERE session_id = ?`,
		`DELETE FROM messages WHERE session_id = ?`,
	} {
		if _, err := tx.Exec(stmt, sessionID); err != nil {
			return err
		}
	}
	return nil
}

// UpdateSessionContextAndCursor atomically persists both the conversation
// context and the event cursor in a single UPDATE statement, ensuring crash
// recovery consistency.
func UpdateSessionContextAndCursor(sessionID, context, workDir string, eventCursor int64) error {
	_, err := DB.Exec(
		`UPDATE agent_sessions SET context = ?, event_cursor = ?, work_dir = ?, updated_at = ? WHERE id = ?`,
		context, eventCursor, workDir, timeutil.NowMs(), sessionID,
	)
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
		"eventCursor":     "event_cursor",
		"mode":            "mode",
	}
	for key, val := range updates {
		col, ok := colMap[key]
		if !ok {
			continue
		}
		if _, err := DB.Exec(
			fmt.Sprintf("UPDATE agent_sessions SET %s = ?, updated_at = ? WHERE id = ?", col),
			fmt.Sprint(val), timeutil.NowMs(), sessionID,
		); err != nil {
			return err
		}
	}
	return nil
}
