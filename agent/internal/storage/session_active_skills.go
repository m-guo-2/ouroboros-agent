package storage

import (
	"crypto/rand"
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
	_, err := DB.Exec(
		`INSERT OR IGNORE INTO session_active_skills (id, session_id, skill_id, source, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		id, sessionID, skillID, source, timeutil.NowMs(),
	)
	return err
}

// GetActiveSessionSkills returns skill IDs dynamically loaded for a session.
func GetActiveSessionSkills(sessionID string) ([]string, error) {
	rows, err := DB.Query(
		`SELECT skill_id FROM session_active_skills WHERE session_id = ? ORDER BY created_at`,
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
