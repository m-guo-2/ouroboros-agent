package api

import (
	"net/http"
	"strings"

	"agent/internal/storage"
)

// GET/POST /api/agents/{agentId}/personas
func handlePersonas(w http.ResponseWriter, r *http.Request, agentID string) {
	switch r.Method {
	case http.MethodGet:
		personas, err := storage.ListPersonas(agentID)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, personas)
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
		p := storage.Persona{
			AgentID:     agentID,
			DisplayName: displayName,
		}
		if v, ok := body["systemPrompt"]; ok && nonEmptyString(v) {
			apiErr(w, http.StatusBadRequest, "persona cannot override systemPrompt; create a new agent instead")
			return
		}
		if v, ok := body["provider"]; ok {
			if s, ok := v.(string); ok {
				p.Provider = &s
			}
		}
		if v, ok := body["model"]; ok {
			if s, ok := v.(string); ok {
				p.Model = &s
			}
		}
		if v, ok := body["sandboxTemplateId"]; ok {
			if s, ok := v.(string); ok && s != "" {
				if !validateSandboxTemplateID(s) {
					apiErr(w, http.StatusBadRequest, "invalid sandboxTemplateId")
					return
				}
				p.SandboxTemplateID = &s
			}
		}
		if v, ok := body["skills"]; ok && v != nil {
			ids := parseSkillIDs(v)
			p.Skills = &ids
		}
		if v, ok := body["subagentModels"]; ok && v != nil {
			p.SubagentModels = parseSubagentModels(v)
		}
		if v, ok := body["subagentSkills"]; ok && v != nil {
			p.SubagentSkills = parseSubagentSkills(v)
		}
		result, err := storage.CreatePersona(p)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		created(w, result)
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// GET/PUT/DELETE /api/agents/{agentId}/personas/{id}
// POST          /api/agents/{agentId}/personas/{id}/clone
func handlePersonaWithID(w http.ResponseWriter, r *http.Request, agentID, personaPath string) {
	parts := strings.SplitN(personaPath, "/", 2)
	id := parts[0]
	subPath := ""
	if len(parts) > 1 {
		subPath = parts[1]
	}

	if subPath == "clone" && r.Method == http.MethodPost {
		var body map[string]interface{}
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		newName, _ := body["displayName"].(string)
		if newName == "" {
			newName = "Copy"
		}
		result, err := storage.ClonePersona(id, newName)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				apiErr(w, http.StatusNotFound, err.Error())
			} else {
				apiErr(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		created(w, result)
		return
	}

	switch r.Method {
	case http.MethodGet:
		p, err := storage.GetPersona(id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if p == nil {
			apiErr(w, http.StatusNotFound, "persona not found")
			return
		}
		ok(w, p)
	case http.MethodPut:
		p, err := storage.GetPersona(id)
		if err != nil || p == nil {
			apiErr(w, http.StatusNotFound, "persona not found")
			return
		}
		var body map[string]interface{}
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if v, ok := body["sandboxTemplateId"].(string); ok && v != "" && !validateSandboxTemplateID(v) {
			apiErr(w, http.StatusBadRequest, "invalid sandboxTemplateId")
			return
		}
		if v, ok := body["systemPrompt"]; ok && nonEmptyString(v) {
			apiErr(w, http.StatusBadRequest, "persona cannot override systemPrompt; create a new agent instead")
			return
		}
		result, err := storage.UpdatePersona(id, body)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, result)
	case http.MethodDelete:
		deleted, err := storage.DeletePersona(id)
		if err != nil {
			if strings.Contains(err.Error(), "referenced") {
				apiErr(w, http.StatusConflict, err.Error())
			} else {
				apiErr(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		if !deleted {
			apiErr(w, http.StatusNotFound, "persona not found")
			return
		}
		ok(w, map[string]bool{"deleted": true})
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func nonEmptyString(v interface{}) bool {
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) != ""
}
