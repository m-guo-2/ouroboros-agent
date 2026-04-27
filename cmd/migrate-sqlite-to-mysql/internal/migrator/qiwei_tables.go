package migrator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// qiweiTables returns the dependency-ordered list of qiwei-scope tables.
//
// qiwei_contacts.follow_user_json is the only JSON column that fans out
// (into qiwei_contact_followers); everything else is a 1:1 copy plus the
// universal soft-delete / timestamp tweaks.
func qiweiTables() []tableMigration {
	return []tableMigration{
		qiweiAccountsTable(),
		qiweiContactsTable(),
		qiweiRoomsTable(),
		qiweiRoomMembersTable(),
		qiweiIdentityLinksTable(),
		qiweiKnownRoomsTable(),
	}
}

func qiweiAccountsTable() tableMigration {
	return tableMigration{
		source: "qiwei_accounts", target: "qiwei_accounts",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT id, COALESCE(guid, ''),
				       COALESCE(token, ''),
				       COALESCE(display_name, ''),
				       COALESCE(enabled, 1),
				       COALESCE(meta_json, '{}'),
				       COALESCE(notes, ''),
				       COALESCE(last_synced_at, 0),
				       COALESCE(created_at, 0), COALESCE(updated_at, 0)
				FROM qiwei_accounts`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, guid, token, name, meta, notes string
			var enabled, lastSyncedAt, createdAt, updatedAt int64
			if err := row.Scan(&id, &guid, &token, &name, &enabled, &meta, &notes, &lastSyncedAt, &createdAt, &updatedAt); err != nil {
				return 0, err
			}
			meta = defaultJSONObject(meta)
			// Legacy rows stored unix seconds; the new schema is ms.
			lastSyncedAt = secToMs(lastSyncedAt)
			createdAt = secToMs(createdAt)
			updatedAt = secToMs(updatedAt)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO qiwei_accounts
				(id, guid, token, display_name, enabled, meta_json, notes,
				 last_synced_at, created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				id, guid, token, name, enabled, meta, notes, lastSyncedAt, createdAt, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

func qiweiContactsTable() tableMigration {
	children := []string{"qiwei_contact_followers"}
	return tableMigration{
		source: "qiwei_contacts", target: "qiwei_contacts",
		children: children,
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT account_id, user_id,
				       COALESCE(external_user_id, ''),
				       COALESCE(source, ''),
				       COALESCE(nickname, ''),
				       COALESCE(remark, ''),
				       COALESCE(name, ''),
				       COALESCE(corp_name, ''),
				       COALESCE(position, ''),
				       COALESCE(mobile, ''),
				       COALESCE(avatar, ''),
				       COALESCE(gender, 0),
				       COALESCE(follow_user_json, '[]'),
				       COALESCE(raw_json, '{}'),
				       COALESCE(last_synced_at, 0),
				       COALESCE(created_at, 0), COALESCE(updated_at, 0)
				FROM qiwei_contacts`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, childCounts map[string]int64) (int64, error) {
			var accountID, userID, externalID, source, nickname, remark, name, corp, position, mobile, avatar string
			var gender int64
			var followers, rawJSON string
			var lastSyncedAt, createdAt, updatedAt int64
			if err := row.Scan(&accountID, &userID, &externalID, &source, &nickname, &remark, &name, &corp, &position, &mobile, &avatar, &gender, &followers, &rawJSON, &lastSyncedAt, &createdAt, &updatedAt); err != nil {
				return 0, err
			}
			rawJSON = defaultJSONObject(rawJSON)
			lastSyncedAt = secToMs(lastSyncedAt)
			createdAt = secToMs(createdAt)
			updatedAt = secToMs(updatedAt)

			if _, err := tx.ExecContext(ctx, `
				INSERT INTO qiwei_contacts
				(account_id, user_id, external_user_id, source, nickname, remark,
				 name, corp_name, position, mobile, avatar, gender, raw_json,
				 last_synced_at, created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				accountID, userID, externalID, source, nickname, remark, name, corp,
				position, mobile, avatar, gender, rawJSON,
				lastSyncedAt, createdAt, updatedAt); err != nil {
				return 0, err
			}

			n, err := insertContactFollowers(ctx, tx, accountID, userID, followers, createdAt)
			if err != nil {
				return 0, err
			}
			childCounts["qiwei_contact_followers"] += n

			return 1, nil
		},
		verifyChildren: func(ctx context.Context, db *sql.DB) (map[string]int64, error) {
			out := map[string]int64{}
			rows, err := db.QueryContext(ctx, `SELECT COALESCE(follow_user_json, '[]') FROM qiwei_contacts`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			for rows.Next() {
				var s string
				if err := rows.Scan(&s); err != nil {
					return nil, err
				}
				out["qiwei_contact_followers"] += jsonArrayCount(s)
			}
			return out, rows.Err()
		},
	}
}

func qiweiRoomsTable() tableMigration {
	return tableMigration{
		source: "qiwei_rooms", target: "qiwei_rooms",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT account_id, room_id,
				       COALESCE(name, ''),
				       COALESCE(owner_id, ''),
				       COALESCE(announcement, ''),
				       COALESCE(notice, ''),
				       COALESCE(member_count, 0),
				       COALESCE(raw_json, '{}'),
				       COALESCE(last_synced_at, 0),
				       COALESCE(created_at, 0), COALESCE(updated_at, 0)
				FROM qiwei_rooms`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var accountID, roomID, name, owner, announcement, notice, raw string
			var memberCount, lastSyncedAt, createdAt, updatedAt int64
			if err := row.Scan(&accountID, &roomID, &name, &owner, &announcement, &notice, &memberCount, &raw, &lastSyncedAt, &createdAt, &updatedAt); err != nil {
				return 0, err
			}
			raw = defaultJSONObject(raw)
			lastSyncedAt = secToMs(lastSyncedAt)
			createdAt = secToMs(createdAt)
			updatedAt = secToMs(updatedAt)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO qiwei_rooms
				(account_id, room_id, name, owner_id, announcement, notice,
				 member_count, raw_json, last_synced_at, created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				accountID, roomID, name, owner, announcement, notice,
				memberCount, raw, lastSyncedAt, createdAt, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

func qiweiRoomMembersTable() tableMigration {
	return tableMigration{
		source: "qiwei_room_members", target: "qiwei_room_members",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT account_id, room_id, member_id,
				       COALESCE(name, ''),
				       COALESCE(role, ''),
				       COALESCE(joined_at, 0),
				       COALESCE(created_at, 0), COALESCE(updated_at, 0)
				FROM qiwei_room_members`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var accountID, roomID, memberID, name, role string
			var joinedAt, createdAt, updatedAt int64
			if err := row.Scan(&accountID, &roomID, &memberID, &name, &role, &joinedAt, &createdAt, &updatedAt); err != nil {
				return 0, err
			}
			joinedAt = secToMs(joinedAt)
			createdAt = secToMs(createdAt)
			updatedAt = secToMs(updatedAt)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO qiwei_room_members
				(account_id, room_id, member_id, name, role, joined_at,
				 created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				accountID, roomID, memberID, name, role, joinedAt, createdAt, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

func qiweiIdentityLinksTable() tableMigration {
	return tableMigration{
		source: "qiwei_identity_links", target: "qiwei_identity_links",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT account_id, contact_user_id, downstream_channel,
				       downstream_user_id,
				       COALESCE(downstream_meta_json, '{}'),
				       COALESCE(created_at, 0), COALESCE(updated_at, 0)
				FROM qiwei_identity_links`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var accountID, contactUserID, downstreamChannel, downstreamUserID, meta string
			var createdAt, updatedAt int64
			if err := row.Scan(&accountID, &contactUserID, &downstreamChannel, &downstreamUserID, &meta, &createdAt, &updatedAt); err != nil {
				return 0, err
			}
			meta = defaultJSONObject(meta)
			createdAt = secToMs(createdAt)
			updatedAt = secToMs(updatedAt)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO qiwei_identity_links
				(account_id, contact_user_id, downstream_channel, downstream_user_id,
				 downstream_meta_json, created_at, updated_at, deleted_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
				accountID, contactUserID, downstreamChannel, downstreamUserID, meta,
				createdAt, updatedAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

func qiweiKnownRoomsTable() tableMigration {
	return tableMigration{
		source: "qiwei_known_rooms", target: "qiwei_known_rooms",
		selectAt: func(ctx context.Context, db *sql.DB) (*sql.Rows, error) {
			return db.QueryContext(ctx, `
				SELECT account_id, room_id,
				       COALESCE(first_seen_at, 0)
				FROM qiwei_known_rooms`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var accountID, roomID string
			var firstSeenAt int64
			if err := row.Scan(&accountID, &roomID, &firstSeenAt); err != nil {
				return 0, err
			}
			firstSeenAt = secToMs(firstSeenAt)
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO qiwei_known_rooms
				(account_id, room_id, first_seen_at, deleted_at)
				VALUES (?, ?, ?, 0)`,
				accountID, roomID, firstSeenAt); err != nil {
				return 0, err
			}
			return 1, nil
		},
	}
}

// secToMs upgrades a legacy unix-seconds column to ms. Values that already
// look like ms (>= year 2001 in seconds, ie > ~10^11 when truly seconds)
// are left alone — the heuristic catches the only place qiwei mixed both
// units.
func secToMs(v int64) int64 {
	if v == 0 {
		return 0
	}
	if v < 1_000_000_000_000 {
		return v * 1000
	}
	return v
}

// insertContactFollowers fans the JSON array stored in
// qiwei_contacts.follow_user_json into qiwei_contact_followers rows.
func insertContactFollowers(ctx context.Context, tx *sql.Tx, accountID, userID, raw string, createdAt int64) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return 0, nil
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return 0, fmt.Errorf("contact %s/%s follow_user_json: %w", accountID, userID, err)
	}
	for i, item := range arr {
		followerID, _ := item["userid"].(string)
		if followerID == "" {
			followerID, _ = item["userId"].(string)
		}
		remark, _ := item["remark"].(string)
		desc, _ := item["description"].(string)
		payload, _ := json.Marshal(item)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO qiwei_contact_followers
			(account_id, contact_user_id, follower_user_id, remark, description,
			 raw_json, position, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			accountID, userID, followerID, remark, desc,
			defaultJSONObject(string(payload)), i, createdAt); err != nil {
			return 0, err
		}
	}
	return int64(len(arr)), nil
}
