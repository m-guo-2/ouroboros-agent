package storage

import (
	"crypto/rand"
	"database/sql"
	"fmt"

	"agent/internal/timeutil"
)

// ActivateSessionSkill records a dynamically loaded skill for a session.
// Uses INSERT OR IGNORE for idempotency.
func ActivateSessionSkill(sessionID, skillID, source string) error {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := fmt.Sprintf("sas-%x", b)
	if source == "" {
		source = "hook"
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingOrder int64
	err = tx.QueryRow(
		`SELECT activation_order FROM session_active_skills WHERE session_id = ? AND skill_id = ?`,
		sessionID, skillID,
	).Scan(&existingOrder)
	switch err {
	case nil:
		return tx.Commit()
	case sql.ErrNoRows:
	default:
		return err
	}

	var nextOrder int64
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(activation_order), 0) + 1 FROM session_active_skills WHERE session_id = ?`,
		sessionID,
	).Scan(&nextOrder); err != nil {
		return err
	}

	_, err = tx.Exec(
		`INSERT INTO session_active_skills (id, session_id, skill_id, source, created_at, activation_order)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, sessionID, skillID, source, timeutil.NowMs(), nextOrder,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// GetActiveSessionSkills returns skill IDs dynamically loaded for a session.
func GetActiveSessionSkills(sessionID string) ([]string, error) {
	rows, err := DB.Query(
		`SELECT skill_id FROM session_active_skills
		 WHERE session_id = ?
		 ORDER BY activation_order ASC, created_at ASC, id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeactivateSessionSkill removes a dynamically loaded skill from a session.
// Returns true if a record was actually deleted.
func DeactivateSessionSkill(sessionID, skillID string) (bool, error) {
	res, err := DB.Exec(
		`DELETE FROM session_active_skills WHERE session_id = ? AND skill_id = ?`,
		sessionID, skillID,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
