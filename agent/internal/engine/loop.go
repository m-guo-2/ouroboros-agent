package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"agent/internal/logger"
	"agent/internal/sanitize"
	"agent/internal/types"
)

const DefaultMaxIterations = 25

type AgentLoopConfig struct {
	LLMClient       LLMClient
	SystemPrompt    string
	Messages        []types.AgentMessage
	Tools           []types.RegisteredTool // static tool list (used when Registry is nil)
	Registry        *ToolRegistry          // if set, tools are read from registry each iteration
	OnNewMessages   func(messages []types.AgentMessage) error
	BeforeModelCall func(ctx context.Context, messages []types.AgentMessage) ([]types.AgentMessage, error)
	MaxIterations   int
	Model           string
	DrainNewEvents  func() []types.AgentMessage
	HasNewEvents    func() bool
}

const maxConsecutivePreemptions = 3

type AgentLoopResult struct {
	FinalText        string
	Messages         []types.AgentMessage
	Usage            types.TokenUsage
	LLMCallCount     int
	ToolCallCount    int
	ToolFailureCount int
	LLMDurationMs    int64
	ToolDurationMs   int64
	CostKnown        bool
	HitMaxIterations bool
	EventPreempted   bool
}

func estimateCost(model string, inputTokens, outputTokens int) (float64, bool) {
	type pricing struct{ in, out float64 }
	table := map[string]pricing{
		"claude-opus-4-5":            {15, 75},
		"claude-sonnet-4-5":          {3, 15},
		"claude-3-5-sonnet-20241022": {3, 15},
		"claude-3-5-haiku-20241022":  {0.8, 4},
		"claude-3-haiku-20240307":    {0.25, 1.25},
		"gpt-4o":                     {5, 15},
		"gpt-4o-mini":                {0.15, 0.60},
		"gpt-4-turbo":                {10, 30},
		"deepseek-chat":              {0.27, 1.10},
		"deepseek-reasoner":          {0.55, 2.19},
	}
	p, ok := table[model]
	if !ok {
		return 0, false
	}
	return float64(inputTokens)/1e6*p.in + float64(outputTokens)/1e6*p.out, true
}

