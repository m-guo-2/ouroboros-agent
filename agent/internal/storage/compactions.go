package storage

import (
	"encoding/json"
	"fmt"

	"agent/internal/timeutil"
	"agent/internal/types"
)

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

func SaveCompactionArchive(compactionID int64, sessionID string, messages []types.AgentMessage) error {
	data, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("marshal archived messages: %w", err)
	}
	_, err = DB.Exec(
		`INSERT INTO context_compaction_archives
		 (session_id, compaction_id, archived_messages, message_count, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		sessionID, compactionID, string(data), len(messages), timeutil.NowMs(),
	)
	return err
}
