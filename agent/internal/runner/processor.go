package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"agent/internal/channels"
	"agent/internal/engine"
	"agent/internal/engine/ostools"
	"agent/internal/logger"
	"agent/internal/subagent"
	"agent/internal/storage"
	"agent/internal/types"
)

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
- ` + "`" + `replyToChannelMessageId` + "`" + `（可选）：要回复的上游消息 ID。不传则不带 reply_to，由你自行决定是否回复某条消息

**使用原则：**
- 需要回复用户时，调用此工具发送，不要依赖直接文字输出。
- 可以分多次调用，不必等所有工作完成后才回复。
- 当用户的问题需要较长处理时间时，先发一条确认消息，再继续处理。

**关于历史消息的理解：**
历史消息只包含客观事件记录，不包含你过去的推理过程：
- ` + "`" + `user` + "`" + ` 消息 = 某个用户发来的内容。消息头格式：[` + "`" + `昵称 (渠道ID) | via 渠道 | type=消息类型` + "`" + `]。其中 via 表示来源渠道（feishu/wecom/webui），type 仅在非文本消息时出现（image/file/audio 等）
- ` + "`" + `assistant` + "`" + ` 消息（tool_use block）= 你过去执行的工具调用动作
- ` + "`" + `tool_result` + "`" + ` block = 工具执行返回的客观结果
- 你过去通过 ` + "`" + `send_channel_message` + "`" + ` 发送的内容会出现在对应的 tool_use 和 tool_result 中`

	parts = append(parts, builtin)

	if skillsAddition != "" {
		parts = append(parts, skillsAddition)
	}

	return strings.Join(parts, "\n\n")
}

func toPersistableMessages(loopMessages []types.AgentMessage) []storage.MessageData {
	var result []storage.MessageData
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
			result = append(result, storage.MessageData{
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
			result = append(result, storage.MessageData{
				Role:        "tool_result",
				Content:     string(b),
				MessageType: "structured",
			})
		}
	}
	return result
}

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

func formatUserMessage(senderName, channelUserID, channel, messageType, channelMessageID, content string) string {
	var sender string
	if senderName != "" {
		sender = fmt.Sprintf("%s (%s)", senderName, channelUserID)
	} else {
		sender = channelUserID
	}

	var meta []string
	meta = append(meta, sender)
	if channel != "" {
		meta = append(meta, fmt.Sprintf("via %s", channel))
	}
	if channelMessageID != "" {
		meta = append(meta, fmt.Sprintf("msg_id=%s", channelMessageID))
	}
	if messageType != "" && messageType != "text" {
		meta = append(meta, fmt.Sprintf("type=%s", messageType))
	}
	return fmt.Sprintf("[%s]\n%s", strings.Join(meta, " | "), content)
}

func reconstructHistoryFromMessages(sessionID string) []types.AgentMessage {
	dbMessages, err := storage.GetSessionMessages(sessionID, 100)
	if err != nil || len(dbMessages) == 0 {
		return nil
	}

	var result []types.AgentMessage
	for _, msg := range dbMessages {
		switch msg.Role {
		case "user":
			if msg.MessageType == "structured" {
				continue
			}
			text := formatUserMessage(msg.SenderName, msg.SenderID, msg.Channel, msg.MessageType, msg.ChannelMessageID, msg.Content)
			result = append(result, types.AgentMessage{
				Role:    "user",
				Content: []types.ContentBlock{{Type: "text", Text: text}},
			})
		case "assistant":
			if msg.MessageType == "structured" {
				var blocks []types.ContentBlock
				if err := json.Unmarshal([]byte(msg.Content), &blocks); err == nil {
					result = append(result, types.AgentMessage{
						Role:    "assistant",
						Content: blocks,
					})
				}
			}
		case "tool_result":
			if msg.MessageType == "structured" {
				var blocks []types.ContentBlock
				if err := json.Unmarshal([]byte(msg.Content), &blocks); err == nil {
					result = append(result, types.AgentMessage{
						Role:    "user",
						Content: blocks,
					})
				}
			}
		}
	}

	if len(result) > 0 {
		last := result[len(result)-1]
		if last.Role == "user" && len(last.Content) > 0 && last.Content[0].Type == "text" {
			result = result[:len(result)-1]
		}
	}

	return result
}

func registerSubagentTools(
	registry *engine.ToolRegistry,
	llmClient engine.LLMClient,
	modelName string,
	agentID string,
	userID string,
	channel string,
	channelUserID string,
	channelConversationID string,
	traceID string,
	sessionID string,
	messages []types.AgentMessage,
) {
	manager := subagent.DefaultManager()

	notifyMain := func(job *subagent.Job, failed bool) {
		if job == nil {
			return
		}

		var content string
		if failed {
			content = fmt.Sprintf(
				"【subagent完成通知】\nsubagent=%s\njobId=%s\nstatus=%s\nerror=%s\n\n请基于失败原因决定是否重试、降级或改用其他路径继续任务。",
				job.Profile, job.ID, job.Status, strings.TrimSpace(job.Error),
			)
		} else {
			content = fmt.Sprintf(
				"【subagent完成通知】\nsubagent=%s\njobId=%s\nstatus=%s\n\n总结：\n%s\n\n请基于该结果继续主任务。",
				job.Profile, job.ID, job.Status, strings.TrimSpace(job.Result),
			)
		}

		err := EnqueueProcessRequest(context.Background(), ProcessRequest{
			UserID:                userID,
			AgentID:               agentID,
			Content:               content,
			Channel:               channel,
			ChannelUserID:         channelUserID,
			ChannelConversationID: channelConversationID,
			MessageType:           "text",
			MessageID:             fmt.Sprintf("subagent-event-%d", time.Now().UnixNano()),
			SessionID:             sessionID,
		})
		if err != nil {
			logger.Warn(context.Background(), "subagent 完成事件入队失败",
				"jobId", job.ID, "error", err.Error())
		}
	}

	registry.RegisterBuiltin("run_subagent_async", "异步启动一个 subagent 子任务，立即返回 jobId。子任务完成后可用 get_subagent_status 查询自然语言总结。", types.JSONSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"task": map[string]interface{}{"type": "string", "description": "要交给 subagent 的任务描述"},
			"subagent": map[string]interface{}{"type": "string", "description": "子代理类型，支持 developer / file_analysis，默认 developer"},
			"timeout_seconds": map[string]interface{}{"type": "integer", "description": "超时秒数（可选，默认 600）"},
		},
		Required: []string{"task"},
	}, func(c context.Context, input map[string]interface{}) (interface{}, error) {
		task, _ := input["task"].(string)
		task = strings.TrimSpace(task)
		if task == "" {
			return nil, fmt.Errorf("task is required")
		}

		profile, _ := input["subagent"].(string)
		timeout := 10 * time.Minute
		if t, ok := input["timeout_seconds"].(float64); ok && t > 0 {
			timeout = time.Duration(int(t)) * time.Second
		}

		job, err := manager.Start(subagent.StartRequest{
			Profile:       profile,
			Task:          task,
			Model:         modelName,
			LLMClient:     llmClient,
			Messages:      messages,
			Tools:         registry.GetAll(),
			ParentTraceID: traceID,
			SessionID:     sessionID,
			Timeout:       timeout,
			OnCompleted: func(j *subagent.Job) {
				notifyMain(j, false)
			},
			OnFailed: func(j *subagent.Job) {
				notifyMain(j, true)
			},
		})
		if err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"jobId":     job.ID,
			"status":    job.Status,
			"name":      job.Name,
			"subagent":  job.Profile,
			"detailDir": job.DetailDir,
			"message":   "subagent 已启动，请稍后用 get_subagent_status 查询结果",
		}, nil
	})

	registry.RegisterBuiltin("get_subagent_status", "查询 subagent 异步任务状态。完成时返回自然语言总结 result。", types.JSONSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"job_id": map[string]interface{}{"type": "string", "description": "subagent jobId"},
		},
		Required: []string{"job_id"},
	}, func(c context.Context, input map[string]interface{}) (interface{}, error) {
		jobID, _ := input["job_id"].(string)
		jobID = strings.TrimSpace(jobID)
		if jobID == "" {
			return nil, fmt.Errorf("job_id is required")
		}
		job, ok := manager.Get(jobID)
		if !ok {
			return nil, fmt.Errorf("subagent job not found: %s", jobID)
		}
		return map[string]interface{}{
			"jobId":         job.ID,
			"name":          job.Name,
			"subagent":      job.Profile,
			"status":        job.Status,
			"result":        job.Result,
			"error":         job.Error,
			"detailDir":     job.DetailDir,
			"subTraceId":    job.SubTraceID,
			"parentTraceId": job.ParentTraceID,
		}, nil
	})
}

func processOneEvent(ctx context.Context, worker *SessionWorker, request QueuedRequest) error {
	logger.Business(ctx, "开始处理事件",
		"agentId", request.AgentID, "channel", request.Channel, "userId", request.UserID)
	logger.Business(ctx, "加载配置中", "traceEvent", "thinking", "source", "system")

	agentConfig, err := storage.GetAgentConfig(request.AgentID)
	if err != nil || agentConfig == nil {
		logger.Error(ctx, "Agent 配置未找到",
			"agentId", request.AgentID, "error", fmt.Sprint(err))
		return fmt.Errorf("agent not found: %s", request.AgentID)
	}

	provider := agentConfig.Provider
	modelName := agentConfig.Model
	if provider == "" || modelName == "" {
		logger.Error(ctx, "Agent 缺少 provider/model",
			"provider", provider, "model", modelName)
		return fmt.Errorf("agent %s missing provider/model", request.AgentID)
	}

	logger.Detail(ctx, "Agent 配置已加载", "provider", provider, "model", modelName)

	credentials, _ := storage.GetProviderCredentials(provider)
	if credentials == nil {
		credentials = &storage.ProviderCredentials{}
	}

	skillsCtx, err := storage.GetSkillsContext(request.AgentID)
	if err != nil || skillsCtx == nil {
		skillsCtx = &storage.SkillContext{
			Tools:         []types.ToolDefinition{},
			ToolExecutors: map[string]storage.SkillToolExecutor{},
			SkillDocs:     map[string]string{},
		}
	}

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

	shellSession := ostools.NewShellSession(worker.WorkDir)

	registry := engine.NewToolRegistry()
	ostools.RegisterOSTools(registry, shellSession)

	registry.RegisterBuiltin("send_channel_message", "向当前渠道发送消息。content 填要发出去的话。", types.JSONSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"content":                 map[string]interface{}{"type": "string", "description": "消息内容"},
			"channel":                 map[string]interface{}{"type": "string", "description": "目标渠道"},
			"channelUserId":           map[string]interface{}{"type": "string", "description": "渠道用户 ID"},
			"channelConversationId":   map[string]interface{}{"type": "string", "description": "群聊 ID"},
			"messageType":             map[string]interface{}{"type": "string", "description": "消息类型"},
			"replyToChannelMessageId": map[string]interface{}{"type": "string", "description": "回复目标的上游消息 ID（可选）"},
		},
		Required: []string{"content"},
	}, func(c context.Context, input map[string]interface{}) (interface{}, error) {
		channel := request.Channel
		if ch, ok := input["channel"].(string); ok && ch != "" {
			channel = ch
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

		outMsg := channels.OutgoingMessage{
			Channel:               channel,
			ChannelUserID:         channelUserID,
			Content:               content,
			ChannelConversationID: channelConvID,
			SessionID:             worker.SessionID,
			TraceID:               request.TraceID,
		}
		if mt, ok := input["messageType"].(string); ok && mt != "" {
			outMsg.MessageType = mt
		}
		if replyTo, ok := input["replyToChannelMessageId"].(string); ok && replyTo != "" {
			outMsg.ReplyToChannelMessageID = replyTo
		}

		if err := channels.SendToChannel(outMsg); err != nil {
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

	var historyMessages []types.AgentMessage
	var histSource string

	sessionData, err := storage.GetSession(worker.SessionID)
	if err == nil && sessionData != nil && sessionData.Context != "" {
		_ = json.Unmarshal([]byte(sessionData.Context), &historyMessages)
		histSource = "session"
	}

	if len(historyMessages) == 0 {
		historyMessages = reconstructHistoryFromMessages(worker.SessionID)
		if len(historyMessages) > 0 {
			histSource = "db_reconstruct"
		} else {
			histSource = "empty"
		}
	}

	historyMessages = truncateByFullTurns(historyMessages, 10)

	currentUserContent := formatUserMessage(request.SenderName, request.ChannelUserID, request.Channel, request.MessageType, request.ChannelMessageID, request.Content)

	messages := append(historyMessages, types.AgentMessage{
		Role:    "user",
		Content: []types.ContentBlock{{Type: "text", Text: currentUserContent}},
	})

	registerSubagentTools(
		registry,
		llmClient,
		modelName,
		request.AgentID,
		request.UserID,
		request.Channel,
		request.ChannelUserID,
		request.ChannelConversationID,
		request.TraceID,
		worker.SessionID,
		messages,
	)

	logger.Detail(ctx, "历史消息加载诊断",
		"source", histSource, "historyCount", len(historyMessages), "totalMessages", len(messages),
		"diagnostic", engine.BuildToolUseDiagnostic(messages))

	loopResult, err := engine.RunAgentLoop(ctx, engine.AgentLoopConfig{
		LLMClient:     llmClient,
		SystemPrompt:  systemPrompt,
		Messages:      messages,
		Tools:         registry.GetAll(),
		Model:         modelName,
		MaxIterations: 25,
		OnNewMessages: func(iterMessages []types.AgentMessage) error {
			persistable := toPersistableMessages(iterMessages)
			for _, msg := range persistable {
				initiator := ""
				if msg.Role == "assistant" {
					initiator = "agent"
				}
				_, _ = storage.SaveMessage(map[string]interface{}{
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
		logger.Error(ctx, "引擎错误", "traceEvent", "error", "error", err.Error())
		return err
	}

	if cwd := shellSession.CWD(); cwd != "" && cwd != worker.WorkDir {
		worker.WorkDir = cwd
		logger.Detail(ctx, "工作目录已更新", "cwd", cwd)
	}

	updateData := map[string]interface{}{
		"workDir": worker.WorkDir,
	}
	if loopResult != nil && len(loopResult.Messages) > 0 {
		contextBytes, err := json.Marshal(loopResult.Messages)
		if err == nil {
			updateData["context"] = string(contextBytes)
		}
	}
	_ = storage.UpdateSession(worker.SessionID, updateData)

	return nil
}
