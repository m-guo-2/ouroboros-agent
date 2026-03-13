package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	sharedlogger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

var completedTraceCache struct {
	mu    sync.RWMutex
	items map[string]*executionTrace
}

func init() {
	completedTraceCache.items = make(map[string]*executionTrace)
}

type tracesHandler struct {
	reader sharedlogger.LogReader
}

func (h *tracesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/traces")
	path = strings.TrimPrefix(path, "/")

	if path == "" {
		h.listTraces(w, r)
		return
	}
	if path == "active" {
		ok(w, []interface{}{})
		return
	}
	if path == "recent-summaries" || strings.HasPrefix(path, "recent-summaries") {
		h.listRecentSummaries(w, r)
		return
	}

	parts := strings.SplitN(path, "/", 3)
	traceID := parts[0]
	if len(parts) >= 2 && parts[1] == "llm-io" {
		if len(parts) == 3 && parts[2] != "" {
			h.serveLLMIO(w, parts[2])
		} else {
			h.listLLMIORefs(w, traceID)
		}
		return
	}

	h.serveTrace(w, r, traceID)
}

// --------------------------------------------------------------------------
// Trace types
// --------------------------------------------------------------------------

type executionStep struct {
	Index        int         `json:"index"`
	Iteration    int         `json:"iteration"`
	Timestamp    int64       `json:"timestamp"`
	Type         string      `json:"type"`
	Thinking     string      `json:"thinking,omitempty"`
	Source       string      `json:"source,omitempty"`
	ToolCallID   string      `json:"toolCallId,omitempty"`
	ToolName     string      `json:"toolName,omitempty"`
	ToolInput    interface{} `json:"toolInput,omitempty"`
	ToolResult   interface{} `json:"toolResult,omitempty"`
	ToolDuration interface{} `json:"toolDuration,omitempty"`
	ToolSuccess  interface{} `json:"toolSuccess,omitempty"`
	Content      string      `json:"content,omitempty"`
	Error        string      `json:"error,omitempty"`
	Model        string      `json:"model,omitempty"`
	InputTokens  interface{} `json:"inputTokens,omitempty"`
	OutputTokens interface{} `json:"outputTokens,omitempty"`
	DurationMs   interface{} `json:"durationMs,omitempty"`
	StopReason   string      `json:"stopReason,omitempty"`
	CostUsd      interface{} `json:"costUsd,omitempty"`
	LLMIORef     string      `json:"llmIORef,omitempty"`
	AbsorbRound   int `json:"absorbRound,omitempty"`
	AbsorbedCount int `json:"absorbedCount,omitempty"`
	TokensBefore  int `json:"tokensBefore,omitempty"`
	TokensAfter   int `json:"tokensAfter,omitempty"`
	ArchivedCount int `json:"archivedCount,omitempty"`
}

type executionTrace struct {
	ID           string          `json:"id"`
	SessionID    string          `json:"sessionId"`
	AgentID      string          `json:"agentId,omitempty"`
	UserID       string          `json:"userId,omitempty"`
	Channel      string          `json:"channel,omitempty"`
	Status       string          `json:"status"`
	StartedAt    int64           `json:"startedAt"`
	CompletedAt  *int64          `json:"completedAt,omitempty"`
	InputTokens  float64         `json:"inputTokens"`
	OutputTokens float64         `json:"outputTokens"`
	TotalCostUsd float64         `json:"totalCostUsd"`
	Steps        []executionStep `json:"steps"`
}

func parseTimestamp(ts string) int64 {
	if ts == "" {
		return time.Now().UnixMilli()
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t, err = time.Parse(time.RFC3339, ts)
	}
	if err != nil {
		return time.Now().UnixMilli()
	}
	return t.UnixMilli()
}

