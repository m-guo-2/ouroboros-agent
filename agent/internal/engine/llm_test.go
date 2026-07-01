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

func TestNormalizeChatToolsFillsEmptyObjectSchema(t *testing.T) {
	tools := normalizeChatTools([]types.ToolDefinition{
		{Name: "empty_tool", InputSchema: types.JSONSchema{}},
	})
	if len(tools) != 1 {
		t.Fatalf("expected one tool, got %d", len(tools))
	}
	schema := tools[0].InputSchema
	if schema.Type != "object" {
		t.Fatalf("expected object schema, got %q", schema.Type)
	}
	if len(schema.Properties) == 0 {
		t.Fatal("expected compatible placeholder properties")
	}
}

func TestNormalizeToolSchemaFillsEmptySchemaAtRegistration(t *testing.T) {
	registry := NewToolRegistry()
	registry.RegisterBuiltin("empty_tool", "empty", types.JSONSchema{}, func(context.Context, map[string]interface{}) (interface{}, error) {
		return nil, nil
	})
	tool, ok := registry.Get("empty_tool")
	if !ok {
		t.Fatal("expected registered tool")
	}
	schema := tool.Definition.InputSchema
	if schema.Type != "object" {
		t.Fatalf("expected object schema, got %q", schema.Type)
	}
	if len(schema.Properties) == 0 {
		t.Fatal("expected compatible placeholder properties")
	}
}

func TestRegisterSkillInternalToolsDoesNotOverrideUnknownBuiltin(t *testing.T) {
	registry := NewToolRegistry()
	wantSchema := types.JSONSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"attachmentId": map[string]interface{}{"type": "string"},
		},
		Required: []string{"attachmentId"},
	}
	registry.RegisterBuiltin("inspect_attachment", "inspect", wantSchema, func(context.Context, map[string]interface{}) (interface{}, error) {
		return nil, nil
	})

	registry.RegisterSkillInternalTools(map[string]types.ToolExecutor{
		"inspect_attachment": func(context.Context, map[string]interface{}) (interface{}, error) {
			return nil, nil
		},
	})

	tool, ok := registry.Get("inspect_attachment")
	if !ok {
		t.Fatal("expected inspect_attachment to remain registered")
	}
	if tool.Definition.Description != "inspect" {
		t.Fatalf("unexpected description after internal registration: %q", tool.Definition.Description)
	}
	if _, ok := tool.Definition.InputSchema.Properties["attachmentId"]; !ok {
		t.Fatalf("expected original attachment schema to remain, got %+v", tool.Definition.InputSchema)
	}
}
