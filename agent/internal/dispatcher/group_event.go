package dispatcher

import (
	"encoding/json"
	"net/http"

	"agent/internal/logger"
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

	agentID := evt.AgentID
	if agentID == "" {
		agentID = "default-agent-config"
	}

	status := "active"
	if evt.EventType == "group_dissolved" {
		status = "dissolved"
	}

	if err := storage.UpsertChannelGroup(agentID, evt.Channel, evt.ChannelGroupID, evt.GroupName, status); err != nil {
		logger.Error(r.Context(), "群事件处理失败",
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

	logger.Business(r.Context(), "群事件已处理",
		"channel", evt.Channel,
		"groupId", evt.ChannelGroupID,
		"eventType", evt.EventType,
		"groupName", evt.GroupName,
	)

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
