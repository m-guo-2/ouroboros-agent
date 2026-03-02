package api

import (
	"net/http"
	"strings"

	"agent/internal/channels"
)

// handleServices provides a stub /api/services endpoint.
// In the merged architecture there is only one process (this Go binary), but we
// expose basic channel adapter health so the admin dashboard can show status.
func handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	type serviceInfo struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Type    string `json:"type"`
		Healthy bool   `json:"healthy"`
	}

	result := []serviceInfo{
		{Name: "agent", Status: "running", Type: "core", Healthy: true},
	}

	for _, name := range channels.RegisteredChannels() {
		adapter := channels.GetAdapter(name)
		healthy := adapter != nil && adapter.HealthCheck()
		status := "degraded"
		if healthy {
			status = "running"
		}
		result = append(result, serviceInfo{
			Name:    strings.Title(name), //nolint:staticcheck
			Status:  status,
			Type:    "channel",
			Healthy: healthy,
		})
	}

	ok(w, result)
}
