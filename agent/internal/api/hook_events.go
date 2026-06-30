package api

import "net/http"

// HookEventDef describes a known hook event for the admin UI.
type HookEventDef struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

var knownHookEvents = []HookEventDef{
	{Name: "session_started", Label: "新会话开始", Description: "用户在一个会话中发送首条消息时触发"},
	{Name: "participant_discovered", Label: "发现会话参与者", Description: "某用户首次出现在当前会话或群里时触发，并携带 first_seen/seen_before 关系事实"},
	{Name: "contact_added", Label: "新好友添加", Description: "渠道侧新好友关系建立时触发"},
	{Name: "group_joined", Label: "入群", Description: "Agent 被拉入新群时触发"},
}

// GET /api/hook-events
func handleHookEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ok(w, knownHookEvents)
}
