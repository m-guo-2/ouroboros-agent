package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"

	"channel-qiwei/internal/timeutil"
)

const maxStoredJSONBytes = 64 * 1024

var (
	ErrContactNotFound      = errors.New("qiwei contact not found")
	ErrRoomNotFound         = errors.New("qiwei room not found")
	ErrIdentityLinkNotFound = errors.New("qiwei identity link not found")
)

type Contact struct {
	AccountID      string `json:"accountId"`
	UserID         string `json:"userId"`
	ExternalUserID string `json:"externalUserId"`
	Source         string `json:"source"`
	Nickname       string `json:"nickname"`
	RealName       string `json:"realName"`
	Alias          string `json:"alias"`
	Remark         string `json:"remark"`
	AvatarURL      string `json:"avatarUrl"`
	Gender         string `json:"gender"`
	CorpID         string `json:"corpId"`
	CorpName       string `json:"corpName"`
	FollowUserJSON string `json:"followUserJson"`
	RawJSON        string `json:"rawJson"`
	FirstSeenAt    int64  `json:"firstSeenAt"`
	LastSyncedAt   int64  `json:"lastSyncedAt"`
	UpdatedAt      int64  `json:"updatedAt"`
}

type Room struct {
	AccountID    string `json:"accountId"`
	RoomID       string `json:"roomId"`
	Name         string `json:"name"`
	Announcement string `json:"announcement"`
	Notice       string `json:"notice"`
	OwnerUserID  string `json:"ownerUserId"`
	MemberCount  int64  `json:"memberCount"`
	QRCodeURL    string `json:"qrCodeUrl"`
	RawJSON      string `json:"rawJson"`
	FirstSeenAt  int64  `json:"firstSeenAt"`
	LastSyncedAt int64  `json:"lastSyncedAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

type RoomMember struct {
	AccountID   string `json:"accountId"`
	RoomID      string `json:"roomId"`
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	JoinedAt    int64  `json:"joinedAt"`
	LastSeenAt  int64  `json:"lastSeenAt"`
}

type IdentityLink struct {
	ID                 int64  `json:"id"`
	AccountID          string `json:"accountId"`
	UserID             string `json:"userId"`
	ExternalUserID     string `json:"externalUserId"`
	DownstreamSystem   string `json:"downstreamSystem"`
	DownstreamID       string `json:"downstreamId"`
	DownstreamMetaJSON string `json:"downstreamMetaJson"`
	CreatedAt          int64  `json:"createdAt"`
	UpdatedAt          int64  `json:"updatedAt"`
}

type IdentityLinkUpsertResult struct {
	Link       IdentityLink
	Previous   *IdentityLink
	Created    bool
	Reassigned bool
}

type ContactCounts struct {
	Contacts      int64 `json:"contacts"`
	Rooms         int64 `json:"rooms"`
	RoomMembers   int64 `json:"roomMembers"`
	IdentityLinks int64 `json:"identityLinks"`
}

type contactRepo struct {
	db *sql.DB
}

func newContactRepo(db *sql.DB) *contactRepo { return &contactRepo{db: db} }

// Column lists used for scan calls. follow_user_json is synthesized from the
// split qiwei_contact_followers table and appended after the DB scan.
const contactColumns = `account_id, user_id, external_user_id, source, nickname, real_name,
	alias, remark, avatar_url, gender, corp_id, corp_name, raw_json,
	first_seen_at, last_synced_at, updated_at`

const roomColumns = `account_id, room_id, name, announcement, notice, owner_user_id,
	member_count, qr_code_url, raw_json, first_seen_at, last_synced_at, updated_at`

const roomMemberColumns = `account_id, room_id, user_id, display_name, role, joined_at, last_seen_at`

const identityLinkColumns = `id, account_id, user_id, external_user_id, downstream_system,
	downstream_id, downstream_meta_json, created_at, updated_at`

func scanContact(scanner interface{ Scan(dest ...any) error }) (Contact, error) {
	var c Contact
	err := scanner.Scan(
		&c.AccountID, &c.UserID, &c.ExternalUserID, &c.Source, &c.Nickname, &c.RealName,
		&c.Alias, &c.Remark, &c.AvatarURL, &c.Gender, &c.CorpID, &c.CorpName,
		&c.RawJSON, &c.FirstSeenAt, &c.LastSyncedAt, &c.UpdatedAt,
	)
	return c, err
}

func scanRoom(scanner interface{ Scan(dest ...any) error }) (Room, error) {
	var r Room
	err := scanner.Scan(
		&r.AccountID, &r.RoomID, &r.Name, &r.Announcement, &r.Notice, &r.OwnerUserID,
		&r.MemberCount, &r.QRCodeURL, &r.RawJSON, &r.FirstSeenAt, &r.LastSyncedAt, &r.UpdatedAt,
	)
	return r, err
}

func scanRoomMember(scanner interface{ Scan(dest ...any) error }) (RoomMember, error) {
	var m RoomMember
	err := scanner.Scan(
		&m.AccountID, &m.RoomID, &m.UserID, &m.DisplayName, &m.Role, &m.JoinedAt, &m.LastSeenAt,
	)
	return m, err
}

func scanIdentityLink(scanner interface{ Scan(dest ...any) error }) (IdentityLink, error) {
	var link IdentityLink
	err := scanner.Scan(
		&link.ID, &link.AccountID, &link.UserID, &link.ExternalUserID, &link.DownstreamSystem,
		&link.DownstreamID, &link.DownstreamMetaJSON, &link.CreatedAt, &link.UpdatedAt,
	)
	return link, err
}

func (r *contactRepo) UpsertContact(ctx context.Context, c Contact) error {
	c.AccountID = strings.TrimSpace(c.AccountID)
	c.UserID = strings.TrimSpace(c.UserID)
	if c.AccountID == "" || c.UserID == "" {
		return fmt.Errorf("account_id and user_id are required")
	}
	if c.Source == "" {
		c.Source = "unknown"
	}
	rawJSON, err := marshalRawJSON("contact", c.RawJSON)
	if err != nil {
		return err
	}
	followJSON, err := marshalJSONArray("contact_follow_user", c.FollowUserJSON)
	if err != nil {
		return err
	}
	now := timeutil.NowMs()
	if c.FirstSeenAt == 0 {
		c.FirstSeenAt = now
	}
	if c.LastSyncedAt == 0 {
		c.LastSyncedAt = now
	}
	if c.UpdatedAt == 0 {
		c.UpdatedAt = now
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO qiwei_contacts (
		account_id, user_id, external_user_id, source, nickname, real_name, alias, remark,
		avatar_url, gender, corp_id, corp_name, raw_json,
		first_seen_at, last_synced_at, updated_at, deleted_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0) AS new
	ON DUPLICATE KEY UPDATE
		external_user_id = new.external_user_id,
		source           = new.source,
		nickname         = new.nickname,
		real_name        = new.real_name,
		alias            = new.alias,
		remark           = new.remark,
		avatar_url       = new.avatar_url,
		gender           = new.gender,
		corp_id          = new.corp_id,
		corp_name        = new.corp_name,
		raw_json         = new.raw_json,
		last_synced_at   = new.last_synced_at,
		updated_at       = new.updated_at,
		deleted_at       = 0`,
		c.AccountID, c.UserID, c.ExternalUserID, c.Source, c.Nickname, c.RealName, c.Alias, c.Remark,
		c.AvatarURL, c.Gender, c.CorpID, c.CorpName, rawJSON,
		c.FirstSeenAt, c.LastSyncedAt, c.UpdatedAt,
	); err != nil {
		return err
	}

	if err := replaceContactFollowers(ctx, tx, c.AccountID, c.UserID, followJSON, now); err != nil {
		return err
	}
	return tx.Commit()
}

