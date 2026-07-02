// Package dispatcher receives normalised incoming messages from channel adapters
// (Feishu, QiWei, WebUI) and routes them to the runner — replacing channel-dispatcher.ts.
package dispatcher

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agent/internal/logger"
	"agent/internal/runner"
	"agent/internal/storage"
)

// IncomingMessage is the normalised payload sent by a channel adapter.
type IncomingMessage struct {
	Channel                 string                   `json:"channel"`
	ChannelAccountID        string                   `json:"channelAccountId,omitempty"`
	ChannelAccountShortHash string                   `json:"channelAccountShortHash,omitempty"`
	ChannelUserID           string                   `json:"channelUserId"`
	ChannelMessageID        string                   `json:"channelMessageId"`
	ChannelConversationID   string                   `json:"channelConversationId,omitempty"`
	ChannelConversationName string                   `json:"channelConversationName,omitempty"`
	ConversationType        string                   `json:"conversationType,omitempty"`
	SenderName              string                   `json:"senderName,omitempty"`
	Content                 string                   `json:"content"`
	MessageType             string                   `json:"messageType,omitempty"`
	AgentID                 string                   `json:"agentId,omitempty"`
	Timestamp               int64                    `json:"timestamp,omitempty"`
	Attachments             []storage.AttachmentData `json:"attachments,omitempty"`
	ChannelMeta             map[string]any           `json:"channelMeta,omitempty"`
}