func strField(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

// --------------------------------------------------------------------------
// Trace assembly from LogReader events
// --------------------------------------------------------------------------

func (h *tracesHandler) buildTrace(traceID string) *executionTrace {
	if h.reader == nil {
		return nil
	}

	events, err := h.reader.ReadTraceEvents(traceID)
	if err != nil || len(events) == 0 {
		return nil
	}

	var steps []executionStep
	var startedAt int64
	var completedAtVal int64
	var hasCompleted bool
	status := "running"
	var inputTokens, outputTokens, totalCostUsd float64
	var agentID, userID, channel, sessionID string

	for _, ev := range events {
		row := ev.Raw
		ts := parseTimestamp(strField(row, "time"))
		event := strField(row, "traceEvent")
		iter := 1
		if v, ok := row["iteration"].(float64); ok {
			iter = int(v)
		}

		if sessionID == "" {
			sessionID = strField(row, "sessionId")
		}

		switch event {
		case "start":
			if startedAt == 0 {
				startedAt = ts
				sessionID = strField(row, "sessionId")
				agentID = strField(row, "agentId")
				userID = strField(row, "userId")
				channel = strField(row, "channel")
			}
		case "done":
			completedAtVal = ts
			hasCompleted = true
			if row["error"] != nil {
				status = "error"
			} else {
				status = "completed"
			}
			if v, ok := row["inputTokens"].(float64); ok {
				inputTokens = v
			}
			if v, ok := row["outputTokens"].(float64); ok {
				outputTokens = v
			}
			if v, ok := row["totalCostUsd"].(float64); ok {
				totalCostUsd = v
			}
		case "thinking":
			steps = append(steps, executionStep{
				Index: len(steps), Iteration: iter, Timestamp: ts,
				Type: "thinking", Thinking: strField(row, "thinking"),
				Source: strField(row, "source"),
			})
		case "tool_call":
			steps = append(steps, executionStep{
				Index: len(steps), Iteration: iter, Timestamp: ts,
				Type: "tool_call", ToolCallID: strField(row, "toolCallId"),
				ToolName: strField(row, "tool"), ToolInput: row["toolInput"],
			})
		case "tool_result":
			steps = append(steps, executionStep{
				Index: len(steps), Iteration: iter, Timestamp: ts,
				Type: "tool_result", ToolCallID: strField(row, "toolCallId"),
				ToolName: strField(row, "tool"), ToolResult: row["toolResult"],
				ToolDuration: row["toolDuration"], ToolSuccess: row["toolSuccess"],
			})
		case "llm_call":
			steps = append(steps, executionStep{
				Index: len(steps), Iteration: iter, Timestamp: ts,
				Type: "llm_call", Model: strField(row, "model"),
				StopReason: strField(row, "stopReason"), LLMIORef: strField(row, "llmIORef"),
				InputTokens: row["inputTokens"], OutputTokens: row["outputTokens"],
				DurationMs: row["durationMs"], CostUsd: row["costUsd"],
			})
		case "absorb":
			step := executionStep{
				Index: len(steps), Iteration: iter, Timestamp: ts,
				Type: "absorb",
			}
			if v, ok := row["absorbRound"].(float64); ok {
				step.AbsorbRound = int(v)
			}
			if v, ok := row["absorbedCount"].(float64); ok {
				step.AbsorbedCount = int(v)
			}
			steps = append(steps, step)
		case "compact":
			step := executionStep{
				Index: len(steps), Iteration: iter, Timestamp: ts,
				Type: "compact",
			}
			if v, ok := row["tokensBefore"].(float64); ok {
				step.TokensBefore = int(v)
			}
			if v, ok := row["tokensAfter"].(float64); ok {
				step.TokensAfter = int(v)
			}
			if v, ok := row["archivedCount"].(float64); ok {
				step.ArchivedCount = int(v)
			}
			steps = append(steps, step)
		case "error":
			errMsg := strField(row, "error")
			if errMsg == "" {
				errMsg = strField(row, "msg")
			}
			steps = append(steps, executionStep{
				Index: len(steps), Iteration: iter, Timestamp: ts,
				Type: "error", Error: errMsg,
			})
		case "empty_response_retry":
			steps = append(steps, executionStep{
				Index: len(steps), Iteration: iter, Timestamp: ts,
				Type: "empty_response_retry",
			})
		case "attachment_guard":
			steps = append(steps, executionStep{
				Index: len(steps), Iteration: iter, Timestamp: ts,
				Type: "attachment_guard",
			})
		}
	}

	if startedAt == 0 && len(steps) == 0 {
		return nil
	}
	if startedAt == 0 && len(steps) > 0 {
		startedAt = steps[0].Timestamp
	}

	t := &executionTrace{
		ID: traceID, SessionID: sessionID, AgentID: agentID,
		UserID: userID, Channel: channel, Status: status,
		StartedAt: startedAt, InputTokens: inputTokens,
		OutputTokens: outputTokens, TotalCostUsd: totalCostUsd,
		Steps: steps,
	}
	if hasCompleted {
		t.CompletedAt = &completedAtVal
	}
	if t.SessionID == "" {
		t.SessionID = traceID
	}
	return t
}

func (h *tracesHandler) serveTrace(w http.ResponseWriter, r *http.Request, traceID string) {
	completedTraceCache.mu.RLock()
	if cached, hit := completedTraceCache.items[traceID]; hit {
		completedTraceCache.mu.RUnlock()
		ok(w, cached)
		return
	}
	completedTraceCache.mu.RUnlock()

	t := h.buildTrace(traceID)
	if t == nil {
		apiErr(w, http.StatusNotFound, "trace not found")
		return
	}

	if t.Status == "completed" || t.Status == "error" {
		completedTraceCache.mu.Lock()
		completedTraceCache.items[traceID] = t
		completedTraceCache.mu.Unlock()
	}

	ok(w, t)
}

func (h *tracesHandler) listTraces(w http.ResponseWriter, r *http.Request) {
	if h.reader == nil {
		ok(w, []interface{}{})
		return
	}

	q := r.URL.Query()
	limit := 50
	if l := q.Get("limit"); l != "" {
		if n, err := parseInt(l); err == nil && n > 0 {
			limit = n
		}
	}

	summaries, err := h.reader.ListTraces(sharedlogger.TraceFilter{
		SessionID: q.Get("sessionId"),
		Limit:     limit,
		Days:      14,
	})
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to list traces")
		return
	}

	type traceSummary struct {
		ID        string `json:"id"`
		StartedAt int64  `json:"startedAt"`
	}
	result := make([]traceSummary, 0, len(summaries))
	for _, s := range summaries {
		result = append(result, traceSummary{
			ID:        s.ID,
			StartedAt: parseTimestamp(s.StartedAt),
		})
	}
	ok(w, result)
}

func (h *tracesHandler) listRecentSummaries(w http.ResponseWriter, r *http.Request) {
	h.listTraces(w, r)
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errInvalidInt
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

var errInvalidInt = &parseError{"invalid integer"}

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }

func (h *tracesHandler) serveLLMIO(w http.ResponseWriter, ref string) {
	if h.reader == nil {
		apiErr(w, http.StatusNotFound, "llm-io ref not found")
		return
	}

	data, err := h.reader.ReadLLMIO(ref)
	if err != nil {
		apiErr(w, http.StatusNotFound, "llm-io ref not found")
		return
	}
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		apiErr(w, http.StatusInternalServerError, "failed to parse llm-io file")
		return
	}
	ok(w, v)
}

func (h *tracesHandler) listLLMIORefs(w http.ResponseWriter, traceID string) {
	if h.reader == nil {
		ok(w, []string{})
		return
	}

	refs, err := h.reader.ListLLMIORefs(traceID)
	if err != nil {
		refs = []string{}
	}
	ok(w, refs)
}
