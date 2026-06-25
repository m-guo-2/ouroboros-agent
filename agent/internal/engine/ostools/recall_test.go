package ostools

import (
	"context"
	"strings"
	"testing"

	"agent/internal/storage"
	"agent/internal/types"
)

func TestRecallContextSearchesCompactionArchive(t *testing.T) {
	storage.SetupTestDB(t)

	compactionID, err := storage.SaveCompaction(storage.CompactionData{
		SessionID:            "sess-recall-tool",
		Summary:              "用户要求保留事实。",
		ArchivedMessageCount: 2,
	})
	if err != nil {
		t.Fatalf("SaveCompaction: %v", err)
	}

	if err := storage.SaveCompactionArchive(compactionID, "sess-recall-tool", []types.AgentMessage{
		{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "用户消息才是最关键的信息"}}},
		{Role: "assistant", Content: []types.ContentBlock{{Type: "text", Text: "已记录"}}},
	}); err != nil {
		t.Fatalf("SaveCompactionArchive: %v", err)
	}

	result, err := recallExecutor("sess-recall-tool")(context.Background(), map[string]interface{}{
		"query": "最关键的信息",
		"mode":  "search",
	})
	if err != nil {
		t.Fatalf("recallExecutor: %v", err)
	}

	out := result.(map[string]interface{})
	if out["found"] != true {
		t.Fatalf("expected found=true, got %#v", out)
	}
	messages := out["messages"].([]map[string]interface{})
	if len(messages) != 1 {
		t.Fatalf("expected 1 recalled message, got %d", len(messages))
	}
	if messages[0]["role"] != "user" || !strings.Contains(messages[0]["content"].(string), "用户消息才是最关键的信息") {
		t.Fatalf("unexpected recalled message: %#v", messages[0])
	}
}

func TestRecallContextRecentReadsCompactionArchive(t *testing.T) {
	storage.SetupTestDB(t)

	compactionID, err := storage.SaveCompaction(storage.CompactionData{
		SessionID: "sess-recall-recent",
		Summary:   "summary",
	})
	if err != nil {
		t.Fatalf("SaveCompaction: %v", err)
	}

	if err := storage.SaveCompactionArchive(compactionID, "sess-recall-recent", []types.AgentMessage{
		{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "第一条归档消息"}}},
		{Role: "assistant", Content: []types.ContentBlock{
			{Type: "tool_use", Name: "send_channel_message", Input: map[string]interface{}{"content": "发送内容"}},
			{Type: "tool_result", Content: `{"success":true}`},
		}},
	}); err != nil {
		t.Fatalf("SaveCompactionArchive: %v", err)
	}

	result, err := recallExecutor("sess-recall-recent")(context.Background(), map[string]interface{}{
		"query": "anything",
		"mode":  "recent",
	})
	if err != nil {
		t.Fatalf("recallExecutor: %v", err)
	}

	out := result.(map[string]interface{})
	if out["found"] != true {
		t.Fatalf("expected found=true, got %#v", out)
	}
	messages := out["messages"].([]map[string]interface{})
	if len(messages) != 2 {
		t.Fatalf("expected 2 recalled messages, got %d", len(messages))
	}
	if !strings.Contains(messages[1]["content"].(string), "[tool_use: send_channel_message]") {
		t.Fatalf("expected formatted tool_use, got %#v", messages[1])
	}
}
