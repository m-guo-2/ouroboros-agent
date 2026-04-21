package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func newTestContactRepo(t *testing.T) *contactRepo {
	t.Helper()
	accountRepo := newTestRepo(t)
	ctx := context.Background()
	for _, acc := range []Account{
		{ID: "acc-1", GUID: "guid-1", Token: "token-1", Enabled: true},
		{ID: "acc-2", GUID: "guid-2", Token: "token-2", Enabled: true},
	} {
		if _, err := accountRepo.CreateAccount(ctx, acc); err != nil {
			t.Fatalf("seed account %s: %v", acc.ID, err)
		}
	}
	return newContactRepo(accountRepo.db)
}

func TestUpsertContactInsertAndUpdate(t *testing.T) {
	repo := newTestContactRepo(t)
	ctx := context.Background()

	first := Contact{
		AccountID:      "acc-1",
		UserID:         "user-1",
		ExternalUserID: "ext-1",
		Source:         "external",
		Nickname:       "Alice",
		RawJSON:        `{"foo":"bar"}`,
	}
	if err := repo.UpsertContact(ctx, first); err != nil {
		t.Fatalf("insert contact: %v", err)
	}
	inserted, err := repo.GetContact(ctx, "acc-1", "user-1")
	if err != nil {
		t.Fatalf("get inserted contact: %v", err)
	}
	if inserted.FirstSeenAt == 0 || inserted.LastSyncedAt == 0 || inserted.UpdatedAt == 0 {
		t.Fatalf("expected timestamps, got %+v", inserted)
	}

	time.Sleep(time.Second)
	update := Contact{
		AccountID:      "acc-1",
		UserID:         "user-1",
		ExternalUserID: "ext-2",
		Source:         "external",
		Nickname:       "Alice Updated",
		Remark:         "VIP",
		RawJSON:        `{"foo":"baz"}`,
	}
	if err := repo.UpsertContact(ctx, update); err != nil {
		t.Fatalf("update contact: %v", err)
	}
	updated, err := repo.GetContact(ctx, "acc-1", "user-1")
	if err != nil {
		t.Fatalf("get updated contact: %v", err)
	}
	if updated.FirstSeenAt != inserted.FirstSeenAt {
		t.Fatalf("first_seen_at should stay stable, got %d want %d", updated.FirstSeenAt, inserted.FirstSeenAt)
	}
	if updated.ExternalUserID != "ext-2" || updated.Nickname != "Alice Updated" || updated.Remark != "VIP" {
		t.Fatalf("contact fields not updated: %+v", updated)
	}
	if updated.LastSyncedAt < inserted.LastSyncedAt || updated.UpdatedAt < inserted.UpdatedAt {
		t.Fatalf("expected sync timestamps to advance: before=%+v after=%+v", inserted, updated)
	}
}

func TestUpsertIdentityLinkCreateIdempotentAndOverride(t *testing.T) {
	repo := newTestContactRepo(t)
	ctx := context.Background()

	created, err := repo.UpsertIdentityLink(ctx, IdentityLink{
		AccountID:          "acc-1",
		UserID:             "user-1",
		ExternalUserID:     "ext-1",
		DownstreamSystem:   "weapp",
		DownstreamID:       "openid-1",
		DownstreamMetaJSON: `{"appid":"wx123"}`,
	})
	if err != nil {
		t.Fatalf("create identity link: %v", err)
	}
	if !created.Created || created.Link.ID == 0 {
		t.Fatalf("expected create result with id, got %+v", created)
	}

	idempotent, err := repo.UpsertIdentityLink(ctx, IdentityLink{
		AccountID:          "acc-1",
		UserID:             "user-1",
		ExternalUserID:     "ext-1",
		DownstreamSystem:   "weapp",
		DownstreamID:       "openid-1",
		DownstreamMetaJSON: `{"appid":"wx123","scene":"returning"}`,
	})
	if err != nil {
		t.Fatalf("idempotent identity link update: %v", err)
	}
	if idempotent.Created || idempotent.Reassigned {
		t.Fatalf("expected idempotent update, got %+v", idempotent)
	}
	if idempotent.Link.ID != created.Link.ID {
		t.Fatalf("expected same row id, got %d want %d", idempotent.Link.ID, created.Link.ID)
	}

	override, err := repo.UpsertIdentityLink(ctx, IdentityLink{
		AccountID:          "acc-2",
		UserID:             "user-2",
		ExternalUserID:     "ext-2",
		DownstreamSystem:   "weapp",
		DownstreamID:       "openid-1",
		DownstreamMetaJSON: `{"appid":"wx456"}`,
	})
	if err != nil {
		t.Fatalf("override identity link: %v", err)
	}
	if !override.Reassigned || override.Previous == nil {
		t.Fatalf("expected override to report previous row, got %+v", override)
	}
	if override.Previous.ExternalUserID != "ext-1" || override.Link.ExternalUserID != "ext-2" {
		t.Fatalf("expected external user id override, got prev=%+v now=%+v", override.Previous, override.Link)
	}
}

func TestUpsertContactRawJSONGuard(t *testing.T) {
	repo := newTestContactRepo(t)
	ctx := context.Background()

	large := map[string]any{"payload": strings.Repeat("x", maxStoredJSONBytes*2)}
	raw, err := json.Marshal(large)
	if err != nil {
		t.Fatalf("marshal raw payload: %v", err)
	}
	if err := repo.UpsertContact(ctx, Contact{
		AccountID: "acc-1",
		UserID:    "user-1",
		RawJSON:   string(raw),
	}); err != nil {
		t.Fatalf("insert large contact: %v", err)
	}
	got, err := repo.GetContact(ctx, "acc-1", "user-1")
	if err != nil {
		t.Fatalf("get guarded contact: %v", err)
	}
	if !strings.Contains(got.RawJSON, `"truncated":true`) {
		t.Fatalf("expected truncated marker, got %s", got.RawJSON)
	}
}
