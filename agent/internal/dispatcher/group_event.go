package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"agent/internal/logger"
	"agent/internal/runner"
	"agent/internal/storage"
)

// GroupEvent is the payload sent by a channel adapter to report group lifecycle events.
type GroupEvent struct {
	Channel        string         `json:"channel"`
	AgentID        string         `json:"agentId"`
	ChannelGroupID string         `json:"channelGroupId"`
	EventType      string         `json:"eventType"`
	GroupName      string         `json:"groupName,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
}

var validGroupEventTypes = map[string]bool{
	"group_name_changed": true,
	"member_joined":      true,
	"member_removed":     true,
	"member_quit":        true,
	"group_dissolved":    true,
	"group_created":      true,
	"group_joined":       true,
}

// HandleGroupEvent is an HTTP handler for POST /api/channels/group-event.
func HandleGroupEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var evt GroupEvent
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "error": "invalid JSON",
		})
		return
	}

	if evt.Channel == "" || evt.ChannelGroupID == "" || evt.EventType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "error": "missing required fields: channel, channelGroupId, eventType",
		})
		return
	}
	if !validGroupEventTypes[evt.EventType] {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "error": "invalid eventType",
		})
		return
	}

	ctx := r.Context()
	agentID := evt.AgentID
	if agentID == "" {
		agentID = "default-agent-config"
	}

	status := "active"
	if evt.EventType == "group_dissolved" {
		status = "dissolved"
	}

	if err := storage.UpsertChannelGroup(agentID, evt.Channel, evt.ChannelGroupID, evt.GroupName, status); err != nil {
		logger.Error(ctx, "群事件处理失败",
			"channel", evt.Channel,
			"groupId", evt.ChannelGroupID,
			"eventType", evt.EventType,
			"error", err.Error(),
		)
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "error": "storage error",
		})
		return
	}

	logger.Business(ctx, "群事件已处理",
		"channel", evt.Channel,
		"groupId", evt.ChannelGroupID,
		"eventType", evt.EventType,
		"groupName", evt.GroupName,
	)

	if evt.EventType == "group_joined" {
		go handleGroupJoined(evt, agentID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleGroupJoined creates a session, dispatches hooks, injects a synthetic
// trigger message, and enqueues the session for processing so the LLM can
// proactively greet the group.
func handleGroupJoined(evt GroupEvent, agentID string) {
	ctx := context.Background()

	sessionKey := evt.Channel + ":" + evt.ChannelGroupID

	// Idempotency: skip if this group already has a session.
	existing, _ := storage.FindSessionByKey(agentID, sessionKey)
	if existing != nil {
		logger.Detail(ctx, "group_joined 跳过：群已有 session",
			"agentId", agentID, "groupId", evt.ChannelGroupID, "sessionId", existing.ID)
		return
	}

	agentCfg, err := storage.GetAgentConfig(agentID)
	if err != nil || agentCfg == nil {
		logger.Error(ctx, "group_joined: agent 配置未找到", "agentId", agentID)
		return
	}

	title := evt.GroupName
	if title == "" {
		title = "群聊"
	}
	if len(title) > 30 {
		title = title[:30] + "..."
	}

	session, err := storage.CreateSession(map[string]interface{}{
		"id":                    newID(),
		"title":                 title,
		"userId":                "system",
		"agentId":               agentCfg.ID,
		"channel":               evt.Channel,
		"sessionKey":            sessionKey,
		"channelConversationId": evt.ChannelGroupID,
		"channelName":           evt.GroupName,
	})
	if err != nil || session == nil {
		logger.Error(ctx, "group_joined: 创建 session 失败", "error", fmt.Sprint(err))
		return
	}

	traceID := fmt.Sprintf("trace-%x", randBytes(8))
	ctx = logger.WithTrace(ctx, traceID, session.ID)

	logger.Business(ctx, "group_joined: 新 session 创建",
		"sessionId", session.ID, "agentId", agentCfg.ID,
		"groupId", evt.ChannelGroupID, "groupName", evt.GroupName)

	// Dispatch hooks to activate ephemeral skills (e.g. welcome/icebreaker).
	runner.DispatchHooks(ctx, agentCfg.Hooks, "group_joined", session.ID)

	// Inject a synthetic message so processSession has an event to drain.
	content := fmt.Sprintf("[群事件] Bot 加入群聊「%s」", evt.GroupName)
	savedMsg, _ := storage.SaveMessage(map[string]interface{}{
		"sessionId":   session.ID,
		"role":        "user",
		"content":     content,
		"messageType": "text",
		"channel":     evt.Channel,
		"traceId":     traceID,
		"initiator":   "system",
		"senderName":  "系统",
		"senderId":    "",
	})
	var msgID int64
	if savedMsg != nil {
		msgID = savedMsg.ID
		_ = storage.AppendSessionEvent(session.ID, msgID)
	}

	_ = storage.UpdateSession(session.ID, map[string]interface{}{
		"executionStatus": "processing",
	})

	enqueueErr := runner.EnqueueProcessRequest(ctx, runner.ProcessRequest{
		UserID:                "system",
		AgentID:               agentCfg.ID,
		Content:               content,
		Channel:               evt.Channel,
		ChannelConversationID: evt.ChannelGroupID,
		MessageID:             msgID,
		SessionID:             session.ID,
		TraceID:               traceID,
	})
	if enqueueErr != nil {
		logger.Error(ctx, "group_joined: 入队失败", "error", enqueueErr.Error())
		_ = storage.UpdateSession(session.ID, map[string]interface{}{
			"executionStatus": "interrupted",
		})
	}
}
