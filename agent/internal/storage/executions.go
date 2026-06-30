package storage

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"agent/internal/timeutil"
)

type Execution struct {
	ID                string  `json:"id"`
	SessionID         string  `json:"sessionId"`
	AgentID           string  `json:"agentId"`
	UserID            string  `json:"userId"`
	Channel           string  `json:"channel"`
	SandboxTemplateID string  `json:"sandboxTemplateId"`
	Provider          string  `json:"provider"`
	Model             string  `json:"model"`
	Status            string  `json:"status"`
	DeliveryStatus    string  `json:"deliveryStatus"`
	FailureStage      string  `json:"failureStage,omitempty"`
	ErrorMessage      string  `json:"errorMessage,omitempty"`
	RequestCount      int     `json:"requestCount"`
	LLMCallCount      int     `json:"llmCallCount"`
	ToolCallCount     int     `json:"toolCallCount"`
	ToolFailureCount  int     `json:"toolFailureCount"`
	InputTokens       int64   `json:"inputTokens"`
	OutputTokens      int64   `json:"outputTokens"`
	TotalCostUSD      float64 `json:"totalCostUsd"`
	CostKnown         bool    `json:"costKnown"`
	QueuedAt          int64   `json:"queuedAt"`
	StartedAt         int64   `json:"startedAt"`
	CompletedAt       int64   `json:"completedAt"`
	RepliedAt         int64   `json:"repliedAt"`
	DurationMs        int64   `json:"durationMs"`
	LLMDurationMs     int64   `json:"llmDurationMs"`
	ToolDurationMs    int64   `json:"toolDurationMs"`
}

type ExecutionUsage struct {
	InputTokens      int64
	OutputTokens     int64
	TotalCostUSD     float64
	LLMCallCount     int
	ToolCallCount    int
	ToolFailureCount int
	LLMDurationMs    int64
	ToolDurationMs   int64
	CostKnown        bool
}

type ExecutionFilter struct {
	From    int64
	To      int64
	Before  int64
	AgentID string
	Channel string
	Status  string
	Outcome string
	Search  string
	Limit   int
}

type ExecutionOverview struct {
	RequestCount         int64   `json:"requestCount"`
	ExecutionCount       int64   `json:"executionCount"`
	RepliedCount         int64   `json:"repliedCount"`
	NoReplyCount         int64   `json:"noReplyCount"`
	ExecutionFailedCount int64   `json:"executionFailedCount"`
	SendFailedCount      int64   `json:"sendFailedCount"`
	RunningCount         int64   `json:"runningCount"`
	ReplyRate            float64 `json:"replyRate"`
	AvgDurationMs        int64   `json:"avgDurationMs"`
	P95DurationMs        int64   `json:"p95DurationMs"`
	InputTokens          int64   `json:"inputTokens"`
	OutputTokens         int64   `json:"outputTokens"`
	TotalCostUSD         float64 `json:"totalCostUsd"`
	CostCoverage         float64 `json:"costCoverage"`
}

type ExecutionTrendPoint struct {
	Timestamp     int64   `json:"timestamp"`
	RequestCount  int64   `json:"requestCount"`
	RepliedCount  int64   `json:"repliedCount"`
	FailedCount   int64   `json:"failedCount"`
	AvgDurationMs int64   `json:"avgDurationMs"`
	TotalCostUSD  float64 `json:"totalCostUsd"`
}

type FailureBreakdown struct {
	Stage string `json:"stage"`
	Count int64  `json:"count"`
}

type ExecutionListItem struct {
	Execution
	RequestSummary string `json:"requestSummary"`
	Outcome        string `json:"outcome"`
}

type ExecutionRequest struct {
	ID        int64  `json:"id"`
	Content   string `json:"content"`
	Status    string `json:"status"`
	Outcome   string `json:"outcome"`
	CreatedAt int64  `json:"createdAt"`
}

