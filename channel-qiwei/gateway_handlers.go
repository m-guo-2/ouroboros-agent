package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

const tagGateway = "gateway"

var ErrGatewayRefreshRateLimited = errors.New("refresh rate limit exceeded")

func (a *app) registerGatewayRoutes(mux *http.ServeMux) {
	auth := a.gatewayAuthMiddleware
	mux.Handle("/api/qiwei/gateway/contacts/resolve", auth(http.HandlerFunc(a.handleGatewayResolveContact)))
	mux.Handle("/api/qiwei/gateway/contacts/search", auth(http.HandlerFunc(a.handleGatewaySearchContacts)))
	mux.Handle("/api/qiwei/gateway/rooms/", auth(http.HandlerFunc(a.handleGatewayRooms)))
	mux.Handle("/api/qiwei/gateway/identity-links", auth(http.HandlerFunc(a.handleGatewayIdentityLinks)))
}

func (a *app) gatewayAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := strings.TrimSpace(a.cfg.GatewayToken)
		if want == "" {
			http.NotFound(w, r)
			return
		}
		got := strings.TrimSpace(r.Header.Get("X-Gateway-Token"))
		if got == "" || got != want {
			writeJSON(w, http.StatusUnauthorized, apiResponse{Success: false, Error: "invalid gateway token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) handleGatewayResolveContact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	rt, err := a.resolveRuntimeForFacade(r.URL.Query().Get("accountId"))
	if err != nil {
		writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
		return
	}
	senderID := strings.TrimSpace(r.URL.Query().Get("senderId"))
	externalUserID := strings.TrimSpace(r.URL.Query().Get("externalUserId"))
	if senderID == "" && externalUserID == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "senderId or externalUserId is required"})
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1"

	repo := newContactRepo(a.db)
	var contact Contact
	switch {
	case senderID != "":
		contact, err = a.resolveGatewayContactByUserID(r.Context(), rt, senderID, refresh)
	case externalUserID != "":
		contact, err = a.resolveGatewayContactByExternalUserID(r.Context(), rt, externalUserID, refresh)
	}
	if err != nil {
		writeJSON(w, statusForGatewayLookup(err), apiResponse{Success: false, Error: err.Error()})
		return
	}
	links, err := repo.ListIdentityLinksForContact(r.Context(), rt.AccountID(), contact.UserID, contact.ExternalUserID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: map[string]any{
		"contact":         contact,
		"externalUserId":  contact.ExternalUserID,
		"identityLinks":   links,
		"lastSyncedAt":    contact.LastSyncedAt,
		"channelIdentity": rt.channelIdentity(),
	}})
}

func (a *app) handleGatewaySearchContacts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	rt, err := a.resolveRuntimeForFacade(r.URL.Query().Get("accountId"))
	if err != nil {
		writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
		return
	}
	limit := parseIntOrDefault(r.URL.Query().Get("limit"), 20)
	contacts, err := newContactRepo(a.db).SearchContacts(r.Context(), rt.AccountID(), r.URL.Query().Get("q"), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: contacts})
}

func (a *app) handleGatewayRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
		return
	}
	rt, err := a.resolveRuntimeForFacade(r.URL.Query().Get("accountId"))
	if err != nil {
		writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/qiwei/gateway/rooms/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "roomId is required"})
		return
	}
	parts := strings.Split(path, "/")
	roomID := parts[0]
	room, err := rt.gateway.ResolveRoom(r.Context(), roomID)
	if err != nil {
		writeJSON(w, statusForGatewayLookup(err), apiResponse{Success: false, Error: err.Error()})
		return
	}
	if len(parts) == 1 {
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: room})
		return
	}
	if len(parts) == 2 && parts[1] == "members" {
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: room.Members})
		return
	}
	writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Error: "unknown room sub-resource"})
}

