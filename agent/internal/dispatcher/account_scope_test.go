package dispatcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestMatchAgentByChannelPrefersExactBindingWithoutDB(t *testing.T) {
	agents := []storage.AgentConfig{
		{
			ID: "agent-wildcard",
			Channels: []storage.ChannelBinding{
				{ChannelType: "qiwei", ChannelIdentifier: "*"},
			},
		},
		{
			ID: "agent-exact",
			Channels: []storage.ChannelBinding{
				{ChannelType: "qiwei", ChannelIdentifier: "accountHash:hash-a"},
			},
		},
	}
	msg := IncomingMessage{
		Channel:                 "qiwei",
		ChannelAccountShortHash: "hash-a",
	}

	if got := matchAgentByChannel(agents, msg, false); got == nil || got.ID != "agent-exact" {
		t.Fatalf("exact match = %+v, want agent-exact", got)
	}
	if got := matchAgentByChannel(agents, IncomingMessage{Channel: "qiwei"}, true); got == nil || got.ID != "agent-wildcard" {
		t.Fatalf("wildcard match = %+v, want agent-wildcard", got)
	}
}

func TestMatchAgentByChannelPrefersAccountIDOverAccountHash(t *testing.T) {
	agents := []storage.AgentConfig{
		{
			ID: "agent-account-hash",
			Channels: []storage.ChannelBinding{
				{ChannelType: "qiwei", ChannelIdentifier: "accountHash:hash-a"},
			},
		},
		{
			ID: "agent-account-id",
			Channels: []storage.ChannelBinding{
				{ChannelType: "qiwei", ChannelIdentifier: "account:qw_a"},
			},
		},
	}
	msg := IncomingMessage{
		Channel:                 "qiwei",
		ChannelAccountID:        "qw_a",
		ChannelAccountShortHash: "hash-a",
	}

	if got := matchAgentByChannel(agents, msg, false); got == nil || got.ID != "agent-account-id" {
		t.Fatalf("match = %+v, want agent-account-id", got)
	}
}

func TestMatchAgentByChannelPrefersAccountHashOverConversationDerivedHash(t *testing.T) {
	agents := []storage.AgentConfig{
		{
			ID: "agent-conversation-hash",
			Channels: []storage.ChannelBinding{
				{ChannelType: "qiwei", ChannelIdentifier: "accountHash:conv-hash"},
			},
		},
		{
			ID: "agent-account-hash",
			Channels: []storage.ChannelBinding{
				{ChannelType: "qiwei", ChannelIdentifier: "accountHash:acct-hash"},
			},
		},
	}
	msg := IncomingMessage{
		Channel:                 "qiwei",
		ChannelAccountShortHash: "acct-hash",
		ChannelConversationID:   "room@conv-hash",
	}

	if got := matchAgentByChannel(agents, msg, false); got == nil || got.ID != "agent-account-hash" {
		t.Fatalf("match = %+v, want agent-account-hash", got)
	}
}

