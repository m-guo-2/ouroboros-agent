package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"agent-go/internal/engine"
	"agent-go/internal/serverclient"
	"agent-go/internal/types"
)

func createTraceReporter(server *serverclient.Client, base types.TraceEventPayload) func(types.AgentEvent) {
	return func(event types.AgentEvent) {
		fullEvent := types.TraceEventPayload{
			TraceID:    base.TraceID,
			SessionID:  base.SessionID,
			AgentID:    base.AgentID,
			UserID:     base.UserID,
			Channel:    base.Channel,
			AgentEvent: event,
		}

		tag := fmt.Sprintf("[react-engine] sid=%s tid=%s", base.SessionID, base.TraceID)
		if event.Type == "thinking" {
			preview := event.Thinking
			if len(preview) > 100 {
				preview = preview[:100]
			}
			fmt.Printf("%s think: %q\n", tag, preview)
		} else if event.Type == "tool_call" {
			fmt.Printf("%s call: %s id=%s\n", tag, event.ToolName, event.ToolCallID)
		} else if event.Type == "tool_result" {
			ok := "fail"
			if event.ToolSuccess != nil && *event.ToolSuccess {
				ok = "ok"
			}
			fmt.Printf("%s result: %s %s %dms\n", tag, event.ToolName, ok, event.ToolDuration)
		} else if event.Type == "done" {
			if event.Usage != nil {
				fmt.Printf("%s done: in=%d out=%d\n", tag, event.Usage.InputTokens, event.Usage.OutputTokens)
			}
		} else if event.Type == "error" {
			fmt.Printf("%s error: %s\n", tag, event.Error)
		}

		server.ReportTraceEvent(fullEvent)
	}
}

func buildSystemPrompt(agentSystemPrompt, skillsAddition string) string {
	var parts []string
	if agentSystemPrompt != "" {
		parts = append(parts, agentSystemPrompt)
	}

	builtin := `## 消息回复与输出原则

**核心原则：你的文字输出（推理过程、思考内容）对用户完全不可见。用户只能收到你通过 ` + "`" + `send_channel_message` + "`" + ` 工具主动发送的消息。**

所有需要传达给用户的内容，都必须通过 ` + "`" + `send_channel_message` + "`" + ` 工具发送，而非直接输出文字。

**参数：**
- ` + "`" + `content` + "`" + `（必填）：要发送的消息内容
- ` + "`" + `messageType` + "`" + `（可选）：消息类型，默认 text；支持 text / image / file / rich_text
- ` + "`" + `channel` + "`" + ` / ` + "`" + `channelUserId` + "`" + ` / ` + "`" + `channelConversationId` + "`" + `：默认取自消息来源，通常不需要手动填写

**使用原则：**
- 需要回复用户时，调用此工具发送，不要依赖直接文字输出。
- 可以分多次调用，不必等所有工作完成后才回复。
- 当用户的问题需要较长处理时间时，先发一条确认消息，再继续处理。

**关于历史消息的理解：**
历史消息只包含客观事件记录，不包含你过去的推理过程：
- ` + "`" + `user` + "`" + ` 消息 = 某个用户发来的内容。消息前的 [` + "`" + `名字` + "`" + `] 标记表示发送者身份（群聊场景有多个不同用户）
- ` + "`" + `assistant` + "`" + ` 消息（tool_use block）= 你过去执行的工具调用动作
- ` + "`" + `tool_result` + "`" + ` block = 工具执行返回的客观结果
- 你过去通过 ` + "`" + `send_channel_message` + "`" + ` 发送的内容会出现在对应的 tool_use 和 tool_result 中`

	parts = append(parts, builtin)

	if skillsAddition != "" {
		parts = append(parts, skillsAddition)
	}

	return strings.Join(parts, "\n\n")
}


func toPersistableMessages(loopMessages []types.AgentMessage) []serverclient.MessageData {
	var result []serverclient.MessageData
	for _, msg := range loopMessages {
		if msg.Role == "assistant" {
			var toolUseBlocks []types.ContentBlock
			for _, b := range msg.Content {
				if b.Type == "tool_use" {
					toolUseBlocks = append(toolUseBlocks, b)
				}
			}
			if len(toolUseBlocks) == 0 {
				continue
			}
			b, _ := json.Marshal(toolUseBlocks)
			result = append(result, serverclient.MessageData{
				Role:        "assistant",
				Content:     string(b),
				MessageType: "structured",
			})
		} else if msg.Role == "user" {
			var toolResultBlocks []types.ContentBlock
			for _, b := range msg.Content {
				if b.Type == "tool_result" {
					toolResultBlocks = append(toolResultBlocks, b)
				}
			}
			if len(toolResultBlocks) == 0 {
				continue
			}
			b, _ := json.Marshal(toolResultBlocks)
			result = append(result, serverclient.MessageData{
				Role:        "tool_result",
				Content:     string(b),
				MessageType: "structured",
			})
		}
	}
	return result
}

