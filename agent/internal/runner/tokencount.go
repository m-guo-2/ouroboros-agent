package runner

import (
	"encoding/json"
	"strings"
	"sync"

	"agent/internal/types"

	"github.com/tiktoken-go/tokenizer"
)

const (
	tokenizerCl100k = "cl100k_base"
	tokenizerO200k  = "o200k_base"
)

var modelContextWindows = map[string]int{
	"claude-opus-4-8":              1000000,
	"claude-opus-4-6":              1000000,
	"claude-opus-4-5":              200000,
	"claude-sonnet-4-5":            200000,
	"claude-3-5-sonnet-20241022":   200000,
	"claude-3-5-haiku-20241022":    200000,
	"claude-3-haiku-20240307":      200000,
	"gpt-5.5":                      1000000,
	"gpt-5.4":                      1000000,
	"gpt-5.4-mini":                 400000,
	"gpt-5.4-nano":                 400000,
	"gpt-4.1":                      1000000,
	"gpt-4.1-mini":                 1000000,
	"gpt-4.1-nano":                 1000000,
	"gpt-4o":                       128000,
	"gpt-4o-mini":                  128000,
	"gpt-4-turbo":                  128000,
	"gpt-4":                        8192,
	"gpt-3.5-turbo":                16384,
	"deepseek-v4-pro":              1000000,
	"deepseek-v4-flash":            1000000,
	"deepseek-chat":                65536,
	"deepseek-reasoner":            65536,
	"kimi-k2.7-code":               256000,
	"kimi-k2.7-code-highspeed":     256000,
	"kimi-k2.6":                    256000,
	"kimi-k2.5":                    256000,
	"kimi-k2":                      128000,
	"moonshot-v1-8k":               8192,
	"moonshot-v1-32k":              32768,
	"moonshot-v1-128k":             131072,
	"glm-4-long":                   1000000,
	"glm-4-plus":                   128000,
	"glm-4-flash":                  128000,
	"glm-4-flashx":                 128000,
	"doubao-seed-2-1-pro":          256000,
	"doubao-seed-2-1-turbo":        256000,
	"doubao-seed-1-8":              256000,
	"doubao-1-5-pro-256k":          256000,
	"doubao-1-5-pro-32k":           32768,
	"doubao-1-5-lite-32k":          32768,
	"doubao-1-5-thinking-pro-250k": 250000,
	"seedance":                     256000,
	"seeddance":                    256000,
}

const defaultContextWindow = 128000

func GetContextWindow(model string) int {
	if w, ok := modelContextWindows[model]; ok {
		return w
	}
	if strings.Contains(model, "claude") {
		return 200000
	}
	if strings.HasPrefix(model, "gpt-5") || strings.HasPrefix(model, "gpt-4.1") {
		return 1000000
	}
	if strings.HasPrefix(model, "gpt-4o") {
		return 128000
	}
	if strings.Contains(model, "deepseek") {
		if strings.Contains(model, "v4") {
			return 1000000
		}
		return 65536
	}
	if strings.Contains(model, "kimi-k2.7") || strings.Contains(model, "kimi-k2.6") || strings.Contains(model, "kimi-k2.5") {
		return 256000
	}
	if strings.Contains(model, "kimi-k2") {
		return 128000
	}
	if strings.Contains(model, "moonshot-v1-128k") {
		return 131072
	}
	if strings.Contains(model, "moonshot-v1-32k") {
		return 32768
	}
	if strings.Contains(model, "moonshot-v1-8k") {
		return 8192
	}
	if strings.Contains(model, "doubao-seed") || strings.Contains(model, "seedance") || strings.Contains(model, "seeddance") {
		return contextWindowFromModelSuffix(model, 256000)
	}
	if strings.Contains(model, "doubao") {
		return contextWindowFromModelSuffix(model, defaultContextWindow)
	}
	return defaultContextWindow
}

func GetContextWindowForProvider(provider, model string) int {
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.ToLower(strings.TrimSpace(model))
	if window := GetContextWindow(m); window != defaultContextWindow || p == "" {
		return window
	}
	switch p {
	case "claude", "anthropic":
		return 200000
	case "openai":
		return defaultContextWindow
	case "deepseek":
		if strings.Contains(m, "v4") {
			return 1000000
		}
		return 65536
	case "kimi", "moonshot":
		if strings.Contains(m, "k2.7") || strings.Contains(m, "k2.6") || strings.Contains(m, "k2.5") {
			return 256000
		}
		return 128000
	case "glm", "zhipu":
		if strings.Contains(m, "long") {
			return 1000000
		}
		return 128000
	case "volcengine", "ark":
		if strings.Contains(m, "seed") {
			return contextWindowFromModelSuffix(m, 256000)
		}
		return contextWindowFromModelSuffix(m, defaultContextWindow)
	default:
		return defaultContextWindow
	}
}

func contextWindowFromModelSuffix(model string, fallback int) int {
	switch {
	case strings.Contains(model, "256k"):
		return 256000
	case strings.Contains(model, "250k"):
		return 250000
	case strings.Contains(model, "128k"):
		return 128000
	case strings.Contains(model, "64k"):
		return 65536
	case strings.Contains(model, "32k"):
		return 32768
	case strings.Contains(model, "16k"):
		return 16384
	case strings.Contains(model, "8k"):
		return 8192
	default:
		return fallback
	}
}

