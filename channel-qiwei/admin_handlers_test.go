package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// adminTestApp spins up an *app backed by a fresh on-disk SQLite database
// with the admin token configured. It returns the app and its handler so
// tests can talk to admin endpoints end-to-end.
func adminTestApp(t *testing.T, token string) (*app, http.Handler) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "qiwei.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := Config{
		APIBaseURL:    "http://example.com",
		RequestTimout: 1,
		AdminToken:    token,
	}
	a := newApp(cfg, db)
	if err := a.reloadRegistry(context.Background()); err != nil {
		t.Fatalf("reload: %v", err)
	}
	return a, a.routes()
}

func adminReq(method, path, token string, body any) *http.Request {
	var buf io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		buf = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, buf)
	if token != "" {
		req.Header.Set("X-Admin-Token", token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func decodeResponse(t *testing.T, rr *httptest.ResponseRecorder) apiResponse {
	t.Helper()
	var resp apiResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode json: %v body=%s", err, rr.Body.String())
	}
	return resp
}

func TestAdminAuthRejectsMissingToken(t *testing.T) {
	_, handler := adminTestApp(t, "secret")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, adminReq(http.MethodGet, "/api/qiwei/_admin/accounts", "", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminAuthRejectsWrongToken(t *testing.T) {
	_, handler := adminTestApp(t, "secret")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, adminReq(http.MethodGet, "/api/qiwei/_admin/accounts", "nope", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong token, got %d", rr.Code)
	}
}

func TestAdminRoutesAllowWhenTokenEmpty(t *testing.T) {
	// Empty AdminToken switches the admin API into "debug mode": the
	// middleware logs a warning but lets every request through so local
	// admin UIs can iterate without juggling credentials. This test pins
	// that contract — production deployments should set a token and rely
	// on the explicit 401 paths instead.
	_, handler := adminTestApp(t, "")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, adminReq(http.MethodGet, "/api/qiwei/_admin/accounts", "", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 in debug mode, got %d body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeResponse(t, rr)
	if !resp.Success {
		t.Fatalf("expected success=true, got %+v", resp)
	}
}

func TestAdminCreateAccountMasksTokenAndUpdatesRegistry(t *testing.T) {
	a, handler := adminTestApp(t, "secret")

	body := map[string]any{
		"guid":        "guid-xyz",
		"token":       "very-secret-token",
		"displayName": "demo",
		"agentId":     "agent-1",
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, adminReq(http.MethodPost, "/api/qiwei/_admin/accounts", "secret", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeResponse(t, rr)
	if !resp.Success {
		t.Fatalf("expected success, got %+v", resp)
	}
	data := resp.Data.(map[string]any)
	if tok, _ := data["token_preview"].(string); tok != "very***" {
		t.Fatalf("expected masked token preview, got %q", tok)
	}
	if _, hasToken := data["token"]; hasToken {
		t.Fatalf("raw token should not be exposed in response")
	}
	if a.currentRegistry().Count() != 1 {
		t.Fatalf("expected registry to contain 1 account after create, got %d",
			a.currentRegistry().Count())
	}
}

func TestAdminCreateAccountConflictOnDuplicateGUID(t *testing.T) {
	_, handler := adminTestApp(t, "secret")

	body := map[string]any{"guid": "dup", "token": "t1"}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, adminReq(http.MethodPost, "/api/qiwei/_admin/accounts", "secret", body))
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 first create, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, adminReq(http.MethodPost, "/api/qiwei/_admin/accounts", "secret", body))
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate guid, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminSoftDeleteRemovesFromRegistry(t *testing.T) {
	a, handler := adminTestApp(t, "secret")

	body := map[string]any{"guid": "to-delete", "token": "tok"}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, adminReq(http.MethodPost, "/api/qiwei/_admin/accounts", "secret", body))
	resp := decodeResponse(t, rr)
	id := resp.Data.(map[string]any)["id"].(string)
	if a.currentRegistry().Count() != 1 {
		t.Fatalf("setup: expected 1 account, got %d", a.currentRegistry().Count())
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, adminReq(http.MethodDelete,
		fmt.Sprintf("/api/qiwei/_admin/accounts/%s", id), "secret", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 on soft delete, got %d body=%s", rr.Code, rr.Body.String())
	}
	if a.currentRegistry().Count() != 0 {
		t.Fatalf("expected registry to drop disabled account, got %d",
			a.currentRegistry().Count())
	}
}

func TestAdminListAccountsIncludesDisabledRows(t *testing.T) {
	a, handler := adminTestApp(t, "secret")

	// create two accounts, disable one
	create := func(guid string) string {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, adminReq(http.MethodPost, "/api/qiwei/_admin/accounts", "secret",
			map[string]any{"guid": guid, "token": "t"}))
		if rr.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", guid, rr.Code, rr.Body.String())
		}
		return decodeResponse(t, rr).Data.(map[string]any)["id"].(string)
	}
	create("g1")
	id2 := create("g2")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, adminReq(http.MethodDelete,
		fmt.Sprintf("/api/qiwei/_admin/accounts/%s", id2), "secret", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("soft delete: %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, adminReq(http.MethodGet, "/api/qiwei/_admin/accounts", "secret", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list: %d", rr.Code)
	}
	resp := decodeResponse(t, rr)
	list := resp.Data.([]any)
	if len(list) != 2 {
		t.Fatalf("expected list to include disabled rows, got %d", len(list))
	}
	if a.currentRegistry().Count() != 1 {
		t.Fatalf("registry should only have the enabled account, got %d", a.currentRegistry().Count())
	}
}

func TestAdminReloadRegistryEndpoint(t *testing.T) {
	_, handler := adminTestApp(t, "secret")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, adminReq(http.MethodPost, "/api/qiwei/_admin/reload", "secret", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("reload: %d body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeResponse(t, rr)
	data := resp.Data.(map[string]any)
	if int(data["accounts"].(float64)) != 0 {
		t.Fatalf("expected accounts=0, got %v", data["accounts"])
	}
}

func TestAdminUnknownGuidsReturnsBufferedEvents(t *testing.T) {
	a, handler := adminTestApp(t, "secret")
	a.unknownGuids.Record(unknownGuidEvent{GUID: "ghost-1", Cmd: 15000, MsgType: 1, MsgSvrID: "m1", At: 1})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, adminReq(http.MethodGet, "/api/qiwei/_admin/unknown_guids", "secret", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("list unknown: %d body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeResponse(t, rr)
	arr := resp.Data.([]any)
	if len(arr) != 1 {
		t.Fatalf("expected one buffered event, got %d", len(arr))
	}
	first := arr[0].(map[string]any)
	if first["guid"] != "ghost-1" {
		t.Fatalf("unexpected payload: %+v", first)
	}
}

func TestMaskTokenForms(t *testing.T) {
	cases := map[string]string{
		"":       "",
		"ab":     "ab***",
		"abc":    "abc***",
		"abcd":   "abcd***",
		"abcde":  "abcd***",
		"abcdef": "abcd***",
	}
	for in, want := range cases {
		if got := maskToken(in); got != want {
			t.Fatalf("maskToken(%q)=%q, want %q", in, got, want)
		}
	}
}
