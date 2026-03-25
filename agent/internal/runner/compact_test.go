package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"agent/internal/engine"
	"agent/internal/storage"
	"agent/internal/types"
)

func msg(role, text string) types.AgentMessage {
	return types.AgentMessage{
		Role:    role,
		Content: []types.ContentBlock{{Type: "text", Text: text}},
	}
}

func toolResultMsg(toolUseID, content string) types.AgentMessage {
	return types.AgentMessage{
		Role: "user",
		Content: []types.ContentBlock{{
			Type:      "tool_result",
			ToolUseID: toolUseID,
			Content:   content,
		}},
	}
}

// --- findRetainStart tests ---

func TestFindRetainStart_Normal(t *testing.T) {
	// 10 user turns, each followed by assistant reply = 20 messages total.
	// Should retain last 5 user turns = last 10 messages, archiveEnd = 10.
	var messages []types.AgentMessage
	for i := 0; i < 10; i++ {
		messages = append(messages, msg("user", "question"))
		messages = append(messages, msg("assistant", "answer"))
	}

	start := findRetainStart(messages, 5, 0.50)
	retained := messages[start:]

	userTurns := 0
	for _, m := range retained {
		if m.Role == "user" {
			userTurns++
		}
	}
	if userTurns != 5 {
		t.Fatalf("expected 5 user turns retained, got %d (start=%d)", userTurns, start)
	}
}

func TestFindRetainStart_50PercentCap(t *testing.T) {
	// 4 user turns + 4 assistant = 8 messages. 5 recent turns would cover all 4,
	// but 50% of 8 = 4, so only the last 2 turns (4 messages) should be retained.
	var messages []types.AgentMessage
	for i := 0; i < 4; i++ {
		messages = append(messages, msg("user", "q"))
		messages = append(messages, msg("assistant", "a"))
	}

	start := findRetainStart(messages, 5, 0.50)
	retainCount := len(messages) - start
	maxAllowed := len(messages) / 2

	if retainCount > maxAllowed {
		t.Fatalf("retained %d messages, exceeds 50%% cap of %d (start=%d)", retainCount, maxAllowed, start)
	}
	if retainCount == 0 {
		t.Fatal("expected at least some retained messages")
	}
}

func TestFindRetainStart_FewerThanNTurns(t *testing.T) {
	// Only 2 user turns = 4 messages. Request 5 turns.
	// Should retain all (no archivable content) since all fits within 50%.
	messages := []types.AgentMessage{
		msg("user", "first"),
		msg("assistant", "a1"),
		msg("user", "second"),
		msg("assistant", "a2"),
	}

	start := findRetainStart(messages, 5, 0.50)
	// With 4 messages, 50% = 2. Two turns = 4 messages > 2, so 50% cap kicks in.
	// The last turn starts at index 2 (2 messages from there), which is exactly 50%.
	retainCount := len(messages) - start
	if retainCount > len(messages)/2+1 {
		t.Fatalf("retained %d messages, should be around 50%% of %d", retainCount, len(messages))
	}
}

func TestFindRetainStart_SkipsToolResultTurns(t *testing.T) {
	// tool_result user messages should not count as turn boundaries
	messages := []types.AgentMessage{
		msg("user", "question 1"),
		msg("assistant", "answer 1"),
		toolResultMsg("t1", "result 1"),
		msg("assistant", "after tool"),
		msg("user", "question 2"),
		msg("assistant", "answer 2"),
	}

	start := findRetainStart(messages, 5, 0.50)
	// Only 2 real user turns. 50% of 6 = 3.
	// Turn 2 starts at index 4 (2 msgs), turn 1 starts at index 0 (6 msgs > 3).
	// So should retain from turn 2 = index 4.
	retainCount := len(messages) - start
	if retainCount > 3 {
		t.Fatalf("retained %d messages, expected ≤3 (50%% of 6)", retainCount)
	}
}

// --- buildSummaryLayer tests ---

func TestBuildSummaryLayer(t *testing.T) {
	layer := buildSummaryLayer(15, "test summary")
	if len(layer) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(layer))
	}
	if layer[0].Role != "user" {
		t.Fatalf("expected user role, got %s", layer[0].Role)
	}
	if layer[1].Role != "assistant" {
		t.Fatalf("expected assistant role, got %s", layer[1].Role)
	}
	text := layer[0].Content[0].Text
	if !contains(text, "[历史上下文摘要]") {
		t.Fatalf("missing summary prefix in: %s", text)
	}
	if !contains(text, "15 条消息已归档") {
		t.Fatalf("missing archive count in: %s", text)
	}
}

