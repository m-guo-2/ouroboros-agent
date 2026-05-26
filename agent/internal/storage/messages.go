package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"agent/internal/timeutil"
)

const messagesSelectCols = `id, session_id, role, content,
	message_type, channel, channel_message_id,
	trace_id, initiator, sender_name, sender_id,
	channel_meta, created_at`

// CountSessionMessages returns the number of messages for a session.
func CountSessionMessages(sessionID string) (int, error) {
	var n int
	err := DB.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id = ?", sessionID).Scan(&n)
	return n, err
}

// CountSessionMessagesBatch returns message counts keyed by session id.
func CountSessionMessagesBatch(sessionIDs []string) (map[string]int, error) {
	counts := make(map[string]int, len(sessionIDs))
	if len(sessionIDs) == 0 {
		return counts, nil
	}

	placeholders := make([]string, 0, len(sessionIDs))
	args := make([]interface{}, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		placeholders = append(placeholders, "?")
		args = append(args, sessionID)
	}

	rows, err := DB.Query(
		"SELECT session_id, COUNT(*) FROM messages WHERE session_id IN ("+strings.Join(placeholders, ",")+") GROUP BY session_id",
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sessionID string
		var count int
		if err := rows.Scan(&sessionID, &count); err != nil {
			return nil, err
		}
		counts[sessionID] = count
	}
	return counts, rows.Err()
}

