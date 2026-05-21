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
					       COALESCE(short_hash, ''),
					       COALESCE(display_name, ''),
					       COALESCE(agent_id, ''),
					       COALESCE(enabled, 1),
					       COALESCE(self_user_id, ''),
					       COALESCE(self_name, ''),
					       COALESCE(self_alias, ''),
					       COALESCE(self_avatar_url, ''),
					       COALESCE(self_corp_name, ''),
					       COALESCE(self_synced_at, 0),
					       COALESCE(meta_json, '{}'),
					       COALESCE(notes, ''),
					       COALESCE(created_at, 0), COALESCE(updated_at, 0)
					FROM qiwei_accounts`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id, guid, token, shortHash, name, agentID string
			var selfUserID, selfName, selfAlias, selfAvatarURL, selfCorpName string
			var meta, notes string
			var enabled, selfSyncedAt, createdAt, updatedAt int64
			if err := row.Scan(
				&id, &guid, &token, &shortHash, &name, &agentID, &enabled,
				&selfUserID, &selfName, &selfAlias, &selfAvatarURL, &selfCorpName,
				&selfSyncedAt, &meta, &notes, &createdAt, &updatedAt,
			); err != nil {
				return 0, err
			}
			cleanTextFields(&id, &guid, &token, &shortHash, &name, &agentID,
				&selfUserID, &selfName, &selfAlias, &selfAvatarURL, &selfCorpName,
				&meta, &notes)
			meta = defaultJSONObject(meta)
			// Legacy rows stored unix seconds; the new schema is ms.
			selfSyncedAt = secToMs(selfSyncedAt)
			createdAt = secToMs(createdAt)
			updatedAt = secToMs(updatedAt)
			if _, err := tx.ExecContext(ctx, `
					INSERT INTO qiwei_accounts
					(id, guid, token, short_hash, display_name, agent_id, enabled,
					 self_user_id, self_name, self_alias, self_avatar_url, self_corp_name,
					 self_synced_at, meta_json, notes, created_at, updated_at, deleted_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				id, guid, token, shortHash, name, agentID, enabled,
				selfUserID, selfName, selfAlias, selfAvatarURL, selfCorpName,
				selfSyncedAt, meta, notes, createdAt, updatedAt); err != nil {
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
					       COALESCE(real_name, ''),
					       COALESCE(alias, ''),
					       COALESCE(remark, ''),
					       COALESCE(avatar_url, ''),
					       COALESCE(gender, ''),
					       COALESCE(corp_id, ''),
					       COALESCE(corp_name, ''),
					       COALESCE(follow_user_json, '[]'),
					       COALESCE(raw_json, '{}'),
					       COALESCE(first_seen_at, 0),
					       COALESCE(last_synced_at, 0),
					       COALESCE(updated_at, 0)
					FROM qiwei_contacts`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, childCounts map[string]int64) (int64, error) {
			var accountID, userID, externalID, source, nickname, realName, alias, remark string
			var avatarURL, gender, corpID, corpName string
			var followers, rawJSON string
			var firstSeenAt, lastSyncedAt, updatedAt int64
			if err := row.Scan(
				&accountID, &userID, &externalID, &source, &nickname, &realName,
				&alias, &remark, &avatarURL, &gender, &corpID, &corpName,
				&followers, &rawJSON, &firstSeenAt, &lastSyncedAt, &updatedAt,
			); err != nil {
				return 0, err
			}
			cleanTextFields(&accountID, &userID, &externalID, &source, &nickname,
				&realName, &alias, &remark, &avatarURL, &gender, &corpID, &corpName,
				&followers, &rawJSON)
			rawJSON = defaultJSONObject(rawJSON)
			firstSeenAt = secToMs(firstSeenAt)
			lastSyncedAt = secToMs(lastSyncedAt)
			updatedAt = secToMs(updatedAt)

			if _, err := tx.ExecContext(ctx, `
					INSERT INTO qiwei_contacts
					(account_id, user_id, external_user_id, source, nickname, real_name,
					 alias, remark, avatar_url, gender, corp_id, corp_name, raw_json,
					 first_seen_at, last_synced_at, updated_at, deleted_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				accountID, userID, externalID, source, nickname, realName,
				alias, remark, avatarURL, gender, corpID, corpName, rawJSON,
				firstSeenAt, lastSyncedAt, updatedAt); err != nil {
				return 0, err
			}

			n, err := insertContactFollowers(ctx, tx, accountID, userID, followers, firstSeenAt)
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
					       COALESCE(announcement, ''),
					       COALESCE(notice, ''),
					       COALESCE(owner_user_id, ''),
					       COALESCE(member_count, 0),
					       COALESCE(qr_code_url, ''),
					       COALESCE(raw_json, '{}'),
					       COALESCE(first_seen_at, 0),
					       COALESCE(last_synced_at, 0),
					       COALESCE(updated_at, 0)
					FROM qiwei_rooms`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var accountID, roomID, name, announcement, notice, ownerUserID, qrCodeURL, raw string
			var memberCount, firstSeenAt, lastSyncedAt, updatedAt int64
			if err := row.Scan(
				&accountID, &roomID, &name, &announcement, &notice, &ownerUserID,
				&memberCount, &qrCodeURL, &raw, &firstSeenAt, &lastSyncedAt, &updatedAt,
			); err != nil {
				return 0, err
			}
			cleanTextFields(&accountID, &roomID, &name, &announcement, &notice,
				&ownerUserID, &qrCodeURL, &raw)
			raw = defaultJSONObject(raw)
			firstSeenAt = secToMs(firstSeenAt)
			lastSyncedAt = secToMs(lastSyncedAt)
			updatedAt = secToMs(updatedAt)
			if _, err := tx.ExecContext(ctx, `
					INSERT INTO qiwei_rooms
					(account_id, room_id, name, announcement, notice, owner_user_id,
					 member_count, qr_code_url, raw_json, first_seen_at, last_synced_at,
					 updated_at, deleted_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				accountID, roomID, name, announcement, notice, ownerUserID,
				memberCount, qrCodeURL, raw, firstSeenAt, lastSyncedAt, updatedAt); err != nil {
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
					SELECT account_id, room_id, user_id,
					       COALESCE(display_name, ''),
					       COALESCE(role, ''),
					       COALESCE(joined_at, 0),
					       COALESCE(last_seen_at, 0)
					FROM qiwei_room_members`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var accountID, roomID, userID, displayName, role string
			var joinedAt, lastSeenAt int64
			if err := row.Scan(&accountID, &roomID, &userID, &displayName, &role, &joinedAt, &lastSeenAt); err != nil {
				return 0, err
			}
			cleanTextFields(&accountID, &roomID, &userID, &displayName, &role)
			joinedAt = secToMs(joinedAt)
			lastSeenAt = secToMs(lastSeenAt)
			if _, err := tx.ExecContext(ctx, `
					INSERT INTO qiwei_room_members
					(account_id, room_id, user_id, display_name, role, joined_at,
					 last_seen_at, deleted_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, 0)`,
				accountID, roomID, userID, displayName, role, joinedAt, lastSeenAt); err != nil {
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
					SELECT id, COALESCE(account_id, ''),
					       COALESCE(user_id, ''),
					       COALESCE(external_user_id, ''),
					       downstream_system,
					       downstream_id,
					       COALESCE(downstream_meta_json, '{}'),
					       COALESCE(created_at, 0), COALESCE(updated_at, 0)
					FROM qiwei_identity_links`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var id int64
			var accountID, userID, externalUserID, downstreamSystem, downstreamID, meta string
			var createdAt, updatedAt int64
			if err := row.Scan(&id, &accountID, &userID, &externalUserID, &downstreamSystem, &downstreamID, &meta, &createdAt, &updatedAt); err != nil {
				return 0, err
			}
			cleanTextFields(&accountID, &userID, &externalUserID, &downstreamSystem, &downstreamID, &meta)
			meta = defaultJSONObject(meta)
			createdAt = secToMs(createdAt)
			updatedAt = secToMs(updatedAt)
			if _, err := tx.ExecContext(ctx, `
					INSERT INTO qiwei_identity_links
					(id, account_id, user_id, external_user_id, downstream_system,
					 downstream_id, downstream_meta_json, created_at, updated_at, deleted_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				id, accountID, userID, externalUserID, downstreamSystem, downstreamID, meta,
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
					       COALESCE(created_at, 0)
					FROM qiwei_known_rooms`)
		},
		migrate: func(ctx context.Context, tx *sql.Tx, row *sql.Rows, _ map[string]int64) (int64, error) {
			var accountID, roomID string
			var firstSeenAt int64
			if err := row.Scan(&accountID, &roomID, &firstSeenAt); err != nil {
				return 0, err
			}
			cleanTextFields(&accountID, &roomID)
			firstSeenAt = secToMs(firstSeenAt)
			if _, err := tx.ExecContext(ctx, `
					INSERT INTO qiwei_known_rooms
					(account_id, room_id, created_at, deleted_at)
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
	raw = strings.ToValidUTF8(raw, "\uFFFD")
	var arr []any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return 0, fmt.Errorf("contact %s/%s follow_user_json: %w", accountID, userID, err)
	}
	for i, item := range arr {
		var followerID string
		switch v := item.(type) {
		case string:
			followerID = v
		case map[string]any:
			followerID, _ = v["userid"].(string)
			if followerID == "" {
				followerID, _ = v["userId"].(string)
			}
			if followerID == "" {
				followerID, _ = v["follow_user_id"].(string)
			}
		}
		if followerID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
				INSERT INTO qiwei_contact_followers
				(account_id, user_id, follow_user_id, position, created_at)
				VALUES (?, ?, ?, ?, ?)`,
			accountID, userID, followerID, i, createdAt); err != nil {
			return 0, err
		}
	}
	return int64(len(arr)), nil
}
