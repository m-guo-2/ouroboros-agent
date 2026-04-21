package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func gatewayTestApp(t *testing.T, token string) (*app, http.Handler) {
	t.Helper()
	cfg := Config{
		APIBaseURL:          "http://example.com",
		RequestTimout:       1,
		GatewayToken:        token,
		ContactSyncEnabled:  false,
		ContactSyncInterval: time.Hour,
	}
	a := newTestAppWithCfg(t, cfg)
	return a, a.routes()
}

func gatewayReq(method, path, token string, body any) *http.Request {
	var buf *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		buf = bytes.NewReader(raw)
	} else {
		buf = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	if token != "" {
		req.Header.Set("X-Gateway-Token", token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestGatewayAuthRejectsMissingToken(t *testing.T) {
	_, handler := gatewayTestApp(t, "secret")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, gatewayReq(http.MethodGet, "/api/qiwei/gateway/contacts/search?q=a", "", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without gateway token, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGatewayRoutesHiddenWhenTokenEmpty(t *testing.T) {
	_, handler := gatewayTestApp(t, "")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, gatewayReq(http.MethodGet, "/api/qiwei/gateway/contacts/search?q=a", "ignored", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when gateway disabled, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGatewayResolveContactAndSearch(t *testing.T) {
	a, handler := gatewayTestApp(t, "secret")
	rt := testRuntime(t, a)
	repo := newContactRepo(a.db)
	ctx := context.Background()
	if err := repo.UpsertContact(ctx, Contact{
		AccountID:      rt.AccountID(),
		UserID:         "user-1",
		ExternalUserID: "ext-1",
		Nickname:       "Alice",
		Remark:         "VIP Alice",
		Source:         "external",
		RawJSON:        `{"userId":"user-1"}`,
	}); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	if _, err := repo.UpsertIdentityLink(ctx, IdentityLink{
		AccountID:        rt.AccountID(),
		UserID:           "user-1",
		ExternalUserID:   "ext-1",
		DownstreamSystem: "weapp",
		DownstreamID:     "openid-1",
	}); err != nil {
		t.Fatalf("seed identity link: %v", err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, gatewayReq(http.MethodGet, "/api/qiwei/gateway/contacts/resolve?senderId=user-1", "secret", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("resolve contact: %d body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeResponse(t, rr)
	data := resp.Data.(map[string]any)
	if data["externalUserId"] != "ext-1" {
		t.Fatalf("expected external user id, got %+v", data)
	}
	links := data["identityLinks"].([]any)
	if len(links) != 1 {
		t.Fatalf("expected one identity link, got %+v", links)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, gatewayReq(http.MethodGet, "/api/qiwei/gateway/contacts/search?q=VIP", "secret", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("search contacts: %d body=%s", rr.Code, rr.Body.String())
	}
	resp = decodeResponse(t, rr)
	results := resp.Data.([]any)
	if len(results) != 1 {
		t.Fatalf("expected one search result, got %+v", results)
	}
}

func TestGatewayRoomsAndIdentityLinks(t *testing.T) {
	a, handler := gatewayTestApp(t, "secret")
	rt := testRuntime(t, a)
	repo := newContactRepo(a.db)
	ctx := context.Background()

	if err := repo.UpsertRoom(ctx, Room{
		AccountID: rt.AccountID(),
		RoomID:    "room-1",
		Name:      "测试群",
		RawJSON:   `{"roomId":"room-1"}`,
	}); err != nil {
		t.Fatalf("seed room: %v", err)
	}
	if err := repo.ReplaceRoomMembers(ctx, rt.AccountID(), "room-1", []RoomMember{
		{AccountID: rt.AccountID(), RoomID: "room-1", UserID: "owner-1", DisplayName: "群主", Role: "owner"},
		{AccountID: rt.AccountID(), RoomID: "room-1", UserID: "member-1", DisplayName: "成员", Role: "member"},
	}); err != nil {
		t.Fatalf("seed room members: %v", err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, gatewayReq(http.MethodGet, "/api/qiwei/gateway/rooms/room-1/members", "secret", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("room members: %d body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeResponse(t, rr)
	members := resp.Data.([]any)
	if len(members) != 2 {
		t.Fatalf("expected two room members, got %+v", members)
	}

	postBody := map[string]any{
		"userId":           "user-1",
		"externalUserId":   "ext-1",
		"downstreamSystem": "weapp",
		"downstreamId":     "openid-1",
		"meta":             map[string]any{"appid": "wx123"},
	}
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, gatewayReq(http.MethodPost, "/api/qiwei/gateway/identity-links", "secret", postBody))
	if rr.Code != http.StatusCreated {
		t.Fatalf("create identity link: %d body=%s", rr.Code, rr.Body.String())
	}

	postBody["externalUserId"] = "ext-2"
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, gatewayReq(http.MethodPost, "/api/qiwei/gateway/identity-links", "secret", postBody))
	if rr.Code != http.StatusOK {
		t.Fatalf("update identity link: %d body=%s", rr.Code, rr.Body.String())
	}
}
