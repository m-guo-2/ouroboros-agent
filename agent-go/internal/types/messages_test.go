package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAgentMessageUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		expected AgentMessage
	}{
		{
			name:     "string content",
			jsonData: `{"role": "user", "content": "Hello world"}`,
			expected: AgentMessage{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: "Hello world"},
				},
			},
		},
		{
			name:     "array of blocks",
			jsonData: `{"role": "assistant", "content": [{"type": "text", "text": "Thinking..."}, {"type": "tool_use", "id": "t1", "name": "get_weather", "input": {"city": "Beijing"}}]}`,
			expected: AgentMessage{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "text", Text: "Thinking..."},
					{Type: "tool_use", ID: "t1", Name: "get_weather", Input: map[string]interface{}{"city": "Beijing"}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg AgentMessage
			err := json.Unmarshal([]byte(tt.jsonData), &msg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if msg.Role != tt.expected.Role {
				t.Errorf("expected role %s, got %s", tt.expected.Role, msg.Role)
			}

			if len(msg.Content) != len(tt.expected.Content) {
				t.Fatalf("expected %d blocks, got %d", len(tt.expected.Content), len(msg.Content))
			}

			for i, b := range msg.Content {
				expectedB := tt.expected.Content[i]
				if b.Type != expectedB.Type {
					t.Errorf("block %d: expected type %s, got %s", i, expectedB.Type, b.Type)
				}
				if b.Type == "tool_use" {
					if !reflect.DeepEqual(b.Input, expectedB.Input) {
						t.Errorf("block %d: expected input %v, got %v", i, expectedB.Input, b.Input)
					}
				} else {
					if b.Text != expectedB.Text {
						t.Errorf("block %d: expected text %s, got %s", i, expectedB.Text, b.Text)
					}
				}
			}
		})
	}
}
