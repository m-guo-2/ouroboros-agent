// Package dispatcher receives normalised incoming messages from channel adapters
// (Feishu, QiWei, WebUI) and routes them to the runner — replacing channel-dispatcher.ts.
package dispatcher

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"agent/internal/logger"
	"agent/internal/runner"
	"agent/internal/storage"
)

// IncomingMessage is the normalised payload sent by a channel adapter.
type IncomingMessage struct {
	Channel                 string                   `json:"channel"`
	ChannelUserID           string                   `json:"channelUserId"`
	ChannelMessageID        string                   `json:"channelMessageId"`
	ChannelConversationID   string                   `json:"channelConversationId,omitempty"`
	ChannelConversationName string                   `json:"channelConversationName,omitempty"`
	SenderName              string                   `json:"senderName,omitempty"`
	Content                 string                   `json:"content"`
	MessageType             string                   `json:"messageType,omitempty"`
	AgentID                 string                   `json:"agentId,omitempty"`
	Timestamp               int64                    `json:"timestamp,omitempty"`
	Attachments             []storage.AttachmentData `json:"attachments,omitempty"`
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
	dedupeKey := msg.ChannelMessageID
	if msg.AgentID != "" {
		dedupeKey = msg.ChannelMessageID + ":" + msg.AgentID
	}
	if storage.IsProcessed(dedupeKey) {
		logger.Detail(ctx, "重复消息跳过", "dedupeKey", dedupeKey)
		return DispatchResult{Success: true, Duplicate: true}
	}
	_ = storage.MarkProcessed(dedupeKey, msg.Channel)

	// 2. Resolve (or create) shadow user.
	userID, isNew, err := storage.ResolveUser(msg.Channel, msg.ChannelUserID, msg.SenderName)
	if err != nil {
		logger.Error(ctx, "用户解析失败", "error", err.Error())
		return DispatchResult{Success: false, Error: "user resolution failed"}
	}
	if isNew {
		logger.Business(ctx, "新用户创建", "userId", userID, "channel", msg.Channel)
	}

	// 3. Locate target agent.
	agentID := msg.AgentID
	if agentID == "" {
		agentID = "default-agent-config"
	}
	agentCfg, err := storage.GetAgentConfig(agentID)
	if err != nil || agentCfg == nil {
		logger.Error(ctx, "Agent 配置未找到", "agentId", agentID)
		return DispatchResult{Success: false, Error: "no agent available"}
	}

	// 4. Session management.
	sessionKey := resolveSessionKey(msg)
	session, err := storage.FindSessionByKey(agentCfg.ID, sessionKey)
	if err != nil {
		logger.Error(ctx, "查询 session 失败", "error", err.Error())
	}

	// Legacy fallback: match by channelConversationId when session_key was absent.
	if session == nil && msg.ChannelConversationID != "" {
		session, _ = storage.FindSessionByConversationID(msg.ChannelConversationID, agentCfg.ID)
		if session != nil {
			// Back-fill missing fields on the legacy session.
			patch := map[string]string{}
			if session.SessionKey == "" {
				patch["sessionKey"] = sessionKey
			}
			if session.ChannelConversationID == "" {
				patch["channelConversationId"] = msg.ChannelConversationID
			}
			if len(patch) > 0 {
				_ = storage.PatchSession(session.ID, patch)
				session.SessionKey = sessionKey
			}
		}
	}

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

	// 5. Persist the incoming user message.
	msgID := newID()
	_, _ = storage.SaveMessage(map[string]interface{}{
		"id":               msgID,
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
		MessageID:             msgID,
		SessionID:             session.ID,
		TraceID:               traceID,
	})
	if enqueueErr != nil {
		logger.Error(dispatchCtx, "入队失败", "error", enqueueErr.Error())
		_ = storage.UpdateSession(session.ID, map[string]interface{}{
			"executionStatus": "interrupted",
		})
		return DispatchResult{Success: false, Error: enqueueErr.Error()}
	}

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
		result := Dispatch(r.Context(), msg)
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
		uniqueID = msg.ChannelUserID
	}
	return msg.Channel + ":" + uniqueID
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
