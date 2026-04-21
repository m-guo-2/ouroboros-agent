package main

import (
	"database/sql"
	"sync"
	"time"
)

// accountRuntime is the per-account state that used to live on *app.
// Everything that depends on "which qiwei account we're talking to" lives
// here: the API client (guid/token), the known-rooms set, name cache,
// msgSvrId dedupe window, self-profile cache, and contact sync bookkeeping.
//
// accountRuntime is read-only after construction: registry reloads replace
// the whole runtime atomically instead of mutating a live one. The few
// mutable fields (selfUserID, contactsLoadedAt) are guarded by their own
// mutex; we never share them across accounts.
type accountRuntime struct {
	account Account

	client    *qiweiClient
	roomStore *roomStore
	nameCache *ttlCache
	dedupe    *ttlSet
	gateway   *ContactGateway

	selfMu     sync.RWMutex
	selfUserID string

	contactsMu       sync.Mutex
	contactsLoadedAt time.Time
}

// newAccountRuntime builds an accountRuntime from a DB row + process config.
// The runtime does NOT touch the network here; identity/contacts are warmed
// lazily or via background sync.
func newAccountRuntime(cfg Config, db *sql.DB, acc Account) *accountRuntime {
	rt := &accountRuntime{
		account:    acc,
		client:     newQiweiClient(cfg, acc.GUID, acc.Token),
		roomStore:  newRoomStore(db, acc.ID),
		nameCache:  newTTLCache(5 * time.Minute),
		dedupe:     newTTLSet(10 * time.Minute),
		selfUserID: acc.SelfUserID,
	}
	rt.gateway = newContactGateway(db, acc.ID, rt.nameCache, newQiWeProtoSource(rt.client))
	return rt
}

// AccountID is a convenience accessor used by logs & registry.
func (rt *accountRuntime) AccountID() string { return rt.account.ID }

// GUID is used to route incoming webhook events back to their runtime.
func (rt *accountRuntime) GUID() string { return rt.account.GUID }

// ShortHash is the 8-char suffix embedded in channelConversationId.
func (rt *accountRuntime) ShortHash() string { return rt.account.ShortHash }

// AgentID is the Agent that owns this qiwei account (N:1 binding).
func (rt *accountRuntime) AgentID() string { return rt.account.AgentID }

// SelfUserID returns the cached self user id, falling back to the account
// snapshot if the in-memory copy has not been populated yet.
func (rt *accountRuntime) SelfUserID() string {
	rt.selfMu.RLock()
	v := rt.selfUserID
	rt.selfMu.RUnlock()
	if v != "" {
		return v
	}
	return rt.account.SelfUserID
}

func (rt *accountRuntime) setSelfUserID(id string) {
	rt.selfMu.Lock()
	rt.selfUserID = id
	rt.selfMu.Unlock()
}

// channelIdentity returns a display-only identity descriptor. The caller
// (forwardToAgent path) attaches this to incomingMessage so agents can render
// the account's nickname / corp name without routing on them.
func (rt *accountRuntime) channelIdentity() *channelIdentity {
	acc := rt.account
	if acc.DisplayName == "" && acc.SelfName == "" && acc.SelfUserID == "" &&
		acc.SelfAlias == "" && acc.SelfCorpName == "" {
		return nil
	}
	selfID := rt.SelfUserID()
	if selfID == "" {
		selfID = acc.SelfUserID
	}
	return &channelIdentity{
		DisplayName: acc.DisplayName,
		Self: channelIdentitySelf{
			UserID:   selfID,
			Name:     acc.SelfName,
			Alias:    acc.SelfAlias,
			CorpName: acc.SelfCorpName,
		},
	}
}
