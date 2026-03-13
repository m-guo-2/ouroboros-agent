package storage

import (
	"fmt"

	"agent/internal/timeutil"
)

type DelayedTask struct {
	ID                    string
	SessionID             string
	AgentID               string
	UserID                string
	Channel               string
	ChannelUserID         string
	ChannelConversationID string
	Task                  string
	ExecuteAt             int64
	Status                string
	CreatedAt             int64
	UpdatedAt             int64
}

func CreateDelayedTask(task *DelayedTask) error {
	if task.ID == "" {
		task.ID = fmt.Sprintf("dt-%d", timeutil.NowMs())
	}
	now := timeutil.NowMs()
	_, err := DB.Exec(
		`INSERT INTO delayed_tasks
			(id, session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id, task, execute_at, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
		task.ID, task.SessionID, task.AgentID, task.UserID,
		task.Channel, task.ChannelUserID, task.ChannelConversationID,
		task.Task, task.ExecuteAt, now, now,
	)
	return err
}

func QueryDueTasks() ([]DelayedTask, error) {
	now := timeutil.NowMs()
	rows, err := DB.Query(
		`SELECT id, session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id,
			task, execute_at, status, created_at
		FROM delayed_tasks
		WHERE status = 'pending' AND execute_at <= ?
		ORDER BY execute_at ASC`, now,
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
	now := timeutil.NowMs()
	res, err := DB.Exec(
		`UPDATE delayed_tasks SET status = 'dispatched', updated_at = ? WHERE id = ? AND status = 'pending'`,
		now, id,
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
	now := timeutil.NowMs()
	res, err := DB.Exec(
		`UPDATE delayed_tasks SET status = 'cancelled', updated_at = ?
		 WHERE id = ? AND session_id = ? AND status = 'pending'`,
		now, id, sessionID,
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
