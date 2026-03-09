package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"agent/internal/storage"
	"agent/internal/types"
)

const (
	tavilyPlatformMaxResults = 10
	tavilySnippetLimit       = 400
)

type TavilyClient struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

type tavilySearchRequest struct {
	Query         string `json:"query"`
	SearchDepth   string `json:"search_depth,omitempty"`
	Topic         string `json:"topic,omitempty"`
	MaxResults    int    `json:"max_results,omitempty"`
	IncludeAnswer bool   `json:"include_answer"`
}

type tavilySearchResponse struct {
	Query        string  `json:"query"`
	Answer       string  `json:"answer"`
	ResponseTime float64 `json:"response_time"`
	Results      []struct {
		Title   string  `json:"title"`
		URL     string  `json:"url"`
		Content string  `json:"content"`
		Score   float64 `json:"score"`
	} `json:"results"`
}

func RegisterTavilyTool(registry *ToolRegistry) {
	registry.RegisterBuiltin(
		"tavily_search",
		"使用 Tavily 执行联网检索，返回受控预算的结构化结果。适合查询最新网页信息、来源链接和简短摘要。",
		types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "要检索的查询语句",
				},
				"max_results": map[string]interface{}{
					"type":        "integer",
					"description": "返回结果数上限；会被平台最大值限制",
				},
				"search_depth": map[string]interface{}{
					"type":        "string",
					"description": "检索深度：basic / advanced / fast / ultra-fast",
				},
				"topic": map[string]interface{}{
					"type":        "string",
					"description": "检索主题：general / news / finance",
				},
			},
			Required: []string{"query"},
		},
		func(ctx context.Context, input map[string]interface{}) (interface{}, error) {
			query, _ := input["query"].(string)
			query = strings.TrimSpace(query)
			if query == "" {
				return nil, fmt.Errorf("query is required")
			}

			cfg, err := storage.GetTavilyConfig()
			if err != nil {
				return nil, fmt.Errorf("load tavily config: %w", err)
			}
			if cfg == nil || !cfg.Enabled {
				return nil, fmt.Errorf("tavily search is disabled; enable it in settings first")
			}
			if cfg.APIKey == "" {
				return nil, fmt.Errorf("tavily API key is not configured")
			}

			returnBudget := cfg.MaxResults
			if requested, ok := input["max_results"].(float64); ok && requested > 0 {
				returnBudget = clampInt(int(requested), 1, tavilyPlatformMaxResults)
			} else {
				returnBudget = clampInt(returnBudget, 1, tavilyPlatformMaxResults)
			}
			upstreamMaxResults := returnBudget
			if upstreamMaxResults < tavilyPlatformMaxResults {
				upstreamMaxResults++
			}

			searchDepth := cfg.SearchDepth
			if raw, ok := input["search_depth"].(string); ok && strings.TrimSpace(raw) != "" {
				searchDepth = normalizeSearchDepth(raw)
			}
			topic := cfg.SearchTopic
			if raw, ok := input["topic"].(string); ok && strings.TrimSpace(raw) != "" {
				topic = normalizeTopic(raw)
			}

			client := TavilyClient{
				APIKey:  cfg.APIKey,
				BaseURL: cfg.BaseURL,
				Client:  &http.Client{Timeout: 20 * time.Second},
			}
			resp, err := client.Search(ctx, tavilySearchRequest{
				Query:         query,
				SearchDepth:   searchDepth,
				Topic:         topic,
				MaxResults:    upstreamMaxResults,
				IncludeAnswer: true,
			})
			if err != nil {
				return nil, err
			}

			totalResults := len(resp.Results)
			truncated := totalResults > returnBudget
			if truncated {
				resp.Results = resp.Results[:returnBudget]
			}
			results := make([]map[string]interface{}, 0, len(resp.Results))
			for _, item := range resp.Results {
				results = append(results, map[string]interface{}{
					"title":   strings.TrimSpace(item.Title),
					"url":     strings.TrimSpace(item.URL),
					"score":   item.Score,
					"snippet": truncateText(item.Content, tavilySnippetLimit),
				})
			}
			returnedResults := len(results)

			out := map[string]interface{}{
				"query":            query,
				"results":          results,
				"total_results":    totalResults,
				"returned_results": returnedResults,
				"truncated":        truncated,
			}
			if answer := strings.TrimSpace(resp.Answer); answer != "" {
				out["answer"] = truncateText(answer, tavilySnippetLimit)
			}
			if resp.ResponseTime > 0 {
				out["response_time_ms"] = int(resp.ResponseTime * 1000)
			}
			return out, nil
		},
	)
}

func (c TavilyClient) Search(ctx context.Context, req tavilySearchRequest) (*tavilySearchResponse, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, fmt.Errorf("tavily API key is not configured")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		baseURL = storage.DefaultTavilyBaseURL
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal tavily request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build tavily request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("tavily upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(respBytes))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("tavily upstream request failed: %s", msg)
	}

	var out tavilySearchResponse
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, fmt.Errorf("decode tavily response: %w", err)
	}
	return &out, nil
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func normalizeSearchDepth(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "advanced", "basic", "fast", "ultra-fast":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return storage.DefaultTavilySearchDepth
	}
}

func normalizeTopic(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "general", "news", "finance":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return storage.DefaultTavilySearchTopic
	}
}

func truncateText(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 || s == "" {
		return s
	}
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:limit])) + "..."
}
