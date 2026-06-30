package storage

import (
	"reflect"
	"testing"
)

func TestActiveSessionSkillsPreserveActivationOrder(t *testing.T) {
	cleanup := setupSkillTestDB(t)
	defer cleanup()

	sessionID := "sess-1"
	expected := []string{"skill-b", "skill-a", "skill-c"}
	for _, skillID := range expected {
		if err := ActivateSessionSkill(sessionID, skillID, "hook"); err != nil {
			t.Fatalf("activate %s: %v", skillID, err)
		}
	}

	got, err := GetActiveSessionSkills(sessionID)
	if err != nil {
		t.Fatalf("get active session skills: %v", err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("unexpected skill order: got %v want %v", got, expected)
	}
}

func TestActivateSessionSkillIsIdempotentAndKeepsOriginalOrder(t *testing.T) {
	cleanup := setupSkillTestDB(t)
	defer cleanup()

	sessionID := "sess-2"
	if err := ActivateSessionSkill(sessionID, "skill-a", "hook"); err != nil {
		t.Fatalf("activate skill-a: %v", err)
	}
	if err := ActivateSessionSkill(sessionID, "skill-b", "hook"); err != nil {
		t.Fatalf("activate skill-b: %v", err)
	}
	if err := ActivateSessionSkill(sessionID, "skill-a", "hook"); err != nil {
		t.Fatalf("reactivate skill-a: %v", err)
	}

	got, err := GetActiveSessionSkills(sessionID)
	if err != nil {
		t.Fatalf("get active session skills: %v", err)
	}
	want := []string{"skill-a", "skill-b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected skill order after duplicate activation: got %v want %v", got, want)
	}

	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM session_active_skills WHERE session_id = ?`, sessionID).Scan(&count); err != nil {
		t.Fatalf("count active session skills: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 active skills, got %d", count)
	}
}

func TestExpireSessionSkillsBeforeContext(t *testing.T) {
	cleanup := setupSkillTestDB(t)
	defer cleanup()

	sessionID := "sess-expire"
	if err := ActivateSessionSkillWithOptions(ActivateSessionSkillOptions{
		SessionID:          sessionID,
		SkillID:            "icebreaker",
		Source:             "hook",
		SourceEvent:        "participant_discovered",
		ScopeType:          "participant",
		ScopeKey:           "user-1",
		ActivatedAtSeq:     10,
		ExpiresAfterEvents: 2,
	}); err != nil {
		t.Fatalf("activate skill: %v", err)
	}

	expired, err := ExpireSessionSkillsBeforeContext(sessionID, 11)
	if err != nil {
		t.Fatalf("expire at seq 11: %v", err)
	}
	if expired != 0 {
		t.Fatalf("expired at seq 11 = %d, want 0", expired)
	}

	active, err := GetActiveSessionSkills(sessionID)
	if err != nil {
		t.Fatalf("get active skills: %v", err)
	}
	if !reflect.DeepEqual(active, []string{"icebreaker"}) {
		t.Fatalf("active skills before expiry = %v", active)
	}

	expired, err = ExpireSessionSkillsBeforeContext(sessionID, 12)
	if err != nil {
		t.Fatalf("expire at seq 12: %v", err)
	}
	if expired != 1 {
		t.Fatalf("expired at seq 12 = %d, want 1", expired)
	}

	active, err = GetActiveSessionSkills(sessionID)
	if err != nil {
		t.Fatalf("get active skills after expiry: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active skills after expiry = %v, want empty", active)
	}
}
