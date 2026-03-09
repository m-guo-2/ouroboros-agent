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
	"agent/internal/sanitize"
	"agent/internal/storage"
	"agent/internal/subagent"
	"agent/internal/types"
)

// BuildSystemPrompt performs {{skills}} template expansion on the agent's
// system prompt. The prompt stored in the database is the single source of
// truth — no hidden builtin segments are appended.
func BuildSystemPrompt(agentSystemPrompt, skillsSnippet string) string {
	result := agentSystemPrompt + "\n\n" + skillsSnippet
	// if strings.Contains(result, "{{skills}}") {
	// 	result = strings.ReplaceAll(result, "{{skills}}", skillsSnippet)
	// }
	return result
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
				Content:     sanitize.RedactSecrets(string(b)),
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
				Content:     sanitize.RedactSecrets(string(b)),
				MessageType: "structured",
			})
		}
	}
	return result
}

func redactMessagesForStorage(messages []types.AgentMessage) []types.AgentMessage {
	redacted := make([]types.AgentMessage, len(messages))
	for i, msg := range messages {
		redacted[i] = types.AgentMessage{
			Role:    msg.Role,
			Content: make([]types.ContentBlock, len(msg.Content)),
		}
		for j, block := range msg.Content {
			redactedBlock := block
			if redactedBlock.Text != "" {
				redactedBlock.Text = sanitize.RedactSecrets(redactedBlock.Text)
			}
			if redactedBlock.Content != "" {
				redactedBlock.Content = sanitize.RedactSecrets(redactedBlock.Content)
			}
			if redactedBlock.Input != nil {
				inputJSON, err := json.Marshal(redactedBlock.Input)
				if err == nil {
					redactedJSON := sanitize.RedactSecrets(string(inputJSON))
					var inputMap map[string]interface{}
					if err := json.Unmarshal([]byte(redactedJSON), &inputMap); err == nil {
						redactedBlock.Input = inputMap
					}
				}
			}
			redacted[i].Content[j] = redactedBlock
		}
	}
	return redacted
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

func formatUserMessage(senderName, channel, messageType, channelMessageID, content string) string {
	if senderName != "" && !strings.HasPrefix(content, senderName+": ") {
		content = senderName + ": " + content
	}

	var meta []string
	if channel != "" {
		meta = append(meta, fmt.Sprintf("via %s", channel))
	}
	if channelMessageID != "" {
		meta = append(meta, fmt.Sprintf("msg_id=%s", channelMessageID))
	}
	if messageType != "" && messageType != "text" {
		meta = append(meta, fmt.Sprintf("type=%s", messageType))
	}
	if len(meta) == 0 {
		return content
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
			text := formatUserMessage(msg.SenderName, msg.Channel, msg.MessageType, msg.ChannelMessageID, msg.Content)
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
				"【subagent完成通知】\nsubagent=%s\njobId=%s\nstatus=%s\nerror=%s\n\n已产生影响：\n%s\n\n请基于失败原因决定是否重试、降级或改用其他路径继续任务。",
				job.Profile, job.ID, job.Status, strings.TrimSpace(job.Error), formatImpacts(job.Impacts),
			)
		} else {
			content = fmt.Sprintf(
				"【subagent完成通知】\nsubagent=%s\njobId=%s\nstatus=%s\n\n总结：\n%s\n\n已产生影响：\n%s\n\n请基于该结果继续主任务。",
				job.Profile, job.ID, job.Status, strings.TrimSpace(job.Result), formatImpacts(job.Impacts),
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
			"task":            map[string]interface{}{"type": "string", "description": "要交给 subagent 的任务描述"},
			"subagent":        map[string]interface{}{"type": "string", "description": "子代理类型，支持 developer / file_analysis / web_research，默认 developer"},
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
			OnCanceled: func(j *subagent.Job) {
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
			"impacts":       job.Impacts,
			"impactSummary": formatImpacts(job.Impacts),
			"detailDir":     job.DetailDir,
			"subTraceId":    job.SubTraceID,
			"parentTraceId": job.ParentTraceID,
		}, nil
	})

	registry.RegisterBuiltin("cancel_subagent", "取消一个正在运行的 subagent 任务，并返回截至当前已产生的实际影响。", types.JSONSchema{
		Type: "object",
		Properties: map[string]interface{}{
			"job_id": map[string]interface{}{"type": "string", "description": "subagent jobId"},
			"reason": map[string]interface{}{"type": "string", "description": "取消原因（可选）"},
		},
		Required: []string{"job_id"},
	}, func(c context.Context, input map[string]interface{}) (interface{}, error) {
		jobID, _ := input["job_id"].(string)
		jobID = strings.TrimSpace(jobID)
		if jobID == "" {
			return nil, fmt.Errorf("job_id is required")
		}
		reason, _ := input["reason"].(string)
		job, err := manager.Cancel(jobID, reason)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"jobId":         job.ID,
			"subagent":      job.Profile,
			"status":        job.Status,
			"error":         job.Error,
			"impacts":       job.Impacts,
			"impactSummary": formatImpacts(job.Impacts),
		}, nil
	})
}