// DispatchResult is returned synchronously to the caller.
type DispatchResult struct {
	Success   bool   `json:"success"`
	Duplicate bool   `json:"duplicate,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	UserID    string `json:"userId,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Dispatch processes a normalised incoming message end-to-end:
//  1. Dedup
//  2. User resolution
//  3. Agent config lookup
//  4. Session get-or-create
//  5. Save user message
//  6. Enqueue to runner (direct function call, no HTTP)
func Dispatch(ctx context.Context, msg IncomingMessage) DispatchResult {
	// 1. Deduplication.
	dedupeKey := resolveDedupeKey(msg)
	if dedupeKey == "" {
		dedupeKey = msg.ChannelMessageID
	}
	if msg.AgentID != "" {
		dedupeKey += ":" + msg.AgentID
	}
	if storage.IsProcessed(dedupeKey) {
		logger.Detail(ctx, "重复消息跳过", "dedupeKey", dedupeKey)
		return DispatchResult{Success: true, Duplicate: true}
	}
	_ = storage.MarkProcessed(dedupeKey, msg.Channel)

	// 2. Resolve (or create) shadow user.
	userKey := resolveChannelUserKey(msg)
	userID, isNew, err := storage.ResolveUser(msg.Channel, userKey, msg.SenderName)
	if err != nil {
		logger.Error(ctx, "用户解析失败", "error", err.Error())
		return DispatchResult{Success: false, Error: "user resolution failed"}
	}
	if isNew {
		logger.Business(ctx, "新用户创建", "userId", userID, "channel", msg.Channel)
	}

	// 3. Locate target agent.
	agentCfg, err := resolveTargetAgent(msg)
	if err != nil || agentCfg == nil {
		logger.Error(ctx, "Agent 配置未找到", "agentId", msg.AgentID, "error", fmt.Sprint(err))
		return DispatchResult{Success: false, Error: "no agent available"}
	}

	// 4. Session management.
	sessionKey := resolveSessionKey(msg)
	session, err := storage.FindSessionByKey(agentCfg.ID, sessionKey)
	if err != nil {
		logger.Error(ctx, "查询 session 失败", "error", err.Error())
	}

	// Legacy fallback: match by channelConversationId when session_key was absent.
	// Older Qiwei sessions used a raw room/user id before multi-account suffixes
	// were introduced, so also try the raw part of "{rawId}@{shortHash}".
	if session == nil && msg.ChannelConversationID != "" {
		session, _ = storage.FindSessionByConversationID(msg.ChannelConversationID, agentCfg.ID)
		if session == nil {
			rawID, shortHash := splitConversationShortHash(msg.ChannelConversationID)
			if shortHash != "" && rawID != "" {
				session, _ = storage.FindSessionByConversationID(rawID, agentCfg.ID)
			}
		}
		if session != nil {
			// Back-fill stale/missing fields on the legacy session.
			patch := map[string]string{}
			if session.SessionKey != sessionKey {
				patch["sessionKey"] = sessionKey
			}
			if session.ChannelConversationID != msg.ChannelConversationID {
				patch["channelConversationId"] = msg.ChannelConversationID
			}
			if len(patch) > 0 {
				_ = storage.PatchSession(session.ID, patch)
				session.SessionKey = sessionKey
			}
		}
	}

	isNewSession := false
	if session == nil {
		// Create new session.
		title := msg.Content
		if title == "" && len(msg.Attachments) > 0 {
			title = "[" + msg.Attachments[0].Kind + "]"
		}
		if len(title) > 30 {
			title = title[:30] + "..."
		}
		session, err = storage.CreateSession(map[string]interface{}{
			"id":                    newID(),
			"title":                 title,
			"userId":                userID,
			"agentId":               agentCfg.ID,
			"channel":               msg.Channel,
			"sessionKey":            sessionKey,
			"channelConversationId": msg.ChannelConversationID,
			"channelName":           msg.ChannelConversationName,
		})
		if err != nil || session == nil {
			logger.Error(ctx, "创建 session 失败", "error", fmt.Sprint(err))
			return DispatchResult{Success: false, Error: "session creation failed"}
		}
		logger.Business(ctx, "新 Session 创建",
			"sessionId", session.ID, "agentId", agentCfg.ID)
		isNewSession = true
	} else {
		// Patch any new metadata onto existing session.
		patch := map[string]string{}
		if msg.ChannelConversationID != "" && session.ChannelConversationID == "" {
			patch["channelConversationId"] = msg.ChannelConversationID
		}
		if msg.ChannelConversationName != "" && session.ChannelName == "" {
			patch["channelName"] = msg.ChannelConversationName
		}
		if session.SessionKey == "" {
			patch["sessionKey"] = sessionKey
		}
		if len(patch) > 0 {
			_ = storage.PatchSession(session.ID, patch)
		}
	}

	if isGroupClearCommand(msg) {
		runner.ResetSessionRuntime(session.ID)
		count, countErr := storage.CountSessionMessages(session.ID)
		if countErr != nil {
			logger.Error(ctx, "查询清理前消息数失败", "sessionId", session.ID, "error", countErr.Error())
			return DispatchResult{Success: false, SessionID: session.ID, UserID: userID, Error: "clear count failed"}
		}
		if err := storage.ClearSessionHistory(session.ID); err != nil {
			logger.Error(ctx, "群历史清理失败", "sessionId", session.ID, "error", err.Error())
			return DispatchResult{Success: false, SessionID: session.ID, UserID: userID, Error: "clear history failed"}
		}
		logger.Business(ctx, "GM 清理群历史",
			"sessionId", session.ID,
			"sessionKey", session.SessionKey,
			"messageCount", count)
		return DispatchResult{Success: true, SessionID: session.ID, UserID: userID}
	}

	firstInSession := false
	if msg.ChannelUserID != "" {
		seenParticipant, pErr := storage.HasSessionParticipant(session.ID, msg.ChannelUserID)
		if pErr != nil {
			logger.Warn(ctx, "查询 session participant 失败", "sessionId", session.ID, "channelUserId", msg.ChannelUserID, "error", pErr.Error())
		}
		firstInSession = !seenParticipant
	}
	relation, seenErr := storage.RecordAgentUserSeen(agentCfg.ID, userID)
	if seenErr != nil {
		logger.Warn(ctx, "记录 agent 用户见过状态失败", "agentId", agentCfg.ID, "userId", userID, "error", seenErr.Error())
		relation = "unknown"
	}
	if firstInSession {
		if msg.ChannelMeta == nil {
			msg.ChannelMeta = map[string]any{}
		}
		msg.ChannelMeta["participantDiscovery"] = map[string]any{
			"relation": relation,
		}
	}

	// Generate trace ID and carry upstream request ID if present.
	traceID := fmt.Sprintf("trace-%x", randBytes(8))
	dispatchCtx := logger.WithTrace(ctx, traceID, session.ID)
	if upstreamReqID := logger.GetRequestID(ctx); upstreamReqID != "" {
		dispatchCtx = logger.WithRequestID(dispatchCtx, upstreamReqID)
	}

	logger.Business(dispatchCtx, "消息派发",
		"traceEvent", "start",
		"agentId", agentCfg.ID, "userId", userID,
		"channel", msg.Channel, "sessionId", session.ID)
	_ = storage.SaveLifecycleEvent(map[string]any{
		"sessionId":        session.ID,
		"traceId":          traceID,
		"channelMessageId": msg.ChannelMessageID,
		"stage":            "dispatch_accepted",
		"summary":          "消息进入 agent 派发流程",
		"payload": map[string]any{
			"channel":     msg.Channel,
			"messageType": msg.MessageType,
			"senderName":  msg.SenderName,
		},
	})

	// 5. Persist the incoming user message and append to session event log.
	savedMsg, saveErr := storage.SaveMessage(map[string]interface{}{
		"sessionId":        session.ID,
		"role":             "user",
		"content":          msg.Content,
		"messageType":      msg.MessageType,
		"channel":          msg.Channel,
		"channelMessageId": msg.ChannelMessageID,
		"traceId":          traceID,
		"initiator":        "user",
		"senderName":       msg.SenderName,
		"senderId":         msg.ChannelUserID,
		"attachments":      msg.Attachments,
		"channelMeta":      msg.ChannelMeta,
	})
	if saveErr != nil || savedMsg == nil {
		_ = storage.SaveLifecycleEvent(map[string]any{
			"sessionId":        session.ID,
			"traceId":          traceID,
			"channelMessageId": msg.ChannelMessageID,
			"stage":            "message_saved",
			"status":           "failed",
			"summary":          "消息入库失败",
			"payload":          map[string]any{"error": fmt.Sprint(saveErr)},
		})
		return DispatchResult{Success: false, Error: "message save failed"}
	}
	var msgID int64
	msgID = savedMsg.ID
	_ = storage.SaveLifecycleEvent(map[string]any{
		"sessionId":        session.ID,
		"messageId":        msgID,
		"traceId":          traceID,
		"channelMessageId": msg.ChannelMessageID,
		"stage":            "message_saved",
		"summary":          "消息已保存",
		"payload": map[string]any{
			"content":       msg.Content,
			"messageType":   msg.MessageType,
			"attachmentNum": len(msg.Attachments),
		},
	})
	eventSeq, err := storage.AppendSessionEventAndGetSeq(session.ID, msgID)
	if err != nil {
		_ = storage.SaveLifecycleEvent(map[string]any{
			"sessionId":        session.ID,
			"messageId":        msgID,
			"traceId":          traceID,
			"channelMessageId": msg.ChannelMessageID,
			"stage":            "session_event_appended",
			"status":           "failed",
			"summary":          "消息事件追加失败",
			"payload":          map[string]any{"error": err.Error()},
		})
		return DispatchResult{Success: false, Error: "session event append failed"}
	}
	if msg.ChannelUserID != "" {
		inserted, pErr := storage.UpsertSessionParticipant(session.ID, msg.ChannelUserID, userID, msgID, eventSeq)
		if pErr != nil {
			logger.Warn(dispatchCtx, "记录 session participant 失败",
				"sessionId", session.ID, "channelUserId", msg.ChannelUserID, "error", pErr.Error())
		}
		firstInSession = firstInSession && inserted
	}
	if isNewSession {
		runner.DispatchHooksWithPayload(dispatchCtx, agentCfg.Hooks, "session_started", runner.HookPayload{
			SessionID:      session.ID,
			ScopeType:      "session",
			ActivatedAtSeq: eventSeq,
		})
	}
	if firstInSession {
		runner.DispatchHooksWithPayload(dispatchCtx, agentCfg.Hooks, "participant_discovered", runner.HookPayload{
			SessionID:      session.ID,
			ScopeType:      "participant",
			ScopeKey:       msg.ChannelUserID,
			ActivatedAtSeq: eventSeq,
			Relation:       relation,
		})
	}
	_ = storage.SaveLifecycleEvent(map[string]any{
		"sessionId":        session.ID,
		"messageId":        msgID,
		"traceId":          traceID,
		"channelMessageId": msg.ChannelMessageID,
		"stage":            "session_event_appended",
		"summary":          "消息已进入待处理事件流",
	})

	// 6. Update session to processing and enqueue.
	_ = storage.UpdateSession(session.ID, map[string]interface{}{
		"executionStatus": "processing",
	})

	enqueueErr := runner.EnqueueProcessRequest(dispatchCtx, runner.ProcessRequest{
		UserID:                userID,
		AgentID:               agentCfg.ID,
		Content:               msg.Content,
		Channel:               msg.Channel,
		ChannelUserID:         msg.ChannelUserID,
		ChannelConversationID: msg.ChannelConversationID,
		ChannelMessageID:      msg.ChannelMessageID,
		SenderName:            msg.SenderName,
		MessageType:           msg.MessageType,
		Attachments:           msg.Attachments,
		ChannelMeta:           msg.ChannelMeta,
		MessageID:             msgID,
		SessionID:             session.ID,
		TraceID:               traceID,
	})
	if enqueueErr != nil {
		logger.Error(dispatchCtx, "入队失败", "error", enqueueErr.Error())
		_ = storage.SaveLifecycleEvent(map[string]any{
			"sessionId":        session.ID,
			"messageId":        msgID,
			"traceId":          traceID,
			"channelMessageId": msg.ChannelMessageID,
			"stage":            "worker_notified",
			"status":           "failed",
			"summary":          "worker 通知失败",
			"payload":          map[string]any{"error": enqueueErr.Error()},
		})
		_ = storage.UpdateSession(session.ID, map[string]interface{}{
			"executionStatus": "interrupted",
		})
		return DispatchResult{Success: false, Error: enqueueErr.Error()}
	}
	_ = storage.SaveLifecycleEvent(map[string]any{
		"sessionId":        session.ID,
		"messageId":        msgID,
		"traceId":          traceID,
		"channelMessageId": msg.ChannelMessageID,
		"stage":            "worker_notified",
		"summary":          "worker 已收到处理通知",
	})

	return DispatchResult{Success: true, SessionID: session.ID, UserID: userID}
}

// HandleIncoming is an HTTP handler for POST /api/channels/incoming.
func HandleIncoming(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg IncomingMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "invalid JSON",
		})
		return
	}

	if msg.Channel == "" || msg.ChannelUserID == "" || msg.ChannelMessageID == "" || (msg.Content == "" && len(msg.Attachments) == 0) {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "missing required fields: channel, channelUserId, channelMessageId, and one of content or attachments",
		})
		return
	}

	validChannels := map[string]bool{"feishu": true, "qiwei": true, "webui": true}
	if !validChannels[msg.Channel] {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false, "error": "invalid channel",
		})
		return
	}

	if msg.MessageType == "" {
		msg.MessageType = "text"
	}
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().UnixMilli()
	}

	// Respond immediately; dispatch runs in background.
	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"success": true, "message": "Message accepted for processing",
	})

	go func() {
		result := Dispatch(context.Background(), msg)
		if !result.Success && !result.Duplicate {
			logger.Error(context.Background(), "消息派发失败",
				"error", result.Error, "channel", msg.Channel)
		}
	}()
}

