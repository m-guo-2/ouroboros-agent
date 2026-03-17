package storage

import (
	"fmt"

	"agent/internal/timeutil"
)

type DelayedTask struct {
	ID                    int64  `json:"id"`
	SessionID             string `json:"sessionId"`
	AgentID               string `json:"agentId"`
	UserID                string `json:"userId"`
	Channel               string `json:"channel"`
	ChannelUserID         string `json:"channelUserId"`
	ChannelConversationID string `json:"channelConversationId"`
	Task                  string `json:"task"`
	ExecuteAt             int64  `json:"executeAt"`
	Status                string `json:"status"`
	CreatedAt             int64  `json:"createdAt"`
	UpdatedAt             int64  `json:"updatedAt"`
}

func CreateDelayedTask(task *DelayedTask) error {
	now := timeutil.NowMs()
	res, err := DB.Exec(
		`INSERT INTO delayed_tasks
			(session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id, task, execute_at, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
		task.SessionID, task.AgentID, task.UserID,
		task.Channel, task.ChannelUserID, task.ChannelConversationID,
		task.Task, task.ExecuteAt, now, now,
	)
	if err != nil {
		return err
	}
	task.ID, _ = res.LastInsertId()
	return nil
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

func MarkTaskDispatched(id int64) error {
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
		return fmt.Errorf("task %d not found or already dispatched", id)
	}
	return nil
}

func CancelDelayedTask(id int64, sessionID string) error {
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
		return fmt.Errorf("task %d not found, not in this session, or not pending", id)
	}
	return nil
}

func ListDelayedTasksBySession(sessionID, status string) ([]DelayedTask, error) {
	var query string
	var args []interface{}

	if status == "" || status == "all" {
		query = `SELECT id, session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id,
			task, execute_at, status, created_at, updated_at
		FROM delayed_tasks
		WHERE session_id = ?
		ORDER BY execute_at DESC`
		args = []interface{}{sessionID}
	} else {
		query = `SELECT id, session_id, agent_id, user_id, channel, channel_user_id, channel_conversation_id,
			task, execute_at, status, created_at, updated_at
		FROM delayed_tasks
		WHERE session_id = ? AND status = ?
		ORDER BY execute_at DESC`
		args = []interface{}{sessionID, status}
	}

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list delayed tasks: %w", err)
	}
	defer rows.Close()

	var tasks []DelayedTask
	for rows.Next() {
		var t DelayedTask
		if err := rows.Scan(
			&t.ID, &t.SessionID, &t.AgentID, &t.UserID,
			&t.Channel, &t.ChannelUserID, &t.ChannelConversationID,
			&t.Task, &t.ExecuteAt, &t.Status, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan delayed task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
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
