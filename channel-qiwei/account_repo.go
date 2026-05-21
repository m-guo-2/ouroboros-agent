package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"channel-qiwei/internal/timeutil"
)

// Account mirrors one row of the qiwei_accounts table.
// Keep it flat so HTTP handlers can serialize it directly (after masking
// the token).
type Account struct {
	ID            string `json:"id"`
	GUID          string `json:"guid"`
	Token         string `json:"token,omitempty"`
	ShortHash     string `json:"shortHash"`
	DisplayName   string `json:"displayName"`
	AgentID       string `json:"agentId"`
	Enabled       bool   `json:"enabled"`
	SelfUserID    string `json:"selfUserId"`
	SelfName      string `json:"selfName"`
	SelfAlias     string `json:"selfAlias"`
	SelfAvatarURL string `json:"selfAvatarUrl"`
	SelfCorpName  string `json:"selfCorpName"`
	SelfSyncedAt  int64  `json:"selfSyncedAt"`
	MetaJSON      string `json:"metaJson"`
	Notes         string `json:"notes"`
	CreatedAt     int64  `json:"createdAt"`
	UpdatedAt     int64  `json:"updatedAt"`
}

// ErrAccountNotFound is returned by repo lookups when the target row is absent.
var ErrAccountNotFound = errors.New("qiwei account not found")

// accountRepo encapsulates all SQL against qiwei_accounts. Keeping SQL in one
// file keeps the rest of the code database-agnostic.
type accountRepo struct {
	db *sql.DB
}

func newAccountRepo(db *sql.DB) *accountRepo { return &accountRepo{db: db} }

const accountColumns = `id, guid, token, short_hash, display_name, agent_id, enabled,
	self_user_id, self_name, self_alias, self_avatar_url, self_corp_name, self_synced_at,
	meta_json, notes, created_at, updated_at`

func scanAccount(scanner interface {
	Scan(dest ...any) error
}) (Account, error) {
	var a Account
	var enabled int64
	err := scanner.Scan(
		&a.ID, &a.GUID, &a.Token, &a.ShortHash, &a.DisplayName, &a.AgentID, &enabled,
		&a.SelfUserID, &a.SelfName, &a.SelfAlias, &a.SelfAvatarURL, &a.SelfCorpName, &a.SelfSyncedAt,
		&a.MetaJSON, &a.Notes, &a.CreatedAt, &a.UpdatedAt,
	)
	a.Enabled = enabled != 0
	return a, err
}

// ListAccounts returns every live account row, ordered by created_at ascending.
// Soft-deleted rows (deleted_at != 0) are filtered out.
func (r *accountRepo) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+accountColumns+` FROM qiwei_accounts
		 WHERE deleted_at = 0
		 ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *accountRepo) GetAccount(ctx context.Context, id string) (Account, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM qiwei_accounts
		 WHERE id = ? AND deleted_at = 0`, id)
	a, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	return a, err
}

func (r *accountRepo) GetAccountByGUID(ctx context.Context, guid string) (Account, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM qiwei_accounts
		 WHERE guid = ? AND deleted_at = 0`, guid)
	a, err := scanAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrAccountNotFound
	}
	return a, err
}

// CreateAccount inserts a new row. The caller MUST set GUID/Token/AgentID;
// id, short_hash and timestamps are filled here if empty.
func (r *accountRepo) CreateAccount(ctx context.Context, a Account) (Account, error) {
	a.GUID = strings.TrimSpace(a.GUID)
	a.Token = strings.TrimSpace(a.Token)
	if a.GUID == "" || a.Token == "" {
		return Account{}, fmt.Errorf("guid and token are required")
	}
	now := timeutil.NowMs()
	if a.ID == "" {
		a.ID = "qw_" + deriveShortHash(a.GUID)
	}
	if a.ShortHash == "" {
		a.ShortHash = deriveShortHash(a.GUID)
	}
	if a.MetaJSON == "" {
		a.MetaJSON = "{}"
	}
	if a.CreatedAt == 0 {
		a.CreatedAt = now
	}
	a.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `INSERT INTO qiwei_accounts (
		id, guid, token, short_hash, display_name, agent_id, enabled,
		self_user_id, self_name, self_alias, self_avatar_url, self_corp_name, self_synced_at,
		meta_json, notes, created_at, updated_at, deleted_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		a.ID, a.GUID, a.Token, a.ShortHash, a.DisplayName, a.AgentID, boolToInt(a.Enabled),
		a.SelfUserID, a.SelfName, a.SelfAlias, a.SelfAvatarURL, a.SelfCorpName, a.SelfSyncedAt,
		a.MetaJSON, a.Notes, a.CreatedAt, a.UpdatedAt,
	)
	if err != nil {
		return Account{}, err
	}
	return a, nil
}

