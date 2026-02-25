package types

import (
	"encoding/json"
	"fmt"
)

// ContentBlock represents a generic block of content in an AgentMessage.
// It can be a text block, a tool_use block, or a tool_result block.
type ContentBlock struct {
	Type string `json:"type"`

	// TextBlock fields
	Text string `json:"text,omitempty"`

	// ToolUseBlock fields
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`

	// ToolResultBlock fields
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"` // For tool_result, content is string
	IsError   bool   `json:"is_error,omitempty"`
}

// AgentMessage represents a message in the conversation history.
type AgentMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// UnmarshalJSON custom unmarshaler for AgentMessage to handle Content
// being either a string or an array of ContentBlocks.
func (m *AgentMessage) UnmarshalJSON(data []byte) error {
	type Alias AgentMessage
	aux := &struct {
		Content json.RawMessage `json:"content"`
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Try to unmarshal Content as a string
	var strContent string
	if err := json.Unmarshal(aux.Content, &strContent); err == nil {
		m.Content = []ContentBlock{
			{
				Type: "text",
				Text: strContent,
			},
		}
		return nil
	}

	// Try to unmarshal Content as an array of ContentBlocks
	var blocks []ContentBlock
	if err := json.Unmarshal(aux.Content, &blocks); err == nil {
		m.Content = blocks
		return nil
	}

	return fmt.Errorf("AgentMessage.Content must be a string or an array of ContentBlocks")
}

// MarshalJSON custom marshaler for AgentMessage
func (m *AgentMessage) MarshalJSON() ([]byte, error) {
	type Alias AgentMessage
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	})
}
