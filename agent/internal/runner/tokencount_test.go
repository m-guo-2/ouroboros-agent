package runner

import (
	"testing"

	"agent/internal/types"
)

func TestGetContextWindowForCommonModels(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		want     int
	}{
		{name: "openai gpt-5", provider: "openai", model: "gpt-5.5", want: 1000000},
		{name: "openai gpt-5 mini", provider: "openai", model: "gpt-5.4-mini", want: 400000},
		{name: "openai gpt-4.1", provider: "openai", model: "gpt-4.1", want: 1000000},
		{name: "openai gpt-4o", provider: "openai", model: "gpt-4o-mini", want: 128000},
		{name: "claude opus 4.8", provider: "claude", model: "claude-opus-4-8", want: 1000000},
		{name: "claude sonnet 4.5", provider: "claude", model: "claude-sonnet-4-5", want: 200000},
		{name: "deepseek v4", provider: "deepseek", model: "deepseek-v4-pro", want: 1000000},
		{name: "deepseek legacy alias", provider: "deepseek", model: "deepseek-chat", want: 65536},
		{name: "kimi k2.6", provider: "kimi", model: "kimi-k2.6", want: 256000},
		{name: "kimi k2.5", provider: "kimi", model: "kimi-k2.5", want: 256000},
		{name: "kimi moonshot 128k", provider: "moonshot", model: "moonshot-v1-128k", want: 131072},
		{name: "volcengine doubao 256k", provider: "volcengine", model: "doubao-1-5-pro-256k", want: 256000},
		{name: "volcengine doubao seed", provider: "volcengine", model: "doubao-seed-2-1-pro", want: 256000},
		{name: "volcengine seedance alias", provider: "volcengine", model: "seeddance", want: 256000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetContextWindowForProvider(tt.provider, tt.model); got != tt.want {
				t.Fatalf("window = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEstimateRequestTokensIncludesPromptToolsAndOutputReserve(t *testing.T) {
	messages := []types.AgentMessage{{
		Role:    "user",
		Content: []types.ContentBlock{{Type: "text", Text: "帮我检查上下文压缩"}},
	}}
	tools := []types.ToolDefinition{{
		Name:        "send_channel_message",
		Description: "向当前会话发送消息",
		InputSchema: types.JSONSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"content": map[string]interface{}{"type": "string"},
			},
		},
	}}

	withoutTools := EstimateRequestTokens("openai", "gpt-4o-mini", "system prompt", nil, messages, 0)
	withTools := EstimateRequestTokens("openai", "gpt-4o-mini", "system prompt", tools, messages, 8192)

	if withTools.ContextWindow != 128000 {
		t.Fatalf("context window = %d, want 128000", withTools.ContextWindow)
	}
	if withTools.Tokens <= withoutTools.Tokens {
		t.Fatalf("expected tools and reserved output to increase token estimate: with=%d without=%d", withTools.Tokens, withoutTools.Tokens)
	}
	if withTools.Method != "request" {
		t.Fatalf("method = %q, want request", withTools.Method)
	}
}
