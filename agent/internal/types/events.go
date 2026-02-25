package types

// TokenUsage holds information about prompt tokens and generated tokens.
type TokenUsage struct {
	InputTokens  int     `json:"inputTokens"`
	OutputTokens int     `json:"outputTokens"`
	TotalCostUsd float64 `json:"totalCostUsd"`
}

// AgentEvent represents events produced during an Agent execution loop (Observability).
type AgentEvent struct {
	Type          string      `json:"type"` // "thinking", "tool_call", "tool_result", "error", "done", "model_io"
	Timestamp     int64       `json:"timestamp"`
	Iteration     int         `json:"iteration,omitempty"`
	Thinking      string      `json:"thinking,omitempty"`
	Source        string      `json:"source,omitempty"` // "model" or "system"
	ToolCallID    string      `json:"toolCallId,omitempty"`
	ToolName      string      `json:"toolName,omitempty"`
	ToolInput     interface{} `json:"toolInput,omitempty"`
	ToolResult    interface{} `json:"toolResult,omitempty"`
	ToolDuration  int64       `json:"toolDuration,omitempty"` // milliseconds
	ToolSuccess   *bool       `json:"toolSuccess,omitempty"`
	Error         string      `json:"error,omitempty"`
	Usage         *TokenUsage `json:"usage,omitempty"`
	ModelInput    interface{} `json:"modelInput,omitempty"`
	ModelOutput   interface{} `json:"modelOutput,omitempty"`
}

// TraceEventPayload represents the payload sent to the Server for tracing.
type TraceEventPayload struct {
	TraceID   string `json:"traceId"`
	SessionID string `json:"sessionId"`
	AgentID   string `json:"agentId,omitempty"`
	UserID    string `json:"userId,omitempty"`
	Channel   string `json:"channel,omitempty"`
	
	// Embed the properties of AgentEvent
	AgentEvent
}