// AccountPatch carries an editable subset of Account fields. nil means "leave
// unchanged"; we use pointers to distinguish "absent" from "zero value".
type AccountPatch struct {
	Token       *string
	DisplayName *string
	AgentID     *string
	Enabled     *bool
	MetaJSON    *string
	Notes       *string
}

func (r *accountRepo) UpdateAccount(ctx context.Context, id string, patch AccountPatch) (Account, error) {
	sets := []string{"updated_at = ?"}
	args := []any{timeutil.NowMs()}

	if patch.Token != nil {
		sets = append(sets, "token = ?")
		args = append(args, strings.TrimSpace(*patch.Token))
	}
	if patch.DisplayName != nil {
		sets = append(sets, "display_name = ?")
		args = append(args, *patch.DisplayName)
	}
	if patch.AgentID != nil {
		sets = append(sets, "agent_id = ?")
		args = append(args, *patch.AgentID)
	}
	if patch.Enabled != nil {
		sets = append(sets, "enabled = ?")
		args = append(args, boolToInt(*patch.Enabled))
	}
	if patch.MetaJSON != nil {
		sets = append(sets, "meta_json = ?")
		args = append(args, *patch.MetaJSON)
	}
	if patch.Notes != nil {
		sets = append(sets, "notes = ?")
		args = append(args, *patch.Notes)
	}

	args = append(args, id)
	res, err := r.db.ExecContext(ctx,
		`UPDATE qiwei_accounts SET `+strings.Join(sets, ", ")+`
		 WHERE id = ? AND deleted_at = 0`, args...)
	if err != nil {
		return Account{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Account{}, ErrAccountNotFound
	}
	return r.GetAccount(ctx, id)
}

// SoftDeleteAccount marks the row as deleted (deleted_at=now) and also flips
// enabled=0 for backwards-compatibility with code still gating on enabled.
func (r *accountRepo) SoftDeleteAccount(ctx context.Context, id string) error {
	now := timeutil.NowMs()
	res, err := r.db.ExecContext(ctx,
		`UPDATE qiwei_accounts SET enabled = 0, deleted_at = ?, updated_at = ?
		 WHERE id = ? AND deleted_at = 0`,
		now, now, id,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAccountNotFound
	}
	return nil
}

// PurgeAccount irreversibly removes the account row and its known-room
// tracking entries. Callers should prefer SoftDeleteAccount.
func (r *accountRepo) PurgeAccount(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM qiwei_known_rooms WHERE account_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM qiwei_accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAccountNotFound
	}
	return tx.Commit()
}

// HardDeleteAccount is retained as an alias for PurgeAccount to avoid churning
// any admin tooling that still references the old name.
func (r *accountRepo) HardDeleteAccount(ctx context.Context, id string) error {
	return r.PurgeAccount(ctx, id)
}

// ProfileUpdate carries the identity snapshot fetched from /user/getProfile.
type ProfileUpdate struct {
	SelfUserID    string
	SelfName      string
	SelfAlias     string
	SelfAvatarURL string
	SelfCorpName  string
}

func (r *accountRepo) UpdateProfile(ctx context.Context, id string, p ProfileUpdate) error {
	now := timeutil.NowMs()
	res, err := r.db.ExecContext(ctx, `UPDATE qiwei_accounts SET
		self_user_id = ?, self_name = ?, self_alias = ?, self_avatar_url = ?,
		self_corp_name = ?, self_synced_at = ?, updated_at = ?
		WHERE id = ? AND deleted_at = 0`,
		validUTF8Text(p.SelfUserID), validUTF8Text(p.SelfName), validUTF8Text(p.SelfAlias), validUTF8Text(p.SelfAvatarURL),
		validUTF8Text(p.SelfCorpName), now, now, id,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAccountNotFound
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
