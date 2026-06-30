package dispatcher

import (
	"context"
	"reflect"
	"testing"

	"agent/internal/storage"
)

func TestResolveChannelAccountScopePrefersExplicitAccountID(t *testing.T) {
	msg := IncomingMessage{
		Channel:                 "qiwei",
		ChannelAccountID:        "qw_a",
		ChannelAccountShortHash: "hash-b",
		ChannelConversationID:   "user@hash-c",
	}

	if got, want := resolveChannelAccountScope(msg), "account:qw_a"; got != want {
		t.Fatalf("scope = %q, want %q", got, want)
	}
}

func TestResolveChannelAccountScopeFallsBackToConversationSuffix(t *testing.T) {
	msg := IncomingMessage{
		Channel:               "qiwei",
		ChannelConversationID: "room-1@63aop6x2",
	}

	if got, want := resolveChannelAccountScope(msg), "accountHash:63aop6x2"; got != want {
		t.Fatalf("scope = %q, want %q", got, want)
	}
}

func TestResolveChannelUserKeyIsolatesSameUserAcrossAccounts(t *testing.T) {
	a := IncomingMessage{Channel: "qiwei", ChannelAccountID: "qw_a", ChannelUserID: "user-1"}
	b := IncomingMessage{Channel: "qiwei", ChannelAccountID: "qw_b", ChannelUserID: "user-1"}

	if resolveChannelUserKey(a) == resolveChannelUserKey(b) {
		t.Fatalf("expected account-scoped user keys to differ")
	}
}

func TestResolveDedupeKeyIsolatesSameMessageAcrossAccounts(t *testing.T) {
	a := IncomingMessage{Channel: "qiwei", ChannelAccountID: "qw_a", ChannelMessageID: "1001"}
	b := IncomingMessage{Channel: "qiwei", ChannelAccountID: "qw_b", ChannelMessageID: "1001"}

	if resolveDedupeKey(a) == resolveDedupeKey(b) {
		t.Fatalf("expected account-scoped dedupe keys to differ")
	}
}

func TestDispatchActivatesParticipantDiscoveredHookForSessionParticipant(t *testing.T) {
	storage.SetupTestDB(t)

	agent, err := storage.CreateAgentConfig(storage.AgentConfig{
		ID:          "agent-hook-test",
		DisplayName: "Hook Test",
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		IsActive:    true,
		Hooks: []storage.Hook{
			{
				Event: "participant_discovered",
				Actions: []storage.HookAction{
					{Type: "activate_skill", SkillID: "icebreaker", ScopeType: "participant", ExpiresAfterEvents: 3},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	result := Dispatch(context.Background(), IncomingMessage{
		Channel:               "webui",
		ChannelUserID:         "new-user-1",
		ChannelMessageID:      "msg-1",
		ChannelConversationID: "room-1",
		SenderName:            "New User",
		Content:               "hello",
		MessageType:           "text",
		AgentID:               agent.ID,
	})
	if !result.Success {
		t.Fatalf("dispatch failed: %s", result.Error)
	}

	activeSkills, err := storage.GetActiveSessionSkills(result.SessionID)
	if err != nil {
		t.Fatalf("get active skills: %v", err)
	}
	if !reflect.DeepEqual(activeSkills, []string{"icebreaker"}) {
		t.Fatalf("active skills = %v, want [icebreaker]", activeSkills)
	}

	messages, err := storage.GetSessionMessages(result.SessionID, 10)
	if err != nil {
		t.Fatalf("get session messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected one saved user message, got %d", len(messages))
	}
	discovery, ok := messages[0].ChannelMeta["participantDiscovery"].(map[string]any)
	if !ok {
		t.Fatalf("participantDiscovery meta missing: %+v", messages[0].ChannelMeta)
	}
	if discovery["relation"] != "first_seen" {
		t.Fatalf("relation = %v, want first_seen", discovery["relation"])
	}
}

func TestDispatchGroupClearCommandClearsSessionWithoutSavingCommand(t *testing.T) {
	storage.SetupTestDB(t)

	agent, err := storage.CreateAgentConfig(storage.AgentConfig{
		ID:          "agent-clear-test",
		DisplayName: "Clear Test",
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		IsActive:    true,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	session, err := storage.CreateSession(map[string]interface{}{
		"id":                    "sess-clear-test",
		"agentId":               agent.ID,
		"userId":                "user-1",
		"channel":               "qiwei",
		"sessionKey":            "qiwei:room-1",
		"channelConversationId": "room-1",
		"channelName":           "Test Room",
		"title":                 "Test Room",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	saved, err := storage.SaveMessage(map[string]interface{}{
		"sessionId":        session.ID,
		"role":             "user",
		"content":          "hello",
		"messageType":      "text",
		"channel":          "qiwei",
		"channelMessageId": "msg-before",
		"initiator":        "user",
		"senderName":       "Alice",
		"senderId":         "user-1",
	})
	if err != nil {
		t.Fatalf("seed message: %v", err)
	}
	if err := storage.AppendSessionEvent(session.ID, saved.ID); err != nil {
		t.Fatalf("seed session event: %v", err)
	}
	if err := storage.UpdateSession(session.ID, map[string]interface{}{
		"context":         "old context",
		"eventCursor":     99,
		"workDir":         "/tmp/agent-sessions/" + session.ID,
		"executionStatus": "processing",
	}); err != nil {
		t.Fatalf("seed session state: %v", err)
	}
	if _, err := storage.SaveSessionFacts(session.ID, []string{"old memory"}, "test"); err != nil {
		t.Fatalf("seed facts: %v", err)
	}

	clear := Dispatch(context.Background(), IncomingMessage{
		Channel:               "qiwei",
		ChannelUserID:         "gm-user",
		ChannelMessageID:      "msg-clear",
		ChannelConversationID: "room-1",
		ConversationType:      "group",
		SenderName:            "GM",
		Content:               "#clear",
		MessageType:           "text",
		AgentID:               agent.ID,
	})
	if !clear.Success {
		t.Fatalf("clear dispatch failed: %s", clear.Error)
	}
	if clear.SessionID != session.ID {
		t.Fatalf("clear session = %s, want %s", clear.SessionID, session.ID)
	}

	messages, err := storage.GetSessionMessages(session.ID, 10)
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected all messages cleared, got %d", len(messages))
	}
	facts, err := storage.GetSessionFacts(session.ID)
	if err != nil {
		t.Fatalf("get facts: %v", err)
	}
	if len(facts) != 0 {
		t.Fatalf("expected facts cleared, got %d", len(facts))
	}
	clearedSession, err := storage.GetSession(session.ID)
	if err != nil || clearedSession == nil {
		t.Fatalf("get session: %v", err)
	}
	if clearedSession.Context != "" || clearedSession.EventCursor != 0 || clearedSession.WorkDir != "" || clearedSession.ExecutionStatus != "idle" {
		t.Fatalf("session was not reset: %+v", clearedSession)
	}
}
