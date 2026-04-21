package main

import (
	"testing"
)

// Per-account dedupe is one of the core guarantees of the multi-account
// refactor: two qiwei accounts that happen to observe the same MsgSvrID
// (e.g. a service account broadcasting into two sub-corps) must each run
// the message through its own handlers exactly once.
//
// The old code used a single global ttlSet on *app. This test guards the
// invariant that each accountRuntime now owns its own dedupe window.
func TestDedupeIsolatedPerAccount(t *testing.T) {
	cfg := Config{APIBaseURL: "http://x", RequestTimout: 1}
	rt1 := newAccountRuntime(cfg, nil, Account{ID: "qw_a", GUID: "ga", ShortHash: "hasha"})
	rt2 := newAccountRuntime(cfg, nil, Account{ID: "qw_b", GUID: "gb", ShortHash: "hashb"})

	msgID := "shared-msg-svr-id"

	if rt1.dedupe.Seen(msgID) {
		t.Fatalf("rt1 first sighting must not be flagged as duplicate")
	}
	if rt2.dedupe.Seen(msgID) {
		t.Fatalf("rt2 first sighting must not be flagged as duplicate "+
			"(dedupe should be per-account, not shared); got=%v", true)
	}

	if !rt1.dedupe.Seen(msgID) {
		t.Fatalf("rt1 second sighting must be flagged as duplicate")
	}
	if !rt2.dedupe.Seen(msgID) {
		t.Fatalf("rt2 second sighting must be flagged as duplicate")
	}
}

// Sanity check: nameCache / roomStore pointers differ per account so we
// can't accidentally share state at construction time.
func TestAccountRuntimeIsolatedResources(t *testing.T) {
	cfg := Config{APIBaseURL: "http://x", RequestTimout: 1}
	rt1 := newAccountRuntime(cfg, nil, Account{ID: "qw_a", GUID: "ga", ShortHash: "hasha"})
	rt2 := newAccountRuntime(cfg, nil, Account{ID: "qw_b", GUID: "gb", ShortHash: "hashb"})

	if rt1.dedupe == rt2.dedupe {
		t.Fatalf("dedupe must not be shared across accounts")
	}
	if rt1.nameCache == rt2.nameCache {
		t.Fatalf("nameCache must not be shared across accounts")
	}
	if rt1.roomStore == rt2.roomStore {
		t.Fatalf("roomStore must not be shared across accounts")
	}
	if rt1.client == rt2.client {
		t.Fatalf("qiweiClient must not be shared across accounts")
	}
}
