package storage

import (
	"database/sql"
	"fmt"
)

// CountSessionMessages returns the number of messages for a session.
func CountSessionMessages(sessionID string) (int, error) {
	var n int
	err := DB.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id = ?", sessionID).Scan(&n)
	return n, err
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
		        COALESCE(created_at,'')
		 FROM messages WHERE id = ?`, msgID,
	).Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.MessageType, &m.Channel,
		&m.ChannelMessageID, &m.TraceID, &m.Initiator, &m.SenderName, &m.SenderID, &m.CreatedAt)
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
		        COALESCE(created_at,'')
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
			&m.SenderName, &m.SenderID, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// GetMessagesBefore returns messages for a session created before the given time, oldest-first.
func GetMessagesBefore(sessionID, beforeTime string, limit int) ([]MessageData, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, role, content,
		        COALESCE(message_type,'text'), COALESCE(channel,''), COALESCE(channel_message_id,''),
		        COALESCE(trace_id,''), COALESCE(initiator,''),
		        COALESCE(sender_name,''), COALESCE(sender_id,''),
		        COALESCE(created_at,'')
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
			&m.SenderName, &m.SenderID, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// SearchMessages returns messages for a session whose content matches the query string.
func SearchMessages(sessionID, query, beforeTime string, limit int) ([]MessageData, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, role, content,
		        COALESCE(message_type,'text'), COALESCE(channel,''), COALESCE(channel_message_id,''),
		        COALESCE(trace_id,''), COALESCE(initiator,''),
		        COALESCE(sender_name,''), COALESCE(sender_id,''),
		        COALESCE(created_at,'')
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
			&m.SenderName, &m.SenderID, &m.CreatedAt,
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

	if sessionID == "" || role == "" || content == "" {
		return nil, fmt.Errorf("sessionId, role, content are required")
	}

	_, err := DB.Exec(
		`INSERT INTO messages
		 (id, session_id, role, content, message_type, channel, channel_message_id, trace_id, initiator, sender_name, sender_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, sessionID, role, content, msgType, channel, channelMessageID, traceID, initiator, senderName, senderID,
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
	}, nil
}