// replaceContactFollowers rewrites the qiwei_contact_followers rows for one
// contact inside the caller's transaction. Empty / "[]" input wipes all
// follower rows; any other JSON array of strings is re-inserted in order.
func replaceContactFollowers(ctx context.Context, tx *sql.Tx, accountID, userID, followJSON string, now int64) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM qiwei_contact_followers WHERE account_id = ? AND user_id = ?`,
		accountID, userID,
	); err != nil {
		return err
	}
	followJSON = strings.TrimSpace(followJSON)
	if followJSON == "" || followJSON == "[]" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(followJSON), &ids); err != nil {
		// Tolerate legacy object format: [{ "userid": "...", ... }, ...]
		var objs []map[string]any
		if err2 := json.Unmarshal([]byte(followJSON), &objs); err2 != nil {
			return fmt.Errorf("parse follow_user_json: %w", err)
		}
		for _, o := range objs {
			for _, k := range []string{"userid", "UserID", "user_id", "id"} {
				if v, ok := o[k].(string); ok && v != "" {
					ids = append(ids, v)
					break
				}
			}
		}
	}
	for i, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT IGNORE INTO qiwei_contact_followers
			 (account_id, user_id, follow_user_id, position, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			accountID, userID, id, i, now,
		); err != nil {
			return err
		}
	}
	return nil
}

// loadContactFollowers assembles the follow_user_json field for one contact
// from its qiwei_contact_followers rows.
func (r *contactRepo) loadContactFollowers(ctx context.Context, accountID, userID string) (string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT follow_user_id FROM qiwei_contact_followers
		 WHERE account_id = ? AND user_id = ? ORDER BY position ASC`,
		accountID, userID,
	)
	if err != nil {
		return "[]", err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "[]", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "[]", err
	}
	if len(ids) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return "[]", err
	}
	return string(b), nil
}

