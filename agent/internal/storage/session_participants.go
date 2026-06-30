package storage

import "agent/internal/timeutil"

func HasSessionParticipant(sessionID, channelUserID string) (bool, error) {
	var exists bool
	err := DB.QueryRow(
		`SELECT EXISTS(
			SELECT 1 FROM session_participants
			WHERE session_id = ? AND channel_user_id = ?
		)`,
		sessionID, channelUserID,
	).Scan(&exists)
	return exists, err
}

func UpsertSessionParticipant(sessionID, channelUserID, userID string, firstSeenMessageID, firstSeenSeq int64) (bool, error) {
	now := timeutil.NowMs()
	res, err := DB.Exec(
		`INSERT IGNORE INTO session_participants
		 (session_id, channel_user_id, user_id, first_seen_message_id, first_seen_seq, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessionID, channelUserID, userID, firstSeenMessageID, firstSeenSeq, now, now,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return true, nil
	}
	_, err = DB.Exec(
		`UPDATE session_participants
		 SET user_id = ?, updated_at = ?
		 WHERE session_id = ? AND channel_user_id = ?`,
		userID, now, sessionID, channelUserID,
	)
	return false, err
}

func RecordAgentUserSeen(agentID, userID string) (string, error) {
	now := timeutil.NowMs()
	res, err := DB.Exec(
		`INSERT IGNORE INTO agent_user_seen
		 (agent_id, user_id, first_seen_at, last_seen_at, seen_count)
		 VALUES (?, ?, ?, ?, 1)`,
		agentID, userID, now, now,
	)
	if err != nil {
		return "", err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return "first_seen", nil
	}
	_, err = DB.Exec(
		`UPDATE agent_user_seen
		 SET last_seen_at = ?, seen_count = seen_count + 1
		 WHERE agent_id = ? AND user_id = ?`,
		now, agentID, userID,
	)
	if err != nil {
		return "", err
	}
	return "seen_before", nil
}
