package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sharedlogger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

var modelFetchClient = sharedlogger.NewClient("model-discovery", 15*time.Second)

type availableModel struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	ContextLen  *int    `json:"contextLength,omitempty"`
	Description *string `json:"description,omitempty"`
}

func fetchAvailableModels(provider, apiKey, baseURL string) ([]availableModel, error) {
	switch strings.ToLower(provider) {
	case "claude", "anthropic":
		return fetchClaudeModels(apiKey)
	case "openai":
		return fetchOpenAIModels(apiKey, baseURL)
	case "kimi", "moonshot":
		return fetchKimiModels(apiKey)
	case "glm", "zhipu":
		return fetchGLMModels(apiKey)
	case "deepseek":
		return fetchDeepSeekModels(apiKey)
	case "volcengine", "ark":
		return fetchVolcengineModels(apiKey, baseURL)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func fetchClaudeModels(apiKey string) ([]availableModel, error) {
	base := "https://api.anthropic.com"
	req, _ := http.NewRequest(http.MethodGet, base+"/v1/models?limit=100", nil)
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := modelFetchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	var data struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return nil, nil
	}
	var out []availableModel
	for _, m := range data.Data {
		if strings.Contains(m.ID, "claude") {
			name := m.DisplayName
			if name == "" {
				name = m.ID
			}
			out = append(out, availableModel{ID: m.ID, Name: name, Provider: "claude"})
		}
	}
	return out, nil
}

func fetchOpenAIModels(apiKey, baseURL string) ([]availableModel, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	req, _ := http.NewRequest(http.MethodGet, base+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := modelFetchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return nil, nil
	}
	var out []availableModel
	for _, m := range data.Data {
		if strings.Contains(m.ID, "gpt") || strings.Contains(m.ID, "o1") || strings.Contains(m.ID, "o3") || baseURL != "" {
			out = append(out, availableModel{ID: m.ID, Name: m.ID, Provider: "openai"})
		}
	}
	return out, nil
}

func fetchKimiModels(apiKey string) ([]availableModel, error) {
	base := "https://api.moonshot.cn/v1"
	req, _ := http.NewRequest(http.MethodGet, base+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := modelFetchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return nil, nil
	}
	var out []availableModel
	for _, m := range data.Data {
		out = append(out, availableModel{ID: m.ID, Name: m.ID, Provider: "kimi"})
	}
	return out, nil
}

func fetchGLMModels(apiKey string) ([]availableModel, error) {
	base := "https://open.bigmodel.cn/api/paas/v4"
	req, _ := http.NewRequest(http.MethodGet, base+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := modelFetchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return nil, nil
	}
	var out []availableModel
	for _, m := range data.Data {
		out = append(out, availableModel{ID: m.ID, Name: m.ID, Provider: "glm"})
	}
	if len(out) == 0 {
		out = []availableModel{
			{ID: "glm-4-plus", Name: "GLM-4 Plus", Provider: "glm"},
			{ID: "glm-4-flash", Name: "GLM-4 Flash", Provider: "glm"},
			{ID: "glm-4-long", Name: "GLM-4 Long", Provider: "glm"},
			{ID: "glm-4-flashx", Name: "GLM-4 FlashX", Provider: "glm"},
		}
	}
	return out, nil
}

func fetchDeepSeekModels(apiKey string) ([]availableModel, error) {
	base := "https://api.deepseek.com"
	req, _ := http.NewRequest(http.MethodGet, base+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := modelFetchClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return deepSeekFallbackModels(), nil
	}
	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return deepSeekFallbackModels(), nil
	}
	var out []availableModel
	for _, m := range data.Data {
		if strings.Contains(m.ID, "deepseek") {
			out = append(out, availableModel{ID: m.ID, Name: m.ID, Provider: "deepseek"})
		}
	}
	if len(out) == 0 {
		return deepSeekFallbackModels(), nil
	}
	return out, nil
}

func fetchVolcengineModels(apiKey, baseURL string) ([]availableModel, error) {
	if baseURL == "" {
		baseURL = "https://ark.cn-beijing.volces.com/api/v3"
	}
	req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := modelFetchClient.Do(req)
	if err != nil {
		return volcengineFallbackModels(), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return volcengineFallbackModels(), nil
	}
	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&data) != nil {
		return volcengineFallbackModels(), nil
	}
	var out []availableModel
	for _, m := range data.Data {
		out = append(out, availableModel{ID: m.ID, Name: m.ID, Provider: "volcengine"})
	}
	if len(out) == 0 {
		return volcengineFallbackModels(), nil
	}
	return out, nil
}

func volcengineFallbackModels() []availableModel {
	return []availableModel{
		{ID: "doubao-1-5-pro-256k", Name: "Doubao 1.5 Pro 256K", Provider: "volcengine"},
		{ID: "doubao-1-5-pro-32k", Name: "Doubao 1.5 Pro 32K", Provider: "volcengine"},
		{ID: "doubao-1-5-lite-32k", Name: "Doubao 1.5 Lite 32K", Provider: "volcengine"},
		{ID: "doubao-1-5-thinking-pro-250k", Name: "Doubao 1.5 Thinking Pro", Provider: "volcengine"},
	}
}

func deepSeekFallbackModels() []availableModel {
	return []availableModel{
		{ID: "deepseek-chat", Name: "DeepSeek Chat (V3)", Provider: "deepseek"},
		{ID: "deepseek-reasoner", Name: "DeepSeek Reasoner (R1)", Provider: "deepseek"},
	}
}
