package eventlog

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"agent/internal/storage"
)

func setupTestDB(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	if err := storage.Init(dbPath); err != nil {
		t.Fatalf("init db: %v", err)
	}
	return func() {
		if storage.DB != nil {
			storage.DB.Close()
			storage.DB = nil
		}
		os.RemoveAll(dir)
	}
}

func saveTestMessage(t *testing.T, sessionID, content string) string {
	t.Helper()
	msg, err := storage.SaveMessage(map[string]interface{}{
		"sessionId": sessionID,
		"role":      "user",
		"content":   content,
	})
	if err != nil {
		t.Fatalf("save message: %v", err)
	}
	return msg.ID
}

func appendEvent(t *testing.T, sessionID, messageID string) {
	t.Helper()
	if err := storage.AppendSessionEvent(sessionID, messageID); err != nil {
		t.Fatalf("append event: %v", err)
	}
}

func TestDrainNewBatchReturnsAll(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	sid := "sess-test"
	createSession(t, sid)

	for i := 0; i < 3; i++ {
		msgID := saveTestMessage(t, sid, "msg")
		appendEvent(t, sid, msgID)
	}

	el := New(sid, 0)

	events, err := el.DrainNew()
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Second drain should return empty
	events2, err := el.DrainNew()
	if err != nil {
		t.Fatalf("drain2: %v", err)
	}
	if len(events2) != 0 {
		t.Fatalf("expected 0 events on second drain, got %d", len(events2))
	}
}

func TestHasNewDoesNotAdvanceCursor(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	sid := "sess-test"
	createSession(t, sid)

	msgID := saveTestMessage(t, sid, "hello")
	appendEvent(t, sid, msgID)

	el := New(sid, 0)

	has, err := el.HasNew()
	if err != nil {
		t.Fatalf("has new: %v", err)
	}
	if !has {
		t.Fatal("expected HasNew to return true")
	}

	// HasNew again should still return true (cursor not moved)
	has2, err := el.HasNew()
	if err != nil {
		t.Fatalf("has new 2: %v", err)
	}
	if !has2 {
		t.Fatal("expected HasNew to still return true")
	}

	// DrainNew should return the event
	events, err := el.DrainNew()
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestCursorRestore(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	sid := "sess-test"
	createSession(t, sid)

	for i := 0; i < 5; i++ {
		msgID := saveTestMessage(t, sid, "msg")
		appendEvent(t, sid, msgID)
	}

	el := New(sid, 0)
	events, _ := el.DrainNew()
	if len(events) != 5 {
		t.Fatalf("expected 5, got %d", len(events))
	}
	savedCursor := el.Cursor()

	// Simulate restart: create new EventLog with saved cursor
	el2 := New(sid, savedCursor)

	// Add 2 more events
	for i := 0; i < 2; i++ {
		msgID := saveTestMessage(t, sid, "new msg")
		appendEvent(t, sid, msgID)
	}

	events2, _ := el2.DrainNew()
	if len(events2) != 2 {
		t.Fatalf("expected 2 events after restore, got %d", len(events2))
	}
}

func TestEmptyDrainAndHasNew(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	sid := "sess-test"
	createSession(t, sid)

	el := New(sid, 0)

	has, _ := el.HasNew()
	if has {
		t.Fatal("expected HasNew false on empty")
	}

	events, _ := el.DrainNew()
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func createSession(t *testing.T, sessionID string) {
	t.Helper()
	_, err := storage.DB.Exec(
		`INSERT OR IGNORE INTO agent_sessions (id, title, created_at, updated_at) VALUES (?, 'test', 0, 0)`,
		sessionID,
	)
	if err != nil && err != sql.ErrNoRows {
		t.Fatalf("create session: %v", err)
	}
}
