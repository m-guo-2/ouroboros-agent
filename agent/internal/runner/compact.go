package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"agent/internal/engine"
	"agent/internal/logger"
	"agent/internal/storage"
	"agent/internal/types"
)

const (
	triggerRatio     = 0.60
	targetRatio      = 0.50
	toolResultMaxLen = 1024
	summaryMaxWords  = 200
)

type CompactResult struct {
	Messages         []types.AgentMessage
	Compacted        bool
	ArchivedCount    int
	Summary          string
	TokensBefore     int
	TokensAfter      int
	TruncatedResults int
}

func ShouldCompact(estimate TokenEstimate) bool {
	return estimate.Ratio > triggerRatio
}

func CompactContext(
	ctx context.Context,
	messages []types.AgentMessage,
	model string,
	compactLLM engine.LLMClient,
	compactModel string,
	sessionID string,
) (*CompactResult, error) {
	contextWindow := GetContextWindow(model)
	targetTokens := int(float64(contextWindow) * targetRatio)
	tokensBefore := PreciseEstimateTokens(messages)

	if tokensBefore <= targetTokens {
		return &CompactResult{Messages: messages, Compacted: false, TokensBefore: tokensBefore, TokensAfter: tokensBefore}, nil
	}

	boundaries := findTurnBoundaries(messages)
	if len(boundaries) <= 1 {
		truncated, nTrunc := truncateLargeToolResults(messages)
		return &CompactResult{
			Messages:         truncated,
			Compacted:        nTrunc > 0,
			TokensBefore:     tokensBefore,
			TokensAfter:      PreciseEstimateTokens(truncated),
			TruncatedResults: nTrunc,
		}, nil
	}

	archiveEnd := 0
	for i := 1; i < len(boundaries); i++ {
		candidate := messages[boundaries[i]:]
		est := PreciseEstimateTokens(candidate)
		// 2 (summary + ack) overhead ~ 100 tokens
		if est+100 <= targetTokens {
			archiveEnd = boundaries[i]
			break
		}
	}

	if archiveEnd == 0 {
		archiveEnd = boundaries[len(boundaries)-1]
	}

	archived := messages[:archiveEnd]
	retained := messages[archiveEnd:]

	retained, nTrunc := truncateLargeToolResults(retained)

	retained = sanitizeOrphanToolBlocks(retained)

	summary := generateSummary(ctx, archived, compactLLM, compactModel)

	summaryMsg := types.AgentMessage{
		Role: "user",
		Content: []types.ContentBlock{{
			Type: "text",
			Text: fmt.Sprintf("[Context Compact]\nPrevious conversation (%d messages, archived at %s):\n\n%s\n\n---\nFull history available via recall_context tool.",
				len(archived), time.Now().Format(time.RFC3339), summary),
		}},
	}
	ackMsg := types.AgentMessage{
		Role: "assistant",
		Content: []types.ContentBlock{{
			Type: "text",
			Text: "Understood. I have the conversation context. How can I help?",
		}},
	}

	compacted := make([]types.AgentMessage, 0, 2+len(retained))
	compacted = append(compacted, summaryMsg, ackMsg)
	compacted = append(compacted, retained...)

	tokensAfter := PreciseEstimateTokens(compacted)

	err := storage.SaveCompaction(storage.CompactionData{
		SessionID:            sessionID,
		Summary:              summary,
		ArchivedBeforeTime:   time.Now().Format(time.RFC3339),
		ArchivedMessageCount: len(archived),
		TokenCountBefore:     tokensBefore,
		TokenCountAfter:      tokensAfter,
		CompactModel:         compactModel,
	})
	if err != nil {
		logger.Warn(ctx, "压缩元数据写入失败，回退到硬截断",
			"error", err.Error(), "sessionId", sessionID)
		fallback := truncateByFullTurns(messages, 10)
		return &CompactResult{
			Messages:     fallback,
			Compacted:    true,
			TokensBefore: tokensBefore,
			TokensAfter:  PreciseEstimateTokens(fallback),
		}, nil
	}

	logger.Business(ctx, "上下文压缩完成",
		"sessionId", sessionID,
		"tokensBefore", tokensBefore,
		"tokensAfter", tokensAfter,
		"archivedMessages", len(archived),
		"truncatedResults", nTrunc,
		"compactModel", compactModel)

	return &CompactResult{
		Messages:         compacted,
		Compacted:        true,
		ArchivedCount:    len(archived),
		Summary:          summary,
		TokensBefore:     tokensBefore,
		TokensAfter:      tokensAfter,
		TruncatedResults: nTrunc,
	}, nil
}

// findTurnBoundaries returns the starting index of each user text turn.
// A "turn" begins at a user message whose content is text (not tool_result).
func findTurnBoundaries(messages []types.AgentMessage) []int {
	var boundaries []int
	for i, msg := range messages {
		if msg.Role != "user" {
			continue
		}
		isToolResult := false
		for _, b := range msg.Content {
			if b.Type == "tool_result" {
				isToolResult = true
				break
			}
		}
		if !isToolResult {
			boundaries = append(boundaries, i)
		}
	}
	return boundaries
}