func (r *contactRepo) GetContact(ctx context.Context, accountID, userID string) (Contact, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+contactColumns+` FROM qiwei_contacts
		 WHERE account_id = ? AND user_id = ? AND deleted_at = 0`,
		accountID, userID,
	)
	c, err := scanContact(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Contact{}, ErrContactNotFound
	}
	if err != nil {
		return Contact{}, err
	}
	c.FollowUserJSON, _ = r.loadContactFollowers(ctx, c.AccountID, c.UserID)
	return c, nil
}

func (r *contactRepo) GetContactByExternalUserID(ctx context.Context, accountID, externalUserID string) (Contact, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+contactColumns+` FROM qiwei_contacts
		 WHERE account_id = ? AND external_user_id = ? AND deleted_at = 0
		 ORDER BY last_synced_at DESC, updated_at DESC
		 LIMIT 1`,
		accountID, externalUserID,
	)
	c, err := scanContact(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Contact{}, ErrContactNotFound
	}
	if err != nil {
		return Contact{}, err
	}
	c.FollowUserJSON, _ = r.loadContactFollowers(ctx, c.AccountID, c.UserID)
	return c, nil
}

func (r *contactRepo) SearchContacts(ctx context.Context, accountID, query string, limit int) ([]Contact, error) {
	if limit <= 0 {
		limit = 20
	}
	pattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := r.db.QueryContext(ctx, `SELECT `+contactColumns+` FROM qiwei_contacts
		WHERE account_id = ? AND deleted_at = 0 AND (
			user_id LIKE ? OR external_user_id LIKE ? OR nickname LIKE ? OR real_name LIKE ? OR alias LIKE ? OR remark LIKE ?
		)
		ORDER BY updated_at DESC, user_id ASC
		LIMIT ?`,
		accountID, pattern, pattern, pattern, pattern, pattern, pattern, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Contact
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Populate follow_user_json for each hit (typically short results).
	for i := range out {
		out[i].FollowUserJSON, _ = r.loadContactFollowers(ctx, out[i].AccountID, out[i].UserID)
	}
	return out, nil
}

func (r *contactRepo) UpsertRoom(ctx context.Context, room Room) error {
	room.AccountID = strings.TrimSpace(room.AccountID)
	room.RoomID = strings.TrimSpace(room.RoomID)
	if room.AccountID == "" || room.RoomID == "" {
		return fmt.Errorf("account_id and room_id are required")
	}
	rawJSON, err := marshalRawJSON("room", room.RawJSON)
	if err != nil {
		return err
	}
	now := timeutil.NowMs()
	if room.FirstSeenAt == 0 {
		room.FirstSeenAt = now
	}
	if room.LastSyncedAt == 0 {
		room.LastSyncedAt = now
	}
	if room.UpdatedAt == 0 {
		room.UpdatedAt = now
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO qiwei_rooms (
		account_id, room_id, name, announcement, notice, owner_user_id, member_count,
		qr_code_url, raw_json, first_seen_at, last_synced_at, updated_at, deleted_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0) AS new
	ON DUPLICATE KEY UPDATE
		name           = new.name,
		announcement   = new.announcement,
		notice         = new.notice,
		owner_user_id  = new.owner_user_id,
		member_count   = new.member_count,
		qr_code_url    = new.qr_code_url,
		raw_json       = new.raw_json,
		last_synced_at = new.last_synced_at,
		updated_at     = new.updated_at,
		deleted_at     = 0`,
		room.AccountID, room.RoomID, room.Name, room.Announcement, room.Notice, room.OwnerUserID,
		room.MemberCount, room.QRCodeURL, rawJSON, room.FirstSeenAt, room.LastSyncedAt, room.UpdatedAt,
	)
	return err
}

func (r *contactRepo) GetRoom(ctx context.Context, accountID, roomID string) (Room, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+roomColumns+` FROM qiwei_rooms
		 WHERE account_id = ? AND room_id = ? AND deleted_at = 0`,
		accountID, roomID,
	)
	room, err := scanRoom(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Room{}, ErrRoomNotFound
	}
	return room, err
}

func (r *contactRepo) ReplaceRoomMembers(ctx context.Context, accountID, roomID string, members []RoomMember) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM qiwei_room_members WHERE account_id = ? AND room_id = ?`,
		accountID, roomID,
	); err != nil {
		return err
	}
	if len(members) == 0 {
		return tx.Commit()
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO qiwei_room_members (
		account_id, room_id, user_id, display_name, role, joined_at, last_seen_at, deleted_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, 0)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := timeutil.NowMs()
	for _, member := range members {
		if strings.TrimSpace(member.UserID) == "" {
			continue
		}
		if member.Role == "" {
			member.Role = "member"
		}
		if member.LastSeenAt == 0 {
			member.LastSeenAt = now
		}
		if _, err := stmt.ExecContext(ctx,
			accountID, roomID, member.UserID, member.DisplayName, member.Role, member.JoinedAt, member.LastSeenAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *contactRepo) DeleteRoomMember(ctx context.Context, accountID, roomID, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM qiwei_room_members WHERE account_id = ? AND room_id = ? AND user_id = ?`,
		accountID, roomID, userID,
	)
	return err
}

func (r *contactRepo) ListRoomMembers(ctx context.Context, accountID, roomID string) ([]RoomMember, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+roomMemberColumns+` FROM qiwei_room_members
		 WHERE account_id = ? AND room_id = ? AND deleted_at = 0
		 ORDER BY role DESC, last_seen_at DESC, user_id ASC`,
		accountID, roomID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RoomMember
	for rows.Next() {
		member, err := scanRoomMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, member)
	}
	return out, rows.Err()
}

func (r *contactRepo) UpsertIdentityLink(ctx context.Context, link IdentityLink) (IdentityLinkUpsertResult, error) {
	link.DownstreamSystem = strings.TrimSpace(link.DownstreamSystem)
	link.DownstreamID = strings.TrimSpace(link.DownstreamID)
	if link.DownstreamSystem == "" || link.DownstreamID == "" {
		return IdentityLinkUpsertResult{}, fmt.Errorf("downstream_system and downstream_id are required")
	}
	metaJSON, err := marshalJSONObject("identity_link_meta", link.DownstreamMetaJSON)
	if err != nil {
		return IdentityLinkUpsertResult{}, err
	}
	now := timeutil.NowMs()
	existing, err := r.ResolveIdentityLink(ctx, link.DownstreamSystem, link.DownstreamID)
	if err != nil && !errors.Is(err, ErrIdentityLinkNotFound) {
		return IdentityLinkUpsertResult{}, err
	}
	if errors.Is(err, ErrIdentityLinkNotFound) {
		res, err := r.db.ExecContext(ctx, `INSERT INTO qiwei_identity_links (
			account_id, user_id, external_user_id, downstream_system, downstream_id,
			downstream_meta_json, created_at, updated_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
			link.AccountID, link.UserID, link.ExternalUserID, link.DownstreamSystem, link.DownstreamID,
			metaJSON, now, now,
		)
		if err != nil {
			return IdentityLinkUpsertResult{}, err
		}
		id, _ := res.LastInsertId()
		link.ID = id
		link.DownstreamMetaJSON = metaJSON
		link.CreatedAt = now
		link.UpdatedAt = now
		return IdentityLinkUpsertResult{Link: link, Created: true}, nil
	}

	_, err = r.db.ExecContext(ctx, `UPDATE qiwei_identity_links SET
		account_id = ?, user_id = ?, external_user_id = ?, downstream_meta_json = ?, updated_at = ?
		WHERE id = ? AND deleted_at = 0`,
		link.AccountID, link.UserID, link.ExternalUserID, metaJSON, now, existing.ID,
	)
	if err != nil {
		return IdentityLinkUpsertResult{}, err
	}
	updated := existing
	updated.AccountID = link.AccountID
	updated.UserID = link.UserID
	updated.ExternalUserID = link.ExternalUserID
	updated.DownstreamMetaJSON = metaJSON
	updated.UpdatedAt = now
	return IdentityLinkUpsertResult{
		Link:       updated,
		Previous:   &existing,
		Reassigned: existing.ExternalUserID != "" && existing.ExternalUserID != link.ExternalUserID,
	}, nil
}

func (r *contactRepo) ResolveIdentityLink(ctx context.Context, downstreamSystem, downstreamID string) (IdentityLink, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+identityLinkColumns+` FROM qiwei_identity_links
		 WHERE downstream_system = ? AND downstream_id = ? AND deleted_at = 0`,
		downstreamSystem, downstreamID,
	)
	link, err := scanIdentityLink(row)
	if errors.Is(err, sql.ErrNoRows) {
		return IdentityLink{}, ErrIdentityLinkNotFound
	}
	return link, err
}

func (r *contactRepo) ListIdentityLinksForContact(ctx context.Context, accountID, userID, externalUserID string) ([]IdentityLink, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+identityLinkColumns+` FROM qiwei_identity_links
		WHERE account_id = ? AND deleted_at = 0 AND (user_id = ? OR external_user_id = ?)
		ORDER BY updated_at DESC, id DESC`,
		accountID, userID, externalUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IdentityLink
	for rows.Next() {
		link, err := scanIdentityLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

func (r *contactRepo) CountByAccount(ctx context.Context, accountID string) (ContactCounts, error) {
	var counts ContactCounts
	queries := []struct {
		dst *int64
		sql string
	}{
		{&counts.Contacts, `SELECT COUNT(*) FROM qiwei_contacts WHERE account_id = ? AND deleted_at = 0`},
		{&counts.Rooms, `SELECT COUNT(*) FROM qiwei_rooms WHERE account_id = ? AND deleted_at = 0`},
		{&counts.RoomMembers, `SELECT COUNT(*) FROM qiwei_room_members WHERE account_id = ? AND deleted_at = 0`},
		{&counts.IdentityLinks, `SELECT COUNT(*) FROM qiwei_identity_links WHERE account_id = ? AND deleted_at = 0`},
	}
	for _, q := range queries {
		if err := r.db.QueryRowContext(ctx, q.sql, accountID).Scan(q.dst); err != nil {
			return ContactCounts{}, err
		}
	}
	return counts, nil
}

func marshalJSONArray(kind, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "[]", nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return "", fmt.Errorf("decode %s: %w", kind, err)
	}
	value, ok := decoded.([]any)
	if !ok {
		return "[]", nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", kind, err)
	}
	return string(encoded), nil
}

func marshalJSONObject(kind, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}", nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return "", fmt.Errorf("decode %s: %w", kind, err)
	}
	obj, ok := decoded.(map[string]any)
	if !ok {
		return "{}", nil
	}
	encoded, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", kind, err)
	}
	return string(encoded), nil
}

func marshalRawJSON(kind, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}", nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return "", fmt.Errorf("decode %s: %w", kind, err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", kind, err)
	}
	if len(encoded) <= maxStoredJSONBytes {
		return string(encoded), nil
	}
	logger.Warn(context.Background(), "contact raw json exceeded limit",
		"kind", kind,
		"bytes", len(encoded),
		"limit", maxStoredJSONBytes,
	)
	prefix := string(encoded[:maxStoredJSONBytes])
	wrapped, err := json.Marshal(map[string]any{
		"truncated":    true,
		"originalSize": len(encoded),
		"prefix":       prefix,
	})
	if err != nil {
		return "", fmt.Errorf("encode truncated %s: %w", kind, err)
	}
	return string(wrapped), nil
}
