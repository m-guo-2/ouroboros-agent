package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// OpenDB opens (or creates) the channel-qiwei SQLite database and
// executes its schema.
func OpenDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// SQLite is single-writer; WAL mode allows concurrent readers.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("exec %q: %w", p, err)
		}
	}

	if err := runSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run schema: %w", err)
	}
	return db, nil
}

func runSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS qiwei_accounts (
			id              TEXT PRIMARY KEY,
			guid            TEXT NOT NULL,
			token           TEXT NOT NULL,
			short_hash      TEXT NOT NULL,
			display_name    TEXT NOT NULL DEFAULT '',
			agent_id        TEXT NOT NULL DEFAULT '',
			enabled         INTEGER NOT NULL DEFAULT 1,
			self_user_id    TEXT NOT NULL DEFAULT '',
			self_name       TEXT NOT NULL DEFAULT '',
			self_alias      TEXT NOT NULL DEFAULT '',
			self_avatar_url TEXT NOT NULL DEFAULT '',
			self_corp_name  TEXT NOT NULL DEFAULT '',
			self_synced_at  INTEGER NOT NULL DEFAULT 0,
			meta_json       TEXT NOT NULL DEFAULT '{}',
			notes           TEXT NOT NULL DEFAULT '',
			created_at      INTEGER NOT NULL DEFAULT 0,
			updated_at      INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_qiwei_accounts_guid
			ON qiwei_accounts(guid)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_qiwei_accounts_short_hash
			ON qiwei_accounts(short_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_qiwei_accounts_agent
			ON qiwei_accounts(agent_id)`,

		`CREATE TABLE IF NOT EXISTS qiwei_known_rooms (
			account_id TEXT NOT NULL,
			room_id    TEXT NOT NULL,
			created_at INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (account_id, room_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_qiwei_known_rooms_account
			ON qiwei_known_rooms(account_id)`,

		`CREATE TABLE IF NOT EXISTS qiwei_contacts (
			account_id       TEXT NOT NULL,
			user_id          TEXT NOT NULL,
			external_user_id TEXT NOT NULL DEFAULT '',
			source           TEXT NOT NULL DEFAULT 'unknown',
			nickname         TEXT NOT NULL DEFAULT '',
			real_name        TEXT NOT NULL DEFAULT '',
			alias            TEXT NOT NULL DEFAULT '',
			remark           TEXT NOT NULL DEFAULT '',
			avatar_url       TEXT NOT NULL DEFAULT '',
			gender           TEXT NOT NULL DEFAULT '',
			corp_id          TEXT NOT NULL DEFAULT '',
			corp_name        TEXT NOT NULL DEFAULT '',
			follow_user_json TEXT NOT NULL DEFAULT '[]',
			raw_json         TEXT NOT NULL DEFAULT '{}',
			first_seen_at    INTEGER NOT NULL DEFAULT 0,
			last_synced_at   INTEGER NOT NULL DEFAULT 0,
			updated_at       INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (account_id, user_id),
			FOREIGN KEY (account_id) REFERENCES qiwei_accounts(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_qiwei_contacts_external_user
			ON qiwei_contacts(account_id, external_user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_qiwei_contacts_name_lookup
			ON qiwei_contacts(account_id, nickname, real_name, alias, remark)`,

		`CREATE TABLE IF NOT EXISTS qiwei_rooms (
			account_id     TEXT NOT NULL,
			room_id        TEXT NOT NULL,
			name           TEXT NOT NULL DEFAULT '',
			announcement   TEXT NOT NULL DEFAULT '',
			notice         TEXT NOT NULL DEFAULT '',
			owner_user_id  TEXT NOT NULL DEFAULT '',
			member_count   INTEGER NOT NULL DEFAULT 0,
			qr_code_url    TEXT NOT NULL DEFAULT '',
			raw_json       TEXT NOT NULL DEFAULT '{}',
			first_seen_at  INTEGER NOT NULL DEFAULT 0,
			last_synced_at INTEGER NOT NULL DEFAULT 0,
			updated_at     INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (account_id, room_id),
			FOREIGN KEY (account_id) REFERENCES qiwei_accounts(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_qiwei_rooms_owner
			ON qiwei_rooms(account_id, owner_user_id)`,

		`CREATE TABLE IF NOT EXISTS qiwei_room_members (
			account_id   TEXT NOT NULL,
			room_id      TEXT NOT NULL,
			user_id      TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			role         TEXT NOT NULL DEFAULT 'member',
			joined_at    INTEGER NOT NULL DEFAULT 0,
			last_seen_at INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (account_id, room_id, user_id),
			FOREIGN KEY (account_id) REFERENCES qiwei_accounts(id) ON DELETE CASCADE,
			FOREIGN KEY (account_id, room_id) REFERENCES qiwei_rooms(account_id, room_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_qiwei_room_members_user
			ON qiwei_room_members(account_id, user_id)`,

		`CREATE TABLE IF NOT EXISTS qiwei_identity_links (
			id                 INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id         TEXT NOT NULL DEFAULT '',
			user_id            TEXT NOT NULL DEFAULT '',
			external_user_id   TEXT NOT NULL DEFAULT '',
			downstream_system  TEXT NOT NULL,
			downstream_id      TEXT NOT NULL,
			downstream_meta_json TEXT NOT NULL DEFAULT '{}',
			created_at         INTEGER NOT NULL DEFAULT 0,
			updated_at         INTEGER NOT NULL DEFAULT 0,
			UNIQUE (downstream_system, downstream_id),
			FOREIGN KEY (account_id) REFERENCES qiwei_accounts(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_qiwei_identity_links_external
			ON qiwei_identity_links(account_id, external_user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_qiwei_identity_links_user
			ON qiwei_identity_links(account_id, user_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			head := stmt
			if len(head) > 80 {
				head = head[:80]
			}
			return fmt.Errorf("exec schema stmt: %w\nSQL: %s", err, head)
		}
	}
	return nil
}

// deriveShortHash returns a URL-safe 8-char short hash of the given guid.
// Uses SHA-256 → first 5 bytes → base32 (no padding, lowercase).
// 5 bytes → 8 chars of base32, giving us a 40-bit space (collision probability
// negligible for our expected account count).
func deriveShortHash(guid string) string {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(guid))
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:5])
	return strings.ToLower(enc)
}
