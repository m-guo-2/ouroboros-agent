package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlePersonasRejectsSystemPromptOverride(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/agents/agent-1/personas", bytes.NewBufferString(`{
		"displayName": "Office",
		"systemPrompt": "new identity"
	}`))
	rec := httptest.NewRecorder()

	handlePersonas(rec, req, "agent-1")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "create a new agent") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}