// ---------------------------------------------------------------------------

func resolveSessionKey(msg IncomingMessage) string {
	uniqueID := msg.ChannelConversationID
	if uniqueID == "" {
		uniqueID = resolveChannelUserKey(msg)
	}
	return msg.Channel + ":" + uniqueID
}

func resolveTargetAgent(msg IncomingMessage) (*storage.AgentConfig, error) {
	if strings.TrimSpace(msg.AgentID) != "" {
		return storage.GetAgentConfig(strings.TrimSpace(msg.AgentID))
	}

	agents, err := storage.GetActiveAgents()
	if err != nil {
		return nil, err
	}
	if cfg := matchAgentByChannel(agents, msg, false); cfg != nil {
		return cfg, nil
	}
	if cfg := matchAgentByChannel(agents, msg, true); cfg != nil {
		return cfg, nil
	}
	return storage.GetAgentConfig("default-agent-config")
}

func matchAgentByChannel(agents []storage.AgentConfig, msg IncomingMessage, allowWildcard bool) *storage.AgentConfig {
	channel := strings.TrimSpace(msg.Channel)
	if channel == "" {
		return nil
	}
	candidates := channelBindingCandidates(msg)
	for _, candidate := range candidates {
		for i := range agents {
			for _, binding := range agents[i].Channels {
				if strings.TrimSpace(binding.ChannelType) != channel {
					continue
				}
				if strings.TrimSpace(binding.ChannelIdentifier) == candidate {
					return &agents[i]
				}
			}
		}
	}
	if !allowWildcard {
		return nil
	}
	for i := range agents {
		for _, binding := range agents[i].Channels {
			if strings.TrimSpace(binding.ChannelType) != channel {
				continue
			}
			if strings.TrimSpace(binding.ChannelIdentifier) == "*" {
				return &agents[i]
			}
		}
	}
	return nil
}

