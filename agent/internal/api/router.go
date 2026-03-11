// Package api provides the admin HTTP API, replacing the TypeScript server's express routes.
// All routes are prefixed /api/ and mounted onto an existing *http.ServeMux.
package api

import (
	"encoding/json"
	"net/http"
)

// Mount registers all admin API routes on mux.
// staticDir is the path to the built admin SPA (dist/); pass "" to skip static serving.
func Mount(mux *http.ServeMux, logDir string) {
	// Agent sessions
	mux.HandleFunc("/api/agent-sessions", handleSessions)
	mux.HandleFunc("/api/agent-sessions/", handleSessionsWithID)

	// Messages
	mux.HandleFunc("/api/messages", handleMessages)

	// Settings
	mux.HandleFunc("/api/settings", handleSettings)
	mux.HandleFunc("/api/settings/provider-models", handleSettingsProviderModels)
	mux.HandleFunc("/api/settings/", handleSettingsWithKey)

	// Agents
	mux.HandleFunc("/api/agents", handleAgents)
	mux.HandleFunc("/api/agents/", handleAgentsWithID)

	// Models (LLM config)
	mux.HandleFunc("/api/models", handleModels)
	mux.HandleFunc("/api/models/", handleModelsWithID)

	// Skills
	mux.HandleFunc("/api/skills", handleSkills)
	mux.HandleFunc("/api/skills/refresh", handleSkillsRefresh)
	mux.HandleFunc("/api/skills/", handleSkillsWithID)

	// Users
	mux.HandleFunc("/api/users", handleUsers)
	mux.HandleFunc("/api/users/", handleUsersWithID)

	// Traces (JSONL log reader)
	th := &tracesHandler{logDir: logDir}
	mux.Handle("/api/traces", th)
	mux.Handle("/api/traces/", th)

	// Services status
	mux.HandleFunc("/api/services", handleServices)

	// Channel adapters status
	mux.HandleFunc("/api/channels", handleChannelsStatus)
}

// --------------------------------------------------------------------------
// Shared JSON helpers
// --------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "data": data})
}

func created(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": data})
}

func apiErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]interface{}{"success": false, "error": msg})
}

func decodeBody(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}
