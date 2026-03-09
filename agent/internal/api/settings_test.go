package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"agent/internal/storage"
)

func TestHandleSettingsReturnsTavilyDefaultsAndGroups(t *testing.T) {
	initSettingsTestDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec := httptest.NewRecorder()
	handleSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Success bool                      `json:"success"`
		Data    map[string]string         `json:"data"`
		Groups  map[string]map[string]any `json:"groups"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Success {
		t.Fatalf("expected success=true")
	}
	if body.Data["base_url.tavily"] != storage.DefaultTavilyBaseURL {
		t.Fatalf("unexpected default base url: %q", body.Data["base_url.tavily"])
	}
	if body.Data["enabled.tavily"] != "false" {
		t.Fatalf("unexpected default enabled flag: %q", body.Data["enabled.tavily"])
	}
	if _, ok := body.Groups["tavily"]; !ok {
		t.Fatalf("expected tavily group in settings response")
	}
}

func TestHandleSettingsPutPersistsTavilyValues(t *testing.T) {
	initSettingsTestDB(t)

	payload := map[string]string{
		"enabled.tavily":      "true",
		"api_key.tavily":      "tvly-test",
		"base_url.tavily":     "https://example.tavily.test",
		"tavily.search_depth": "advanced",
		"tavily.search_topic": "news",
		"tavily.max_results":  "7",
	}
	b, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	handleSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body struct {
		Success bool              `json:"success"`
		Data    map[string]string `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Success {
		t.Fatalf("expected success=true")
	}
	for k, want := range payload {
		if got := body.Data[k]; got != want {
			t.Fatalf("expected %s=%q, got %q", k, want, got)
		}
	}
}

func initSettingsTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "settings-test.sqlite")
	if err := storage.Init(dbPath); err != nil {
		t.Fatalf("init db: %v", err)
	}
}
