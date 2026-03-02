package api

import (
	"net/http"

	"agent/internal/channels"
)

// GET /api/channels — returns registration status of known channel adapters.
func handleChannelsStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	registered := channels.RegisteredChannels()
	type channelStatus struct {
		Name    string `json:"name"`
		Healthy bool   `json:"healthy"`
	}
	var statuses []channelStatus
	for _, name := range registered {
		adapter := channels.GetAdapter(name)
		healthy := adapter != nil && adapter.HealthCheck()
		statuses = append(statuses, channelStatus{Name: name, Healthy: healthy})
	}
	ok(w, statuses)
}
