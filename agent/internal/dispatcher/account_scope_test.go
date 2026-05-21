package dispatcher

import "testing"

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
