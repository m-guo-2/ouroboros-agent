package ostools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"agent/internal/engine"
	"agent/internal/storage"
	"agent/internal/timeutil"
	"agent/internal/types"
)

const recallMaxResults = 20

var recallToolDef = types.ToolDefinition{
	Name: "recall_context",
	Description: "Retrieve earlier conversation history that was compressed out of the active context. " +
		"Use when you need to reference earlier discussions, decisions, or information that is no longer in the current context window.",
	InputSchema: types.JSONSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "What you're looking for in the archived history (keyword or topic)",
			},
			"mode": map[string]interface{}{
				"type":        "string",
				"description": "Search mode: 'search' for keyword matching, 'recent' for most recent archived messages, 'summary' for compression summaries. Default: search",
			},
		},
		Required: []string{"query"},
	},
}

func recallExecutor(sessionID string) types.ToolExecutor {
	return func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
		query, _ := input["query"].(string)
		if strings.TrimSpace(query) == "" {
			return nil, fmt.Errorf("query is required")
		}
		mode, _ := input["mode"].(string)
		if mode == "" {
			mode = "search"
		}

		switch mode {
		case "summary":
			return recallSummary(sessionID)
		case "recent":
			return recallRecent(sessionID)
		default:
			return recallSearch(sessionID, query)
		}
	}
}

func RegisterRecallContext(registry *engine.ToolRegistry, sessionID string) {
	registry.RegisterBuiltin(
		recallToolDef.Name,
		recallToolDef.Description,
		recallToolDef.InputSchema,
		recallExecutor(sessionID),
	)
}

// NewRecallContextTool creates a standalone RegisteredTool for recall_context.
// Used by subagent to inject without a ToolRegistry.
func NewRecallContextTool(sessionID string) types.RegisteredTool {
	return types.RegisteredTool{
		Definition: recallToolDef,
		Execute:    recallExecutor(sessionID),
		Source:     "builtin",
		SourceName: "system",
	}
}

func recallSearch(sessionID, query string) (interface{}, error) {
	compaction, err := storage.GetLatestCompaction(sessionID)
	if err != nil {
		return map[string]interface{}{
			"found":   false,
			"message": "No compressed context found for this session",
		}, nil
	}

	msgs, err := storage.SearchMessages(sessionID, query, compaction.ArchivedBeforeTime, recallMaxResults)
	if err != nil {
		return nil, fmt.Errorf("search archived messages: %w", err)
	}

	if len(msgs) == 0 {
		return map[string]interface{}{
			"found":   false,
			"query":   query,
			"message": "No matching messages found in archived history",
			"summary": compaction.Summary,
		}, nil
	}

	return map[string]interface{}{
		"found":    true,
		"query":    query,
		"count":    len(msgs),
		"messages": formatRecalledMessages(msgs),
	}, nil
}

func recallRecent(sessionID string) (interface{}, error) {
	compaction, err := storage.GetLatestCompaction(sessionID)
	if err != nil {
		return map[string]interface{}{
			"found":   false,
			"message": "No compressed context found for this session",
		}, nil
	}

	msgs, err := storage.GetMessagesBefore(sessionID, compaction.ArchivedBeforeTime, recallMaxResults)
	if err != nil {
		return nil, fmt.Errorf("get archived messages: %w", err)
	}

	return map[string]interface{}{
		"found":    len(msgs) > 0,
		"count":    len(msgs),
		"messages": formatRecalledMessages(msgs),
		"summary":  compaction.Summary,
	}, nil
}

func recallSummary(sessionID string) (interface{}, error) {
	compactions, err := storage.ListCompactions(sessionID)
	if err != nil || len(compactions) == 0 {
		return map[string]interface{}{
			"found":   false,
			"message": "No compression history found",
		}, nil
	}

	summaries := make([]map[string]interface{}, 0, len(compactions))
	for _, c := range compactions {
		summaries = append(summaries, map[string]interface{}{
			"compactedAt":     timeutil.FormatCST(c.CreatedAt),
			"summary":         c.Summary,
			"archivedCount":   c.ArchivedMessageCount,
			"tokensBefore":    c.TokenCountBefore,
			"tokensAfter":     c.TokenCountAfter,
			"compressionRate": fmt.Sprintf("%.0f%%", float64(c.TokenCountAfter)/float64(c.TokenCountBefore)*100),
		})
	}

	return map[string]interface{}{
		"found":       true,
		"compactions": summaries,
	}, nil
}

func formatRecalledMessages(msgs []storage.MessageData) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(msgs))
	for _, m := range msgs {
		content := m.Content
		if m.MessageType == "structured" {
			var blocks []types.ContentBlock
			if json.Unmarshal([]byte(m.Content), &blocks) == nil {
				var parts []string
				for _, b := range blocks {
					switch b.Type {
					case "text":
						parts = append(parts, b.Text)
					case "tool_use":
						inputJSON, _ := json.Marshal(b.Input)
						parts = append(parts, fmt.Sprintf("[tool_use: %s] %s", b.Name, string(inputJSON)))
					case "tool_result":
						parts = append(parts, fmt.Sprintf("[tool_result] %s", b.Content))
					}
				}
				content = strings.Join(parts, "\n")
			}
		}
		if len(content) > 2000 {
			content = content[:2000] + "\n...[truncated]"
		}

		out = append(out, map[string]interface{}{
			"role":      m.Role,
			"content":   content,
			"createdAt": timeutil.FormatCST(m.CreatedAt),
		})
	}
	return out
}
