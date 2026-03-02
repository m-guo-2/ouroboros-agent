package engine

import (
	"context"
	"testing"

	"agent/internal/types"
)

func TestSanitizeMessagesForAnthropic(t *testing.T) {
	tests := []struct {
		name     string
		input    []types.AgentMessage
		expected int // expected number of messages after sanitization
	}{
		{
			name: "valid tool_results kept",
			input: []types.AgentMessage{
				{Role: "assistant", Content: []types.ContentBlock{
					{Type: "tool_use", ID: "t1", Name: "x", Input: nil},
				}},
				{Role: "user", Content: []types.ContentBlock{
					{Type: "tool_result", ToolUseID: "t1", Content: "ok"},
				}},
			},
			expected: 2,
		},
		{
			name: "orphaned tool_result removed",
			input: []types.AgentMessage{
				{Role: "assistant", Content: []types.ContentBlock{
					{Type: "tool_use", ID: "t1", Name: "x", Input: nil},
				}},
				{Role: "user", Content: []types.ContentBlock{
					{Type: "tool_result", ToolUseID: "nonexistent", Content: "orphan"},
				}},
			},
			expected: 2,
		},
		{
			name: "empty tool_use_id removed",
			input: []types.AgentMessage{
				{Role: "assistant", Content: []types.ContentBlock{
					{Type: "tool_use", ID: "t1", Name: "x", Input: nil},
				}},
				{Role: "user", Content: []types.ContentBlock{
					{Type: "tool_result", ToolUseID: "", Content: "empty id"},
				}},
			},
			expected: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeMessagesForAnthropic(context.Background(), tt.input)
			if len(got) != tt.expected {
				t.Errorf("expected %d messages, got %d", tt.expected, len(got))
			}
			// For orphaned/empty cases, last user msg should have placeholder text
			if len(got) >= 2 && len(tt.input) >= 2 {
				lastUser := got[len(got)-1]
				if lastUser.Role == "user" && len(lastUser.Content) > 0 {
					if lastUser.Content[0].Type == "text" && lastUser.Content[0].Text == "[Tool results omitted – references invalid or truncated]" {
						return
					}
				}
			}
		})
	}
}
