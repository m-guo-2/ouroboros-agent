package runner

import (
	"encoding/json"
	"strings"
	"sync"

	"agent/internal/types"

	"github.com/tiktoken-go/tokenizer"
)

var modelContextWindows = map[string]int{
	"claude-opus-4-5":            200000,
	"claude-sonnet-4-5":          200000,
	"claude-3-5-sonnet-20241022": 200000,
	"claude-3-5-haiku-20241022":  200000,
	"claude-3-haiku-20240307":    200000,
	"gpt-4o":                     128000,
	"gpt-4o-mini":                128000,
	"gpt-4-turbo":                128000,
	"gpt-4":                      8192,
	"gpt-3.5-turbo":              16384,
}

const defaultContextWindow = 128000

func GetContextWindow(model string) int {
	if w, ok := modelContextWindows[model]; ok {
		return w
	}
	if strings.Contains(model, "claude") {
		return 200000
	}
	return defaultContextWindow
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
