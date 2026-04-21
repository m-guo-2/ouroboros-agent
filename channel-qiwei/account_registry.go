package main

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// accountRegistry is an immutable snapshot of all active accounts, indexed
// three ways so routing is O(1) regardless of which key we have in hand.
// Hot reloads create a brand-new registry and swap the pointer atomically —
// we never mutate a live registry in place.
type accountRegistry struct {
	mu          sync.RWMutex
	all         []*accountRuntime
	byID        map[string]*accountRuntime
	byGUID      map[string]*accountRuntime
	byShortHash map[string]*accountRuntime
}

func newAccountRegistry() *accountRegistry {
	return &accountRegistry{
		byID:        map[string]*accountRuntime{},
		byGUID:      map[string]*accountRuntime{},
		byShortHash: map[string]*accountRuntime{},
	}
}

// loadRegistry builds a registry from DB rows. Only enabled accounts are
// indexed; disabled rows remain in the DB but are invisible to routing.
func loadRegistry(ctx context.Context, db *sql.DB, cfg Config) (*accountRegistry, error) {
	repo := newAccountRepo(db)
	accounts, err := repo.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	reg := newAccountRegistry()
	for _, acc := range accounts {
		if !acc.Enabled {
			continue
		}
		rt := newAccountRuntime(cfg, db, acc)
		reg.add(rt)
	}
	return reg, nil
}

func (r *accountRegistry) add(rt *accountRuntime) {
	r.all = append(r.all, rt)
	r.byID[rt.AccountID()] = rt
	r.byGUID[rt.GUID()] = rt
	if sh := rt.ShortHash(); sh != "" {
		r.byShortHash[sh] = rt
	}
}

// GetByGUID returns the runtime that owns a given qiwei guid; webhook
// callbacks use this to route inbound events.
func (r *accountRegistry) GetByGUID(guid string) (*accountRuntime, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.byGUID[guid]
	return rt, ok
}

// GetByShortHash is used by the downstream send path to map the suffix of
// channelConversationId back to an account.
func (r *accountRegistry) GetByShortHash(sh string) (*accountRuntime, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.byShortHash[sh]
	return rt, ok
}

// GetByID is used by the admin API.
func (r *accountRegistry) GetByID(id string) (*accountRuntime, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.byID[id]
	return rt, ok
}

// All returns a snapshot slice of runtimes. The returned slice is safe to
// iterate without holding the registry lock (registry is immutable after
// construction; the caller receives a copy).
func (r *accountRegistry) All() []*accountRuntime {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*accountRuntime, len(r.all))
	copy(out, r.all)
	return out
}

// Count returns the number of enabled accounts currently routed.
func (r *accountRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.all)
}

// Default returns the unique runtime when exactly one account is registered,
// and (nil, false) otherwise. Downstream callers use this to stay compatible
// with the legacy single-account API that has no channelConversationId.
func (r *accountRegistry) Default() (*accountRuntime, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.all) == 1 {
		return r.all[0], true
	}
	return nil, false
}