func StartExecution(id, sessionID, agentID, userID, channel string, messageIDs []int64) error {
	if id == "" || sessionID == "" {
		return fmt.Errorf("execution id and session id are required")
	}
	now := timeutil.NowMs()
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO agent_executions
		(id, session_id, agent_id, user_id, channel, status, request_count, queued_at, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'running', ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE status = 'running', delivery_status = 'not_started', failure_stage = '', error_message = '',
		request_count = VALUES(request_count), llm_call_count = 0, tool_call_count = 0, tool_failure_count = 0,
		input_tokens = 0, output_tokens = 0, total_cost_usd = 0, cost_known = 1, started_at = VALUES(started_at),
		completed_at = 0, replied_at = 0, duration_ms = 0, llm_duration_ms = 0, tool_duration_ms = 0,
		updated_at = VALUES(updated_at)`,
		id, sessionID, agentID, userID, channel, len(messageIDs), now, now, now, now)
	if err != nil {
		return err
	}
	if err := attachExecutionMessages(tx, id, messageIDs, "processing"); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE agent_executions SET queued_at = COALESCE((
		SELECT MIN(created_at) FROM messages WHERE execution_id = ? AND role = 'user'
	), started_at) WHERE id = ?`, id, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func RecoverInterruptedExecutions() error {
	now := timeutil.NowMs()
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`UPDATE messages m JOIN agent_executions e ON e.id = m.execution_id
		SET m.processing_status = 'failed', m.processing_outcome = 'execution_failed'
		WHERE e.status = 'running' AND m.role = 'user'`)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE agent_executions SET status = 'failed', failure_stage = 'internal',
		error_message = 'service restarted during execution', completed_at = ?,
		duration_ms = GREATEST(0, ? - started_at), updated_at = ? WHERE status = 'running'`, now, now, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func AttachExecutionMessages(id string, messageIDs []int64) error {
	if len(messageIDs) == 0 {
		return nil
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := attachExecutionMessages(tx, id, messageIDs, "processing"); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE agent_executions SET request_count = (
		SELECT COUNT(*) FROM messages WHERE execution_id = ? AND role = 'user'
	), updated_at = ? WHERE id = ?`, id, timeutil.NowMs(), id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func attachExecutionMessages(tx *sql.Tx, id string, messageIDs []int64, status string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	marks := make([]string, len(messageIDs))
	args := make([]any, 0, len(messageIDs)+2)
	args = append(args, id, status)
	for i, messageID := range messageIDs {
		marks[i] = "?"
		args = append(args, messageID)
	}
	_, err := tx.Exec(`UPDATE messages SET execution_id = ?, processing_status = ? WHERE id IN (`+strings.Join(marks, ",")+`)`, args...)
	return err
}

func SetExecutionModel(id, provider, model string) error {
	_, err := DB.Exec(`UPDATE agent_executions SET provider = ?, model = ?, updated_at = ? WHERE id = ?`,
		provider, model, timeutil.NowMs(), id)
	return err
}

func SetExecutionSandboxTemplate(id, sandboxTemplateID string) error {
	_, err := DB.Exec(`UPDATE agent_executions SET sandbox_template_id = ?, updated_at = ? WHERE id = ?`,
		normalizeSandboxTemplateID(sandboxTemplateID), timeutil.NowMs(), id)
	return err
}

func AddExecutionUsage(id string, usage ExecutionUsage) error {
	_, err := DB.Exec(`UPDATE agent_executions SET
		input_tokens = input_tokens + ?, output_tokens = output_tokens + ?, total_cost_usd = total_cost_usd + ?,
		cost_known = cost_known AND ?,
		llm_call_count = llm_call_count + ?, tool_call_count = tool_call_count + ?, tool_failure_count = tool_failure_count + ?,
		llm_duration_ms = llm_duration_ms + ?, tool_duration_ms = tool_duration_ms + ?, updated_at = ?
		WHERE id = ?`, usage.InputTokens, usage.OutputTokens, usage.TotalCostUSD, usage.CostKnown,
		usage.LLMCallCount, usage.ToolCallCount, usage.ToolFailureCount,
		usage.LLMDurationMs, usage.ToolDurationMs, timeutil.NowMs(), id)
	return err
}

func CompleteExecution(id, outcome string) error {
	now := timeutil.NowMs()
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`UPDATE agent_executions SET status = 'completed', completed_at = ?,
		duration_ms = GREATEST(0, ? - started_at), updated_at = ? WHERE id = ?`, now, now, now, id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE messages SET processing_status = 'completed', processing_outcome = ?
		WHERE execution_id = ? AND role = 'user'`, outcome, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func FailExecution(id, stage string, cause error) error {
	now := timeutil.NowMs()
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`UPDATE agent_executions SET status = 'failed', failure_stage = ?, error_message = ?,
		completed_at = ?, duration_ms = GREATEST(0, ? - started_at), updated_at = ? WHERE id = ?`,
		stage, message, now, now, now, id)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE messages SET processing_status = 'failed', processing_outcome = 'execution_failed'
		WHERE execution_id = ? AND role = 'user'`, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func StartExecutionDelivery(id string) error {
	_, err := DB.Exec(`UPDATE agent_executions SET delivery_status = 'sending', updated_at = ? WHERE id = ?`, timeutil.NowMs(), id)
	return err
}

func CompleteExecutionDelivery(id string, sendErr error) error {
	now := timeutil.NowMs()
	if sendErr == nil {
		_, err := DB.Exec(`UPDATE agent_executions SET delivery_status = 'sent', replied_at = ?,
			error_message = CASE WHEN failure_stage = 'channel' THEN '' ELSE error_message END,
			failure_stage = CASE WHEN failure_stage = 'channel' THEN '' ELSE failure_stage END,
			updated_at = ? WHERE id = ?`, now, now, id)
		return err
	}
	_, err := DB.Exec(`UPDATE agent_executions SET delivery_status = 'failed', failure_stage = 'channel', error_message = ?, updated_at = ? WHERE id = ?`,
		sendErr.Error(), now, id)
	return err
}

func GetExecution(id string) (*Execution, error) {
	row := DB.QueryRow(executionSelect+` WHERE id = ?`, id)
	execution, err := scanExecution(row.Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return execution, err
}

func ListExecutions(filter ExecutionFilter) ([]ExecutionListItem, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 10)
	if filter.From > 0 {
		clauses = append(clauses, "e.started_at >= ?")
		args = append(args, filter.From)
	}
	if filter.To > 0 {
		clauses = append(clauses, "e.started_at < ?")
		args = append(args, filter.To)
	}
	if filter.Before > 0 {
		clauses = append(clauses, "e.started_at < ?")
		args = append(args, filter.Before)
	}
	if filter.AgentID != "" {
		clauses = append(clauses, "e.agent_id = ?")
		args = append(args, filter.AgentID)
	}
	if filter.Channel != "" {
		clauses = append(clauses, "e.channel = ?")
		args = append(args, filter.Channel)
	}
	if filter.Status != "" {
		clauses = append(clauses, "e.status = ?")
		args = append(args, filter.Status)
	}
	if filter.Outcome != "" {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM messages om WHERE om.execution_id = e.id AND om.processing_outcome = ?)")
		args = append(args, filter.Outcome)
	}
	if filter.Search != "" {
		clauses = append(clauses, `(e.id LIKE ? OR e.session_id LIKE ? OR EXISTS (
			SELECT 1 FROM messages sm WHERE sm.execution_id = e.id AND sm.content LIKE ?))`)
		like := "%" + filter.Search + "%"
		args = append(args, like, like, like)
	}

	query := `SELECT e.id, e.session_id, e.agent_id, e.user_id, e.channel, e.sandbox_template_id, e.provider, e.model, e.status,
		e.delivery_status, e.failure_stage, e.error_message, e.request_count, e.llm_call_count, e.tool_call_count,
		e.tool_failure_count, e.input_tokens, e.output_tokens, e.total_cost_usd, e.cost_known, e.queued_at, e.started_at,
		e.completed_at, e.replied_at, e.duration_ms, e.llm_duration_ms, e.tool_duration_ms,
		COALESCE((SELECT LEFT(m.content, 200) FROM messages m WHERE m.execution_id = e.id AND m.role = 'user' ORDER BY m.id LIMIT 1), ''),
		CASE
			WHEN e.status = 'failed' THEN 'execution_failed'
			WHEN e.delivery_status = 'failed' THEN 'send_failed'
			WHEN e.delivery_status = 'sent' THEN 'replied'
			WHEN e.status = 'completed' THEN 'no_reply'
			ELSE ''
		END
		FROM agent_executions e WHERE ` + strings.Join(clauses, " AND ") + ` ORDER BY e.started_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ExecutionListItem, 0)
	for rows.Next() {
		var item ExecutionListItem
		err := rows.Scan(&item.ID, &item.SessionID, &item.AgentID, &item.UserID, &item.Channel,
			&item.SandboxTemplateID, &item.Provider, &item.Model, &item.Status, &item.DeliveryStatus, &item.FailureStage,
			&item.ErrorMessage, &item.RequestCount, &item.LLMCallCount, &item.ToolCallCount,
			&item.ToolFailureCount, &item.InputTokens, &item.OutputTokens, &item.TotalCostUSD, &item.CostKnown,
			&item.QueuedAt, &item.StartedAt, &item.CompletedAt, &item.RepliedAt, &item.DurationMs,
			&item.LLMDurationMs, &item.ToolDurationMs, &item.RequestSummary, &item.Outcome)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func ListExecutionRequests(id string) ([]ExecutionRequest, error) {
	rows, err := DB.Query(`SELECT id, content, processing_status, processing_outcome, created_at
		FROM messages WHERE execution_id = ? AND role = 'user' ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ExecutionRequest, 0)
	for rows.Next() {
		var item ExecutionRequest
		if err := rows.Scan(&item.ID, &item.Content, &item.Status, &item.Outcome, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func GetExecutionOverview(filter ExecutionFilter) (*ExecutionOverview, error) {
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 6)
	if filter.From > 0 {
		clauses = append(clauses, "started_at >= ?")
		args = append(args, filter.From)
	}
	if filter.To > 0 {
		clauses = append(clauses, "started_at < ?")
		args = append(args, filter.To)
	}
	if filter.AgentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, filter.AgentID)
	}
	if filter.Channel != "" {
		clauses = append(clauses, "channel = ?")
		args = append(args, filter.Channel)
	}
	where := strings.Join(clauses, " AND ")

	var overview ExecutionOverview
	var avgDuration float64
	var knownCostRequests int64
	err := DB.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(request_count), 0),
		COALESCE(SUM(CASE WHEN delivery_status = 'sent' THEN request_count ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'completed' AND delivery_status = 'not_started' THEN request_count ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'failed' THEN request_count ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN delivery_status = 'failed' THEN request_count ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0),
		COALESCE(AVG(CASE WHEN duration_ms > 0 THEN duration_ms END), 0),
		COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_cost_usd), 0),
		COALESCE(SUM(CASE WHEN cost_known = 1 THEN request_count ELSE 0 END), 0)
		FROM agent_executions WHERE `+where, args...).Scan(
		&overview.ExecutionCount, &overview.RequestCount, &overview.RepliedCount, &overview.NoReplyCount,
		&overview.ExecutionFailedCount, &overview.SendFailedCount, &overview.RunningCount,
		&avgDuration, &overview.InputTokens, &overview.OutputTokens, &overview.TotalCostUSD, &knownCostRequests)
	if err != nil {
		return nil, err
	}
	overview.AvgDurationMs = int64(avgDuration)
	messageClauses := []string{"m.role = 'user'", "m.initiator = 'user'"}
	messageArgs := make([]any, 0, 4)
	if filter.From > 0 {
		messageClauses = append(messageClauses, "m.created_at >= ?")
		messageArgs = append(messageArgs, filter.From)
	}
	if filter.To > 0 {
		messageClauses = append(messageClauses, "m.created_at < ?")
		messageArgs = append(messageArgs, filter.To)
	}
	if filter.AgentID != "" {
		messageClauses = append(messageClauses, "s.agent_id = ?")
		messageArgs = append(messageArgs, filter.AgentID)
	}
	if filter.Channel != "" {
		messageClauses = append(messageClauses, "m.channel = ?")
		messageArgs = append(messageArgs, filter.Channel)
	}
	if err := DB.QueryRow(`SELECT COUNT(*) FROM messages m JOIN agent_sessions s ON s.id = m.session_id
		WHERE `+strings.Join(messageClauses, " AND "), messageArgs...).Scan(&overview.RequestCount); err != nil {
		return nil, err
	}
	if overview.RequestCount > 0 {
		overview.ReplyRate = float64(overview.RepliedCount) / float64(overview.RequestCount)
		overview.CostCoverage = float64(knownCostRequests) / float64(overview.RequestCount)
	} else {
		overview.ReplyRate = 0
	}

	rows, err := DB.Query(`SELECT duration_ms FROM agent_executions WHERE `+where+`
		AND duration_ms > 0 ORDER BY duration_ms`, args...)
	if err != nil {
		return nil, err
	}
	var durations []int64
	for rows.Next() {
		var duration int64
		if err := rows.Scan(&duration); err != nil {
			rows.Close()
			return nil, err
		}
		durations = append(durations, duration)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		index := (len(durations)*95 + 99) / 100
		overview.P95DurationMs = durations[index-1]
	}
	return &overview, nil
}

func ListExecutionTrend(filter ExecutionFilter, bucketMs int64) ([]ExecutionTrendPoint, error) {
	if bucketMs <= 0 {
		bucketMs = 60 * 60 * 1000
	}
	clauses, args := executionTimeClauses(filter)
	query := `SELECT CAST(FLOOR(started_at / ?) * ? AS UNSIGNED), SUM(request_count),
		SUM(CASE WHEN delivery_status = 'sent' THEN request_count ELSE 0 END),
		SUM(CASE WHEN status = 'failed' OR delivery_status = 'failed' THEN request_count ELSE 0 END),
		COALESCE(AVG(CASE WHEN duration_ms > 0 THEN duration_ms END), 0), COALESCE(SUM(total_cost_usd), 0)
		FROM agent_executions WHERE ` + strings.Join(clauses, " AND ") + ` GROUP BY 1 ORDER BY 1`
	queryArgs := []any{bucketMs, bucketMs}
	queryArgs = append(queryArgs, args...)
	rows, err := DB.Query(query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := make([]ExecutionTrendPoint, 0)
	for rows.Next() {
		var point ExecutionTrendPoint
		var avgDuration float64
		if err := rows.Scan(&point.Timestamp, &point.RequestCount, &point.RepliedCount, &point.FailedCount,
			&avgDuration, &point.TotalCostUSD); err != nil {
			return nil, err
		}
		point.AvgDurationMs = int64(avgDuration)
		points = append(points, point)
	}
	return points, rows.Err()
}

func ListFailureBreakdown(filter ExecutionFilter) ([]FailureBreakdown, error) {
	clauses, args := executionTimeClauses(filter)
	clauses = append(clauses, "(status = 'failed' OR delivery_status = 'failed')")
	rows, err := DB.Query(`SELECT CASE WHEN delivery_status = 'failed' THEN 'channel'
		WHEN failure_stage = '' THEN 'internal' ELSE failure_stage END, SUM(request_count)
		FROM agent_executions WHERE `+strings.Join(clauses, " AND ")+` GROUP BY 1 ORDER BY 2 DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]FailureBreakdown, 0)
	for rows.Next() {
		var item FailureBreakdown
		if err := rows.Scan(&item.Stage, &item.Count); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func executionTimeClauses(filter ExecutionFilter) ([]string, []any) {
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 4)
	if filter.From > 0 {
		clauses = append(clauses, "started_at >= ?")
		args = append(args, filter.From)
	}
	if filter.To > 0 {
		clauses = append(clauses, "started_at < ?")
		args = append(args, filter.To)
	}
	if filter.AgentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, filter.AgentID)
	}
	if filter.Channel != "" {
		clauses = append(clauses, "channel = ?")
		args = append(args, filter.Channel)
	}
	return clauses, args
}