func channelBindingCandidates(msg IncomingMessage) []string {
	candidates := []string{}
	seen := map[string]bool{}
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			candidates = append(candidates, v)
			seen[v] = true
		}
	}
	if msg.ChannelAccountID != "" {
		add("account:" + msg.ChannelAccountID)
	}
	add(msg.ChannelAccountID)
	if msg.ChannelAccountShortHash != "" {
		add("accountHash:" + msg.ChannelAccountShortHash)
	}
	add(msg.ChannelAccountShortHash)
	add(msg.ChannelConversationID)
	rawID, shortHash := splitConversationShortHash(msg.ChannelConversationID)
	add(rawID)
	add(shortHash)
	if shortHash != "" {
		add("accountHash:" + shortHash)
	}
	return candidates
}

func isGroupClearCommand(msg IncomingMessage) bool {
	return strings.TrimSpace(msg.ConversationType) == "group" &&
		strings.EqualFold(strings.TrimSpace(msg.SenderName), "GM") &&
		clearCommandText(msg.Content) == "#clear"
}

func clearCommandText(content string) string {
	text := strings.TrimSpace(content)
	if text == "#clear" {
		return text
	}
	if idx := strings.LastIndex(text, ":"); idx >= 0 {
		return strings.TrimSpace(text[idx+1:])
	}
	return text
}

