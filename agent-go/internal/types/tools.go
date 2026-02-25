package types

import "context"

// ToolDefinition corresponds to the tool definition in Anthropic API
// typically adhering to a JSON Schema structure.
type ToolDefinition struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema JSONSchema `json:"input_schema"`
}

// JSONSchema simplified representation
type JSONSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

// ToolExecutor defines the signature for a tool execution function.
// Takes input map, returns result or error.
type ToolExecutor func(ctx context.Context, input map[string]interface{}) (interface{}, error)

// RegisteredTool represents a tool that has been registered along with its executor.
type RegisteredTool struct {
	Definition ToolDefinition
	Execute    ToolExecutor
	Source     string // "skill", "mcp", or "builtin"
	SourceName string // e.g., skill name, MCP server name, or "system"
}
