package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"agent-go/internal/types"
)

const DefaultMaxIterations = 25

type AgentEventHandler func(event types.AgentEvent)

type AgentLoopConfig struct {
	LLMClient     LLMClient
	SystemPrompt  string
	Messages      []types.AgentMessage
	Tools         []types.RegisteredTool
	OnEvent       AgentEventHandler
	OnNewMessages func(messages []types.AgentMessage) error
	MaxIterations int
	Model         string
}

type AgentLoopResult struct {
	FinalText        string
	Messages         []types.AgentMessage
	Usage            types.TokenUsage
	HitMaxIterations bool
}

func buildModelInputSummary(systemPrompt string, messages []types.AgentMessage, tools []types.ToolDefinition, model string) interface{} {
	if model == "" {
		model = "unknown"
	}

	spPreview := systemPrompt
	if len(spPreview) > 500 {
		spPreview = spPreview[:500]
	}

	var msgsSummary []map[string]interface{}
	for _, m := range messages {
		var contentParts []string
		for _, b := range m.Content {
			if b.Type == "text" {
				t := b.Text
				if len(t) > 200 {
					t = t[:200]
				}
				contentParts = append(contentParts, "[text] "+t)
			} else if b.Type == "tool_use" {
				contentParts = append(contentParts, "[tool_use:"+b.Name+"]")
			} else if b.Type == "tool_result" {
				c := b.Content
				if len(c) > 150 {
					c = c[:150]
				}
				contentParts = append(contentParts, "[tool_result:"+b.ToolUseID+"] "+c)
			} else {
				contentParts = append(contentParts, "[unknown]")
			}
		}
		
		joined := strings.Join(contentParts, " | ")
		if len(joined) > 400 {
			joined = joined[:400]
		}
		
		msgsSummary = append(msgsSummary, map[string]interface{}{
			"role":    m.Role,
			"content": joined,
		})
	}

	var toolNames []string
	for _, t := range tools {
		toolNames = append(toolNames, t.Name)
	}

	return map[string]interface{}{
		"model":               model,
		"systemPromptPreview": spPreview,
		"messageCount":        len(messages),
		"messages":            msgsSummary,
		"toolNames":           toolNames,
	}
}

func buildModelOutputSummary(response *LLMResponse) interface{} {
	var textParts []string
	var toolCalls []map[string]interface{}

	for _, b := range response.Content {
		if b.Type == "text" {
			textParts = append(textParts, b.Text)
		} else if b.Type == "tool_use" {
			inputBytes, _ := json.Marshal(b.Input)
			inputPreview := string(inputBytes)
			if len(inputPreview) > 200 {
				inputPreview = inputPreview[:200]
			}
			toolCalls = append(toolCalls, map[string]interface{}{
				"name":         b.Name,
				"id":           b.ID,
				"inputPreview": inputPreview,
			})
		}
	}

	textContent := strings.Join(textParts, "")
	if len(textContent) > 1500 {
		textContent = textContent[:1500]
	}

	res := map[string]interface{}{
		"content":      textContent,
		"stopReason":   response.StopReason,
		"inputTokens":  response.Usage.InputTokens,
		"outputTokens": response.Usage.OutputTokens,
	}
	if len(toolCalls) > 0 {
		res["toolCalls"] = toolCalls
	}

	return res
}

