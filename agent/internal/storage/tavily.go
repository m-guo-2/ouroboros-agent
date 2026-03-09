package storage

import (
	"strconv"
	"strings"
)

const (
	DefaultTavilyBaseURL     = "https://api.tavily.com"
	DefaultTavilySearchDepth = "basic"
	DefaultTavilySearchTopic = "general"
	DefaultTavilyMaxResults  = 5
)

type TavilyConfig struct {
	APIKey      string
	BaseURL     string
	Enabled     bool
	SearchDepth string
	SearchTopic string
	MaxResults  int
}

func GetTavilyConfig() (*TavilyConfig, error) {
	apiKey, err := GetSettingValue("api_key.tavily")
	if err != nil {
		return nil, err
	}
	baseURL, err := GetSettingValue("base_url.tavily")
	if err != nil {
		return nil, err
	}
	enabledRaw, err := GetSettingValue("enabled.tavily")
	if err != nil {
		return nil, err
	}
	searchDepth, err := GetSettingValue("tavily.search_depth")
	if err != nil {
		return nil, err
	}
	searchTopic, err := GetSettingValue("tavily.search_topic")
	if err != nil {
		return nil, err
	}
	maxResultsRaw, err := GetSettingValue("tavily.max_results")
	if err != nil {
		return nil, err
	}

	cfg := &TavilyConfig{
		APIKey:      strings.TrimSpace(apiKey),
		BaseURL:     strings.TrimSpace(baseURL),
		Enabled:     parseBoolSetting(enabledRaw),
		SearchDepth: normalizeTavilySearchDepth(searchDepth),
		SearchTopic: normalizeTavilySearchTopic(searchTopic),
		MaxResults:  normalizeTavilyMaxResults(maxResultsRaw),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultTavilyBaseURL
	}
	return cfg, nil
}

func WithDefaultSettings(values map[string]string) map[string]string {
	out := make(map[string]string, len(values)+5)
	for k, v := range values {
		out[k] = v
	}
	if strings.TrimSpace(out["base_url.tavily"]) == "" {
		out["base_url.tavily"] = DefaultTavilyBaseURL
	}
	if _, ok := out["enabled.tavily"]; !ok {
		out["enabled.tavily"] = "false"
	}
	if strings.TrimSpace(out["tavily.search_depth"]) == "" {
		out["tavily.search_depth"] = DefaultTavilySearchDepth
	}
	if strings.TrimSpace(out["tavily.search_topic"]) == "" {
		out["tavily.search_topic"] = DefaultTavilySearchTopic
	}
	if strings.TrimSpace(out["tavily.max_results"]) == "" {
		out["tavily.max_results"] = strconv.Itoa(DefaultTavilyMaxResults)
	}
	return out
}

func parseBoolSetting(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizeTavilySearchDepth(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "advanced", "basic", "fast", "ultra-fast":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return DefaultTavilySearchDepth
	}
}

func normalizeTavilySearchTopic(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "general", "news", "finance":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return DefaultTavilySearchTopic
	}
}

func normalizeTavilyMaxResults(v string) int {
	if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
		if n > 20 {
			return 20
		}
		return n
	}
	return DefaultTavilyMaxResults
}
