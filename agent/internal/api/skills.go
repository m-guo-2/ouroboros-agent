package api

import (
	"net/http"
	"strings"

	"agent/internal/storage"
)

// GET/POST /api/skills
func handleSkills(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		skills, err := storage.GetAllSkills()
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if skills == nil {
			skills = []storage.SkillRecord{}
		}
		ok(w, skills)
	case http.MethodPost:
		var body map[string]interface{}
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		name, _ := body["name"].(string)
		if name == "" {
			apiErr(w, http.StatusBadRequest, "name is required")
			return
		}
		rec := storage.SkillRecord{
			Name:     name,
			Type:     "knowledge",
			Enabled:  true,
			Triggers: []interface{}{},
			Tools:    []interface{}{},
		}
		if v, ok := body["id"].(string); ok {
			rec.ID = v
		}
		if v, ok := body["description"].(string); ok {
			rec.Description = v
		}
		if v, ok := body["version"].(string); ok {
			rec.Version = v
		}
		if v, ok := body["type"].(string); ok {
			rec.Type = v
		}
		if v, ok := body["enabled"].(bool); ok {
			rec.Enabled = v
		}
		if v, ok := body["readme"].(string); ok {
			rec.Readme = v
		}
		if v, ok := body["triggers"]; ok {
			if arr, ok := v.([]interface{}); ok {
				rec.Triggers = arr
			}
		}
		if v, ok := body["tools"]; ok {
			if arr, ok := v.([]interface{}); ok {
				rec.Tools = arr
			}
		}
		created, err := storage.CreateSkill(rec)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "data": created})
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// GET/PUT/DELETE /api/skills/{id}[/context]
func handleSkillsWithID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/skills/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]
	if id == "" {
		apiErr(w, http.StatusBadRequest, "missing skill id")
		return
	}
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}

	// GET /api/skills/{agentId}/context — compile skill context for an agent
	if sub == "context" && r.Method == http.MethodGet {
		var agentSkills []string
		agentCfg, _ := storage.GetAgentConfig(id)
		if agentCfg != nil {
			agentSkills = agentCfg.Skills
		}
		ctx, err := storage.GetSkillsContext(id, agentSkills)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, ctx)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s, err := storage.GetSkillByID(id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if s == nil {
			apiErr(w, http.StatusNotFound, "skill not found")
			return
		}
		ok(w, s)
	case http.MethodPut:
		existing, err := storage.GetSkillByID(id)
		if err != nil || existing == nil {
			apiErr(w, http.StatusNotFound, "skill not found")
			return
		}
		var body map[string]interface{}
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		updated, err := storage.UpdateSkill(id, body)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, updated)
	case http.MethodDelete:
		deleted, err := storage.DeleteSkill(id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !deleted {
			apiErr(w, http.StatusNotFound, "skill not found")
			return
		}
		ok(w, map[string]bool{"deleted": true})
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
