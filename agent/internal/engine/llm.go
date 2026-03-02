package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"agent/internal/logger"
	"net/http"
	"regexp"
	"strings"

	"agent/internal/types"
)

type LLMResponse struct {
	Content    []types.ContentBlock
	StopReason string
	Usage      struct {
		InputTokens  int
		OutputTokens int
	}
	RawRequest  json.RawMessage // original JSON sent to LLM API
	RawResponse json.RawMessage // original JSON received from LLM API
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

// sanitizeMessagesForAnthropic removes tool_result blocks with empty or orphaned tool_use_id
// to avoid "tool_call_id is not found" API errors. tool_use_id must match assistant's id exactly
// (use API's original value, no transformation).
// Uses a two-pass approach: first collect all valid tool_use IDs from assistants, then filter
// tool_results, so message order cannot cause valid tool_results to be wrongly dropped.
func sanitizeMessagesForAnthropic(ctx context.Context, messages []types.AgentMessage) []types.AgentMessage {
	validToolUseIDs := make(map[string]bool)
	for _, msg := range messages {
		if msg.Role == "assistant" {
			for _, b := range msg.Content {
				if b.Type == "tool_use" && b.ID != "" {
					validToolUseIDs[b.ID] = true
				}
			}
		}
	}

	var result []types.AgentMessage
	var dropped []struct{ ToolUseID string `json:"toolUseID"`; Reason string `json:"reason"` }

	for _, msg := range messages {
		if msg.Role == "assistant" {
			result = append(result, msg)
		} else if msg.Role == "user" {
			var filtered []types.ContentBlock
			for _, b := range msg.Content {
				if b.Type != "tool_result" {
					filtered = append(filtered, b)
					continue
				}
				if b.ToolUseID == "" {
					dropped = append(dropped, struct{ ToolUseID string `json:"toolUseID"`; Reason string `json:"reason"` }{"(empty)", "empty tool_use_id"})
					continue
				}
				if validToolUseIDs[b.ToolUseID] {
					filtered = append(filtered, b)
				} else {
					dropped = append(dropped, struct{ ToolUseID string `json:"toolUseID"`; Reason string `json:"reason"` }{b.ToolUseID, "orphaned (no matching tool_use in history)"})
				}
			}
			if len(filtered) == 0 {
				filtered = []types.ContentBlock{{Type: "text", Text: "[Tool results omitted – references invalid or truncated]"}}
			}
			result = append(result, types.AgentMessage{Role: "user", Content: filtered})
		}
	}

	if len(dropped) > 0 {
		logger.Warn(ctx, "Anthropic 消息清洗：已过滤无效 tool_result",
			"droppedCount", len(dropped),
			"dropped", dropped,
			"validToolUseIDs", keys(validToolUseIDs))
	}

	return result
}

func keys(m map[string]bool) []string {
	var k []string
	for s := range m {
		k = append(k, s)
	}
	return k
}

// BuildToolUseDiagnostic returns a compact digest of tool_use/tool_result ID wiring for debugging.
func BuildToolUseDiagnostic(messages []types.AgentMessage) map[string]interface{} {
	var toolUseIDs []string
	var toolResultRefs []string
	var emptyToolUse, emptyToolResult int

	for _, msg := range messages {
		for _, b := range msg.Content {
			if b.Type == "tool_use" {
				if b.ID == "" {
					emptyToolUse++
				} else {
					toolUseIDs = append(toolUseIDs, b.ID)
				}
			} else if b.Type == "tool_result" {
				if b.ToolUseID == "" {
					emptyToolResult++
				} else {
					toolResultRefs = append(toolResultRefs, b.ToolUseID)
				}
			}
		}
	}

	return map[string]interface{}{
		"messageCount":     len(messages),
		"toolUseIDs":       toolUseIDs,
		"toolResultRefs":   toolResultRefs,
		"emptyToolUse":     emptyToolUse,
		"emptyToolResult":  emptyToolResult,
	}
}

func (c *AnthropicClient) Chat(ctx context.Context, params ChatParams) (*LLMResponse, error) {
	model := params.Model
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}

	sanitized := sanitizeMessagesForAnthropic(ctx, params.Messages)
	body := map[string]interface{}{
		"model":      model,
		"max_tokens": c.maxTokens,
		"system":     params.SystemPrompt,
		"messages":   sanitized,
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Anthropic API read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg := string(respBody)
		if strings.Contains(errMsg, "tool_call_id") || strings.Contains(errMsg, "tool_use_id") {
			logger.Error(ctx, "Anthropic API tool_call_id 错误，发送的消息诊断",
				"status", resp.StatusCode,
				"rawError", errMsg,
				"diagnostic", BuildToolUseDiagnostic(sanitized))
		}
		return nil, fmt.Errorf("Anthropic API %d: %s", resp.StatusCode, errMsg)
	}

	var data struct {
		Content    []types.ContentBlock `json:"content"`
		StopReason string               `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &data); err != nil {
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
		RawRequest:  json.RawMessage(reqBody),
		RawResponse: json.RawMessage(respBody),
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
	re := regexp.MustCompile(`^\[([^\]]+)\]\n?`)
	loc := re.FindStringSubmatchIndex(text)
	if loc == nil {
		return "", text
	}

	raw := text[loc[2]:loc[3]]
	rest := text[loc[1]:]

	// Format: "SenderName (channelUserId)" — extract just the display name
	if idx := strings.Index(raw, " ("); idx > 0 {
		return raw[:idx], rest
	}
	return raw, rest
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("OpenAI API read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenAI API %d: %s", resp.StatusCode, string(respBody))
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

	if err := json.Unmarshal(respBody, &data); err != nil {
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
		RawRequest:  json.RawMessage(reqBody),
		RawResponse: json.RawMessage(respBody),
	}, nil
}