const executionSelect = `SELECT id, session_id, agent_id, user_id, channel, sandbox_template_id, provider, model, status,
	delivery_status, failure_stage, error_message, request_count, llm_call_count, tool_call_count,
	tool_failure_count, input_tokens, output_tokens, total_cost_usd, cost_known, queued_at, started_at, completed_at,
	replied_at, duration_ms, llm_duration_ms, tool_duration_ms FROM agent_executions`

func scanExecution(scan func(...any) error) (*Execution, error) {
	var execution Execution
	err := scan(&execution.ID, &execution.SessionID, &execution.AgentID, &execution.UserID, &execution.Channel,
		&execution.SandboxTemplateID, &execution.Provider, &execution.Model, &execution.Status, &execution.DeliveryStatus, &execution.FailureStage,
		&execution.ErrorMessage, &execution.RequestCount, &execution.LLMCallCount, &execution.ToolCallCount,
		&execution.ToolFailureCount, &execution.InputTokens, &execution.OutputTokens, &execution.TotalCostUSD,
		&execution.CostKnown,
		&execution.QueuedAt, &execution.StartedAt, &execution.CompletedAt, &execution.RepliedAt,
		&execution.DurationMs, &execution.LLMDurationMs, &execution.ToolDurationMs)
	if err != nil {
		return nil, err
	}
	execution.SandboxTemplateID = normalizeSandboxTemplateID(execution.SandboxTemplateID)
	return &execution, nil
}
