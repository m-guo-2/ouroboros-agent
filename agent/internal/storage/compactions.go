package storage

import (
	"encoding/json"
	"fmt"

	"agent/internal/timeutil"
	"agent/internal/types"
)

// CompactionData is the header for one compaction run. The archived messages
// themselves live in context_compaction_archives / context_compaction_archived_messages.
type CompactionData struct {
	ID                   int64  `json:"id"`
	SessionID            string `json:"sessionId"`
	Summary              string `json:"summary"`
	ArchivedBeforeTime   int64  `json:"archivedBeforeTime"`
	ArchivedMessageCount int    `json:"archivedMessageCount"`
	TokenCountBefore     int    `json:"tokenCountBefore"`
	TokenCountAfter      int    `json:"tokenCountAfter"`
	CompactModel         string `json:"compactModel"`
	CreatedAt            int64  `json:"createdAt"`
}

func SaveCompaction(data CompactionData) (int64, error) {
	now := timeutil.NowMs()
	result, err := DB.Exec(
		`INSERT INTO context_compactions
		 (session_id, summary, archived_before_time, archived_message_count,
		  token_count_before, token_count_after, compact_model, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		data.SessionID, data.Summary, data.ArchivedBeforeTime,
		data.ArchivedMessageCount, data.TokenCountBefore, data.TokenCountAfter, data.CompactModel, now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetLatestCompaction(sessionID string) (*CompactionData, error) {
	var c CompactionData
	err := DB.QueryRow(
		`SELECT id, session_id, summary, archived_before_time, archived_message_count,
		        token_count_before, token_count_after, COALESCE(compact_model,''),
		        created_at
		 FROM context_compactions
		 WHERE session_id = ?
		 ORDER BY id DESC LIMIT 1`, sessionID,
	).Scan(&c.ID, &c.SessionID, &c.Summary, &c.ArchivedBeforeTime,
		&c.ArchivedMessageCount, &c.TokenCountBefore, &c.TokenCountAfter,
		&c.CompactModel, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func ListCompactions(sessionID string) ([]CompactionData, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, summary, archived_before_time, archived_message_count,
		        token_count_before, token_count_after, COALESCE(compact_model,''),
		        created_at
		 FROM context_compactions
		 WHERE session_id = ?
		 ORDER BY created_at DESC`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list compactions: %w", err)
	}
	defer rows.Close()

	var out []CompactionData
	for rows.Next() {
		var c CompactionData
		if err := rows.Scan(&c.ID, &c.SessionID, &c.Summary, &c.ArchivedBeforeTime,
			&c.ArchivedMessageCount, &c.TokenCountBefore, &c.TokenCountAfter,
			&c.CompactModel, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SaveCompactionArchive persists an archive header plus one row per archived
// message into the split schema. Everything happens inside a single txn so the
// parent archive only becomes visible once its child rows are in place.
func SaveCompactionArchive(compactionID int64, sessionID string, messages []types.AgentMessage) error {
	now := timeutil.NowMs()

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO context_compaction_archives
		 (session_id, compaction_id, message_count, created_at)
		 VALUES (?, ?, ?, ?)`,
		sessionID, compactionID, len(messages), now,
	)
	if err != nil {
		return fmt.Errorf("insert compaction archive header: %w", err)
	}
	archiveID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id: %w", err)
	}

	for i, msg := range messages {
		content, err := encodeAgentMessageContent(msg)
		if err != nil {
			return fmt.Errorf("marshal archived message %d: %w", i, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO context_compaction_archived_messages
			 (archive_id, seq, original_message_id, role, content, message_type, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			archiveID, i, 0, msg.Role, content, "text", now,
		); err != nil {
			return fmt.Errorf("insert archived message %d: %w", i, err)
		}
	}
	return tx.Commit()
}

// encodeAgentMessageContent renders the message's content blocks as JSON so
// the split schema keeps round-trip fidelity for non-trivial block sequences.
// Simple pure-text messages collapse to their plain text for readability.
func encodeAgentMessageContent(m types.AgentMessage) (string, error) {
	if len(m.Content) == 1 && m.Content[0].Type == "text" {
		return m.Content[0].Text, nil
	}
	b, err := json.Marshal(m.Content)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
