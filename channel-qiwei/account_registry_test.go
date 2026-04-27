package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLoadRegistryOnlyIncludesEnabledAccounts(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	repo := newAccountRepo(db)
	ctx := context.Background()

	a1, err := repo.CreateAccount(ctx, Account{GUID: "g1", Token: "t1", Enabled: true})
	if err != nil {
		t.Fatalf("create a1: %v", err)
	}
	a2, err := repo.CreateAccount(ctx, Account{GUID: "g2", Token: "t2", Enabled: true})
	if err != nil {
		t.Fatalf("create a2: %v", err)
	}
	if err := repo.SoftDeleteAccount(ctx, a2.ID); err != nil {
		t.Fatalf("soft delete a2: %v", err)
	}

	reg, err := loadRegistry(ctx, db, Config{APIBaseURL: "http://x", RequestTimout: 1})
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if reg.Count() != 1 {
		t.Fatalf("expected only 1 enabled account in registry, got %d", reg.Count())
	}
	if _, ok := reg.GetByID(a1.ID); !ok {
		t.Fatalf("expected a1 in registry")
	}
	if _, ok := reg.GetByID(a2.ID); ok {
		t.Fatalf("disabled a2 should not be in registry")
	}
	if _, ok := reg.GetByGUID("g2"); ok {
		t.Fatalf("disabled account should not be indexed by guid")
	}
	if _, ok := reg.GetByShortHash(a2.ShortHash); ok {
		t.Fatalf("disabled account should not be indexed by short hash")
	}
}

func TestRegistryDefaultReturnsTrueOnlyForSingleAccount(t *testing.T) {
	cfg := Config{APIBaseURL: "http://x", RequestTimout: 1}
	reg := newAccountRegistry()
	if _, ok := reg.Default(); ok {
		t.Fatalf("empty registry should not have a default")
	}
	reg.add(newAccountRuntime(cfg, nil, Account{ID: "qw_a", GUID: "ga", ShortHash: "hasha"}))
	rt, ok := reg.Default()
	if !ok || rt.AccountID() != "qw_a" {
		t.Fatalf("expected single default = qw_a, got ok=%v rt=%v", ok, rt)
	}
	reg.add(newAccountRuntime(cfg, nil, Account{ID: "qw_b", GUID: "gb", ShortHash: "hashb"}))
	if _, ok := reg.Default(); ok {
		t.Fatalf("multi-account registry should not expose a default")
	}
}

func TestRegistryAtomicSwapIsSafeForConcurrentReaders(t *testing.T) {
	// The app holds registry in an atomic.Pointer; we exercise the pattern
	// directly to prove readers never see a torn snapshot: each reader either
	// sees the old or new runtime, never a mix of maps.
	cfg := Config{APIBaseURL: "http://x", RequestTimout: 1}

	regA := newAccountRegistry()
	regA.add(newAccountRuntime(cfg, nil, Account{ID: "qw_a", GUID: "ga", ShortHash: "hasha"}))
	regB := newAccountRegistry()
	regB.add(newAccountRuntime(cfg, nil, Account{ID: "qw_b", GUID: "gb", ShortHash: "hashb"}))

	var ptr atomic.Pointer[accountRegistry]
	ptr.Store(regA)

	var wg sync.WaitGroup
	readErr := make(chan string, 1)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				if r := ptr.Load(); r == nil || r.Count() != 1 {
					select {
					case readErr <- "registry should always contain exactly 1 account during swap":
					default:
					}
					return
				}
			}
		}()
	}
	for i := 0; i < 50; i++ {
		if i%2 == 0 {
			ptr.Store(regB)
		} else {
			ptr.Store(regA)
		}
	}
	wg.Wait()
	select {
	case msg := <-readErr:
		t.Fatal(msg)
	default:
	}
}
