package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

const tagAdmin = "admin"

// registerAdminRoutes mounts the /api/qiwei/_admin/* family onto the mux.
// Only called when cfg.AdminToken is configured (server.routes gates it).
func (a *app) registerAdminRoutes(mux *http.ServeMux) {
	auth := a.adminAuthMiddleware

	mux.Handle("/api/qiwei/_admin/accounts", auth(http.HandlerFunc(a.handleAdminAccounts)))
	mux.Handle("/api/qiwei/_admin/accounts/", auth(http.HandlerFunc(a.handleAdminAccountByID)))
	mux.Handle("/api/qiwei/_admin/reload", auth(http.HandlerFunc(a.handleAdminReload)))
	mux.Handle("/api/qiwei/_admin/unknown_guids", auth(http.HandlerFunc(a.handleAdminUnknownGuids)))
}

// adminAuthMiddleware enforces X-Admin-Token on every admin request. When
// the configured token is empty we run in "debug" mode and allow all requests
// through — this lets local admin UIs reach the endpoints without managing
// credentials, at the cost of trusting anyone who can reach the port. A
// warning is logged once so operators notice when production gets deployed
// without a token.
func (a *app) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := strings.TrimSpace(a.cfg.AdminToken)
		if want == "" {
			logger.Warn(r.Context(), "admin API 未配置 X-Admin-Token，当前为调试模式放行",
				"tag", tagAdmin, "path", r.URL.Path,
			)
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
		if got == "" || got != want {
			writeJSON(w, http.StatusUnauthorized, apiResponse{Success: false, Error: "invalid admin token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// maskedAccount hides the raw token behind a short preview so that admin
// list/detail responses can be logged or copy-pasted without leaking
// credentials.
type maskedAccount struct {
	ID            string `json:"id"`
	GUID          string `json:"guid"`
	TokenPreview  string `json:"token_preview"`
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

	ContactCount         int64              `json:"contact_count,omitempty"`
	RoomCount            int64              `json:"room_count,omitempty"`
	RoomMemberCount      int64              `json:"room_member_count,omitempty"`
	IdentityLinkCount    int64              `json:"identity_link_count,omitempty"`
	ContactSyncLastAt    int64              `json:"contact_sync_last_at,omitempty"`
	ContactSyncLastError string             `json:"contact_sync_last_error,omitempty"`
	ContactSyncStatus    *contactSyncStatus `json:"contact_sync_status,omitempty"`
}

func maskToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) <= 4 {
		return token + "***"
	}
	return token[:4] + "***"
}

func toMaskedAccount(a Account) maskedAccount {
	return maskedAccount{
		ID:            a.ID,
		GUID:          a.GUID,
		TokenPreview:  maskToken(a.Token),
		ShortHash:     a.ShortHash,
		DisplayName:   a.DisplayName,
		AgentID:       a.AgentID,
		Enabled:       a.Enabled,
		SelfUserID:    a.SelfUserID,
		SelfName:      a.SelfName,
		SelfAlias:     a.SelfAlias,
		SelfAvatarURL: a.SelfAvatarURL,
		SelfCorpName:  a.SelfCorpName,
		SelfSyncedAt:  a.SelfSyncedAt,
		MetaJSON:      a.MetaJSON,
		Notes:         a.Notes,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
}

func (a *app) buildAdminAccountDetail(ctx context.Context, acc Account) (maskedAccount, error) {
	out := toMaskedAccount(acc)
	counts, err := newContactRepo(a.db).CountByAccount(ctx, acc.ID)
	if err != nil {
		return maskedAccount{}, err
	}
	status := a.contactSync.Status(acc.ID)
	out.ContactCount = counts.Contacts
	out.RoomCount = counts.Rooms
	out.RoomMemberCount = counts.RoomMembers
	out.IdentityLinkCount = counts.IdentityLinks
	out.ContactSyncLastAt = status.LastFullSyncAt
	out.ContactSyncLastError = status.LastError
	out.ContactSyncStatus = &status
	return out, nil
}

// handleAdminAccounts serves list (GET) and create (POST).
func (a *app) handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	repo := newAccountRepo(a.db)
	switch r.Method {
	case http.MethodGet:
		accounts, err := repo.ListAccounts(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
			return
		}
		out := make([]maskedAccount, 0, len(accounts))
		for _, acc := range accounts {
			out = append(out, toMaskedAccount(acc))
		}
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: out})
	case http.MethodPost:
		var req struct {
			GUID        string `json:"guid"`
			Token       string `json:"token"`
			DisplayName string `json:"displayName"`
			AgentID     string `json:"agentId"`
			Enabled     *bool  `json:"enabled,omitempty"`
			MetaJSON    string `json:"metaJson,omitempty"`
			Notes       string `json:"notes,omitempty"`
		}
		if err := decodeJSON(r.Body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
			return
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		acc, err := repo.CreateAccount(r.Context(), Account{
			GUID:        req.GUID,
			Token:       req.Token,
			DisplayName: req.DisplayName,
			AgentID:     req.AgentID,
			Enabled:     enabled,
			MetaJSON:    req.MetaJSON,
			Notes:       req.Notes,
		})
		if err != nil {
			if isUniqueConstraintErr(err) {
				writeJSON(w, http.StatusConflict, apiResponse{Success: false, Error: "guid already registered"})
				return
			}
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
			return
		}
		if err := a.reloadRegistry(r.Context()); err != nil {
			logger.Warn(r.Context(), "create 后 reload 失败", "tag", tagAdmin, "error", err.Error())
		}
		logger.Business(r.Context(), "创建账号",
			"tag", tagAdmin, "accountId", acc.ID, "guid", acc.GUID,
		)
		writeJSON(w, http.StatusCreated, apiResponse{Success: true, Data: toMaskedAccount(acc)})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
	}
}

// handleAdminAccountByID dispatches on path suffix:
//
//	/_admin/accounts/{id}             GET/PATCH/DELETE
//	/_admin/accounts/{id}/refresh     POST — trigger a profile sync
//	/_admin/accounts/{id}/resync      POST — trigger a contact full sync
//	/_admin/accounts/{id}/sync_status GET  — inspect contact sync status
//	/_admin/accounts/{id}/hard_delete POST — drop the row entirely
func (a *app) handleAdminAccountByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/qiwei/_admin/accounts/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "id is required"})
		return
	}
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	repo := newAccountRepo(a.db)
	switch action {
	case "":
		a.dispatchAccountCRUD(w, r, repo, id)
	case "refresh":
		a.handleAdminRefreshAccount(w, r, repo, id)
	case "resync":
		a.handleAdminResyncAccount(w, r, id)
	case "sync_status":
		a.handleAdminSyncStatus(w, r, id)
	case "hard_delete":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
			return
		}
		if err := repo.HardDeleteAccount(r.Context(), id); err != nil {
			if errors.Is(err, ErrAccountNotFound) {
				writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
			return
		}
		_ = a.reloadRegistry(r.Context())
		logger.Business(r.Context(), "硬删除账号", "tag", tagAdmin, "accountId", id)
		writeJSON(w, http.StatusOK, apiResponse{Success: true})
	default:
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "unknown sub-resource"})
	}
}

