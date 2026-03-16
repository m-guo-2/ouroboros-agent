package engine

import (
	"context"
	"testing"

	"agent/internal/types"
)

type mockLLMClient struct {
	responses []LLMResponse
	callCount int
}

func (m *mockLLMClient) Chat(ctx context.Context, params ChatParams) (*LLMResponse, error) {
	if m.callCount >= len(m.responses) {
		return &LLMResponse{
			Content:    []types.ContentBlock{{Type: "text", Text: "done"}},
			StopReason: "end_turn",
		}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return &resp, nil
}

func TestCheckpoint1_DrainBeforeLLM(t *testing.T) {
	drainCalls := 0
	drainFunc := func() []types.AgentMessage {
		drainCalls++
		if drainCalls == 1 {
			return []types.AgentMessage{
				{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "new event"}}},
			}
		}
		return nil
	}

	mock := &mockLLMClient{
		responses: []LLMResponse{
			{Content: []types.ContentBlock{{Type: "text", Text: "response"}}, StopReason: "end_turn"},
		},
	}

	result, err := RunAgentLoop(context.Background(), AgentLoopConfig{
		LLMClient:      mock,
		Messages:       []types.AgentMessage{{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "hello"}}}},
		DrainNewEvents: drainFunc,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if drainCalls == 0 {
		t.Fatal("DrainNewEvents was never called")
	}
	if result.FinalText != "response" {
		t.Fatalf("expected 'response', got %q", result.FinalText)
	}
}

func TestCheckpoint2_PreemptToolExecution(t *testing.T) {
	hasNewCalls := 0
	hasNewFunc := func() bool {
		hasNewCalls++
		return hasNewCalls == 1
	}

	drainCalls := 0
	drainFunc := func() []types.AgentMessage {
		drainCalls++
		if drainCalls == 2 {
			return []types.AgentMessage{
				{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "interrupting event"}}},
			}
		}
		return nil
	}

	mock := &mockLLMClient{
		responses: []LLMResponse{
			// First: LLM returns tool_use → will be preempted
			{
				Content: []types.ContentBlock{
					{Type: "tool_use", ID: "t1", Name: "some_tool", Input: map[string]interface{}{"x": 1}},
				},
				StopReason: "tool_use",
			},
			// Second: LLM returns final text after seeing the preemption + drain
			{
				Content:    []types.ContentBlock{{Type: "text", Text: "final after preempt"}},
				StopReason: "end_turn",
			},
		},
	}

	result, err := RunAgentLoop(context.Background(), AgentLoopConfig{
		LLMClient:      mock,
		Messages:       []types.AgentMessage{{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "initial"}}}},
		DrainNewEvents: drainFunc,
		HasNewEvents:   hasNewFunc,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.EventPreempted {
		t.Fatal("expected EventPreempted to be true")
	}
	if result.FinalText != "final after preempt" {
		t.Fatalf("expected 'final after preempt', got %q", result.FinalText)
	}
}

func TestCheckpoint2_LivelockProtection(t *testing.T) {
	mock := &mockLLMClient{
		responses: make([]LLMResponse, 10),
	}
	for i := range mock.responses {
		mock.responses[i] = LLMResponse{
			Content: []types.ContentBlock{
				{Type: "tool_use", ID: "t1", Name: "some_tool", Input: map[string]interface{}{}},
			},
			StopReason: "tool_use",
		}
	}

	// HasNewEvents always returns true → should trigger livelock protection
	hasNewFunc := func() bool { return true }
	drainFunc := func() []types.AgentMessage { return nil }

	result, err := RunAgentLoop(context.Background(), AgentLoopConfig{
		LLMClient:      mock,
		Messages:       []types.AgentMessage{{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "start"}}}},
		Tools:          []types.RegisteredTool{{Definition: types.ToolDefinition{Name: "some_tool"}, Execute: func(ctx context.Context, input map[string]interface{}) (interface{}, error) { return "ok", nil }}},
		DrainNewEvents: drainFunc,
		HasNewEvents:   hasNewFunc,
		MaxIterations:  10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
	// Should not hang — livelock protection allows tools to execute after maxConsecutivePreemptions
	if mock.callCount < 4 {
		t.Fatalf("expected at least 4 LLM calls (3 preemptions + forced execution), got %d", mock.callCount)
	}
}

func TestNilCallbacksBackwardCompatible(t *testing.T) {
	mock := &mockLLMClient{
		responses: []LLMResponse{
			{Content: []types.ContentBlock{{Type: "text", Text: "ok"}}, StopReason: "end_turn"},
		},
	}

	result, err := RunAgentLoop(context.Background(), AgentLoopConfig{
		LLMClient: mock,
		Messages:  []types.AgentMessage{{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "hello"}}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FinalText != "ok" {
		t.Fatalf("expected 'ok', got %q", result.FinalText)
	}
	if result.EventPreempted {
		t.Fatal("EventPreempted should be false with nil callbacks")
	}
}