func resolveDedupeKey(msg IncomingMessage) string {
	msgID := strings.TrimSpace(msg.ChannelMessageID)
	if msgID == "" {
		return ""
	}
	if scope := resolveChannelAccountScope(msg); scope != "" {
		return msg.Channel + ":" + scope + ":" + msgID
	}
	return msgID
}

func resolveChannelUserKey(msg IncomingMessage) string {
	userID := strings.TrimSpace(msg.ChannelUserID)
	if userID == "" {
		return ""
	}
	if scope := resolveChannelAccountScope(msg); scope != "" {
		return scope + ":" + userID
	}
	return userID
}

func resolveChannelAccountScope(msg IncomingMessage) string {
	accountID := strings.TrimSpace(msg.ChannelAccountID)
	if accountID != "" {
		return "account:" + accountID
	}
	shortHash := strings.TrimSpace(msg.ChannelAccountShortHash)
	if shortHash == "" {
		_, shortHash = splitConversationShortHash(msg.ChannelConversationID)
	}
	if shortHash != "" {
		return "accountHash:" + shortHash
	}
	return ""
}

func splitConversationShortHash(conversationID string) (rawID, shortHash string) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return "", ""
	}
	idx := strings.LastIndex(conversationID, "@")
	if idx < 0 {
		return conversationID, ""
	}
	return conversationID[:idx], conversationID[idx+1:]
}

func newID() string {
	return fmt.Sprintf("%x%d", randBytes(6), time.Now().UnixNano()%1e6)
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