func (a *app) dispatchAccountCRUD(w http.ResponseWriter, r *http.Request, repo *accountRepo, id string) {
	switch r.Method {
	case http.MethodGet:
		acc, err := repo.GetAccount(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrAccountNotFound) {
				writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
			return
		}
		detail, err := a.buildAdminAccountDetail(r.Context(), acc)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: detail})
	case http.MethodPatch, http.MethodPut:
		var raw map[string]json.RawMessage
		if err := decodeJSON(r.Body, &raw); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
			return
		}
		patch, err := parseAccountPatch(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
			return
		}
		acc, err := repo.UpdateAccount(r.Context(), id, patch)
		if err != nil {
			if errors.Is(err, ErrAccountNotFound) {
				writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
			return
		}
		_ = a.reloadRegistry(r.Context())
		logger.Business(r.Context(), "更新账号", "tag", tagAdmin, "accountId", id)
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: toMaskedAccount(acc)})
	case http.MethodDelete:
		if err := repo.SoftDeleteAccount(r.Context(), id); err != nil {
			if errors.Is(err, ErrAccountNotFound) {
				writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
			return
		}
		_ = a.reloadRegistry(r.Context())
		logger.Business(r.Context(), "软删除账号", "tag", tagAdmin, "accountId", id)
		writeJSON(w, http.StatusOK, apiResponse{Success: true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
	}
}

func parseAccountPatch(raw map[string]json.RawMessage) (AccountPatch, error) {
	var p AccountPatch
	if v, ok := raw["token"]; ok {
		var s string
		if err := unmarshalSafe(v, &s); err != nil {
			return p, fmt.Errorf("invalid token")
		}
		p.Token = &s
	}
	if v, ok := raw["displayName"]; ok {
		var s string
		if err := unmarshalSafe(v, &s); err != nil {
			return p, fmt.Errorf("invalid displayName")
		}
		p.DisplayName = &s
	}
	if v, ok := raw["agentId"]; ok {
		var s string
		if err := unmarshalSafe(v, &s); err != nil {
			return p, fmt.Errorf("invalid agentId")
		}
		p.AgentID = &s
	}
	if v, ok := raw["enabled"]; ok {
		var b bool
		if err := unmarshalSafe(v, &b); err != nil {
			return p, fmt.Errorf("invalid enabled")
		}
		p.Enabled = &b
	}
	if v, ok := raw["metaJson"]; ok {
		var s string
		if err := unmarshalSafe(v, &s); err != nil {
			return p, fmt.Errorf("invalid metaJson")
		}
		p.MetaJSON = &s
	}
	if v, ok := raw["notes"]; ok {
		var s string
		if err := unmarshalSafe(v, &s); err != nil {
			return p, fmt.Errorf("invalid notes")
		}
		p.Notes = &s
	}
	return p, nil
}

// handleAdminRefreshAccount triggers a blocking profile sync for a single
// account and returns the refreshed row. Useful immediately after creating
// an account so operators can see the real display name.
func (a *app) handleAdminRefreshAccount(w http.ResponseWriter, r *http.Request, repo *accountRepo, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	rt, ok := a.currentRegistry().GetByID(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "account not found or disabled"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := a.syncAccountProfile(ctx, rt, repo); err != nil {
		writeJSON(w, http.StatusBadGateway, apiResponse{Success: false, Error: err.Error()})
		return
	}
	acc, err := repo.GetAccount(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
		return
	}
	_ = a.reloadRegistry(ctx)
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: toMaskedAccount(acc)})
}

func (a *app) handleAdminResyncAccount(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	rt, ok := a.currentRegistry().GetByID(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "account not found or disabled"})
		return
	}
	if a.contactSync != nil {
		a.contactSync.EnqueueFull(rt, "admin")
	}
	writeJSON(w, http.StatusAccepted, apiResponse{Success: true, Data: a.contactSync.Status(id)})
}

func (a *app) handleAdminSyncStatus(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	if _, err := newAccountRepo(a.db).GetAccount(r.Context(), id); err != nil {
		if errors.Is(err, ErrAccountNotFound) {
			writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: a.contactSync.Status(id)})
}

func (a *app) handleAdminReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	if err := a.reloadRegistry(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]any{
		"accounts": a.currentRegistry().Count(),
	}})
}

func (a *app) handleAdminUnknownGuids(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: a.unknownGuids.Snapshot()})
}

// isUniqueConstraintErr reports whether err is a SQLite UNIQUE violation.
// modernc.org/sqlite surfaces these with an error string that contains
// "UNIQUE constraint failed". We match on the substring to stay free of the
// driver-specific error types (which would force an import here).
func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
