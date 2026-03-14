package api

import (
	"net/http"
	"strconv"
	"strings"

	"agent/internal/storage"
)

// GET /api/agent-sessions[?agentId=&userId=&channel=&limit=]
// POST /api/agent-sessions
func handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listSessions(w, r)
	case http.MethodPost:
		createSession(w, r)
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// /api/agent-sessions/{id}[/messages]
func handleSessionsWithID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/agent-sessions/")
	parts := strings.SplitN(path, "/", 2)
	id := parts[0]
	if id == "" {
		apiErr(w, http.StatusBadRequest, "missing session id")
		return
	}
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}

	if sub == "messages" {
		getSessionMessages(w, r, id)
		return
	}
	if sub == "compactions" {
		getSessionCompactions(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		getSession(w, r, id)
	case http.MethodPut:
		updateSession(w, r, id)
	case http.MethodDelete:
		deleteSession(w, r, id)
	default:
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func listSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	agentID := q.Get("agentId")
	userID := q.Get("userId")
	channel := q.Get("channel")
	limit := parseLimit(q.Get("limit"), 50, 200)
	before := parseInt64(q.Get("before"), 0)

	sessions, err := storage.ListSessions(agentID, userID, channel, limit, before)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Enrich with message count.
	type sessionWithCount struct {
		storage.SessionData
		MessageCount int `json:"messageCount"`
	}
	sessionIDs := make([]string, 0, len(sessions))
	for _, s := range sessions {
		sessionIDs = append(sessionIDs, s.ID)
	}
	counts, err := storage.CountSessionMessagesBatch(sessionIDs)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]sessionWithCount, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, sessionWithCount{
			SessionData:  s,
			MessageCount: counts[s.ID],
		})
	}
	ok(w, result)
}

func getSession(w http.ResponseWriter, r *http.Request, id string) {
	s, err := storage.GetSession(id)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s == nil {
		apiErr(w, http.StatusNotFound, "session not found")
		return
	}
	ok(w, s)
}

func createSession(w http.ResponseWriter, r *http.Request) {
	var body map[string]interface{}
	if err := decodeBody(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	s, err := storage.CreateSession(body)
	if err != nil || s == nil {
		apiErr(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	created(w, s)
}

func updateSession(w http.ResponseWriter, r *http.Request, id string) {
	var body map[string]interface{}
	if err := decodeBody(r, &body); err != nil {
		apiErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := storage.UpdateSession(id, body); err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s, _ := storage.GetSession(id)
	ok(w, s)
}

func deleteSession(w http.ResponseWriter, r *http.Request, id string) {
	s, err := storage.GetSession(id)
	if err != nil || s == nil {
		apiErr(w, http.StatusNotFound, "session not found")
		return
	}
	_ = storage.DeleteSessionMessages(id)
	if err := storage.DeleteSession(id); err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ok(w, map[string]bool{"deleted": true})
}

func getSessionMessages(w http.ResponseWriter, r *http.Request, id string) {
	q := r.URL.Query()
	limit := parseLimit(q.Get("limit"), 200, 1000)
	before := parseInt64(q.Get("before"), 0)

	var msgs []storage.MessageData
	var err error
	if before > 0 {
		msgs, err = storage.GetRecentMessagesBefore(id, before, limit)
	} else {
		msgs, err = storage.GetLatestSessionMessages(id, limit)
	}
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if msgs == nil {
		msgs = []storage.MessageData{}
	}
	ok(w, msgs)
}

func getSessionCompactions(w http.ResponseWriter, r *http.Request, id string) {
	compactions, err := storage.ListCompactions(id)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if compactions == nil {
		compactions = []storage.CompactionData{}
	}
	ok(w, compactions)
}

func parseInt64(raw string, defaultValue int64) int64 {
	if raw == "" {
		return defaultValue
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return defaultValue
	}
	return n
}

func parseLimit(raw string, defaultValue, maxValue int) int {
	if raw == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return defaultValue
	}
	if n > maxValue {
		return maxValue
	}
	return n
}