// DeleteSessionMessages removes all messages and their children for a session.
// Messages are an immutable log; deletion is only used for session purge.
func DeleteSessionMessages(sessionID string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE mtc FROM message_tool_calls mtc
		 JOIN messages m ON m.id = mtc.message_id WHERE m.session_id = ?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`DELETE ma FROM message_attachments ma
		 JOIN messages m ON m.id = ma.message_id WHERE m.session_id = ?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM message_lifecycle_events WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM messages WHERE session_id = ?`, sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

// scanMessageCore scans the scalar message columns into MessageData. Children
// (attachments) are loaded by loadMessageAttachments in a separate pass.
func scanMessageCore(scan func(...interface{}) error) (MessageData, error) {
	var m MessageData
	var channelMetaRaw sql.NullString
	err := scan(
		&m.ID, &m.SessionID, &m.Role, &m.Content,
		&m.MessageType, &m.Channel, &m.ChannelMessageID,
		&m.TraceID, &m.Initiator, &m.SenderName, &m.SenderID,
		&channelMetaRaw, &m.CreatedAt,
	)
	if err != nil {
		return m, err
	}
	if channelMetaRaw.Valid && strings.TrimSpace(channelMetaRaw.String) != "" {
		_ = json.Unmarshal([]byte(channelMetaRaw.String), &m.ChannelMeta)
	}
	return m, nil
}

// loadAttachmentsForMessages batch-loads attachments for a slice of messages.
// Avoids N+1 by issuing a single IN (...) query.
func loadAttachmentsForMessages(msgs []MessageData) error {
	if len(msgs) == 0 {
		return nil
	}
	idx := make(map[int64]int, len(msgs))
	args := make([]interface{}, 0, len(msgs))
	placeholders := make([]string, 0, len(msgs))
	for i, m := range msgs {
		idx[m.ID] = i
		args = append(args, m.ID)
		placeholders = append(placeholders, "?")
	}
	rows, err := DB.Query(
		`SELECT message_id, attachment_id, kind, resource_uri,
		        display_name, mime_type, source_type
		 FROM message_attachments
		 WHERE message_id IN (`+strings.Join(placeholders, ",")+`)
		 ORDER BY message_id, seq ASC`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var mid int64
		var a AttachmentData
		if err := rows.Scan(&mid, &a.ID, &a.Kind, &a.ResourceURI, &a.DisplayName, &a.MIMEType, &a.SourceMessageType); err != nil {
			return err
		}
		if i, ok := idx[mid]; ok {
			msgs[i].Attachments = append(msgs[i].Attachments, a)
		}
	}
	return rows.Err()
}

// GetMessageByID retrieves a single message by ID, including its attachments.
func GetMessageByID(msgID int64) (*MessageData, error) {
	row := DB.QueryRow(`SELECT `+messagesSelectCols+` FROM messages WHERE id = ?`, msgID)
	m, err := scanMessageCore(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	msgs := []MessageData{m}
	if err := loadAttachmentsForMessages(msgs); err != nil {
		return nil, err
	}
	return &msgs[0], nil
}

// GetSessionMessages returns up to limit messages for a session, oldest-first.
func GetSessionMessages(sessionID string, limit int) ([]MessageData, error) {
	return queryMessages(`SELECT `+messagesSelectCols+` FROM messages
		WHERE session_id = ? ORDER BY created_at ASC LIMIT ?`, sessionID, limit)
}

// GetLatestSessionMessages returns the most recent N messages for a session,
// in chronological (ASC) order.
func GetLatestSessionMessages(sessionID string, limit int) ([]MessageData, error) {
	msgs, err := queryMessages(`SELECT `+messagesSelectCols+` FROM messages
		WHERE session_id = ? ORDER BY id DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	reverseMessages(msgs)
	return msgs, nil
}

// GetRecentMessagesBefore returns N messages with id < beforeID, ASC order.
func GetRecentMessagesBefore(sessionID string, beforeID int64, limit int) ([]MessageData, error) {
	msgs, err := queryMessages(`SELECT `+messagesSelectCols+` FROM messages
		WHERE session_id = ? AND id < ? ORDER BY id DESC LIMIT ?`, sessionID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	reverseMessages(msgs)
	return msgs, nil
}

// GetMessagesBefore returns messages for a session with id < beforeID, oldest-first.
func GetMessagesBefore(sessionID string, beforeID int64, limit int) ([]MessageData, error) {
	return queryMessages(`SELECT `+messagesSelectCols+` FROM messages
		WHERE session_id = ? AND id < ? ORDER BY id ASC LIMIT ?`, sessionID, beforeID, limit)
}

// SearchMessages returns messages for a session whose content matches the query.
func SearchMessages(sessionID, query string, beforeID int64, limit int) ([]MessageData, error) {
	msgs, err := queryMessages(`SELECT `+messagesSelectCols+` FROM messages
		WHERE session_id = ? AND id < ? AND content LIKE ?
		ORDER BY id DESC LIMIT ?`, sessionID, beforeID, "%"+query+"%", limit)
	if err != nil {
		return nil, err
	}
	return msgs, nil
}

func queryMessages(sqlStr string, args ...interface{}) ([]MessageData, error) {
	rows, err := DB.Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []MessageData
	for rows.Next() {
		m, err := scanMessageCore(rows.Scan)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := loadAttachmentsForMessages(msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func reverseMessages(msgs []MessageData) {
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
}

// SaveMessage inserts a new message row, persists its attachments in
// message_attachments, and returns the stored record with the generated ID.
// All writes happen inside a single transaction to maintain the parent/child
// atomicity contract for the split schema.
func SaveMessage(params map[string]interface{}) (*MessageData, error) {
	sessionID, _ := params["sessionId"].(string)
	role, _ := params["role"].(string)
	content, _ := params["content"].(string)
	msgType, _ := params["messageType"].(string)
	if msgType == "" {
		msgType = "text"
	}
	channel, _ := params["channel"].(string)
	channelMessageID, _ := params["channelMessageId"].(string)
	traceID, _ := params["traceId"].(string)
	initiator, _ := params["initiator"].(string)
	senderName, _ := params["senderName"].(string)
	senderID, _ := params["senderId"].(string)

	var attachments []AttachmentData
	switch v := params["attachments"].(type) {
	case []AttachmentData:
		attachments = v
	case []*AttachmentData:
		for _, item := range v {
			if item != nil {
				attachments = append(attachments, *item)
			}
		}
	}

	if sessionID == "" || role == "" || (content == "" && len(attachments) == 0) {
		return nil, fmt.Errorf("sessionId, role, and one of content or attachments are required")
	}

	channelMetaJSON := "{}"
	if v, ok := params["channelMeta"].(map[string]any); ok && len(v) > 0 {
		if raw, err := json.Marshal(v); err == nil {
			channelMetaJSON = string(raw)
		}
	}

	now := timeutil.NowMs()

	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO messages
		 (session_id, role, content, message_type, channel, channel_message_id,
		  channel_meta, trace_id, initiator, sender_name, sender_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, role, content, msgType, channel, channelMessageID,
		channelMetaJSON, traceID, initiator, senderName, senderID, now,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()

	for i, a := range attachments {
		if _, err := tx.Exec(
			`INSERT INTO message_attachments
			 (message_id, seq, attachment_id, kind, resource_uri, display_name,
			  mime_type, source_type, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, i, a.ID, a.Kind, a.ResourceURI, a.DisplayName, a.MIMEType, a.SourceMessageType, now,
		); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	var channelMeta map[string]any
	if v, ok := params["channelMeta"].(map[string]any); ok {
		channelMeta = v
	}
	return &MessageData{
		ID:               id,
		SessionID:        sessionID,
		Role:             role,
		Content:          content,
		MessageType:      msgType,
		Channel:          channel,
		ChannelMessageID: channelMessageID,
		TraceID:          traceID,
		Initiator:        initiator,
		SenderName:       senderName,
		SenderID:         senderID,
		Attachments:      attachments,
		ChannelMeta:      channelMeta,
		CreatedAt:        now,
	}, nil
}

// FindAttachmentInSession locates an attachment by ID within a session's
// messages. Uses the indexed join on message_attachments rather than scanning
// every attachments_json blob like the old code did.
func FindAttachmentInSession(sessionID, attachmentID string) (AttachmentData, bool) {
	if sessionID == "" || attachmentID == "" {
		return AttachmentData{}, false
	}
	row := DB.QueryRow(
		`SELECT ma.attachment_id, ma.kind, ma.resource_uri,
		        ma.display_name, ma.mime_type, ma.source_type
		 FROM message_attachments ma
		 JOIN messages m ON m.id = ma.message_id
		 WHERE m.session_id = ? AND ma.attachment_id = ?
		 ORDER BY ma.message_id DESC LIMIT 1`,
		sessionID, attachmentID)
	var a AttachmentData
	if err := row.Scan(&a.ID, &a.Kind, &a.ResourceURI, &a.DisplayName, &a.MIMEType, &a.SourceMessageType); err != nil {
		return AttachmentData{}, false
	}
	return a, true
}