func TestResolveTargetAgentUsesChannelBindingWhenAgentIDMissing(t *testing.T) {
	storage.SetupTestDB(t)

	agent, err := storage.CreateAgentConfig(storage.AgentConfig{
		ID:          "agent-bound-test",
		DisplayName: "Bound Test",
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		IsActive:    true,
		Channels: []storage.ChannelBinding{
			{ChannelType: "qiwei", ChannelIdentifier: "account:qw_a"},
		},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	resolved, err := resolveTargetAgent(IncomingMessage{
		Channel:          "qiwei",
		ChannelAccountID: "qw_a",
	})
	if err != nil {
		t.Fatalf("resolve target agent: %v", err)
	}
	if resolved == nil || resolved.ID != agent.ID {
		t.Fatalf("resolved agent = %+v, want %s", resolved, agent.ID)
	}
}

func TestResolveTargetAgentPrefersExactBindingOverWildcard(t *testing.T) {
	storage.SetupTestDB(t)

	wildcard, err := storage.CreateAgentConfig(storage.AgentConfig{
		ID:          "agent-wildcard-test",
		DisplayName: "Wildcard Test",
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		IsActive:    true,
		Channels: []storage.ChannelBinding{
			{ChannelType: "qiwei", ChannelIdentifier: "*"},
		},
	})
	if err != nil {
		t.Fatalf("create wildcard agent: %v", err)
	}
	exact, err := storage.CreateAgentConfig(storage.AgentConfig{
		ID:          "agent-exact-test",
		DisplayName: "Exact Test",
		Provider:    "openai",
		Model:       "gpt-4o-mini",
		IsActive:    true,
		Channels: []storage.ChannelBinding{
			{ChannelType: "qiwei", ChannelIdentifier: "accountHash:hash-a"},
		},
	})
	if err != nil {
		t.Fatalf("create exact agent: %v", err)
	}

	resolved, err := resolveTargetAgent(IncomingMessage{
		Channel:                 "qiwei",
		ChannelAccountShortHash: "hash-a",
	})
	if err != nil {
		t.Fatalf("resolve target agent: %v", err)
	}
	if resolved == nil || resolved.ID != exact.ID {
		t.Fatalf("resolved agent = %+v, want exact %s over wildcard %s", resolved, exact.ID, wildcard.ID)
	}
}

func TestDispatchUsesBoundAgentPromptForActualLLMRequest(t *testing.T) {
	storage.SetupTestDB(t)

	const promptSentinel = "BOUND_AGENT_PROMPT_SENTINEL"
	requests := make(chan map[string]interface{}, 1)
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode llm request: %v", err)
		} else {
			requests <- body
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{"message":{"content":"done"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1}
		}`))
	}))
	defer llm.Close()

	if err := storage.SetSettingValue("api_key.openai", "test-key"); err != nil {
		t.Fatalf("set api key: %v", err)
	}
	if err := storage.SetSettingValue("base_url.openai", llm.URL); err != nil {
		t.Fatalf("set base url: %v", err)
	}

	agent, err := storage.CreateAgentConfig(storage.AgentConfig{
		ID:           "agent-prompt-request-test",
		DisplayName:  "Prompt Request Test",
		SystemPrompt: promptSentinel,
		Provider:     "openai",
		Model:        "gpt-4o-mini",
		IsActive:     true,
		Channels: []storage.ChannelBinding{
			{ChannelType: "qiwei", ChannelIdentifier: "account:qw_prompt"},
		},
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	result := Dispatch(context.Background(), IncomingMessage{
		Channel:                 "qiwei",
		ChannelAccountID:        "qw_prompt",
		ChannelUserID:           "user-prompt",
		ChannelMessageID:        "msg-prompt",
		ChannelConversationID:   "room-prompt",
		ChannelConversationName: "Prompt Room",
		SenderName:              "Alice",
		Content:                 "hello",
		MessageType:             "text",
	})
	if !result.Success {
		t.Fatalf("dispatch failed: %s", result.Error)
	}

	session, err := storage.GetSession(result.SessionID)
	if err != nil || session == nil {
		t.Fatalf("get session: %v", err)
	}
	if session.AgentID != agent.ID {
		t.Fatalf("session agent = %s, want %s", session.AgentID, agent.ID)
	}

	select {
	case body := <-requests:
		messages, ok := body["messages"].([]interface{})
		if !ok || len(messages) == 0 {
			t.Fatalf("llm messages missing: %+v", body["messages"])
		}
		first, ok := messages[0].(map[string]interface{})
		if !ok {
			t.Fatalf("first llm message has unexpected type: %#v", messages[0])
		}
		if first["role"] != "system" {
			t.Fatalf("first llm message role = %v, want system", first["role"])
		}
		content, _ := first["content"].(string)
		if !strings.Contains(content, promptSentinel) {
			t.Fatalf("system prompt does not contain bound agent prompt %q: %q", promptSentinel, content)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for llm request")
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
		Content:               "GM[gm-user] 2026-07-01 10:00:00:#clear",
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

func TestClearCommandTextHandlesQiweiSenderPrefix(t *testing.T) {
	tests := map[string]string{
		"#clear":                                 "#clear",
		" #clear \n":                             "#clear",
		"GM[gm-user] 2026-07-01 10:00:00:#clear": "#clear",
		"GM 2026-07-01 10:00:00: #clear ":        "#clear",
		"hello #clear":                           "hello #clear",
	}
	for input, want := range tests {
		if got := clearCommandText(input); got != want {
			t.Fatalf("clearCommandText(%q) = %q, want %q", input, got, want)
		}
	}
}
