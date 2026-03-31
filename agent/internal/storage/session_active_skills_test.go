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
