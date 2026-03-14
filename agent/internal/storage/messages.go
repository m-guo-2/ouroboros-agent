package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"agent/internal/timeutil"
)

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

// DeleteSessionMessages removes all messages for a session.
func DeleteSessionMessages(sessionID string) error {
	_, err := DB.Exec("DELETE FROM messages WHERE session_id = ?", sessionID)
	return err
}

// GetMessageByID retrieves a single message by ID.
func GetMessageByID(msgID string) (*MessageData, error) {
	var m MessageData
	err := DB.QueryRow(
		`SELECT id, session_id, role, content,
		        COALESCE(message_type,'text'), COALESCE(channel,''), COALESCE(channel_message_id,''),
		        COALESCE(trace_id,''), COALESCE(initiator,''),
		        COALESCE(sender_name,''), COALESCE(sender_id,''),
		        COALESCE(attachments_json,'[]'), created_at
		 FROM messages WHERE id = ?`, msgID,
	).Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.MessageType, &m.Channel,
		&m.ChannelMessageID, &m.TraceID, &m.Initiator, &m.SenderName, &m.SenderID, (*jsonStringSliceAttachment)(&m.Attachments), &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &m, err
}

// GetSessionMessages returns up to limit messages for a session, oldest-first.
func GetSessionMessages(sessionID string, limit int) ([]MessageData, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, role, content,
		        COALESCE(message_type,'text'), COALESCE(channel,''), COALESCE(channel_message_id,''),
		        COALESCE(trace_id,''), COALESCE(initiator,''),
		        COALESCE(sender_name,''), COALESCE(sender_id,''),
		        COALESCE(attachments_json,'[]'), created_at
		 FROM messages WHERE session_id = ?
		 ORDER BY created_at ASC LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageData
	for rows.Next() {
		var m MessageData
		if err := rows.Scan(
			&m.ID, &m.SessionID, &m.Role, &m.Content,
			&m.MessageType, &m.Channel, &m.ChannelMessageID,
			&m.TraceID, &m.Initiator,
			&m.SenderName, &m.SenderID, (*jsonStringSliceAttachment)(&m.Attachments), &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// GetLatestSessionMessages returns the most recent N messages for a session, in chronological (ASC) order.
func GetLatestSessionMessages(sessionID string, limit int) ([]MessageData, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, role, content,
		        COALESCE(message_type,'text'), COALESCE(channel,''), COALESCE(channel_message_id,''),
		        COALESCE(trace_id,''), COALESCE(initiator,''),
		        COALESCE(sender_name,''), COALESCE(sender_id,''),
		        COALESCE(attachments_json,'[]'), created_at
		 FROM messages WHERE session_id = ?
		 ORDER BY created_at DESC LIMIT ?`,
		sessionID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageData
	for rows.Next() {
		var m MessageData
		if err := rows.Scan(
			&m.ID, &m.SessionID, &m.Role, &m.Content,
			&m.MessageType, &m.Channel, &m.ChannelMessageID,
			&m.TraceID, &m.Initiator,
			&m.SenderName, &m.SenderID, (*jsonStringSliceAttachment)(&m.Attachments), &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reverseMessages(msgs)
	return msgs, nil
}

// GetRecentMessagesBefore returns N messages immediately before beforeTime, in chronological (ASC) order.
func GetRecentMessagesBefore(sessionID string, beforeTime int64, limit int) ([]MessageData, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, role, content,
		        COALESCE(message_type,'text'), COALESCE(channel,''), COALESCE(channel_message_id,''),
		        COALESCE(trace_id,''), COALESCE(initiator,''),
		        COALESCE(sender_name,''), COALESCE(sender_id,''),
		        COALESCE(attachments_json,'[]'), created_at
		 FROM messages WHERE session_id = ? AND created_at < ?
		 ORDER BY created_at DESC LIMIT ?`,
		sessionID, beforeTime, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageData
	for rows.Next() {
		var m MessageData
		if err := rows.Scan(
			&m.ID, &m.SessionID, &m.Role, &m.Content,
			&m.MessageType, &m.Channel, &m.ChannelMessageID,
			&m.TraceID, &m.Initiator,
			&m.SenderName, &m.SenderID, (*jsonStringSliceAttachment)(&m.Attachments), &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reverseMessages(msgs)
	return msgs, nil
}

func reverseMessages(msgs []MessageData) {
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
}

// GetMessagesBefore returns messages for a session created before the given time (Unix ms), oldest-first.
func GetMessagesBefore(sessionID string, beforeTime int64, limit int) ([]MessageData, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, role, content,
		        COALESCE(message_type,'text'), COALESCE(channel,''), COALESCE(channel_message_id,''),
		        COALESCE(trace_id,''), COALESCE(initiator,''),
		        COALESCE(sender_name,''), COALESCE(sender_id,''),
		        COALESCE(attachments_json,'[]'), created_at
		 FROM messages WHERE session_id = ? AND created_at < ?
		 ORDER BY created_at ASC LIMIT ?`,
		sessionID, beforeTime, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageData
	for rows.Next() {
		var m MessageData
		if err := rows.Scan(
			&m.ID, &m.SessionID, &m.Role, &m.Content,
			&m.MessageType, &m.Channel, &m.ChannelMessageID,
			&m.TraceID, &m.Initiator,
			&m.SenderName, &m.SenderID, (*jsonStringSliceAttachment)(&m.Attachments), &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// SearchMessages returns messages for a session whose content matches the query string.
func SearchMessages(sessionID, query string, beforeTime int64, limit int) ([]MessageData, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, role, content,
		        COALESCE(message_type,'text'), COALESCE(channel,''), COALESCE(channel_message_id,''),
		        COALESCE(trace_id,''), COALESCE(initiator,''),
		        COALESCE(sender_name,''), COALESCE(sender_id,''),
		        COALESCE(attachments_json,'[]'), created_at
		 FROM messages
		 WHERE session_id = ? AND created_at < ? AND content LIKE ?
		 ORDER BY created_at DESC LIMIT ?`,
		sessionID, beforeTime, "%"+query+"%", limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []MessageData
	for rows.Next() {
		var m MessageData
		if err := rows.Scan(
			&m.ID, &m.SessionID, &m.Role, &m.Content,
			&m.MessageType, &m.Channel, &m.ChannelMessageID,
			&m.TraceID, &m.Initiator,
			&m.SenderName, &m.SenderID, (*jsonStringSliceAttachment)(&m.Attachments), &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// SaveMessage inserts a new message row and returns a stub record.
func SaveMessage(params map[string]interface{}) (*MessageData, error) {
	id := newID()
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
	attachmentsJSON := "[]"
	if len(attachments) > 0 {
		if raw, err := json.Marshal(attachments); err == nil {
			attachmentsJSON = string(raw)
		}
	}

	now := timeutil.NowMs()
	_, err := DB.Exec(
		`INSERT INTO messages
		 (id, session_id, role, content, message_type, channel, channel_message_id, trace_id, initiator, sender_name, sender_id, attachments_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, sessionID, role, content, msgType, channel, channelMessageID, traceID, initiator, senderName, senderID, attachmentsJSON, now,
	)
	if err != nil {
		return nil, err
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
		CreatedAt:        now,
	}, nil
}

type jsonStringSliceAttachment []AttachmentData

func (a *jsonStringSliceAttachment) Scan(src interface{}) error {
	raw, ok := src.(string)
	if !ok {
		if bytes, ok := src.([]byte); ok {
			raw = string(bytes)
		} else {
			*a = nil
			return nil
		}
	}
	if raw == "" {
		*a = nil
		return nil
	}
	var attachments []AttachmentData
	if err := json.Unmarshal([]byte(raw), &attachments); err != nil {
		return err
	}
	*a = attachments
	return nil
}