func truncateLargeToolResults(messages []types.AgentMessage) ([]types.AgentMessage, int) {
	count := 0
	out := make([]types.AgentMessage, len(messages))
	for i, msg := range messages {
		if msg.Role != "user" {
			out[i] = msg
			continue
		}
		blocks := make([]types.ContentBlock, len(msg.Content))
		copy(blocks, msg.Content)
		for j, b := range blocks {
			if b.Type == "tool_result" && len(b.Content) > toolResultMaxLen {
				blocks[j].Content = b.Content[:toolResultMaxLen] +
					"\n...[truncated, use recall_context to retrieve full content]"
				count++
			}
		}
		out[i] = types.AgentMessage{Role: msg.Role, Content: blocks}
	}
	return out, count
}

// sanitizeOrphanToolBlocks removes orphaned tool_use blocks (whose tool_result
// was archived) and orphaned tool_result blocks (whose tool_use was archived).
// This ensures API-level consistency: every tool_use must have a matching tool_result.
func sanitizeOrphanToolBlocks(messages []types.AgentMessage) []types.AgentMessage {
	toolUseIDs := make(map[string]bool)
	toolResultIDs := make(map[string]bool)

	for _, msg := range messages {
		for _, b := range msg.Content {
			if b.Type == "tool_use" && b.ID != "" {
				toolUseIDs[b.ID] = true
			}
			if b.Type == "tool_result" && b.ToolUseID != "" {
				toolResultIDs[b.ToolUseID] = true
			}
		}
	}

	var result []types.AgentMessage
	for _, msg := range messages {
		var filtered []types.ContentBlock
		for _, b := range msg.Content {
			switch b.Type {
			case "tool_use":
				if b.ID != "" && !toolResultIDs[b.ID] {
					continue // orphan tool_use
				}
				filtered = append(filtered, b)
			case "tool_result":
				if b.ToolUseID != "" && !toolUseIDs[b.ToolUseID] {
					continue // orphan tool_result
				}
				filtered = append(filtered, b)
			default:
				filtered = append(filtered, b)
			}
		}
		if len(filtered) > 0 {
			result = append(result, types.AgentMessage{Role: msg.Role, Content: filtered})
		}
	}
	return result
}

func generateSummary(ctx context.Context, messages []types.AgentMessage, llmClient engine.LLMClient, model string) string {
	if llmClient == nil || model == "" {
		return buildFallbackSummary(messages)
	}

	digest := buildMessagesDigest(messages)

	prompt := fmt.Sprintf(
		"Summarize this conversation history in under %d words. "+
			"Focus on: key decisions made, user requirements, technical context, and action items. "+
			"Output in the same language as the conversation.\n\n%s",
		summaryMaxWords, digest)

	summaryMessages := []types.AgentMessage{{
		Role:    "user",
		Content: []types.ContentBlock{{Type: "text", Text: prompt}},
	}}

	resp, err := llmClient.Chat(ctx, engine.ChatParams{
		Messages:     summaryMessages,
		Model:        model,
		SystemPrompt: "You are a concise summarizer. Output only the summary, nothing else.",
	})
	if err != nil {
		logger.Warn(ctx, "摘要 LLM 调用失败，使用 fallback",
			"error", err.Error(), "model", model)
		return buildFallbackSummary(messages)
	}

	for _, block := range resp.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			return strings.TrimSpace(block.Text)
		}
	}

	return buildFallbackSummary(messages)
}

func buildMessagesDigest(messages []types.AgentMessage) string {
	var parts []string
	for _, msg := range messages {
		for _, b := range msg.Content {
			switch b.Type {
			case "text":
				text := b.Text
				if len(text) > 500 {
					text = text[:500] + "..."
				}
				parts = append(parts, fmt.Sprintf("[%s] %s", msg.Role, text))
			case "tool_use":
				inputJSON, _ := json.Marshal(b.Input)
				input := string(inputJSON)
				if len(input) > 200 {
					input = input[:200] + "..."
				}
				parts = append(parts, fmt.Sprintf("[assistant/tool_use] %s(%s)", b.Name, input))
			case "tool_result":
				content := b.Content
				if len(content) > 200 {
					content = content[:200] + "..."
				}
				parts = append(parts, fmt.Sprintf("[tool_result] %s", content))
			}
		}
	}
	result := strings.Join(parts, "\n")
	if len(result) > 8000 {
		result = result[:8000] + "\n...[digest truncated]"
	}
	return result
}

func buildFallbackSummary(messages []types.AgentMessage) string {
	userCount := 0
	toolCount := 0
	var topics []string

	for _, msg := range messages {
		for _, b := range msg.Content {
			if b.Type == "text" && msg.Role == "user" {
				userCount++
				text := b.Text
				if len(text) > 100 {
					text = text[:100] + "..."
				}
				if len(topics) < 3 {
					topics = append(topics, text)
				}
			}
			if b.Type == "tool_use" {
				toolCount++
			}
		}
	}

	return fmt.Sprintf(
		"[Earlier context archived, details available via recall_context]\n"+
			"Stats: %d user messages, %d tool calls.\nTopics: %s",
		userCount, toolCount, strings.Join(topics, " | "))
}
