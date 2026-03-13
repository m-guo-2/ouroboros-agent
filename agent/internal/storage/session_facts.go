package storage

import (
	"fmt"

	"agent/internal/timeutil"
)

type SessionFact struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Fact      string `json:"fact"`
	Category  string `json:"category"`
	CreatedAt int64  `json:"createdAt"`
}

func SaveSessionFacts(sessionID string, facts []string, category string) (int, error) {
	if len(facts) == 0 {
		return 0, nil
	}
	if category == "" {
		category = "general"
	}

	tx, err := DB.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT INTO session_facts (id, session_id, fact, category, created_at) VALUES (?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return 0, fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	now := timeutil.NowMs()
	saved := 0
	for _, f := range facts {
		if f == "" {
			continue
		}
		if _, err := stmt.Exec(newID(), sessionID, f, category, now); err != nil {
			return saved, fmt.Errorf("insert fact: %w", err)
		}
		saved++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return saved, nil
}

func GetSessionFacts(sessionID string) ([]SessionFact, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, fact, category, created_at
		 FROM session_facts
		 WHERE session_id = ?
		 ORDER BY created_at ASC`, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query session facts: %w", err)
	}
	defer rows.Close()

	var out []SessionFact
	for rows.Next() {
		var f SessionFact
		if err := rows.Scan(&f.ID, &f.SessionID, &f.Fact, &f.Category, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
