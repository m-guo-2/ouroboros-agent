package api

import (
	"net/http"
	"strings"

	"agent/internal/runner"
	"agent/internal/storage"
)

func parseSkillBindings(raw interface{}) []storage.SkillBinding {
	var bindings []storage.SkillBinding
	items, ok := raw.([]interface{})
	if !ok {
		return bindings
	}
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		mode, _ := m["mode"].(string)
		if id == "" {
			continue
		}
		bindings = append(bindings, storage.SkillBinding{ID: id, Mode: mode})
	}
	return bindings
}

// GET/POST /api/agents
func handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		agents, err := storage.GetAllAgents()
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if agents == nil {
			agents = []storage.AgentConfig{}
		}
		ok(w, agents)
	case http.MethodPost:
		var body map[string]interface{}
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		displayName, _ := body["displayName"].(string)
		if displayName == "" {
			apiErr(w, http.StatusBadRequest, "displayName is required")
			return
		}
		cfg := storage.AgentConfig{
			DisplayName: displayName,
			IsActive:    true,
		}
		if v, ok := body["systemPrompt"].(string); ok {
			cfg.SystemPrompt = v
		}
		if v, ok := body["modelId"].(string); ok {
			cfg.ModelID = v
		}
		if v, ok := body["provider"].(string); ok {
			cfg.Provider = v
		}
		if v, ok := body["model"].(string); ok {
			cfg.Model = v
		}
		if body["skills"] != nil {
			cfg.Skills = parseSkillBindings(body["skills"])
		}
		if v, ok := body["channels"].([]interface{}); ok {
			for _, c := range v {
				switch cv := c.(type) {
				case map[string]interface{}:
					ct, _ := cv["channelType"].(string)
					ci, _ := cv["channelIdentifier"].(string)
					cfg.Channels = append(cfg.Channels, storage.ChannelBinding{
						ChannelType: ct, ChannelIdentifier: ci,
					})
				case string:
					cfg.Channels = append(cfg.Channels, storage.ChannelBinding{ChannelType: cv})
				}
			}
		}
		created, err := storage.CreateAgentConfig(cfg)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": created})
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// GET/PUT/DELETE /api/agents/{id}[/...]
func handleAgentsWithID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]
	if id == "" {
		apiErr(w, http.StatusBadRequest, "missing agent id")
		return
	}

	// Handle sub-resource paths like /api/agents/{id}/full-prompt
	subPath := ""
	if len(parts) > 1 {
		subPath = parts[1]
	}

	if subPath == "full-prompt" && (r.Method == http.MethodGet || r.Method == http.MethodPost) {
		cfg, err := storage.GetAgentConfig(id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if cfg == nil {
			apiErr(w, http.StatusNotFound, "agent not found")
			return
		}
		if r.Method == http.MethodPost {
			var body map[string]interface{}
			if err := decodeBody(r, &body); err != nil {
				apiErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			if v, ok := body["systemPrompt"].(string); ok {
				cfg.SystemPrompt = v
			}
			if body["skills"] != nil {
				cfg.Skills = parseSkillBindings(body["skills"])
			}
		}
		skillsCtx, err := storage.GetSkillsContext(id, cfg.Skills)
		if err != nil {
			skillsCtx = &storage.SkillContext{LoadableSkillIDs: map[string]bool{}}
		}
		fullPrompt := runner.BuildSystemPrompt(cfg.SystemPrompt, skillsCtx.SkillsSnippet)
		ok(w, map[string]string{"fullPrompt": fullPrompt})
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg, err := storage.GetAgentConfig(id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if cfg == nil {
			apiErr(w, http.StatusNotFound, "agent not found")
			return
		}
		ok(w, cfg)
	case http.MethodPut:
		cfg, err := storage.GetAgentConfig(id)
		if err != nil || cfg == nil {
			apiErr(w, http.StatusNotFound, "agent not found")
			return
		}
		var body map[string]interface{}
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		updated, err := storage.UpdateAgentConfig(id, body)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, updated)
	case http.MethodDelete:
		if id == "default-agent-config" {
			apiErr(w, http.StatusBadRequest, "cannot delete default agent")
			return
		}
		deleted, err := storage.DeleteAgentConfig(id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !deleted {
			apiErr(w, http.StatusNotFound, "agent not found")
			return
		}
		ok(w, map[string]bool{"deleted": true})
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
