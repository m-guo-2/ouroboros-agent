package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type availableModel struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	ContextLen  *int    `json:"contextLength,omitempty"`
	Description *string `json:"description,omitempty"`
}

func fetchAvailableModels(provider, apiKey, baseURL string) ([]availableModel, error) {
	_ = baseURL // model discovery always uses official provider endpoints
	switch strings.ToLower(provider) {
	case "claude", "anthropic":
		return fetchClaudeModels(apiKey)
	case "openai":
		return fetchOpenAIModels(apiKey)
	case "kimi", "moonshot":
		return fetchKimiModels(apiKey)
	case "glm", "zhipu":
		return fetchGLMModels(apiKey)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func fetchClaudeModels(apiKey string) ([]availableModel, error) {
	base := "https://api.anthropic.com"
	req, _ := http.NewRequest(http.MethodGet, base+"/v1/models?limit=100", nil)
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
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

func fetchOpenAIModels(apiKey string) ([]availableModel, error) {
	base := "https://api.openai.com/v1"
	req, _ := http.NewRequest(http.MethodGet, base+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
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
		if strings.Contains(m.ID, "gpt") || strings.Contains(m.ID, "o1") || strings.Contains(m.ID, "o3") {
			out = append(out, availableModel{ID: m.ID, Name: m.ID, Provider: "openai"})
		}
	}
	return out, nil
}

func fetchKimiModels(apiKey string) ([]availableModel, error) {
	base := "https://api.moonshot.cn/v1"
	req, _ := http.NewRequest(http.MethodGet, base+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
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
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
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
			{ID: "glm-4", Name: "GLM-4", Provider: "glm"},
			{ID: "glm-4-long", Name: "GLM-4 Long", Provider: "glm"},
			{ID: "glm-4-flash", Name: "GLM-4 Flash", Provider: "glm"},
		}
	}
	return out, nil
}
