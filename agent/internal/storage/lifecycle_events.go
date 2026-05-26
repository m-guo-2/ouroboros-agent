package storage

import (
	"database/sql"
	"encoding/json"
	"strings"

	"agent/internal/timeutil"
)

type MessageLifecycleEvent struct {
	ID               int64          `json:"id"`
	SessionID        string         `json:"sessionId"`
	MessageID        int64          `json:"messageId,omitempty"`
	TraceID          string         `json:"traceId,omitempty"`
	ChannelMessageID string         `json:"channelMessageId,omitempty"`
	Stage            string         `json:"stage"`
	Status           string         `json:"status"`
	Outcome          string         `json:"outcome,omitempty"`
	Summary          string         `json:"summary,omitempty"`
	Payload          map[string]any `json:"payload,omitempty"`
	CreatedAt        int64          `json:"createdAt"`
}

func SaveLifecycleEvent(params map[string]any) error {
	sessionID, _ := params["sessionId"].(string)
	stage, _ := params["stage"].(string)
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(stage) == "" {
		return nil
	}
	status, _ := params["status"].(string)
	if status == "" {
		status = "success"
	}
	outcome, _ := params["outcome"].(string)
	summary, _ := params["summary"].(string)
	traceID, _ := params["traceId"].(string)
	channelMessageID, _ := params["channelMessageId"].(string)

	var messageID int64
	switch v := params["messageId"].(type) {
	case int64:
		messageID = v
	case int:
		messageID = int64(v)
	case float64:
		messageID = int64(v)
	}

	payloadJSON := "{}"
	if payload, ok := params["payload"].(map[string]any); ok && len(payload) > 0 {
		if b, err := json.Marshal(payload); err == nil {
			payloadJSON = string(b)
		}
	}

	_, err := DB.Exec(
		`INSERT INTO message_lifecycle_events
		 (session_id, message_id, trace_id, channel_message_id, stage, status, outcome, summary, payload_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, messageID, traceID, channelMessageID, stage, status, outcome, summary, payloadJSON, timeutil.NowMs(),
	)
	return err
}

func ListLifecycleEvents(sessionID string) ([]MessageLifecycleEvent, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, message_id, trace_id, channel_message_id, stage, status, outcome, summary, payload_json, created_at
		 FROM message_lifecycle_events
		 WHERE session_id = ?
		 ORDER BY id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []MessageLifecycleEvent
	for rows.Next() {
		ev, err := scanLifecycleEvent(rows.Scan)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

func HasLifecycleEvent(sessionID, traceID, stage string) bool {
	if sessionID == "" || traceID == "" || stage == "" {
		return false
	}
	var exists bool
	err := DB.QueryRow(
		`SELECT EXISTS(
			SELECT 1 FROM message_lifecycle_events
			WHERE session_id = ? AND trace_id = ? AND stage = ?
		)`,
		sessionID, traceID, stage,
	).Scan(&exists)
	return err == nil && exists
}

func HasSuccessfulLifecycleEvent(sessionID, traceID, stage string) bool {
	if sessionID == "" || traceID == "" || stage == "" {
		return false
	}
	var exists bool
	err := DB.QueryRow(
		`SELECT EXISTS(
			SELECT 1 FROM message_lifecycle_events
			WHERE session_id = ? AND trace_id = ? AND stage = ? AND status = 'success'
		)`,
		sessionID, traceID, stage,
	).Scan(&exists)
	return err == nil && exists
}

func DeleteLifecycleEvents(sessionID string) error {
	_, err := DB.Exec(`DELETE FROM message_lifecycle_events WHERE session_id = ?`, sessionID)
	return err
}

func scanLifecycleEvent(scan func(...interface{}) error) (MessageLifecycleEvent, error) {
	var ev MessageLifecycleEvent
	var payloadRaw sql.NullString
	err := scan(
		&ev.ID, &ev.SessionID, &ev.MessageID, &ev.TraceID, &ev.ChannelMessageID,
		&ev.Stage, &ev.Status, &ev.Outcome, &ev.Summary, &payloadRaw, &ev.CreatedAt,
	)
	if err != nil {
		return ev, err
	}
	if payloadRaw.Valid && strings.TrimSpace(payloadRaw.String) != "" {
		_ = json.Unmarshal([]byte(payloadRaw.String), &ev.Payload)
	}
	return ev, nil
}
