package storage

import "fmt"

type CompactionData struct {
	ID                   string `json:"id"`
	SessionID            string `json:"sessionId"`
	Summary              string `json:"summary"`
	ArchivedBeforeTime   string `json:"archivedBeforeTime"`
	ArchivedMessageCount int    `json:"archivedMessageCount"`
	TokenCountBefore     int    `json:"tokenCountBefore"`
	TokenCountAfter      int    `json:"tokenCountAfter"`
	CompactModel         string `json:"compactModel"`
	CreatedAt            string `json:"createdAt"`
}

func SaveCompaction(data CompactionData) error {
	id := newID()
	_, err := DB.Exec(
		`INSERT INTO context_compactions
		 (id, session_id, summary, archived_before_time, archived_message_count,
		  token_count_before, token_count_after, compact_model)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, data.SessionID, data.Summary, data.ArchivedBeforeTime,
		data.ArchivedMessageCount, data.TokenCountBefore, data.TokenCountAfter, data.CompactModel,
	)
	return err
}

func GetLatestCompaction(sessionID string) (*CompactionData, error) {
	var c CompactionData
	err := DB.QueryRow(
		`SELECT id, session_id, summary, archived_before_time, archived_message_count,
		        token_count_before, token_count_after, COALESCE(compact_model,''),
		        COALESCE(created_at,'')
		 FROM context_compactions
		 WHERE session_id = ?
		 ORDER BY created_at DESC LIMIT 1`, sessionID,
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
		        COALESCE(created_at,'')
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