func (a *app) handleGatewayIdentityLinks(w http.ResponseWriter, r *http.Request) {
	repo := newContactRepo(a.db)
	switch r.Method {
	case http.MethodGet:
		system := strings.TrimSpace(r.URL.Query().Get("downstreamSystem"))
		downstreamID := strings.TrimSpace(r.URL.Query().Get("downstreamId"))
		if system == "" || downstreamID == "" {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "downstreamSystem and downstreamId are required"})
			return
		}
		link, err := repo.ResolveIdentityLink(r.Context(), system, downstreamID)
		if err != nil {
			writeJSON(w, statusForGatewayLookup(err), apiResponse{Success: false, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: link})
	case http.MethodPost:
		var req struct {
			AccountID        string         `json:"accountId,omitempty"`
			UserID           string         `json:"userId,omitempty"`
			ExternalUserID   string         `json:"externalUserId,omitempty"`
			DownstreamSystem string         `json:"downstreamSystem"`
			DownstreamID     string         `json:"downstreamId"`
			Meta             map[string]any `json:"meta,omitempty"`
		}
		if err := decodeJSON(r.Body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid json"})
			return
		}
		rt, err := a.resolveRuntimeForFacade(req.AccountID)
		if err != nil {
			writeJSON(w, statusForRouting(err), apiResponse{Success: false, Error: err.Error()})
			return
		}
		metaJSON := "{}"
		if len(req.Meta) > 0 {
			raw, err := json.Marshal(req.Meta)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "invalid meta"})
				return
			}
			metaJSON = string(raw)
		}
		result, err := repo.UpsertIdentityLink(r.Context(), IdentityLink{
			AccountID:          rt.AccountID(),
			UserID:             strings.TrimSpace(req.UserID),
			ExternalUserID:     strings.TrimSpace(req.ExternalUserID),
			DownstreamSystem:   req.DownstreamSystem,
			DownstreamID:       req.DownstreamID,
			DownstreamMetaJSON: metaJSON,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: err.Error()})
			return
		}
		if result.Reassigned && result.Previous != nil {
			logger.Business(r.Context(), "identity link reassigned",
				"tag", tagGateway,
				"downstreamSystem", result.Link.DownstreamSystem,
				"downstreamId", result.Link.DownstreamID,
				"previousExternalUserId", result.Previous.ExternalUserID,
				"newExternalUserId", result.Link.ExternalUserID,
			)
		}
		status := http.StatusOK
		if result.Created {
			status = http.StatusCreated
		}
		writeJSON(w, status, apiResponse{Success: true, Data: result.Link})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
	}
}

func (a *app) resolveGatewayContactByUserID(ctx context.Context, rt *accountRuntime, userID string, refresh bool) (Contact, error) {
	if !refresh {
		return rt.gateway.ResolveContact(ctx, userID)
	}
	key := rt.AccountID() + ":" + userID
	if a.gatewayRefreshLimiter != nil && a.gatewayRefreshLimiter.Seen(key) {
		return Contact{}, ErrGatewayRefreshRateLimited
	}
	return rt.gateway.fetchContact(ctx, userID)
}

func (a *app) resolveGatewayContactByExternalUserID(ctx context.Context, rt *accountRuntime, externalUserID string, refresh bool) (Contact, error) {
	repo := newContactRepo(a.db)
	if !refresh {
		return repo.GetContactByExternalUserID(ctx, rt.AccountID(), externalUserID)
	}
	contact, err := repo.GetContactByExternalUserID(ctx, rt.AccountID(), externalUserID)
	if err != nil {
		return Contact{}, err
	}
	key := rt.AccountID() + ":" + contact.UserID
	if a.gatewayRefreshLimiter != nil && a.gatewayRefreshLimiter.Seen(key) {
		return Contact{}, ErrGatewayRefreshRateLimited
	}
	return rt.gateway.fetchContact(ctx, contact.UserID)
}

func statusForGatewayLookup(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrContactNotFound), errors.Is(err, ErrRoomNotFound), errors.Is(err, ErrIdentityLinkNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrGatewayRefreshRateLimited):
		return http.StatusTooManyRequests
	default:
		return http.StatusBadGateway
	}
}
