package api

import (
	"net/http"

	"agent/internal/storage"
)

// GET /api/messages?sessionId=
func handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		apiErr(w, http.StatusBadRequest, "sessionId is required")
		return
	}
	msgs, err := storage.GetSessionMessages(sessionID, 1000)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if msgs == nil {
		msgs = []storage.MessageData{}
	}
	ok(w, msgs)
}