func RunAgentLoop(ctx context.Context, config AgentLoopConfig) (*AgentLoopResult, error) {
	maxIters := config.MaxIterations
	if maxIters <= 0 {
		maxIters = DefaultMaxIterations
	}

	messages := make([]types.AgentMessage, len(config.Messages))
	copy(messages, config.Messages)

	toolMap := make(map[string]types.RegisteredTool)
	var toolDefs []types.ToolDefinition
	for _, t := range config.Tools {
		toolMap[t.Definition.Name] = t
		toolDefs = append(toolDefs, t.Definition)
	}

	cumulativeUsage := types.TokenUsage{
		InputTokens:  0,
		OutputTokens: 0,
		TotalCostUsd: 0,
	}

	iteration := 0
	var finalText string
	hitMaxIterations := false

	for iteration < maxIters {
		if ctx.Err() != nil {
			config.OnEvent(types.AgentEvent{
				Type:      "error",
				Timestamp: time.Now().UnixMilli(),
				Iteration: iteration,
				Error:     "Agent loop aborted by signal",
			})
			break
		}

		iteration++

		// Step 1: Call LLM
		response, err := config.LLMClient.Chat(ctx, ChatParams{
			Messages:     messages,
			Tools:        toolDefs,
			SystemPrompt: config.SystemPrompt,
			Model:        config.Model,
		})

		if err != nil {
			config.OnEvent(types.AgentEvent{
				Type:      "error",
				Timestamp: time.Now().UnixMilli(),
				Iteration: iteration,
				Error:     err.Error(),
			})
			break
		}

		// Report Model I/O
		config.OnEvent(types.AgentEvent{
			Type:        "model_io",
			Timestamp:   time.Now().UnixMilli(),
			Iteration:   iteration,
			ModelInput:  buildModelInputSummary(config.SystemPrompt, messages, toolDefs, config.Model),
			ModelOutput: buildModelOutputSummary(response),
		})

		cumulativeUsage.InputTokens += response.Usage.InputTokens
		cumulativeUsage.OutputTokens += response.Usage.OutputTokens

		// Step 2: Parse response content
		var textBlocks []types.ContentBlock
		var toolUseBlocks []types.ContentBlock

		for _, block := range response.Content {
			if block.Type == "text" {
				textBlocks = append(textBlocks, block)
				if strings.TrimSpace(block.Text) != "" {
					config.OnEvent(types.AgentEvent{
						Type:      "thinking",
						Timestamp: time.Now().UnixMilli(),
						Iteration: iteration,
						Thinking:  block.Text,
						Source:    "model",
					})
				}
			} else if block.Type == "tool_use" {
				toolUseBlocks = append(toolUseBlocks, block)
			}
		}

		// Step 3: If no tool calls, loop ends
		if len(toolUseBlocks) == 0 {
			var texts []string
			for _, b := range textBlocks {
				texts = append(texts, b.Text)
			}
			finalText = strings.Join(texts, "\n")
			break
		}

		// Append assistant message (keep both text and tool_use blocks intact as per new design)
		var assistantBlocks []types.ContentBlock
		assistantBlocks = append(assistantBlocks, textBlocks...)
		assistantBlocks = append(assistantBlocks, toolUseBlocks...)
		assistantMsg := types.AgentMessage{
			Role:    "assistant",
			Content: assistantBlocks,
		}
		messages = append(messages, assistantMsg)

		// Step 4: Execute tools
		var toolResults []types.ContentBlock

		for _, toolUse := range toolUseBlocks {
			config.OnEvent(types.AgentEvent{
				Type:       "tool_call",
				Timestamp:  time.Now().UnixMilli(),
				Iteration:  iteration,
				ToolCallID: toolUse.ID,
				ToolName:   toolUse.Name,
				ToolInput:  toolUse.Input,
			})

			startedAt := time.Now().UnixMilli()
			registeredTool, ok := toolMap[toolUse.Name]

			if !ok {
				var available []string
				for k := range toolMap {
					available = append(available, k)
				}
				errorMsg := fmt.Sprintf("Tool not found: %s. Available tools: %s", toolUse.Name, strings.Join(available, ", "))
				
				f := false
				config.OnEvent(types.AgentEvent{
					Type:         "tool_result",
					Timestamp:    time.Now().UnixMilli(),
					Iteration:    iteration,
					ToolCallID:   toolUse.ID,
					ToolName:     toolUse.Name,
					ToolResult:   errorMsg,
					ToolDuration: time.Now().UnixMilli() - startedAt,
					ToolSuccess:  &f,
				})

				toolResults = append(toolResults, types.ContentBlock{
					Type:      "tool_result",
					ToolUseID: toolUse.ID,
					Content:   errorMsg,
					IsError:   true,
				})
				continue
			}

			result, err := registeredTool.Execute(ctx, toolUse.Input)
			duration := time.Now().UnixMilli() - startedAt

			if err != nil {
				f := false
				config.OnEvent(types.AgentEvent{
					Type:         "tool_result",
					Timestamp:    time.Now().UnixMilli(),
					Iteration:    iteration,
					ToolCallID:   toolUse.ID,
					ToolName:     toolUse.Name,
					ToolResult:   err.Error(),
					ToolDuration: duration,
					ToolSuccess:  &f,
				})

				toolResults = append(toolResults, types.ContentBlock{
					Type:      "tool_result",
					ToolUseID: toolUse.ID,
					Content:   err.Error(),
					IsError:   true,
				})
			} else {
				var resultStr string
				if str, ok := result.(string); ok {
					resultStr = str
				} else {
					b, _ := json.MarshalIndent(result, "", "  ")
					resultStr = string(b)
				}

				t := true
				config.OnEvent(types.AgentEvent{
					Type:         "tool_result",
					Timestamp:    time.Now().UnixMilli(),
					Iteration:    iteration,
					ToolCallID:   toolUse.ID,
					ToolName:     toolUse.Name,
					ToolResult:   result,
					ToolDuration: duration,
					ToolSuccess:  &t,
				})

				toolResults = append(toolResults, types.ContentBlock{
					Type:      "tool_result",
					ToolUseID: toolUse.ID,
					Content:   resultStr,
				})
			}
		}

		// Step 5: Append tool results
		toolResultMsg := types.AgentMessage{
			Role:    "user",
			Content: toolResults,
		}
		messages = append(messages, toolResultMsg)

		// Step 6: Incremental persistence
		if config.OnNewMessages != nil {
			err := config.OnNewMessages([]types.AgentMessage{assistantMsg, toolResultMsg})
			if err != nil {
				config.OnEvent(types.AgentEvent{
					Type:      "error",
					Timestamp: time.Now().UnixMilli(),
					Iteration: iteration,
					Error:     fmt.Sprintf("Failed to persist iteration messages: %v", err),
					Source:    "system",
				})
			}
		}

		if iteration >= maxIters {
			hitMaxIterations = true
			config.OnEvent(types.AgentEvent{
				Type:      "error",
				Timestamp: time.Now().UnixMilli(),
				Iteration: iteration,
				Error:     fmt.Sprintf("Reached max iterations (%d), stopping loop", maxIters),
				Source:    "system",
			})
		}
	}

	config.OnEvent(types.AgentEvent{
		Type:      "done",
		Timestamp: time.Now().UnixMilli(),
		Usage:     &cumulativeUsage,
	})

	return &AgentLoopResult{
		FinalText:        finalText,
		Messages:         messages,
		Usage:            cumulativeUsage,
		HitMaxIterations: hitMaxIterations,
	}, nil
}
