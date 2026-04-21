package main

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

const tagProfileSync = "profile-sync"

// syncAccountProfile fetches /user/getProfile for one account and mirrors
// the identity fields back to qiwei_accounts. In-memory selfUserID is also
// refreshed so mention-parsing doesn't need a round-trip.
func (a *app) syncAccountProfile(ctx context.Context, rt *accountRuntime, repo *accountRepo) error {
	res, err := rt.client.doAPIRaw(ctx, "/user/getProfile", nil)
	if err != nil {
		return fmt.Errorf("getProfile: %w", err)
	}
	var profile struct {
		UserID    string `json:"userId"`
		Nickname  string `json:"nickname"`
		RealName  string `json:"realName"`
		Alias     string `json:"alias"`
		AvatarURL string `json:"avatarUrl"`
		CorpName  string `json:"corpName"`
	}
	if err := unmarshalSafe(res.Data, &profile); err != nil {
		return fmt.Errorf("parse getProfile: %w", err)
	}
	// Prefer realName (对外显示名) over the nickname when both exist;
	// qiwei user profiles tend to populate both and nickname can be noisy.
	name := firstNonEmpty(decodeMaybeBase64(profile.RealName), decodeMaybeBase64(profile.Nickname))
	upd := ProfileUpdate{
		SelfUserID:    profile.UserID,
		SelfName:      name,
		SelfAlias:     decodeMaybeBase64(profile.Alias),
		SelfAvatarURL: profile.AvatarURL,
		SelfCorpName:  decodeMaybeBase64(profile.CorpName),
	}
	if err := repo.UpdateProfile(ctx, rt.AccountID(), upd); err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	if profile.UserID != "" {
		rt.setSelfUserID(profile.UserID)
	}
	logger.Business(ctx, "同步账号身份完成",
		"tag", tagProfileSync,
		"accountId", rt.AccountID(),
		"selfUserId", profile.UserID,
		"selfName", name,
	)
	return nil
}

// profileSyncState is the per-account backoff ledger used by
// profileSyncLoop. On each attempt we update it under `mu`; on success we
// reset to zero, on failure we bump fails and compute the next attempt.
type profileSyncState struct {
	fails  int
	nextAt time.Time
}

type profileSyncer struct {
	app *app
	mu  sync.Mutex
	// states is keyed by account ID; pruned when the account disappears.
	states map[string]*profileSyncState
}

func newProfileSyncer(a *app) *profileSyncer {
	return &profileSyncer{app: a, states: map[string]*profileSyncState{}}
}

// runOnce attempts a profile sync for every currently-registered account,
// honoring per-account backoff. Returns the number of successes.
func (p *profileSyncer) runOnce(ctx context.Context) int {
	repo := newAccountRepo(p.app.db)
	runtimes := p.app.currentRegistry().All()

	// GC state for accounts that are gone.
	ids := make(map[string]bool, len(runtimes))
	for _, rt := range runtimes {
		ids[rt.AccountID()] = true
	}
	p.mu.Lock()
	for id := range p.states {
		if !ids[id] {
			delete(p.states, id)
		}
	}
	p.mu.Unlock()

	now := time.Now()
	succeeded := 0
	for _, rt := range runtimes {
		if !p.due(rt.AccountID(), now) {
			continue
		}
		callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := p.app.syncAccountProfile(callCtx, rt, repo)
		cancel()
		if err != nil {
			p.recordFailure(rt.AccountID(), now)
			logger.Warn(ctx, "同步账号身份失败",
				"tag", tagProfileSync,
				"accountId", rt.AccountID(),
				"error", err.Error(),
			)
			continue
		}
		p.recordSuccess(rt.AccountID())
		succeeded++
	}
	return succeeded
}

func (p *profileSyncer) due(id string, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	st, ok := p.states[id]
	if !ok {
		return true
	}
	return !now.Before(st.nextAt)
}

func (p *profileSyncer) recordSuccess(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.states[id] = &profileSyncState{fails: 0}
}

func (p *profileSyncer) recordFailure(id string, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.states[id]
	if st == nil {
		st = &profileSyncState{}
		p.states[id] = st
	}
	st.fails++
	st.nextAt = now.Add(backoff(st.fails))
}

// backoff returns min(base * 2^(fails-1), 1h), base = 30s.
func backoff(fails int) time.Duration {
	if fails <= 0 {
		return 0
	}
	base := 30 * time.Second
	max := time.Hour
	shift := fails - 1
	if shift > 20 { // guard against overflow in math.Pow2
		return max
	}
	d := time.Duration(float64(base) * math.Pow(2, float64(shift)))
	if d > max {
		return max
	}
	return d
}

// profileSyncLoop runs p.runOnce at cfg.ProfileSyncInterval cadence until
// ctx is done. Intended to be launched as a goroutine from main.
func (p *profileSyncer) loop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.runOnce(ctx)
		}
	}
}
