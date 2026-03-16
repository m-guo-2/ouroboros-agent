package subagent

import (
	"strings"
	"testing"

	"agent/internal/engine"
	"agent/internal/types"
)

// --- 4.1: buildCanceledResult ---

func TestBuildCanceledResult_WithImpactsAndLoopResult(t *testing.T) {
	impacts := []Impact{
		{Tool: "read_file", Summary: "read file: /tmp/a.go"},
		{Tool: "write_file", Summary: "updated file: /tmp/b.go"},
	}
	loopResult := &engine.AgentLoopResult{
		Messages: []types.AgentMessage{
			{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "task"}}},
			{Role: "assistant", Content: []types.ContentBlock{{Type: "text", Text: "I analyzed the code and found issues."}}},
		},
	}
	result := buildCanceledResult(loopResult, impacts)

	if !strings.HasPrefix(result, "[子任务被中断]") {
		t.Fatalf("expected prefix [子任务被中断], got: %s", result)
	}
	if !strings.Contains(result, "read file: /tmp/a.go") {
		t.Fatal("expected impact 1 in result")
	}
	if !strings.Contains(result, "updated file: /tmp/b.go") {
		t.Fatal("expected impact 2 in result")
	}
	if !strings.Contains(result, "I analyzed the code") {
		t.Fatal("expected last assistant text in result")
	}
}

func TestBuildCanceledResult_NilLoopResult_NoImpacts(t *testing.T) {
	result := buildCanceledResult(nil, nil)

	if !strings.HasPrefix(result, "[子任务被中断]") {
		t.Fatalf("expected prefix [子任务被中断], got: %s", result)
	}
	if !strings.Contains(result, "尚未开始执行") {
		t.Fatal("expected '尚未开始执行' for nil loopResult and no impacts")
	}
}

func TestBuildCanceledResult_NoImpacts_WithLoopResult(t *testing.T) {
	loopResult := &engine.AgentLoopResult{
		Messages: []types.AgentMessage{
			{Role: "assistant", Content: []types.ContentBlock{{Type: "text", Text: "Starting analysis..."}}},
		},
	}
	result := buildCanceledResult(loopResult, nil)

	if !strings.Contains(result, "已执行操作：无") {
		t.Fatal("expected '已执行操作：无'")
	}
	if !strings.Contains(result, "Starting analysis...") {
		t.Fatal("expected last assistant text")
	}
}

func TestBuildCanceledResult_TruncatesLongAssistantText(t *testing.T) {
	longText := strings.Repeat("x", 600)
	loopResult := &engine.AgentLoopResult{
		Messages: []types.AgentMessage{
			{Role: "assistant", Content: []types.ContentBlock{{Type: "text", Text: longText}}},
		},
	}
	result := buildCanceledResult(loopResult, []Impact{{Summary: "did something"}})

	if !strings.Contains(result, "...") {
		t.Fatal("expected truncation indicator '...'")
	}
	if strings.Contains(result, longText) {
		t.Fatal("should not contain full 600-char text")
	}
}

// --- 4.3: filterToolsByProfile excludes subagent tools ---

func TestFilterToolsByProfile_ExcludesSubagentToolsForAllProfiles(t *testing.T) {
	profiles := []string{"developer", "file_analysis", "web_research", "data_report"}
	subagentTools := []string{"run_subagent_async", "get_subagent_status", "cancel_subagent"}

	allTools := []types.RegisteredTool{
		{Definition: types.ToolDefinition{Name: "shell"}},
		{Definition: types.ToolDefinition{Name: "read_file"}},
		{Definition: types.ToolDefinition{Name: "write_file"}},
		{Definition: types.ToolDefinition{Name: "list_dir"}},
		{Definition: types.ToolDefinition{Name: "grep"}},
		{Definition: types.ToolDefinition{Name: "tavily_search"}},
		{Definition: types.ToolDefinition{Name: "recall_context"}},
		{Definition: types.ToolDefinition{Name: "render_card"}},
		{Definition: types.ToolDefinition{Name: "run_subagent_async"}},
		{Definition: types.ToolDefinition{Name: "get_subagent_status"}},
		{Definition: types.ToolDefinition{Name: "cancel_subagent"}},
	}

	for _, profile := range profiles {
		filtered := filterToolsByProfile(profile, allTools)
		for _, tool := range filtered {
			for _, blocked := range subagentTools {
				if tool.Definition.Name == blocked {
					t.Fatalf("profile %q: tool %q should be excluded", profile, blocked)
				}
			}
		}
	}
}

// --- 4.4 + 4.5: buildInitialMessages ---

func TestBuildInitialMessages_TaskOnly(t *testing.T) {
	msgs := buildInitialMessages("", "do something")

	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Fatalf("expected role user, got %s", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content[0].Text, "do something") {
		t.Fatal("expected task text in message")
	}
}

func TestBuildInitialMessages_WithContext(t *testing.T) {
	msgs := buildInitialMessages("user prefers dark theme", "generate a card")

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	contextMsg := msgs[0]
	if contextMsg.Role != "user" {
		t.Fatalf("expected context msg role user, got %s", contextMsg.Role)
	}
	if !strings.HasPrefix(contextMsg.Content[0].Text, "[背景信息]") {
		t.Fatalf("expected context msg to start with [背景信息], got: %s", contextMsg.Content[0].Text)
	}
	if !strings.Contains(contextMsg.Content[0].Text, "user prefers dark theme") {
		t.Fatal("expected context content in message")
	}

	taskMsg := msgs[1]
	if !strings.Contains(taskMsg.Content[0].Text, "generate a card") {
		t.Fatal("expected task text in second message")
	}
}

func TestBuildInitialMessages_WhitespaceContextTreatedAsEmpty(t *testing.T) {
	msgs := buildInitialMessages("   ", "do something")

	if len(msgs) != 1 {
		t.Fatalf("expected 1 message for whitespace context, got %d", len(msgs))
	}
}

// --- 4.6 + 4.7: extractLastAssistantText helper ---

func TestExtractLastAssistantText_Found(t *testing.T) {
	messages := []types.AgentMessage{
		{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "hello"}}},
		{Role: "assistant", Content: []types.ContentBlock{{Type: "text", Text: "first response"}}},
		{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "more"}}},
		{Role: "assistant", Content: []types.ContentBlock{{Type: "text", Text: "last response"}}},
	}
	got := extractLastAssistantText(messages)
	if got != "last response" {
		t.Fatalf("expected 'last response', got %q", got)
	}
}

func TestExtractLastAssistantText_NoAssistant(t *testing.T) {
	messages := []types.AgentMessage{
		{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "hello"}}},
	}
	got := extractLastAssistantText(messages)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}
