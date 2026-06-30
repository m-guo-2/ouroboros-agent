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
			if v, exists := body["sandboxTemplateId"]; exists {
				if s, ok := v.(string); ok && s != "" {
					if !validateSandboxTemplateID(s) {
						apiErr(w, http.StatusBadRequest, "invalid sandboxTemplateId")
						return
					}
					ga.SandboxTemplateID = &s
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

	parts := strings.SplitN(subPath, "/", 2)
	id := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	if action != "" {
		if action == "clear-history" && r.Method == http.MethodPost {
			clearGroupHistory(w, agentID, id)
			return
		}
		apiErr(w, http.StatusNotFound, "not found")
		return
	}

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
		if v, ok := body["sandboxTemplateId"].(string); ok && v != "" && !validateSandboxTemplateID(v) {
			apiErr(w, http.StatusBadRequest, "invalid sandboxTemplateId")
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

func clearGroupHistory(w http.ResponseWriter, agentID, assignmentID string) {
	ga, err := storage.GetGroupAssignmentByID(assignmentID)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ga == nil || ga.AgentID != agentID {
		apiErr(w, http.StatusNotFound, "group assignment not found")
		return
	}
	session, err := storage.FindSessionByKey(agentID, ga.SessionKey)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if session == nil {
		ok(w, map[string]interface{}{"cleared": true, "sessionId": "", "messageCount": 0})
		return
	}
	count, err := storage.CountSessionMessages(session.ID)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := storage.ClearSessionHistory(session.ID); err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ok(w, map[string]interface{}{"cleared": true, "sessionId": session.ID, "messageCount": count})
}
