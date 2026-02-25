package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"agent-go/internal/types"
)

type LLMResponse struct {
	Content    []types.ContentBlock
	StopReason string
	Usage      struct {
		InputTokens  int
		OutputTokens int
	}
}

type ChatParams struct {
	Messages     []types.AgentMessage
	Tools        []types.ToolDefinition
	SystemPrompt string
	Model        string
}

type LLMClient interface {
	Chat(ctx context.Context, params ChatParams) (*LLMResponse, error)
}

// ==================== Anthropic Native Client ====================

type AnthropicClientConfig struct {
	APIKey    string
	BaseURL   string
	MaxTokens int
}

type AnthropicClient struct {
	apiKey    string
	baseURL   string
	maxTokens int
}

func NewAnthropicClient(config AnthropicClientConfig) *AnthropicClient {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	maxTokens := config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}
	return &AnthropicClient{
		apiKey:    config.APIKey,
		baseURL:   baseURL,
		maxTokens: maxTokens,
	}
}

func (c *AnthropicClient) Chat(ctx context.Context, params ChatParams) (*LLMResponse, error) {
	model := params.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}

	body := map[string]interface{}{
		"model":      model,
		"max_tokens": c.maxTokens,
		"system":     params.SystemPrompt,
		"messages":   params.Messages,
	}

	if len(params.Tools) > 0 {
		body["tools"] = params.Tools
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errText, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Anthropic API %d: %s", resp.StatusCode, string(errText))
	}

	var data struct {
		Content    []types.ContentBlock `json:"content"`
		StopReason string               `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	return &LLMResponse{
		Content:    data.Content,
		StopReason: data.StopReason,
		Usage: struct {
			InputTokens  int
			OutputTokens int
		}{
			InputTokens:  data.Usage.InputTokens,
			OutputTokens: data.Usage.OutputTokens,
		},
	}, nil
}

// ==================== OpenAI Compatible Client ====================

type OpenAICompatibleClientConfig struct {
	APIKey    string
	BaseURL   string
	MaxTokens int
}

type OpenAICompatibleClient struct {
	apiKey    string
	baseURL   string
	maxTokens int
}

func NewOpenAICompatibleClient(config OpenAICompatibleClientConfig) *OpenAICompatibleClient {
	baseURL := strings.TrimSuffix(config.BaseURL, "/")
	maxTokens := config.MaxTokens
	if maxTokens == 0 {
		maxTokens = 8192
	}
	return &OpenAICompatibleClient{
		apiKey:    config.APIKey,
		baseURL:   baseURL,
		maxTokens: maxTokens,
	}
}

func extractSenderName(text string) (string, string) {
	re := regexp.MustCompile(`^\[([^\]]+)\]\s*`)
	loc := re.FindStringSubmatchIndex(text)
	if loc != nil {
		return text[loc[2]:loc[3]], text[loc[1]:]
	}
	return "", text
}

func (c *OpenAICompatibleClient) convertMessages(messages []types.AgentMessage, systemPrompt string) []map[string]interface{} {
	var result []map[string]interface{}

	if systemPrompt != "" {
		result = append(result, map[string]interface{}{
			"role":    "system",
			"content": systemPrompt,
		})
	}

	for _, msg := range messages {
		if msg.Role == "user" {
			for _, block := range msg.Content {
				if block.Type == "text" {
					name, text := extractSenderName(block.Text)
					userMsg := map[string]interface{}{
						"role":    "user",
						"content": text,
					}
					if name != "" {
						userMsg["name"] = name
					}
					result = append(result, userMsg)
				} else if block.Type == "tool_result" {
					result = append(result, map[string]interface{}{
						"role":         "tool",
						"tool_call_id": block.ToolUseID,
						"content":      block.Content,
					})
				}
			}
		} else if msg.Role == "assistant" {
			var textParts []string
			var toolCalls []map[string]interface{}

			for _, block := range msg.Content {
				if block.Type == "text" {
					textParts = append(textParts, block.Text)
				} else if block.Type == "tool_use" {
					args, _ := json.Marshal(block.Input)
					toolCalls = append(toolCalls, map[string]interface{}{
						"id":   block.ID,
						"type": "function",
						"function": map[string]interface{}{
							"name":      block.Name,
							"arguments": string(args),
						},
					})
				}
			}

			assistantMsg := map[string]interface{}{
				"role": "assistant",
			}
			if len(textParts) > 0 {
				assistantMsg["content"] = strings.Join(textParts, "\n")
			} else {
				assistantMsg["content"] = nil
			}
			if len(toolCalls) > 0 {
				assistantMsg["tool_calls"] = toolCalls
			}
			result = append(result, assistantMsg)
		}
	}

	return result
}

func (c *OpenAICompatibleClient) Chat(ctx context.Context, params ChatParams) (*LLMResponse, error) {
	openAIMessages := c.convertMessages(params.Messages, params.SystemPrompt)

	model := params.Model
	if model == "" {
		model = "gpt-4"
	}

	body := map[string]interface{}{
		"model":      model,
		"messages":   openAIMessages,
		"max_tokens": c.maxTokens,
	}

	if len(params.Tools) > 0 {
		var tools []map[string]interface{}
		for _, t := range params.Tools {
			tools = append(tools, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.InputSchema,
				},
			})
		}
		body["tools"] = tools
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errText, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API %d: %s", resp.StatusCode, string(errText))
	}

	var data struct {
		Choices []struct {
			Message struct {
				Content   *string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	if len(data.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI API returned 0 choices")
	}

	choice := data.Choices[0]
	var content []types.ContentBlock

	if choice.Message.Content != nil && *choice.Message.Content != "" {
		content = append(content, types.ContentBlock{
			Type: "text",
			Text: *choice.Message.Content,
		})
	}

	for _, tc := range choice.Message.ToolCalls {
		var input map[string]interface{}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
			input = map[string]interface{}{"raw": tc.Function.Arguments}
		}
		content = append(content, types.ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	if len(content) == 0 {
		content = append(content, types.ContentBlock{
			Type: "text",
			Text: "",
		})
	}

	stopMap := map[string]string{
		"stop":       "end_turn",
		"tool_calls": "tool_use",
		"length":     "max_tokens",
	}

	stopReason := stopMap[choice.FinishReason]
	if stopReason == "" {
		stopReason = "end_turn"
	}

	return &LLMResponse{
		Content:    content,
		StopReason: stopReason,
		Usage: struct {
			InputTokens  int
			OutputTokens int
		}{
			InputTokens:  data.Usage.PromptTokens,
			OutputTokens: data.Usage.CompletionTokens,
		},
	}, nil
}