func QuickEstimateTokens(messages []types.AgentMessage) int {
	b, err := json.Marshal(messages)
	if err != nil {
		return 0
	}
	return len(b) / 4
}

var (
	tiktokenOnce sync.Once
	tiktokenEnc  tokenizer.Codec
	encoderMu    sync.Mutex
	encoders     = map[string]tokenizer.Codec{}
)

func getEncoder() tokenizer.Codec {
	tiktokenOnce.Do(func() {
		enc, err := tokenizer.Get(tokenizer.Cl100kBase)
		if err != nil {
			return
		}
		tiktokenEnc = enc
	})
	return tiktokenEnc
}

func getEncoderForProvider(provider, model string) (tokenizer.Codec, string) {
	kind := tokenizerKind(provider, model)

	encoderMu.Lock()
	defer encoderMu.Unlock()
	if enc := encoders[kind]; enc != nil {
		return enc, kind
	}

	var encoding tokenizer.Encoding
	switch kind {
	case tokenizerO200k:
		encoding = tokenizer.O200kBase
	default:
		encoding = tokenizer.Cl100kBase
	}
	enc, err := tokenizer.Get(encoding)
	if err != nil {
		return nil, kind
	}
	encoders[kind] = enc
	return enc, kind
}

func tokenizerKind(provider, model string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.ToLower(strings.TrimSpace(model))
	if p == "openai" || strings.HasPrefix(m, "gpt-") || strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4") {
		if strings.HasPrefix(m, "gpt-4o") ||
			strings.HasPrefix(m, "gpt-4.1") ||
			strings.HasPrefix(m, "gpt-5") ||
			strings.HasPrefix(m, "o1") ||
			strings.HasPrefix(m, "o3") ||
			strings.HasPrefix(m, "o4") {
			return tokenizerO200k
		}
		return tokenizerCl100k
	}
	return tokenizerCl100k
}

func PreciseEstimateTokens(messages []types.AgentMessage) int {
	enc := getEncoder()
	if enc == nil {
		return QuickEstimateTokens(messages)
	}

	total := 0
	for _, msg := range messages {
		total += 4 // per-message overhead: role + formatting
		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				total += countTokens(enc, block.Text)
			case "tool_use":
				total += countTokens(enc, block.Name)
				if block.Input != nil {
					b, _ := json.Marshal(block.Input)
					total += countTokens(enc, string(b))
				}
			case "tool_result":
				total += countTokens(enc, block.Content)
			}
		}
	}
	return total
}

func EstimateContentTokens(provider, model string, text string) int {
	enc, _ := getEncoderForProvider(provider, model)
	if enc == nil {
		return len(text) / 4
	}
	return countTokens(enc, text)
}

func PreciseEstimateTokensForProvider(messages []types.AgentMessage, provider, model string) int {
	enc, _ := getEncoderForProvider(provider, model)
	if enc == nil {
		return QuickEstimateTokens(messages)
	}

	total := 0
	for _, msg := range messages {
		total += 4
		for _, block := range msg.Content {
			switch block.Type {
			case "text":
				total += countTokens(enc, block.Text)
			case "tool_use":
				total += countTokens(enc, block.Name)
				if block.Input != nil {
					b, _ := json.Marshal(block.Input)
					total += countTokens(enc, string(b))
				}
			case "tool_result":
				total += countTokens(enc, block.Content)
			}
		}
	}
	return total
}

func countTokens(enc tokenizer.Codec, text string) int {
	ids, _, _ := enc.Encode(text)
	return len(ids)
}

type TokenEstimate struct {
	Tokens        int
	ContextWindow int
	Ratio         float64 // tokens / contextWindow
	Method        string  // "quick" or "precise"
}

func EstimateTokens(messages []types.AgentMessage, model string) TokenEstimate {
	contextWindow := GetContextWindow(model)

	quick := QuickEstimateTokens(messages)
	lowThreshold := float64(contextWindow) * 0.45
	highThreshold := float64(contextWindow) * 0.75

	if float64(quick) < lowThreshold || float64(quick) > highThreshold {
		return TokenEstimate{
			Tokens:        quick,
			ContextWindow: contextWindow,
			Ratio:         float64(quick) / float64(contextWindow),
			Method:        "quick",
		}
	}

	precise := PreciseEstimateTokens(messages)
	return TokenEstimate{
		Tokens:        precise,
		ContextWindow: contextWindow,
		Ratio:         float64(precise) / float64(contextWindow),
		Method:        "precise",
	}
}

func EstimateRequestTokens(
	provider string,
	model string,
	systemPrompt string,
	tools []types.ToolDefinition,
	messages []types.AgentMessage,
	reservedOutputTokens int,
) TokenEstimate {
	contextWindow := GetContextWindowForProvider(provider, model)
	if reservedOutputTokens < 0 {
		reservedOutputTokens = 0
	}

	systemTokens := EstimateContentTokens(provider, model, systemPrompt)
	toolTokens := EstimateContentTokens(provider, model, marshalForTokenEstimate(tools))
	messageTokens := PreciseEstimateTokensForProvider(messages, provider, model)
	providerOverhead := 8 + len(messages)*4
	tokens := systemTokens + toolTokens + messageTokens + providerOverhead + reservedOutputTokens

	return TokenEstimate{
		Tokens:        tokens,
		ContextWindow: contextWindow,
		Ratio:         float64(tokens) / float64(contextWindow),
		Method:        "request",
	}
}

func marshalForTokenEstimate(v interface{}) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
