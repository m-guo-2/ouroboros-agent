package main

import (
	"errors"
	"strings"
	"testing"
)

func TestEncodeConversationID(t *testing.T) {
	cases := []struct {
		name      string
		rawID     string
		shortHash string
		want      string
	}{
		{"empty raw returns empty", "", "abcd", ""},
		{"no shortHash returns raw", "user-1", "", "user-1"},
		{"normal composition", "user-1", "abcdef12", "user-1@abcdef12"},
		{"raw with @ stays intact and suffix added", "room@weixin", "abcdef12", "room@weixin@abcdef12"},
		{"trims whitespace", "  user-1  ", "  abcd  ", "user-1@abcd"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := encodeConversationID(c.rawID, c.shortHash)
			if got != c.want {
				t.Fatalf("encodeConversationID(%q,%q) = %q, want %q", c.rawID, c.shortHash, got, c.want)
			}
		})
	}
}

func TestDecodeConversationID(t *testing.T) {
	cases := []struct {
		name          string
		composite     string
		wantRaw       string
		wantShortHash string
	}{
		{"empty", "", "", ""},
		{"no suffix", "user-1", "user-1", ""},
		{"normal", "user-1@abcdef12", "user-1", "abcdef12"},
		{"raw contains @ - split on last", "room@weixin@abcdef12", "room@weixin", "abcdef12"},
		{"trims outer whitespace", "  user-1@abcd  ", "user-1", "abcd"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rawID, shortHash := decodeConversationID(c.composite)
			if rawID != c.wantRaw || shortHash != c.wantShortHash {
				t.Fatalf("decodeConversationID(%q) = (%q,%q), want (%q,%q)",
					c.composite, rawID, shortHash, c.wantRaw, c.wantShortHash)
			}
		})
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	rawIDs := []string{"user-1", "room@weixin", "R:123"}
	hashes := []string{"abcdef12", "12345678"}
	for _, raw := range rawIDs {
		for _, h := range hashes {
			enc := encodeConversationID(raw, h)
			gotRaw, gotHash := decodeConversationID(enc)
			if gotRaw != raw || gotHash != h {
				t.Fatalf("round trip mismatch: raw=%q hash=%q -> enc=%q -> (%q,%q)",
					raw, h, enc, gotRaw, gotHash)
			}
		}
	}
}

func TestResolveOutgoingTargetRules(t *testing.T) {
	a1 := Account{ID: "qw_a", GUID: "guid-a", ShortHash: "hashaaaa", Enabled: true}
	a2 := Account{ID: "qw_b", GUID: "guid-b", ShortHash: "hashbbbb", Enabled: true}

	newReg := func(accs ...Account) *accountRegistry {
		reg := newAccountRegistry()
		cfg := Config{APIBaseURL: "http://example", RequestTimout: 1}
		for _, acc := range accs {
			rt := newAccountRuntime(cfg, nil, acc)
			reg.add(rt)
		}
		return reg
	}

	t.Run("single account + no suffix -> default route", func(t *testing.T) {
		reg := newReg(a1)
		rt, raw, err := resolveOutgoingTarget(reg, "user-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rt.AccountID() != a1.ID || raw != "user-1" {
			t.Fatalf("got rt=%s raw=%q, want rt=%s raw=user-1", rt.AccountID(), raw, a1.ID)
		}
	})

	t.Run("single account + correct suffix", func(t *testing.T) {
		reg := newReg(a1)
		rt, raw, err := resolveOutgoingTarget(reg, "user-1@hashaaaa")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rt.AccountID() != a1.ID || raw != "user-1" {
			t.Fatalf("got rt=%s raw=%q", rt.AccountID(), raw)
		}
	})

	t.Run("multi account + suffix picks the right runtime", func(t *testing.T) {
		reg := newReg(a1, a2)
		rt, raw, err := resolveOutgoingTarget(reg, "room-1@hashbbbb")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rt.AccountID() != a2.ID || raw != "room-1" {
			t.Fatalf("got rt=%s raw=%q, want rt=%s raw=room-1", rt.AccountID(), raw, a2.ID)
		}
	})

	t.Run("multi account + no suffix -> ErrAmbiguousAccount", func(t *testing.T) {
		reg := newReg(a1, a2)
		_, _, err := resolveOutgoingTarget(reg, "user-1")
		if !errors.Is(err, ErrAmbiguousAccount) {
			t.Fatalf("expected ErrAmbiguousAccount, got %v", err)
		}
	})

	t.Run("unknown shortHash -> ErrUnknownAccount", func(t *testing.T) {
		reg := newReg(a1, a2)
		_, _, err := resolveOutgoingTarget(reg, "user-1@deadbeef")
		if !errors.Is(err, ErrUnknownAccount) {
			t.Fatalf("expected ErrUnknownAccount, got %v", err)
		}
	})

	t.Run("empty composite is rejected", func(t *testing.T) {
		reg := newReg(a1)
		_, _, err := resolveOutgoingTarget(reg, "")
		if err == nil || !strings.Contains(err.Error(), "channelConversationId") {
			t.Fatalf("expected channelConversationId-required error, got %v", err)
		}
	})
}

func TestResolveByAccountID(t *testing.T) {
	a1 := Account{ID: "qw_a", GUID: "guid-a", ShortHash: "hashaaaa", Enabled: true}
	cfg := Config{APIBaseURL: "http://example", RequestTimout: 1}
	reg := newAccountRegistry()
	reg.add(newAccountRuntime(cfg, nil, a1))

	t.Run("found", func(t *testing.T) {
		rt, err := resolveByAccountID(reg, "qw_a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rt.AccountID() != "qw_a" {
			t.Fatalf("got %s", rt.AccountID())
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := resolveByAccountID(reg, "qw_missing")
		if !errors.Is(err, ErrUnknownAccount) {
			t.Fatalf("expected ErrUnknownAccount, got %v", err)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		_, err := resolveByAccountID(reg, "  ")
		if err == nil {
			t.Fatalf("expected error for empty id")
		}
	})
}