func formatImpacts(impacts []subagent.Impact) string {
	if len(impacts) == 0 {
		return "- 暂无可记录影响"
	}
	var lines []string
	for i, imp := range impacts {
		if i >= 10 {
			lines = append(lines, fmt.Sprintf("- ... 其余 %d 条影响省略", len(impacts)-10))
			break
		}
		lines = append(lines, "- "+strings.TrimSpace(imp.Summary))
	}
	return strings.Join(lines, "\n")
}

func resolveCompactModel(mainModel string) string {
	cheapModels := map[string]string{
		"claude-opus-4-5":            "claude-3-5-haiku-20241022",
		"claude-sonnet-4-5":          "claude-3-5-haiku-20241022",
		"claude-3-5-sonnet-20241022": "claude-3-5-haiku-20241022",
		"claude-3-5-haiku-20241022":  "claude-3-5-haiku-20241022",
		"claude-3-haiku-20240307":    "claude-3-haiku-20240307",
		"gpt-4o":                     "gpt-4o-mini",
		"gpt-4-turbo":                "gpt-4o-mini",
		"gpt-4o-mini":                "gpt-4o-mini",
	}
	if m, ok := cheapModels[mainModel]; ok {
		return m
	}
	if strings.Contains(mainModel, "claude") {
		return "claude-3-5-haiku-20241022"
	}
	return mainModel
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

	skillsCtx, err := storage.GetSkillsContext(request.AgentID, agentConfig.Skills)
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
	ostools.RegisterRecallContext(registry, worker.SessionID)
	engine.RegisterTavilyTool(registry)

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
		"load_skill": func(c context.Context, input map[string]interface{}) (interface{}, error) {
			skillID, ok := input["skill_id"].(string)
			if !ok || skillID == "" {
				return nil, fmt.Errorf("skill_id is required")
			}
			detail, err := storage.GetSkillDetail(skillID)
			if err != nil {
				available := make([]string, 0, len(skillsCtx.SkillDocs))
				for id := range skillsCtx.SkillDocs {
					available = append(available, id)
				}
				return nil, fmt.Errorf("%s. available skills: %s", err.Error(), strings.Join(available, ", "))
			}
			return detail, nil
		},
	}
	registry.RegisterSkills(skillsCtx, internalHandlers)

	systemPrompt := BuildSystemPrompt(agentConfig.SystemPrompt, skillsCtx.SkillsSnippet)

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

	currentUserContent := formatUserMessage(request.SenderName, request.Channel, request.MessageType, request.ChannelMessageID, request.Content)

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

	onNewMessages := func(iterMessages []types.AgentMessage) error {
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
	}

	tools := registry.GetAll()

	for absorbRound := 0; ; absorbRound++ {
		// === Execute ===
		loopResult, err := engine.RunAgentLoop(ctx, engine.AgentLoopConfig{
			LLMClient:     llmClient,
			SystemPrompt:  systemPrompt,
			Messages:      messages,
			Tools:         tools,
			Model:         modelName,
			MaxIterations: 25,
			OnNewMessages: onNewMessages,
		})
		if err != nil {
			logger.Error(ctx, "引擎错误", "traceEvent", "error", "error", err.Error())
			return err
		}

		messages = loopResult.Messages

		if loopResult.FinalText != "" {
			messages = append(messages, types.AgentMessage{
				Role:    "assistant",
				Content: []types.ContentBlock{{Type: "text", Text: loopResult.FinalText}},
			})
		}

		if cwd := shellSession.CWD(); cwd != "" && cwd != worker.WorkDir {
			worker.WorkDir = cwd
			logger.Detail(ctx, "工作目录已更新", "cwd", cwd)
		}

		// === Checkpoint ===
		if len(messages) > 0 {
			estimate := EstimateTokens(messages, modelName)
			logger.Detail(ctx, "上下文 token 估算",
				"tokens", estimate.Tokens, "contextWindow", estimate.ContextWindow,
				"ratio", fmt.Sprintf("%.2f", estimate.Ratio), "method", estimate.Method)

			if ShouldCompact(estimate) {
				compactModel := resolveCompactModel(modelName)
				var compactLLM engine.LLMClient
				if provider == "claude" || strings.Contains(credentials.BaseURL, "anthropic") {
					compactLLM = engine.NewAnthropicClient(engine.AnthropicClientConfig{
						APIKey:    credentials.APIKey,
						BaseURL:   credentials.BaseURL,
						MaxTokens: 2048,
					})
				} else {
					compactLLM = engine.NewOpenAICompatibleClient(engine.OpenAICompatibleClientConfig{
						APIKey:    credentials.APIKey,
						BaseURL:   credentials.BaseURL,
						MaxTokens: 2048,
					})
				}

				result, err := CompactContext(ctx, messages, modelName, compactLLM, compactModel, worker.SessionID)
				if err != nil {
					logger.Warn(ctx, "上下文压缩失败，使用原始消息",
						"error", err.Error(), "sessionId", worker.SessionID)
				} else if result.Compacted {
					messages = result.Messages
					logger.Business(ctx, "上下文压缩",
						"traceEvent", "compact",
						"tokensBefore", result.TokensBefore, "tokensAfter", result.TokensAfter,
						"archivedCount", result.ArchivedCount)
				}
			}

			redactedMessages := redactMessagesForStorage(messages)
			contextBytes, err := json.Marshal(redactedMessages)
			if err == nil {
				_ = storage.UpdateSession(worker.SessionID, map[string]interface{}{
					"workDir": worker.WorkDir,
					"context": string(contextBytes),
				})
			}
		}

		// === Absorb-or-Exit ===
		if absorbRound >= MaxAbsorbRounds {
			logger.Warn(ctx, "达到最大吸纳轮次，退出吸纳循环",
				"maxAbsorbRounds", MaxAbsorbRounds, "sessionId", worker.SessionID)
			break
		}

		pending := popAllPending(worker)
		if len(pending) == 0 {
			break
		}

		var parts []string
		parts = append(parts, fmt.Sprintf("[以下 %d 条消息在处理期间到达]", len(pending)))
		for _, p := range pending {
			parts = append(parts, formatUserMessage(p.SenderName, p.Channel, p.MessageType, p.ChannelMessageID, p.Content))
		}
		merged := strings.Join(parts, "\n\n")
		messages = append(messages, types.AgentMessage{
			Role:    "user",
			Content: []types.ContentBlock{{Type: "text", Text: merged}},
		})

		logger.Business(ctx, "消息吸纳",
			"traceEvent", "absorb",
			"absorbRound", absorbRound+1, "absorbedCount", len(pending))
	}

	return nil
}
