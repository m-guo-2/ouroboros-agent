package storage

// SubagentModelConfig holds per-profile model override for a subagent.
type SubagentModelConfig struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// AgentConfig holds the runtime configuration for an agent.
type AgentConfig struct {
	ID             string                         `json:"id"`
	DisplayName    string                         `json:"displayName"`
	SystemPrompt   string                         `json:"systemPrompt"`
	ModelID        string                         `json:"modelId,omitempty"`
	Provider       string                         `json:"provider,omitempty"`
	Model          string                         `json:"model,omitempty"`
	SubagentModels map[string]SubagentModelConfig `json:"subagentModels,omitempty"`
	Skills         []string                       `json:"skills"`
	SubagentSkills map[string][]string            `json:"subagentSkills,omitempty"`
	Channels       []ChannelBinding               `json:"channels"`
	IsActive       bool                           `json:"isActive"`
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

// SkillContext is the compiled output of bound skills for an agent.
// All skills use progressive loading: Level 1 metadata index in prompt,
// Level 2 full content via load_skill, Level 3 references via load_skill_reference.
type SkillContext struct {
	SkillsSnippet    string          `json:"skillsSnippet"`             // Level 1 metadata index injected into system prompt
	LoadableSkillIDs map[string]bool `json:"loadableSkillIDs"`         // skill IDs that load_skill can load
	Diagnostics      []string        `json:"diagnostics,omitempty"`    // runtime/local-store diagnostics
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
	EventCursor           int64  `json:"eventCursor"`
	CreatedAt             int64  `json:"createdAt"`
	UpdatedAt             int64  `json:"updatedAt"`
	Context               string `json:"-"`
}

type AttachmentData struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	ResourceURI       string `json:"resourceUri"`
	DisplayName       string `json:"displayName,omitempty"`
	MIMEType          string `json:"mimeType,omitempty"`
	SourceMessageType string `json:"sourceMessageType,omitempty"`
}

// MessageData represents a single stored message.
type MessageData struct {
	ID               int64            `json:"id"`
	SessionID        string           `json:"sessionId"`
	Role             string           `json:"role"`
	Content          string           `json:"content"`
	MessageType      string           `json:"messageType"`
	Channel          string           `json:"channel"`
	ChannelMessageID string           `json:"channelMessageId"`
	TraceID          string           `json:"traceId"`
	Initiator        string           `json:"initiator"`
	SenderName       string           `json:"senderName"`
	SenderID         string           `json:"senderId"`
	Attachments      []AttachmentData `json:"attachments,omitempty"`
	ChannelMeta      map[string]any   `json:"channelMeta,omitempty"`
	CreatedAt        int64            `json:"createdAt"`
}
