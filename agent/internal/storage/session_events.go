package storage

import "database/sql"

type SessionEventRow struct {
	Seq       int64
	SessionID string
	MessageID int64
}

func AppendSessionEvent(sessionID string, messageID int64) error {
	_, err := AppendSessionEventAndGetSeq(sessionID, messageID)
	return err
}

func AppendSessionEventAndGetSeq(sessionID string, messageID int64) (int64, error) {
	res, err := DB.Exec(
		`INSERT INTO session_events (session_id, message_id) VALUES (?, ?)`,
		sessionID, messageID,
	)
	if err != nil {
		return 0, err
	}
	seq, _ := res.LastInsertId()
	return seq, nil
}

func GetSessionEventsAfter(sessionID string, afterSeq int64) ([]SessionEventRow, error) {
	rows, err := DB.Query(
		`SELECT seq, session_id, message_id FROM session_events WHERE session_id = ? AND seq > ? ORDER BY seq`,
		sessionID, afterSeq,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []SessionEventRow
	for rows.Next() {
		var e SessionEventRow
		if err := rows.Scan(&e.Seq, &e.SessionID, &e.MessageID); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func HasSessionEventsAfter(sessionID string, afterSeq int64) (bool, error) {
	var exists bool
	err := DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM session_events WHERE session_id = ? AND seq > ?)`,
		sessionID, afterSeq,
	).Scan(&exists)
	return exists, err
}

func GetProcessingSessionsWithPendingEvents() ([]SessionData, error) {
	rows, err := DB.Query(
		sessionSelectSQL + ` WHERE execution_status = 'processing' AND deleted_at = 0`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []SessionData
	for rows.Next() {
		var sd SessionData
		var agentID, userID, sourceChannel, sessionKey, channelConvID, channelName, workDir, mode, ctx sql.NullString
		if err := rows.Scan(
			&sd.ID, &sd.Title, &agentID, &userID, &sourceChannel,
			&sessionKey, &channelConvID, &channelName, &workDir,
			&sd.ExecutionStatus, &mode, &sd.EventCursor, &sd.CreatedAt, &sd.UpdatedAt, &ctx,
		); err != nil {
			return nil, err
		}
		sd.AgentID = agentID.String
		sd.UserID = userID.String
		sd.SourceChannel = sourceChannel.String
		sd.SessionKey = sessionKey.String
		sd.ChannelConversationID = channelConvID.String
		sd.ChannelName = channelName.String
		sd.WorkDir = workDir.String
		sd.Mode = mode.String
		if sd.Mode == "" {
			sd.Mode = "normal"
		}
		sd.Context = ctx.String
		sessions = append(sessions, sd)
	}
	return sessions, rows.Err()
}

func DeleteSessionEvents(sessionID string) error {
	_, err := DB.Exec(`DELETE FROM session_events WHERE session_id = ?`, sessionID)
	return err
}