// --- buildTaskStateLayer tests ---

func TestBuildTaskStateLayer_WithContent(t *testing.T) {
	layer := buildTaskStateLayer("当前目标: 重构压缩\n\n- [ ] 改 compact.go")
	if len(layer) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(layer))
	}
	if !contains(layer[0].Content[0].Text, "[任务状态]") {
		t.Fatalf("missing task state prefix")
	}
}

func TestBuildTaskStateLayer_Empty(t *testing.T) {
	layer := buildTaskStateLayer("")
	if layer != nil {
		t.Fatalf("expected nil for empty task state, got %d messages", len(layer))
	}

	layer = buildTaskStateLayer("  \n  ")
	if layer != nil {
		t.Fatalf("expected nil for whitespace-only task state")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// --- Mock LLM for integration tests ---

type mockCompactLLM struct {
	summaryText   string
	taskStateText string
	callCount     int
	failOnCall    int // -1 = never fail
}

func (m *mockCompactLLM) Chat(ctx context.Context, params engine.ChatParams) (*engine.LLMResponse, error) {
	m.callCount++
	if m.failOnCall >= 0 && m.callCount == m.failOnCall {
		return nil, fmt.Errorf("mock LLM failure")
	}

	text := "mock response"
	if strings.Contains(params.SystemPrompt, "摘要") {
		text = m.summaryText
	} else if strings.Contains(params.SystemPrompt, "任务提取") {
		text = m.taskStateText
	}

	return &engine.LLMResponse{
		Content:    []types.ContentBlock{{Type: "text", Text: text}},
		StopReason: "end_turn",
	}, nil
}

func setupTestDB(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "compact_test.db")
	if err := storage.Init(dbPath); err != nil {
		t.Fatalf("init db: %v", err)
	}
	return func() {
		if storage.DB != nil {
			_ = storage.DB.Close()
			storage.DB = nil
		}
	}
}

func buildLongConversation(userTurns int) []types.AgentMessage {
	// Each message ~2000 chars ≈ 500 tokens. 15 turns × 2 msgs × 500 = 15000 tokens.
	// For gpt-4 (8192 window), 15000 > 8192×0.60 = 4915 → triggers compression.
	filler := strings.Repeat("这是一段用于填充上下文的中文文本，模拟真实对话场景。", 40)
	var msgs []types.AgentMessage
	for i := 0; i < userTurns; i++ {
		msgs = append(msgs,
			msg("user", fmt.Sprintf("Question %d: %s", i+1, filler)),
			msg("assistant", fmt.Sprintf("Answer %d: %s", i+1, filler)),
		)
	}
	return msgs
}

// --- Integration tests ---

func TestCompactContext_FullPipeline(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	sessionID := "test-full-pipeline"

	_, _ = storage.SaveSessionFacts(sessionID, []string{
		"用户的项目使用 Go + SQLite",
		"Agent 系统有上下文压缩功能",
		"用户偏好中文回复",
	}, "context")

	messages := buildLongConversation(15)

	mockLLM := &mockCompactLLM{
		summaryText: "1. **目标/项目**：用户在重构上下文压缩\n2. **当前状态**：完成了存储层\n" +
			"3. **关键决策**：选了固定锚点策略\n4. **失败/回退**：无\n5. **未完成事项**：subagent 适配",
		taskStateText: "当前目标: 重构上下文压缩机制\n\n未完成任务:\n- [ ] 改 compact.go (来源: user)\n- [x] 改 storage 层 (已完成)",
		failOnCall:    -1,
	}

	result, err := CompactContext(
		context.Background(),
		messages, "gpt-4", mockLLM, "gpt-4o-mini", sessionID,
	)
	if err != nil {
		t.Fatalf("CompactContext: %v", err)
	}
	if !result.Compacted {
		t.Fatal("expected compaction to occur")
	}

	allText := ""
	for _, m := range result.Messages {
		for _, b := range m.Content {
			allText += b.Text + "\n"
		}
	}

	if !contains(allText, "[历史上下文摘要]") {
		t.Error("missing summary layer")
	}
	if !contains(allText, "[任务状态]") {
		t.Error("missing task state layer")
	}
	if !contains(allText, "[Agent Memory]") {
		t.Error("missing agent memory layer")
	}

	// Verify four-layer ordering: summary first, then retained, then task state, then agent memory
	summaryIdx := strings.Index(allText, "[历史上下文摘要]")
	taskIdx := strings.Index(allText, "[任务状态]")
	memoryIdx := strings.Index(allText, "[Agent Memory]")
	if summaryIdx >= taskIdx || taskIdx >= memoryIdx {
		t.Errorf("layers out of order: summary@%d, task@%d, memory@%d", summaryIdx, taskIdx, memoryIdx)
	}

	// Verify archive was saved
	compaction, err := storage.GetLatestCompaction(sessionID)
	if err != nil {
		t.Fatalf("GetLatestCompaction: %v", err)
	}
	if compaction.ArchivedMessageCount == 0 {
		t.Error("expected archived messages > 0")
	}

	// Verify archive backup exists
	var archiveCount int
	err = storage.DB.QueryRow(
		`SELECT COUNT(*) FROM context_compaction_archives WHERE session_id = ?`, sessionID,
	).Scan(&archiveCount)
	if err != nil {
		t.Fatalf("query archive: %v", err)
	}
	if archiveCount != 1 {
		t.Errorf("expected 1 archive record, got %d", archiveCount)
	}
}

func TestCompactContext_NoSessionFacts(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	sessionID := "test-no-facts"
	messages := buildLongConversation(15)

	mockLLM := &mockCompactLLM{
		summaryText:   "Summary of conversation",
		taskStateText: "NO_TASKS",
		failOnCall:    -1,
	}

	result, err := CompactContext(
		context.Background(),
		messages, "gpt-4", mockLLM, "gpt-4o-mini", sessionID,
	)
	if err != nil {
		t.Fatalf("CompactContext: %v", err)
	}

	allText := ""
	for _, m := range result.Messages {
		for _, b := range m.Content {
			allText += b.Text + "\n"
		}
	}

	if !contains(allText, "[历史上下文摘要]") {
		t.Error("missing summary layer")
	}
	if contains(allText, "[任务状态]") {
		t.Error("task state layer should not be present (NO_TASKS)")
	}
	if contains(allText, "[Agent Memory]") {
		t.Error("agent memory layer should not be present (no facts)")
	}
}

func TestCompactContext_SummaryLLMFailure(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	sessionID := "test-summary-fail"
	messages := buildLongConversation(15)

	// Fail on call 1 (summary) and call 2 (task extraction)
	mockLLM := &mockCompactLLM{failOnCall: 1}

	result, err := CompactContext(
		context.Background(),
		messages, "gpt-4", mockLLM, "gpt-4o-mini", sessionID,
	)
	if err != nil {
		t.Fatalf("CompactContext: %v", err)
	}

	// Should still compact using fallback summary
	if !result.Compacted {
		t.Fatal("expected compaction even with LLM failure")
	}

	allText := ""
	for _, m := range result.Messages {
		for _, b := range m.Content {
			allText += b.Text + "\n"
		}
	}

	if !contains(allText, "[历史上下文摘要]") {
		t.Error("missing summary layer (should use fallback)")
	}
	if !contains(allText, "recall_context") {
		t.Error("fallback summary should mention recall_context")
	}
}

func TestExtractTaskState_NoTasks(t *testing.T) {
	mockLLM := &mockCompactLLM{
		taskStateText: "NO_TASKS",
		failOnCall:    -1,
	}

	result := ExtractTaskState(context.Background(), []types.AgentMessage{
		msg("user", "hello"),
		msg("assistant", "hi"),
	}, mockLLM, "haiku")

	if result != "" {
		t.Fatalf("expected empty result for NO_TASKS, got %q", result)
	}
}

func TestExtractTaskState_NilLLM(t *testing.T) {
	result := ExtractTaskState(context.Background(), []types.AgentMessage{
		msg("user", "hello"),
	}, nil, "")

	if result != "" {
		t.Fatalf("expected empty result with nil LLM, got %q", result)
	}
}