// truncateByFullTurns implementation truncates history while ensuring we don't break the user->assistant->user alternation
// and don't cut off in the middle of a "turn" (user + tools ...).
// A full turn is typically: user input -> assistant (tool uses) -> user (tool results) -> assistant (final response).
// Since we don't know exact token counts easily here without a tokenizer, we'll implement a simple limit by full turns
// assuming each user message that doesn't contain tool_result is the start of a turn.
func truncateByFullTurns(messages []types.AgentMessage, maxTurns int) []types.AgentMessage {
	if len(messages) == 0 {
		return messages
	}

	var turnStarts []int
	for i, msg := range messages {
		if msg.Role == "user" {
			isToolResult := false
			for _, b := range msg.Content {
				if b.Type == "tool_result" {
					isToolResult = true
					break
				}
			}
			if !isToolResult {
				turnStarts = append(turnStarts, i)
			}
		}
	}

	if len(turnStarts) <= maxTurns {
		return messages
	}

	startIndex := turnStarts[len(turnStarts)-maxTurns]
	return messages[startIndex:]
}

func processOneEvent(ctx context.Context, worker *SessionWorker, request QueuedRequest, server *serverclient.Client) error {
	report := createTraceReporter(server, types.TraceEventPayload{
		TraceID:   request.TraceID,
		SessionID: worker.SessionID,
		AgentID:   request.AgentID,
		UserID:    request.UserID,
		Channel:   request.Channel,
	})

	if !request.TraceStarted {
		report(types.AgentEvent{Type: "start", Timestamp: time.Now().UnixMilli()})
	}

	report(types.AgentEvent{
		Type:      "thinking",
		Timestamp: time.Now().UnixMilli(),
		Thinking:  "Loading Agent Config and Skills...",
		Source:    "system",
	})

	agentConfig, err := server.GetAgentConfig(ctx, request.AgentID)
	if err != nil || agentConfig == nil {
		return fmt.Errorf("agent not found: %s", request.AgentID)
	}

	provider := agentConfig.Provider
	modelName := agentConfig.Model
	if provider == "" || modelName == "" {
		return fmt.Errorf("agent %s missing provider/model", request.AgentID)
	}

	credentials, _ := server.GetProviderCredentials(ctx, provider)
	skillsCtx, _ := server.GetSkillsContext(ctx, request.AgentID)

	var llmClient engine.LLMClient
	if provider == "claude" || strings.Contains(credentials.BaseURL, "anthropic") {
		llmClient = engine.NewAnthropicClient(engine.AnthropicClientConfig{
			APIKey:    credentials.APIKey,
			BaseURL:   credentials.BaseURL,
			MaxTokens: 8192,
		})
	} else {
		llmClient = engine.NewOpenAICompatibleClient(engine.OpenAICompatibleClientConfig{
			APIKey:    credentials.APIKey,
			BaseURL:   credentials.BaseURL,
			MaxTokens: 8192,
		})
	}

	if skillsCtx == nil {
		skillsCtx = &serverclient.SkillContext{}
	}

	registry := engine.NewToolRegistry()
	registry.RegisterBuiltin("send_channel_message", "向当前渠道发送消息。content 填要发出去的话。", types.JSONSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"content":               map[string]interface{}{"type": "string", "description": "消息内容"},
			"channel":               map[string]interface{}{"type": "string", "description": "目标渠道"},
			"channelUserId":         map[string]interface{}{"type": "string", "description": "渠道用户 ID"},
			"channelConversationId": map[string]interface{}{"type": "string", "description": "群聊 ID"},
			"messageType":           map[string]interface{}{"type": "string", "description": "消息类型"},
		},
		Required: []string{"content"},
	}, func(c context.Context, input map[string]interface{}) (interface{}, error) {
		channel := request.Channel
		if c, ok := input["channel"].(string); ok && c != "" {
			channel = c
		}
		channelUserID := request.ChannelUserID
		if cu, ok := input["channelUserId"].(string); ok && cu != "" {
			channelUserID = cu
		}
		content, ok := input["content"].(string)
		if !ok || content == "" {
			return nil, fmt.Errorf("content is required")
		}

		channelConvID := request.ChannelConversationID
		if cc, ok := input["channelConversationId"].(string); ok && cc != "" {
			channelConvID = cc
		}

		msg := map[string]interface{}{
			"channel":               channel,
			"channelUserId":         channelUserID,
			"content":               content,
			"channelConversationId": channelConvID,
			"sessionId":             worker.SessionID,
			"traceId":               request.TraceID,
		}
		if mt, ok := input["messageType"].(string); ok && mt != "" {
			msg["messageType"] = mt
		}

		err := server.SendToChannel(c, msg)
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"success":       true,
			"channel":       channel,
			"channelUserId": channelUserID,
		}, nil
	})

	internalHandlers := map[string]types.ToolExecutor{
		"get_skill_doc": func(c context.Context, input map[string]interface{}) (interface{}, error) {
			skillName, ok := input["skill_name"].(string)
			if !ok || skillName == "" {
				return nil, fmt.Errorf("skill_name is required")
			}
			doc, exists := skillsCtx.SkillDocs[skillName]
			if !exists {
				return nil, fmt.Errorf("skill doc not found: %s", skillName)
			}
			return map[string]string{"skill": skillName, "doc": doc}, nil
		},
	}
	registry.RegisterSkills(skillsCtx, internalHandlers)

	systemPrompt := buildSystemPrompt(agentConfig.SystemPrompt, skillsCtx.SystemPromptAddition)

	report(types.AgentEvent{
		Type:      "thinking",
		Timestamp: time.Now().UnixMilli(),
		Thinking:  "Configuration loaded.",
		Source:    "system",
	})

	var historyMessages []types.AgentMessage

	// Load directly from Session Context (JSON) if it exists, otherwise fallback to empty/legacy.
	sessionData, err := server.GetSession(ctx, worker.SessionID)
	if err == nil && sessionData != nil && sessionData.Context != "" {
		_ = json.Unmarshal([]byte(sessionData.Context), &historyMessages)
	}

	// Filter out the current user message if it accidentally got injected
	// (usually handled by the dispatcher adding to messages table, but context is pure)
	// We'll trust the context is pure and doesn't contain the current message yet.
	// We no longer need: ensureAlternation, removeOrphanedToolUses, removeOrphanedToolResults
	
	// Enforce context length (truncate by full turns)
	historyMessages = truncateByFullTurns(historyMessages, 10) // e.g. keep last 10 turns

	var meta []string
	meta = append(meta, "channel="+request.Channel)
	meta = append(meta, "channelUserId="+request.ChannelUserID)
	if request.ChannelConversationID != "" {
		meta = append(meta, "channelConversationId="+request.ChannelConversationID)
	}
	if request.SenderName != "" {
		meta = append(meta, "senderName="+request.SenderName)
	}

	sender := request.SenderName
	if sender == "" {
		sender = request.ChannelUserID
	}

	currentUserContent := fmt.Sprintf("[消息来源] %s\n[%s]\n%s", strings.Join(meta, " "), sender, request.Content)

	messages := append(historyMessages, types.AgentMessage{
		Role:    "user",
		Content: []types.ContentBlock{{Type: "text", Text: currentUserContent}},
	})

	report(types.AgentEvent{
		Type:      "thinking",
		Timestamp: time.Now().UnixMilli(),
		Thinking:  fmt.Sprintf("Ready, history %d messages, starting ReAct loop...", len(historyMessages)),
		Source:    "system",
	})

	loopResult, err := engine.RunAgentLoop(ctx, engine.AgentLoopConfig{
		LLMClient:     llmClient,
		SystemPrompt:  systemPrompt,
		Messages:      messages,
		Tools:         registry.GetAll(),
		Model:         modelName,
		MaxIterations: 25,
		OnEvent: func(event types.AgentEvent) {
			report(event)
		},
		OnNewMessages: func(iterMessages []types.AgentMessage) error {
			persistable := toPersistableMessages(iterMessages)
			for _, msg := range persistable {
				initiator := ""
				if msg.Role == "assistant" {
					initiator = "agent"
				}
				_, _ = server.SaveMessage(ctx, map[string]interface{}{
					"sessionId":   worker.SessionID,
					"role":        msg.Role,
					"content":     msg.Content,
					"messageType": msg.MessageType,
					"channel":     request.Channel,
					"traceId":     request.TraceID,
					"initiator":   initiator,
				})
			}
			return nil
		},
	})

	if err != nil {
		report(types.AgentEvent{Type: "error", Timestamp: time.Now().UnixMilli(), Error: err.Error()})
		report(types.AgentEvent{Type: "done", Timestamp: time.Now().UnixMilli(), Error: err.Error()})
		return err
	}

	// At the end of the loop, serialize and save the FULL original memory back to session_context
	if loopResult != nil && len(loopResult.Messages) > 0 {
		contextBytes, err := json.Marshal(loopResult.Messages)
		if err == nil {
			_ = server.UpdateSession(ctx, worker.SessionID, map[string]interface{}{
				"context": string(contextBytes),
			})
		}
	}

	return nil
}