func RunAgentLoop(ctx context.Context, config AgentLoopConfig) (*AgentLoopResult, error) {
	maxIters := config.MaxIterations
	if maxIters <= 0 {
		maxIters = DefaultMaxIterations
	}

	messages := make([]types.AgentMessage, len(config.Messages))
	copy(messages, config.Messages)

	buildToolIndex := func() (map[string]types.RegisteredTool, []types.ToolDefinition) {
		var src []types.RegisteredTool
		if config.Registry != nil {
			src = config.Registry.GetAll()
		} else {
			src = config.Tools
		}
		m := make(map[string]types.RegisteredTool, len(src))
		d := make([]types.ToolDefinition, 0, len(src))
		for _, t := range src {
			m[t.Definition.Name] = t
			d = append(d, t.Definition)
		}
		return m, d
	}

	toolMap, toolDefs := buildToolIndex()

	const maxEmptyResponseRetries = 3

	var totalInputTokens, totalOutputTokens int
	var totalCostUsd float64
	var llmCallCount, toolCallCount, toolFailureCount int
	var llmDurationMs, toolDurationMs int64
	costKnown := true
	iteration := 0
	emptyResponseRetries := 0
	consecutivePreemptions := 0
	var finalText string
	var loopErr error
	hitMaxIterations := false
	eventPreempted := false

	for iteration < maxIters {
		if ctx.Err() != nil {
			loopErr = ctx.Err()
			logger.Error(ctx, "引擎循环被中止",
				"traceEvent", "error", "iteration", iteration, "error", loopErr.Error())
			break
		}

		// ═══ Checkpoint 1: drain all new events before LLM call ═══
		if config.DrainNewEvents != nil {
			if newMsgs := config.DrainNewEvents(); len(newMsgs) > 0 {
				messages = append(messages, newMsgs...)
				logger.Business(ctx, "事件合并",
					"traceEvent", "event_drain", "iteration", iteration, "eventCount", len(newMsgs))
			}
		}

		if config.Registry != nil {
			toolMap, toolDefs = buildToolIndex()
		}

		if config.BeforeModelCall != nil {
			checked, err := config.BeforeModelCall(ctx, messages)
			if err != nil {
				loopErr = fmt.Errorf("before model call hook failed at iteration %d: %w", iteration+1, err)
				logger.Error(ctx, "模型调用前检查失败",
					"traceEvent", "error", "iteration", iteration+1, "error", err.Error())
				break
			}
			if checked != nil {
				messages = checked
			}
		}

		iteration++

		llmStart := time.Now()
		response, err := config.LLMClient.Chat(ctx, ChatParams{
			Messages:     messages,
			Tools:        toolDefs,
			SystemPrompt: config.SystemPrompt,
			Model:        config.Model,
		})
		callDurationMs := time.Since(llmStart).Milliseconds()

		if err != nil {
			loopErr = fmt.Errorf("LLM call failed at iteration %d: %w", iteration, err)
			logger.Error(ctx, "LLM 调用失败",
				"traceEvent", "error", "iteration", iteration, "error", err.Error())
			break
		}

		// Write full LLM I/O to dedicated file (detail level)
		llmIORef := logger.WriteLLMIO(ctx, iteration, response.RawRequest, response.RawResponse)

		callCost, callCostKnown := estimateCost(config.Model, response.Usage.InputTokens, response.Usage.OutputTokens)
		if !callCostKnown {
			costKnown = false
		}
		totalInputTokens += response.Usage.InputTokens
		totalOutputTokens += response.Usage.OutputTokens
		totalCostUsd += callCost
		llmCallCount++
		llmDurationMs += callDurationMs

		// Business-level: trace event with metrics + reference to full I/O
		logger.Business(ctx, "LLM 调用",
			"traceEvent", "llm_call",
			"iteration", iteration,
			"model", config.Model,
			"inputTokens", response.Usage.InputTokens,
			"outputTokens", response.Usage.OutputTokens,
			"durationMs", callDurationMs,
			"stopReason", response.StopReason,
			"costUsd", callCost,
			"llmIORef", llmIORef)

		var textBlocks []types.ContentBlock
		var toolUseBlocks []types.ContentBlock
		for _, block := range response.Content {
			if block.Type == "text" {
				textBlocks = append(textBlocks, block)
				if strings.TrimSpace(block.Text) != "" {
					logger.Business(ctx, "思考中",
						"traceEvent", "thinking", "iteration", iteration, "thinking", block.Text)
				}
			} else if block.Type == "tool_use" {
				toolUseBlocks = append(toolUseBlocks, block)
			}
		}

		if len(toolUseBlocks) == 0 {
			var texts []string
			for _, b := range textBlocks {
				texts = append(texts, b.Text)
			}
			joined := strings.Join(texts, "\n")

			if strings.Contains(joined, "Empty response:") && emptyResponseRetries < maxEmptyResponseRetries {
				emptyResponseRetries++
				iteration--
				logger.Warn(ctx, "LLM 返回空响应，立即重试",
					"traceEvent", "empty_response_retry",
					"iteration", iteration,
					"retryCount", emptyResponseRetries,
					"maxRetries", maxEmptyResponseRetries,
					"responseText", joined)
				continue
			}

			finalText = joined
			break
		}

		var assistantBlocks []types.ContentBlock
		assistantBlocks = append(assistantBlocks, textBlocks...)
		assistantBlocks = append(assistantBlocks, toolUseBlocks...)
		assistantMsg := types.AgentMessage{Role: "assistant", Content: assistantBlocks, ReasoningContent: response.ReasoningContent}
		messages = append(messages, assistantMsg)

		// ═══ Checkpoint 2: peek for new events before tool execution ═══
		if config.HasNewEvents != nil &&
			consecutivePreemptions < maxConsecutivePreemptions &&
			config.HasNewEvents() {

			var abandonedResults []types.ContentBlock
			for _, tu := range toolUseBlocks {
				abandonedResults = append(abandonedResults, types.ContentBlock{
					Type:      "tool_result",
					ToolUseID: tu.ID,
					Content:   "新消息到达，工具未执行。将基于最新信息重新决策。",
					IsError:   true,
				})
			}
			abandonedMsg := types.AgentMessage{Role: "user", Content: abandonedResults}
			messages = append(messages, abandonedMsg)

			if config.OnNewMessages != nil {
				_ = config.OnNewMessages([]types.AgentMessage{assistantMsg, abandonedMsg})
			}

			consecutivePreemptions++
			eventPreempted = true
			logger.Business(ctx, "事件抢占",
				"traceEvent", "event_preempt", "iteration", iteration,
				"consecutivePreemptions", consecutivePreemptions,
				"abandonedTools", len(toolUseBlocks))
			continue
		}
		consecutivePreemptions = 0

		var toolResults []types.ContentBlock
		for _, toolUse := range toolUseBlocks {
			toolCallCount++
			toolInputJSON, _ := json.Marshal(toolUse.Input)
			redactedToolInput := sanitize.RedactSecrets(string(toolInputJSON))
			logger.Business(ctx, "工具调用",
				"traceEvent", "tool_call", "iteration", iteration,
				"tool", toolUse.Name, "toolCallId", toolUse.ID, "toolInput", redactedToolInput)

			startedAt := time.Now().UnixMilli()
			registeredTool, ok := toolMap[toolUse.Name]
			if !ok {
				var available []string
				for k := range toolMap {
					available = append(available, k)
				}
				errorMsg := fmt.Sprintf("Tool not found: %s. Available tools: %s", toolUse.Name, strings.Join(available, ", "))
				redactedErrorMsg := sanitize.RedactSecrets(errorMsg)
				dur := time.Now().UnixMilli() - startedAt
				logger.Business(ctx, "工具返回",
					"traceEvent", "tool_result", "iteration", iteration,
					"tool", toolUse.Name, "toolCallId", toolUse.ID,
					"toolSuccess", false, "toolDuration", dur, "toolResult", redactedErrorMsg)
				toolResults = append(toolResults, types.ContentBlock{
					Type: "tool_result", ToolUseID: toolUse.ID, Content: errorMsg, IsError: true,
				})
				toolFailureCount++
				toolDurationMs += dur
				continue
			}

			result, err := registeredTool.Execute(ctx, toolUse.Input)
			duration := time.Now().UnixMilli() - startedAt
			if err != nil {
				toolFailureCount++
				redactedErr := sanitize.RedactSecrets(err.Error())
				logger.Business(ctx, "工具返回",
					"traceEvent", "tool_result", "iteration", iteration,
					"tool", toolUse.Name, "toolCallId", toolUse.ID,
					"toolSuccess", false, "toolDuration", duration, "toolResult", redactedErr)
				toolResults = append(toolResults, types.ContentBlock{
					Type: "tool_result", ToolUseID: toolUse.ID, Content: err.Error(), IsError: true,
				})
			} else {
				var resultStr string
				if str, ok := result.(string); ok {
					resultStr = str
				} else {
					b, _ := json.MarshalIndent(result, "", "  ")
					resultStr = string(b)
				}
				redactedResult := sanitize.RedactSecrets(resultStr)
				logger.Business(ctx, "工具返回",
					"traceEvent", "tool_result", "iteration", iteration,
					"tool", toolUse.Name, "toolCallId", toolUse.ID,
					"toolSuccess", true, "toolDuration", duration, "toolResult", redactedResult)
				toolResults = append(toolResults, types.ContentBlock{
					Type: "tool_result", ToolUseID: toolUse.ID, Content: resultStr,
				})
			}
			toolDurationMs += duration
		}

		toolResultMsg := types.AgentMessage{Role: "user", Content: toolResults}
		messages = append(messages, toolResultMsg)

		if config.OnNewMessages != nil {
			if err := config.OnNewMessages([]types.AgentMessage{assistantMsg, toolResultMsg}); err != nil {
				logger.Error(ctx, "持久化迭代消息失败",
					"traceEvent", "error", "iteration", iteration, "error", err.Error())
			}
		}

		if iteration >= maxIters {
			hitMaxIterations = true
			logger.Warn(ctx, "达到最大迭代数",
				"traceEvent", "error", "iteration", iteration, "maxIters", maxIters)
		}
	}

	logger.Business(ctx, "执行完成",
		"traceEvent", "done",
		"inputTokens", totalInputTokens,
		"outputTokens", totalOutputTokens,
		"totalCostUsd", totalCostUsd)

	return &AgentLoopResult{
		FinalText: finalText,
		Messages:  messages,
		Usage: types.TokenUsage{
			InputTokens:  totalInputTokens,
			OutputTokens: totalOutputTokens,
			TotalCostUsd: totalCostUsd,
		},
		LLMCallCount:     llmCallCount,
		ToolCallCount:    toolCallCount,
		ToolFailureCount: toolFailureCount,
		LLMDurationMs:    llmDurationMs,
		ToolDurationMs:   toolDurationMs,
		CostKnown:        costKnown,
		HitMaxIterations: hitMaxIterations,
		EventPreempted:   eventPreempted,
	}, loopErr
}
