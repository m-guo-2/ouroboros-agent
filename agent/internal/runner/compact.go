package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"agent/internal/engine"
	"agent/internal/engine/ostools"
	"agent/internal/logger"
	"agent/internal/storage"
	"agent/internal/timeutil"
	"agent/internal/types"
)

const (
	triggerRatio     = 0.60
	targetRatio      = 0.50
	toolResultMaxLen = 1024
	handoffMaxWords  = 500

	recentUserTurns = 5
	retainMaxRatio  = 0.50
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

	archiveEnd := findRetainStart(messages, recentUserTurns, retainMaxRatio)

	if archiveEnd == 0 {
		archiveEnd = boundaries[len(boundaries)-1]
	}

	archived := messages[:archiveEnd]
	retained := messages[archiveEnd:]

	retained, nTrunc := truncateLargeToolResults(retained)
	retained = sanitizeOrphanToolBlocks(retained)

	summary := generateSummary(ctx, archived, compactLLM, compactModel)
	taskState := ExtractTaskState(ctx, archived, compactLLM, compactModel)

	// Four-layer assembly
	summaryLayer := buildSummaryLayer(len(archived), summary)
	taskStateLayer := buildTaskStateLayer(taskState)
	agentMemoryLayer := buildAgentMemoryLayer(sessionID, model)

	compacted := make([]types.AgentMessage, 0, len(summaryLayer)+len(retained)+len(taskStateLayer)+len(agentMemoryLayer))
	compacted = append(compacted, summaryLayer...)
	compacted = append(compacted, retained...)
	compacted = append(compacted, taskStateLayer...)
	compacted = append(compacted, agentMemoryLayer...)

	tokensAfter := PreciseEstimateTokens(compacted)

	now := timeutil.NowMs()
	compactionID, err := storage.SaveCompaction(storage.CompactionData{
		SessionID:            sessionID,
		Summary:              summary,
		ArchivedBeforeTime:   now,
		ArchivedMessageCount: len(archived),
		TokenCountBefore:     tokensBefore,
		TokenCountAfter:      tokensAfter,
		CompactModel:         compactModel,
	})
	if err != nil {
		logger.Warn(ctx, "压缩元数据写入失败，回退到硬截断",
			"error", err.Error(), "sessionId", sessionID)
		fallback := TruncateByFullTurns(messages, 10)
		return &CompactResult{
			Messages:     fallback,
			Compacted:    true,
			TokensBefore: tokensBefore,
			TokensAfter:  PreciseEstimateTokens(fallback),
		}, nil
	}

	if archiveErr := storage.SaveCompactionArchive(compactionID, sessionID, archived); archiveErr != nil {
		logger.Warn(ctx, "归档快照写入失败",
			"error", archiveErr.Error(), "sessionId", sessionID, "compactionId", compactionID)
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

// findRetainStart returns the index at which retained messages begin.
// It anchors on the last N user text turns, capped so retained count
// does not exceed maxRatio of total messages.
func findRetainStart(messages []types.AgentMessage, recentTurns int, maxRatio float64) int {
	boundaries := findTurnBoundaries(messages)
	if len(boundaries) == 0 {
		return 0
	}

	maxRetain := int(float64(len(messages)) * maxRatio)
	if maxRetain < 1 {
		maxRetain = 1
	}

	// Walk backwards through turn boundaries, collecting up to recentTurns.
	start := len(messages)
	turnsKept := 0
	for i := len(boundaries) - 1; i >= 0 && turnsKept < recentTurns; i-- {
		candidate := boundaries[i]
		retainCount := len(messages) - candidate
		if retainCount > maxRetain {
			break
		}
		start = candidate
		turnsKept++
	}

	if start == len(messages) {
		// Could not keep even one turn within maxRatio — keep from the last boundary.
		start = boundaries[len(boundaries)-1]
	}

	return start
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
					continue
				}
				filtered = append(filtered, b)
			case "tool_result":
				if b.ToolUseID != "" && !toolUseIDs[b.ToolUseID] {
					continue
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

// --- Layer builders for four-layer assembly ---

func buildSummaryLayer(archivedCount int, summary string) []types.AgentMessage {
	text := fmt.Sprintf(
		"[Compaction Handoff]\n之前的对话（%d 条消息已归档）已压缩为以下状态承接包。摘要只用于继续当前任务，不是唯一事实源；如需确认用户原话、工具完整输出或时间顺序，使用 recall_context 检索原文。\n\n%s",
		archivedCount, summary)

	return []types.AgentMessage{
		{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: text}}},
		{Role: "assistant", Content: []types.ContentBlock{{Type: "text", Text: "好的，我已了解压缩后的任务状态和可召回历史。"}}},
	}
}

func buildTaskStateLayer(taskState string) []types.AgentMessage {
	if strings.TrimSpace(taskState) == "" {
		return nil
	}

	return []types.AgentMessage{
		{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "[任务状态]\n" + taskState}}},
		{Role: "assistant", Content: []types.ContentBlock{{Type: "text", Text: "好的，我已了解当前的任务状态。"}}},
	}
}

