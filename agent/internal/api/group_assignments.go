package api

import (
	"net/http"
	"strings"

	"agent/internal/storage"
)

// GET/POST /api/agents/{agentId}/groups
// GET      /api/agents/{agentId}/groups/discover
func handleGroupAssignments(w http.ResponseWriter, r *http.Request, agentID, subPath string) {
	if subPath == "discover" && r.Method == http.MethodGet {
		groups, err := storage.ListUnconfiguredGroups(agentID)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, groups)
		return
	}

	if subPath == "" {
		switch r.Method {
		case http.MethodGet:
			assignments, err := storage.ListGroupAssignments(agentID)
			if err != nil {
				apiErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			ok(w, assignments)
		case http.MethodPost:
			var body map[string]interface{}
			if err := decodeBody(r, &body); err != nil {
				apiErr(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			sessionKey, _ := body["sessionKey"].(string)
			if sessionKey == "" {
				apiErr(w, http.StatusBadRequest, "sessionKey is required")
				return
			}
			groupName, _ := body["groupName"].(string)
			ga := storage.GroupAssignment{
				AgentID:    agentID,
				SessionKey: sessionKey,
				GroupName:  groupName,
			}
			if v, exists := body["personaId"]; exists {
				if s, ok := v.(string); ok {
					ga.PersonaID = &s
				}
			}
			result, err := storage.CreateGroupAssignment(ga)
			if err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint") {
					apiErr(w, http.StatusConflict, "group assignment already exists for this session_key")
					return
				}
				apiErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			created(w, result)
		default:
			apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// subPath is the assignment ID
	id := subPath

	switch r.Method {
	case http.MethodGet:
		ga, err := storage.GetGroupAssignmentByID(id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if ga == nil {
			apiErr(w, http.StatusNotFound, "group assignment not found")
			return
		}
		ok(w, ga)
	case http.MethodPut:
		ga, err := storage.GetGroupAssignmentByID(id)
		if err != nil || ga == nil {
			apiErr(w, http.StatusNotFound, "group assignment not found")
			return
		}
		var body map[string]interface{}
		if err := decodeBody(r, &body); err != nil {
			apiErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		result, err := storage.UpdateGroupAssignment(id, body)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ok(w, result)
	case http.MethodDelete:
		deleted, err := storage.DeleteGroupAssignment(id)
		if err != nil {
			apiErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !deleted {
			apiErr(w, http.StatusNotFound, "group assignment not found")
			return
		}
		ok(w, map[string]bool{"deleted": true})
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
