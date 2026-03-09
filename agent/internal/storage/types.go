package storage

import "agent/internal/types"

// AgentConfig holds the runtime configuration for an agent.
type AgentConfig struct {
	ID           string           `json:"id"`
	DisplayName  string           `json:"displayName"`
	SystemPrompt string           `json:"systemPrompt"`
	ModelID      string           `json:"modelId,omitempty"`
	Provider     string           `json:"provider,omitempty"`
	Model        string           `json:"model,omitempty"`
	Skills       []string         `json:"skills"`
	Channels     []ChannelBinding `json:"channels"`
	IsActive     bool             `json:"isActive"`
}

// ChannelBinding describes which channel an agent is bound to.
// JSON tags match frontend AgentProfile.channels: { type, identifier }.
type ChannelBinding struct {
	ChannelType       string `json:"type"`
	ChannelIdentifier string `json:"identifier"`
}

// ProviderCredentials holds LLM API credentials for a given provider.
type ProviderCredentials struct {
	Provider string
	APIKey   string
	BaseURL  string
}

// SkillToolExecutor describes how to invoke a skill's tool.
type SkillToolExecutor struct {
	Type    string `json:"type"` // "http" | "shell" | "script" | "internal"
	URL     string `json:"url,omitempty"`
	Method  string `json:"method,omitempty"`
	Command string `json:"command,omitempty"`
	Handler string `json:"handler,omitempty"`
}

// SkillContext is the compiled output of all enabled skills for an agent.
type SkillContext struct {
	SkillsSnippet string // text referenced by {{skills}} template variable
	Tools         []types.ToolDefinition
	ToolExecutors map[string]SkillToolExecutor
	SkillDocs     map[string]string
}

// SessionData represents a persisted agent session.
type SessionData struct {
	ID                    string `json:"id"`
	Title                 string `json:"title"`
	AgentID               string `json:"agentId"`
	UserID                string `json:"userId"`
	SourceChannel         string `json:"sourceChannel"`
	SessionKey            string `json:"sessionKey"`
	ChannelConversationID string `json:"channelConversationId"`
	ChannelName           string `json:"channelName"`
	WorkDir               string `json:"workDir"`
	ExecutionStatus       string `json:"executionStatus"`
	CreatedAt             string `json:"createdAt"`
	UpdatedAt             string `json:"updatedAt"`
	Context               string `json:"-"`
}

// MessageData represents a single stored message.
type MessageData struct {
	ID               string `json:"id"`
	SessionID        string `json:"sessionId"`
	Role             string `json:"role"`
	Content          string `json:"content"`
	MessageType      string `json:"messageType"`
	Channel          string `json:"channel"`
	ChannelMessageID string `json:"channelMessageId"`
	TraceID          string `json:"traceId"`
	Initiator        string `json:"initiator"`
	SenderName       string `json:"senderName"`
	SenderID         string `json:"senderId"`
	CreatedAt        string `json:"createdAt"`
}