func buildAgentMemoryLayer(sessionID, model string) []types.AgentMessage {
	facts, err := storage.GetSessionFacts(sessionID)
	if err != nil || len(facts) == 0 {
		return nil
	}

	contextWindow := GetContextWindow(model)
	budgetChars := int(float64(contextWindow) * factsTokenBudgetRatio * 4)

	var lines []string
	totalChars := 0
	for i := len(facts) - 1; i >= 0; i-- {
		line := fmt.Sprintf("[%s] %s", facts[i].Category, facts[i].Fact)
		if totalChars+len(line) > budgetChars {
			break
		}
		lines = append([]string{line}, lines...)
		totalChars += len(line)
	}

	if len(lines) == 0 {
		return nil
	}

	text := "[Agent Memory]\n以下是本次对话中积累的关键事实，供参考：\n\n" + strings.Join(lines, "\n")

	return []types.AgentMessage{
		{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: text}}},
		{Role: "assistant", Content: []types.ContentBlock{{Type: "text", Text: "好的，我已了解这些上下文信息。"}}},
	}
}

// --- Handoff generation ---

func generateSummary(ctx context.Context, messages []types.AgentMessage, llmClient engine.LLMClient, model string) string {
	if llmClient == nil || model == "" {
		return buildFallbackSummary(messages)
	}

	digest := buildHandoffDigest(messages)

	prompt := fmt.Sprintf(
		"请把以下历史上下文压缩为不超过 %d 词的 Compaction Handoff Packet，用于让后续模型继续当前任务。\n\n"+
			"硬性要求：\n"+
			"- 事实不能编造；不确定就写“需 recall_context 确认”。\n"+
			"- 用户消息优先，尽量保留用户明确要求、纠正、否定、最新口径的原话或近原话。\n"+
			"- 工具调用和执行结果只保留对任务继续有影响的事实：查过什么、改过什么、发过什么、失败过什么、关键路径/错误/结果是什么。\n"+
			"- 不要把工具日志和用户意图平权；不要输出大段 JSON 或 stdout。\n"+
			"- 摘要不是事实源，必须在最后列出需要回查原文的 Recall Pointers。\n\n"+
			"请严格按以下结构输出：\n"+
			"[用户事实]\n- 用户明确要求：...\n- 用户纠正/否定：...\n- 用户给出的关键事实：...\n- 用户最新口径：...\n\n"+
			"[当前目标]\n- ...\n\n"+
			"[关键决策]\n- ...\n\n"+
			"[已完成]\n- ...\n\n"+
			"[未完成]\n- ...\n\n"+
			"[证据与产物]\n- 文件/路径/命令/外部消息口径：...\n\n"+
			"[风险与失败]\n- ...\n\n"+
			"[Recall Pointers]\n- 需要确认用户原话或完整工具输出时，使用 recall_context 查询：...\n\n"+
			"历史上下文 digest：\n%s",
		handoffMaxWords, digest)

	summaryMessages := []types.AgentMessage{{
		Role:    "user",
		Content: []types.ContentBlock{{Type: "text", Text: prompt}},
	}}

	resp, err := llmClient.Chat(ctx, engine.ChatParams{
		Messages:     summaryMessages,
		Model:        model,
		SystemPrompt: "你是一个上下文压缩助手。你的任务是生成准确的状态承接包，优先保护用户事实、当前目标、关键决策、未完成事项和可回查线索。只输出状态承接包，不要输出其他内容。",
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

// --- Task state extraction ---

func ExtractTaskState(ctx context.Context, archived []types.AgentMessage, llmClient engine.LLMClient, model string) string {
	if llmClient == nil || model == "" {
		return ""
	}

	digest := buildMessagesDigest(archived)
	if strings.TrimSpace(digest) == "" {
		return ""
	}

	prompt := "请从以下对话中提取未完成的任务和当前目标。要求：\n\n" +
		"1. 识别用户的主要目标\n" +
		"2. 列出所有任务及其状态（待办/进行中/已完成）\n" +
		"3. 标注每个任务的来源：(来源: user) 表示用户明确要求的，(来源: agent) 表示你自行推断的\n" +
		"4. 如果没有未完成任务，只回复 NO_TASKS\n\n" +
		"输出格式：\n" +
		"当前目标: <描述>\n\n" +
		"未完成任务:\n" +
		"- [ ] <task> (来源: user/agent)\n" +
		"- [x] <task> (已完成)\n\n" +
		"对话内容：\n" + digest

	taskMessages := []types.AgentMessage{{
		Role:    "user",
		Content: []types.ContentBlock{{Type: "text", Text: prompt}},
	}}

	resp, err := llmClient.Chat(ctx, engine.ChatParams{
		Messages:     taskMessages,
		Model:        model,
		SystemPrompt: "你是一个任务提取助手，从对话中识别未完成的任务和目标。只输出任务列表，不要输出其他内容。",
	})
	if err != nil {
		logger.Warn(ctx, "任务提取 LLM 调用失败",
			"error", err.Error(), "model", model)
		return ""
	}

	for _, block := range resp.Content {
		if block.Type == "text" {
			text := strings.TrimSpace(block.Text)
			if text == "NO_TASKS" || text == "" {
				return ""
			}
			return text
		}
	}

	return ""
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

func buildHandoffDigest(messages []types.AgentMessage) string {
	var userFacts []string
	var chronology []string

	for _, msg := range messages {
		for _, b := range msg.Content {
			switch b.Type {
			case "text":
				text := compactText(b.Text, 900)
				line := fmt.Sprintf("[%s] %s", msg.Role, text)
				chronology = append(chronology, line)
				if msg.Role == "user" {
					userFacts = append(userFacts, line)
				}
			case "tool_use":
				inputJSON, _ := json.Marshal(b.Input)
				chronology = append(chronology, fmt.Sprintf("[assistant/tool_use] %s(%s)", b.Name, compactText(string(inputJSON), 260)))
			case "tool_result":
				chronology = append(chronology, fmt.Sprintf("[tool_result] %s", compactText(b.Content, 260)))
			}
		}
	}

	var sections []string
	if len(userFacts) > 0 {
		sections = append(sections, "[用户消息优先摘录]\n"+limitJoined(userFacts, "\n", 6000))
	}
	if len(chronology) > 0 {
		sections = append(sections, "[历史时序摘录]\n"+limitJoined(chronology, "\n", 9000))
	}
	result := strings.Join(sections, "\n\n")
	if len(result) > 14000 {
		result = result[:14000] + "\n...[digest truncated; use recall_context for full archived history]"
	}
	return result
}

func compactText(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

func limitJoined(items []string, sep string, max int) string {
	var out []string
	total := 0
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if total+len(item)+len(sep) > max {
			break
		}
		out = append([]string{item}, out...)
		total += len(item) + len(sep)
	}
	return strings.Join(out, sep)
}

const flushMaxIterations = 3

func FlushMemoryBeforeCompact(
	ctx context.Context,
	messages []types.AgentMessage,
	model string,
	flushLLM engine.LLMClient,
	flushModel string,
	sessionID string,
) {
	if flushLLM == nil || flushModel == "" {
		return
	}

	boundaries := findTurnBoundaries(messages)
	if len(boundaries) <= 1 {
		return
	}

	archiveEnd := findRetainStart(messages, recentUserTurns, retainMaxRatio)
	if archiveEnd == 0 {
		return
	}

	archived := messages[:archiveEnd]
	digest := buildMessagesDigest(archived)
	if strings.TrimSpace(digest) == "" {
		return
	}

	flushPrompt := fmt.Sprintf(
		"上下文即将被压缩，以下较早的对话内容将被归档。"+
			"请提取其中的关键事实（决策、结论、需求、技术细节）并调用 save_memory 保存。"+
			"如果没有需要保存的内容，直接回复 NO_SAVE。\n\n%s", digest)

	flushMessages := []types.AgentMessage{{
		Role:    "user",
		Content: []types.ContentBlock{{Type: "text", Text: flushPrompt}},
	}}

	saveMemoryTool := ostools.NewSaveMemoryTool(sessionID)

	_, err := engine.RunAgentLoop(ctx, engine.AgentLoopConfig{
		LLMClient:     flushLLM,
		SystemPrompt:  "You are a memory extraction assistant. Extract key facts and save them using the save_memory tool. Be concise and factual.",
		Messages:      flushMessages,
		Tools:         []types.RegisteredTool{saveMemoryTool},
		Model:         flushModel,
		MaxIterations: flushMaxIterations,
	})
	if err != nil {
		logger.Warn(ctx, "memory flush 失败，继续正常压缩",
			"error", err.Error(), "sessionId", sessionID)
	}
}

func buildFallbackSummary(messages []types.AgentMessage) string {
	userCount := 0
	toolCount := 0
	var userFacts []string

	for _, msg := range messages {
		for _, b := range msg.Content {
			if b.Type == "text" && msg.Role == "user" {
				userCount++
				text := b.Text
				if len(text) > 180 {
					text = text[:180] + "..."
				}
				userFacts = append(userFacts, "- "+text)
			}
			if b.Type == "tool_use" {
				toolCount++
			}
		}
	}
	if len(userFacts) > 5 {
		userFacts = userFacts[len(userFacts)-5:]
	}

	return fmt.Sprintf(
		"[用户事实]\n%s\n\n"+
			"[当前目标]\n- 摘要模型不可用，需结合最近消息继续判断。\n\n"+
			"[关键决策]\n- 需 recall_context 确认。\n\n"+
			"[已完成]\n- 归档区包含 %d 次工具调用。\n\n"+
			"[未完成]\n- 需结合最近消息和 recall_context 确认。\n\n"+
			"[证据与产物]\n- 完整历史已归档，可通过 recall_context 检索。\n\n"+
			"[风险与失败]\n- 当前为 fallback handoff，可能遗漏细节。\n\n"+
			"[Recall Pointers]\n- 查询用户原话、完整工具输出、历史决策；归档统计：%d 条用户消息，%d 次工具调用。",
		strings.Join(userFacts, "\n"), toolCount, userCount, toolCount)
}
