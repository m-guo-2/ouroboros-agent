package storage

import (
	"testing"

	"agent/internal/types"
)

func setupCompactionsTestDB(t *testing.T) func() {
	t.Helper()
	SetupTestDB(t)
	return func() {} // global DB is reused across tests; reset handled by SetupTestDB
}

func TestSaveCompactionReturnsID(t *testing.T) {
	cleanup := setupCompactionsTestDB(t)
	defer cleanup()

	id, err := SaveCompaction(CompactionData{
		SessionID:            "sess-1",
		Summary:              "test summary",
		ArchivedBeforeTime:   1000,
		ArchivedMessageCount: 5,
		TokenCountBefore:     10000,
		TokenCountAfter:      5000,
		CompactModel:         "haiku",
	})
	if err != nil {
		t.Fatalf("SaveCompaction: %v", err)
	}
	if id <= 0 {
		t.Fatalf("expected positive ID, got %d", id)
	}

	id2, err := SaveCompaction(CompactionData{
		SessionID: "sess-1",
		Summary:   "second",
	})
	if err != nil {
		t.Fatalf("SaveCompaction 2nd: %v", err)
	}
	if id2 <= id {
		t.Fatalf("expected second ID (%d) > first (%d)", id2, id)
	}
}

func TestSaveCompactionArchive(t *testing.T) {
	cleanup := setupCompactionsTestDB(t)
	defer cleanup()

	compactionID, err := SaveCompaction(CompactionData{
		SessionID: "sess-archive",
		Summary:   "archive test",
	})
	if err != nil {
		t.Fatalf("SaveCompaction: %v", err)
	}

	msgs := []types.AgentMessage{
		{Role: "user", Content: []types.ContentBlock{{Type: "text", Text: "hello"}}},
		{Role: "assistant", Content: []types.ContentBlock{{Type: "text", Text: "hi"}}},
	}

	if err := SaveCompactionArchive(compactionID, "sess-archive", msgs); err != nil {
		t.Fatalf("SaveCompactionArchive: %v", err)
	}

	var count int
	err = DB.QueryRow(
		`SELECT message_count FROM context_compaction_archives WHERE compaction_id = ?`,
		compactionID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query archive: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected message_count=2, got %d", count)
	}
}

func TestGetLatestCompaction(t *testing.T) {
	cleanup := setupCompactionsTestDB(t)
	defer cleanup()

	_, err := SaveCompaction(CompactionData{
		SessionID: "sess-latest",
		Summary:   "first",
	})
	if err != nil {
		t.Fatalf("save first: %v", err)
	}
	_, err = SaveCompaction(CompactionData{
		SessionID: "sess-latest",
		Summary:   "second",
	})
	if err != nil {
		t.Fatalf("save second: %v", err)
	}

	c, err := GetLatestCompaction("sess-latest")
	if err != nil {
		t.Fatalf("GetLatestCompaction: %v", err)
	}
	if c.Summary != "second" {
		t.Fatalf("expected latest summary 'second', got %q", c.Summary)
	}
}
