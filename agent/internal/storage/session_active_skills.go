package storage

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"

	"agent/internal/timeutil"
)

type ActivateSessionSkillOptions struct {
	SessionID          string
	SkillID            string
	Source             string
	SourceEvent        string
	ScopeType          string
	ScopeKey           string
	ActivatedAtSeq     int64
	ExpiresAfterEvents int
}

// ActivateSessionSkill records a dynamically loaded skill for a session.
// Uses INSERT OR IGNORE for idempotency.
func ActivateSessionSkill(sessionID, skillID, source string) error {
	return ActivateSessionSkillWithOptions(ActivateSessionSkillOptions{
		SessionID:   sessionID,
		SkillID:     skillID,
		Source:      source,
		ScopeType:   "session",
		SourceEvent: source,
	})
}

func ActivateSessionSkillWithOptions(opts ActivateSessionSkillOptions) error {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := fmt.Sprintf("sas-%x", b)
	if opts.Source == "" {
		opts.Source = "hook"
	}
	if opts.SourceEvent == "" {
		opts.SourceEvent = opts.Source
	}
	opts.ScopeType = strings.TrimSpace(opts.ScopeType)
	if opts.ScopeType == "" {
		opts.ScopeType = "session"
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingOrder int64
	err = tx.QueryRow(
		`SELECT activation_order FROM session_active_skills
		 WHERE session_id = ? AND skill_id = ? AND scope_type = ? AND scope_key = ? AND status = 'active'`,
		opts.SessionID, opts.SkillID, opts.ScopeType, opts.ScopeKey,
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
		opts.SessionID,
	).Scan(&nextOrder); err != nil {
		return err
	}

	now := timeutil.NowMs()
	_, err = tx.Exec(
		`INSERT INTO session_active_skills
		 (id, session_id, skill_id, source, source_event, scope_type, scope_key, status,
		  activated_at_seq, expires_after_events, activation_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?)`,
		id, opts.SessionID, opts.SkillID, opts.Source, opts.SourceEvent, opts.ScopeType, opts.ScopeKey,
		opts.ActivatedAtSeq, opts.ExpiresAfterEvents, nextOrder, now, now,
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
		 WHERE session_id = ? AND status = 'active'
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

func ExpireSessionSkillsBeforeContext(sessionID string, currentSeq int64) (int64, error) {
	if currentSeq <= 0 {
		return 0, nil
	}
	now := timeutil.NowMs()
	res, err := DB.Exec(
		`UPDATE session_active_skills
		 SET status = 'expired', completed_at = ?, completion_reason = 'expires_after_events', updated_at = ?
		 WHERE session_id = ?
		   AND status = 'active'
		   AND expires_after_events > 0
		   AND activated_at_seq > 0
		   AND (? - activated_at_seq) >= expires_after_events`,
		now, now, sessionID, currentSeq,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// DeactivateSessionSkill marks active dynamically loaded skills as completed.
// Returns true if a record was actually changed.
func DeactivateSessionSkill(sessionID, skillID string) (bool, error) {
	now := timeutil.NowMs()
	res, err := DB.Exec(
		`UPDATE session_active_skills
		 SET status = 'completed', completed_at = ?, completion_reason = 'complete_skill', updated_at = ?
		 WHERE session_id = ? AND skill_id = ? AND status = 'active'`,
		now, now, sessionID, skillID,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
