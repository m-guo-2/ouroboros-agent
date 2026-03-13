package storage

import (
	"fmt"
	"time"
)

const sqliteDatetimeFmt = "2006-01-02 15:04:05"

type DelayedTask struct {
	ID                    string
	SessionID             string
	AgentID               string
	UserID                string
	Channel               string
	ChannelUserID         string
	ChannelConversationID string
	Task                  string
	ExecuteAt             string
	Status                string
	CreatedAt             string
	UpdatedAt             string
}

// NormalizeToSQLiteDatetime parses an ISO 8601 string (with or without
// timezone) and returns it in SQLite-comparable UTC format "YYYY-MM-DD HH:MM:SS".
// If parsing fails, the original value is returned unchanged.
func NormalizeToSQLiteDatetime(iso string) string {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, iso); err == nil {
			return t.UTC().Format(sqliteDatetimeFmt)
		}
	}
	return iso
}

func CreateDelayedTask(task *DelayedTask) error {
	if task.ID == "" {
		task.ID = fmt.Sprintf("dt-%d", time.Now().UnixNano())
	}
	task.ExecuteAt = NormalizeToSQLiteDatetime(task.ExecuteAt)
	_, err := DB.Exec(
		`INSERT INTO delayed_tasks
			(id, session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id, task, execute_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
		task.ID, task.SessionID, task.AgentID, task.UserID,
		task.Channel, task.ChannelUserID, task.ChannelConversationID,
		task.Task, task.ExecuteAt,
	)
	return err
}

func QueryDueTasks() ([]DelayedTask, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id,
			task, execute_at, status, created_at
		FROM delayed_tasks
		WHERE status = 'pending' AND execute_at <= datetime('now')
		ORDER BY execute_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query due tasks: %w", err)
	}
	defer rows.Close()

	var tasks []DelayedTask
	for rows.Next() {
		var t DelayedTask
		if err := rows.Scan(
			&t.ID, &t.SessionID, &t.AgentID, &t.UserID,
			&t.Channel, &t.ChannelUserID, &t.ChannelConversationID,
			&t.Task, &t.ExecuteAt, &t.Status, &t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan delayed task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func MarkTaskDispatched(id string) error {
	res, err := DB.Exec(
		`UPDATE delayed_tasks SET status = 'dispatched', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'pending'`,
		id,
	)
	if err != nil {
		return fmt.Errorf("mark task dispatched: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task %s not found or already dispatched", id)
	}
	return nil
}

func CancelDelayedTask(id, sessionID string) error {
	res, err := DB.Exec(
		`UPDATE delayed_tasks SET status = 'cancelled', updated_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND session_id = ? AND status = 'pending'`,
		id, sessionID,
	)
	if err != nil {
		return fmt.Errorf("cancel delayed task: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task %s not found, not in this session, or not pending", id)
	}
	return nil
}

func ListPendingTasksBySession(sessionID string) ([]DelayedTask, error) {
	rows, err := DB.Query(
		`SELECT id, session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id,
			task, execute_at, status, created_at
		FROM delayed_tasks
		WHERE session_id = ? AND status = 'pending'
		ORDER BY execute_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("list pending tasks: %w", err)
	}
	defer rows.Close()

	var tasks []DelayedTask
	for rows.Next() {
		var t DelayedTask
		if err := rows.Scan(
			&t.ID, &t.SessionID, &t.AgentID, &t.UserID,
			&t.Channel, &t.ChannelUserID, &t.ChannelConversationID,
			&t.Task, &t.ExecuteAt, &t.Status, &t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan pending task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
