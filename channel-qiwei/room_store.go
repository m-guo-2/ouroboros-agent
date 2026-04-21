package main

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"time"
)

// roomStore tracks which group rooms this qiwei account has seen.
// Backed by qiwei_known_rooms (per-account) so that the same room seen by two
// accounts is recorded independently (and each account can emit its own
// group_joined event on first sight).
type roomStore struct {
	db        *sql.DB
	accountID string

	mu    sync.Mutex
	rooms map[string]bool
}

func newRoomStore(db *sql.DB, accountID string) *roomStore {
	rs := &roomStore{
		db:        db,
		accountID: accountID,
		rooms:     make(map[string]bool),
	}
	rs.loadFromDB()
	return rs
}

// Add records a room ID. Returns true if this is the first time the account
// sees this room.
func (rs *roomStore) Add(roomID string) bool {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return false
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.rooms[roomID] {
		return false
	}
	rs.rooms[roomID] = true
	rs.persist([]string{roomID})
	return true
}

// Merge records a batch of room IDs (typically from /room/getRoomList).
// The in-memory map is updated and new IDs are persisted in a single statement.
func (rs *roomStore) Merge(roomIDs []string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	var newIDs []string
	for _, id := range roomIDs {
		id = strings.TrimSpace(id)
		if id == "" || rs.rooms[id] {
			continue
		}
		rs.rooms[id] = true
		newIDs = append(newIDs, id)
	}
	if len(newIDs) > 0 {
		rs.persist(newIDs)
	}
}

func (rs *roomStore) loadFromDB() {
	if rs.db == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := rs.db.QueryContext(ctx,
		`SELECT room_id FROM qiwei_known_rooms WHERE account_id = ?`, rs.accountID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil && id != "" {
			rs.rooms[id] = true
		}
	}
}

func (rs *roomStore) persist(ids []string) {
	if rs.db == nil || len(ids) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().Unix()
	tx, err := rs.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO qiwei_known_rooms (account_id, room_id, created_at)
		 VALUES (?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return
	}
	for _, id := range ids {
		if _, err := stmt.ExecContext(ctx, rs.accountID, id, now); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return
		}
	}
	_ = stmt.Close()
	_ = tx.Commit()
}
