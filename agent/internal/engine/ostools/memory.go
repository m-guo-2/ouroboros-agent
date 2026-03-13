package ostools

import (
	"context"
	"fmt"

	"agent/internal/engine"
	"agent/internal/storage"
	"agent/internal/types"
)

var saveMemoryToolDef = types.ToolDefinition{
	Name: "save_memory",
	Description: "Save important facts from the conversation to durable memory. " +
		"Use this when key decisions, conclusions, requirements, or technical details emerge. " +
		"Facts are stored verbatim and will be available in future turns even after context compression.",
	InputSchema: types.JSONSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"facts": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "List of facts to save. Each fact should be a self-contained statement.",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Category: decision, requirement, context, action, general. Default: general",
			},
		},
		Required: []string{"facts"},
	},
}

func saveMemoryExecutor(sessionID string) types.ToolExecutor {
	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		rawFacts, ok := input["facts"]
		if !ok {
			return nil, fmt.Errorf("facts is required")
		}

		var facts []string
		switch v := rawFacts.(type) {
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					facts = append(facts, s)
				}
			}
		default:
			return nil, fmt.Errorf("facts must be an array of strings")
		}

		category, _ := input["category"].(string)
		if category == "" {
			category = "general"
		}

		saved, err := storage.SaveSessionFacts(sessionID, facts, category)
		if err != nil {
			return nil, fmt.Errorf("save facts: %w", err)
		}

		return map[string]interface{}{
			"saved": saved,
		}, nil
	}
}

func RegisterSaveMemory(registry *engine.ToolRegistry, sessionID string) {
	registry.RegisterBuiltin(
		saveMemoryToolDef.Name,
		saveMemoryToolDef.Description,
		saveMemoryToolDef.InputSchema,
		saveMemoryExecutor(sessionID),
	)
}

func NewSaveMemoryTool(sessionID string) types.RegisteredTool {
	return types.RegisteredTool{
		Definition: saveMemoryToolDef,
		Execute:    saveMemoryExecutor(sessionID),
		Source:     "builtin",
		SourceName: "system",
	}
}
