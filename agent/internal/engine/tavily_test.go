package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"agent/internal/storage"
)

func TestTavilySearchToolSuccessAndTruncation(t *testing.T) {
	initTavilyTestDB(t)

	var gotReq tavilySearchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %s", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"query":         gotReq.Query,
			"answer":        "top level answer",
			"response_time": 0.25,
			"results": []map[string]interface{}{
				{"title": "A", "url": "https://a.test", "content": strings.Repeat("a", 500), "score": 0.9},
				{"title": "B", "url": "https://b.test", "content": "second", "score": 0.8},
				{"title": "C", "url": "https://c.test", "content": "third", "score": 0.7},
				{"title": "D", "url": "https://d.test", "content": "fourth", "score": 0.6},
			},
		})
	}))
	defer srv.Close()

	if err := storage.SetMultipleSettings(map[string]string{
		"enabled.tavily":      "true",
		"api_key.tavily":      "test-key",
		"base_url.tavily":     srv.URL,
		"tavily.max_results":  "3",
		"tavily.search_depth": "basic",
		"tavily.search_topic": "general",
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}

	registry := NewToolRegistry()
	RegisterTavilyTool(registry)
	tool, ok := registry.Get("tavily_search")
	if !ok {
		t.Fatalf("tavily_search not registered")
	}

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"query":       "golang release notes",
		"max_results": float64(3),
	})
	if err != nil {
		t.Fatalf("execute tavily_search: %v", err)
	}

	if gotReq.MaxResults != 4 {
		t.Fatalf("expected upstream overfetch max_results=4, got %d", gotReq.MaxResults)
	}

	out := result.(map[string]interface{})
	if out["truncated"] != true {
		t.Fatalf("expected truncated=true, got %#v", out["truncated"])
	}
	if out["total_results"].(int) != 4 {
		t.Fatalf("expected total_results=4, got %#v", out["total_results"])
	}
	if out["returned_results"].(int) != 3 {
		t.Fatalf("expected returned_results=3, got %#v", out["returned_results"])
	}
	results := out["results"].([]map[string]interface{})
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if snippet := results[0]["snippet"].(string); len([]rune(snippet)) > tavilySnippetLimit+3 {
		t.Fatalf("snippet was not truncated: %d", len([]rune(snippet)))
	}
}

func TestTavilySearchToolMissingAPIKey(t *testing.T) {
	initTavilyTestDB(t)

	if err := storage.SetMultipleSettings(map[string]string{
		"enabled.tavily": "true",
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}

	registry := NewToolRegistry()
	RegisterTavilyTool(registry)
	tool, _ := registry.Get("tavily_search")

	_, err := tool.Execute(context.Background(), map[string]interface{}{"query": "latest ai news"})
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("expected missing api key error, got %v", err)
	}
}

func TestTavilySearchToolUpstreamFailure(t *testing.T) {
	initTavilyTestDB(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()

	if err := storage.SetMultipleSettings(map[string]string{
		"enabled.tavily":      "true",
		"api_key.tavily":      "test-key",
		"base_url.tavily":     srv.URL,
		"tavily.max_results":  "2",
		"tavily.search_depth": "basic",
		"tavily.search_topic": "general",
	}); err != nil {
		t.Fatalf("set settings: %v", err)
	}

	registry := NewToolRegistry()
	RegisterTavilyTool(registry)
	tool, _ := registry.Get("tavily_search")

	_, err := tool.Execute(context.Background(), map[string]interface{}{"query": "latest ai news"})
	if err == nil || !strings.Contains(err.Error(), "upstream") {
		t.Fatalf("expected upstream error, got %v", err)
	}
}

func initTavilyTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "agent-test.sqlite")
	if err := storage.Init(dbPath); err != nil {
		t.Fatalf("init db: %v", err)
	}
}
