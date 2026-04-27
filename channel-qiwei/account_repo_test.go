package main

import (
	"context"
	"errors"
	"testing"
)

// newTestRepo opens a fresh MySQL test database (or skips the test when
// TEST_MYSQL_* env is not configured) and returns a repo bound to it.
func newTestRepo(t *testing.T) *accountRepo {
	t.Helper()
	db := openTestDB(t)
	t.Cleanup(func() { _ = db.Close() })
	return newAccountRepo(db)
}

func TestCreateAccountDefaultsIDAndShortHash(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	acc, err := repo.CreateAccount(ctx, Account{
		GUID:    "abcdef-guid-0001",
		Token:   "secret-token",
		AgentID: "agent-1",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if acc.ID == "" {
		t.Fatalf("expected generated id")
	}
	if acc.ShortHash == "" || len(acc.ShortHash) != 8 {
		t.Fatalf("expected 8-char short hash, got %q", acc.ShortHash)
	}
	if acc.ShortHash != deriveShortHash(acc.GUID) {
		t.Fatalf("short hash should derive from guid")
	}
	if acc.CreatedAt == 0 || acc.UpdatedAt == 0 {
		t.Fatalf("expected timestamps populated")
	}
	if acc.MetaJSON != "{}" {
		t.Fatalf("expected meta_json default to be {}, got %q", acc.MetaJSON)
	}
}

func TestCreateAccountRequiresGUIDAndToken(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if _, err := repo.CreateAccount(ctx, Account{Token: "tok"}); err == nil {
		t.Fatalf("expected error for missing guid")
	}
	if _, err := repo.CreateAccount(ctx, Account{GUID: "g"}); err == nil {
		t.Fatalf("expected error for missing token")
	}
}

func TestCreateAccountRejectsDuplicateGUID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if _, err := repo.CreateAccount(ctx, Account{GUID: "dup-guid", Token: "t1"}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if _, err := repo.CreateAccount(ctx, Account{GUID: "dup-guid", Token: "t2"}); err == nil {
		t.Fatalf("expected duplicate guid to fail UNIQUE constraint")
	}
}

func TestListAccountsOrdersByCreatedAt(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	a1, err := repo.CreateAccount(ctx, Account{GUID: "g1", Token: "t1", CreatedAt: 100})
	if err != nil {
		t.Fatalf("create a1: %v", err)
	}
	a2, err := repo.CreateAccount(ctx, Account{GUID: "g2", Token: "t2", CreatedAt: 200})
	if err != nil {
		t.Fatalf("create a2: %v", err)
	}

	list, err := repo.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].ID != a1.ID || list[1].ID != a2.ID {
		t.Fatalf("unexpected list order: %+v", list)
	}
}

func TestUpdateAccountPatchSubset(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	acc, err := repo.CreateAccount(ctx, Account{
		GUID: "g", Token: "t", Enabled: true, DisplayName: "old",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	newName := "new name"
	updated, err := repo.UpdateAccount(ctx, acc.ID, AccountPatch{
		DisplayName: &newName,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.DisplayName != newName {
		t.Fatalf("display_name not updated: %q", updated.DisplayName)
	}
	if updated.Token != acc.Token {
		t.Fatalf("token should remain unchanged when patch is nil")
	}
	if updated.UpdatedAt < acc.UpdatedAt {
		t.Fatalf("updated_at should advance")
	}
}

func TestUpdateAccountMissingReturnsErrNotFound(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	newName := "x"
	if _, err := repo.UpdateAccount(ctx, "does-not-exist", AccountPatch{DisplayName: &newName}); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestSoftDeleteFlipsEnabledAndIsIdempotent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	acc, err := repo.CreateAccount(ctx, Account{GUID: "g", Token: "t", Enabled: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := repo.SoftDeleteAccount(ctx, acc.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	got, err := repo.GetAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get after soft delete: %v", err)
	}
	if got.Enabled {
		t.Fatalf("expected enabled=false after soft delete")
	}
	// Soft delete on missing row should report ErrAccountNotFound.
	if err := repo.SoftDeleteAccount(ctx, "missing"); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected ErrAccountNotFound on soft delete of missing id, got %v", err)
	}
}

func TestHardDeleteRemovesRowAndKnownRooms(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	acc, err := repo.CreateAccount(ctx, Account{GUID: "g", Token: "t", Enabled: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// seed a known room for this account, ensure hard delete cascades
	rs := newRoomStore(repo.db, acc.ID)
	rs.Add("room-1")

	if err := repo.HardDeleteAccount(ctx, acc.ID); err != nil {
		t.Fatalf("hard delete: %v", err)
	}

	if _, err := repo.GetAccount(ctx, acc.ID); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("expected row gone, got %v", err)
	}

	var count int
	if err := repo.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM qiwei_known_rooms WHERE account_id = ?`, acc.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count rooms: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected known rooms purged, got %d", count)
	}
}

func TestUpdateProfileStoresSelfColumns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	acc, err := repo.CreateAccount(ctx, Account{GUID: "g", Token: "t", Enabled: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	p := ProfileUpdate{
		SelfUserID: "self-1", SelfName: "Alice",
		SelfAlias: "@alice", SelfAvatarURL: "http://img/a.png",
		SelfCorpName: "ACME",
	}
	if err := repo.UpdateProfile(ctx, acc.ID, p); err != nil {
		t.Fatalf("update profile: %v", err)
	}
	got, err := repo.GetAccount(ctx, acc.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.SelfUserID != p.SelfUserID || got.SelfName != p.SelfName ||
		got.SelfAlias != p.SelfAlias || got.SelfAvatarURL != p.SelfAvatarURL ||
		got.SelfCorpName != p.SelfCorpName {
		t.Fatalf("profile fields not stored: %+v", got)
	}
	if got.SelfSyncedAt == 0 {
		t.Fatalf("expected self_synced_at to be set")
	}
}

func TestDeriveShortHashDeterministicAndUnique(t *testing.T) {
	a := deriveShortHash("guid-a")
	b := deriveShortHash("guid-a")
	c := deriveShortHash("guid-b")
	if a != b {
		t.Fatalf("derive should be deterministic")
	}
	if a == c {
		t.Fatalf("different guids should derive different short hashes")
	}
	if len(a) != 8 {
		t.Fatalf("expected 8-char short hash, got %d", len(a))
	}
}
