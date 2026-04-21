package api

// Reverse proxy for channel-qiwei admin API.
//
// The channel-qiwei process owns the qiwei_accounts table and the /_admin/*
// endpoints that mutate it. The admin SPA only talks to this agent process
// (`:1997`) though, so we proxy /api/qiwei/_admin/* straight through. This
// keeps the admin UI pointed at one base URL and lets the agent inject the
// X-Admin-Token when one is configured — the browser never sees it.
//
// We deliberately re-dial the target on every request instead of caching a
// *httputil.ReverseProxy: ResolveQiweiBaseURL reads both config and a
// settings callback, so the destination can change at runtime when the port
// is reconfigured, and the cost of re-resolving is negligible for an admin
// panel.

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"agent/internal/config"
	"agent/internal/storage"

	sharedlogger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

// qiweiAdminPrefix is the outward path clients hit. We keep it identical to
// the one channel-qiwei exposes so the proxy is a straight pass-through —
// one fewer mapping to remember.
const qiweiAdminPrefix = "/api/qiwei/_admin/"

func handleQiweiAdminProxy(w http.ResponseWriter, r *http.Request) {
	baseURL := config.ResolveQiweiBaseURL(func(key string) string {
		v, _ := storage.GetSettingValue(key)
		return v
	})
	target, err := url.Parse(baseURL)
	if err != nil {
		sharedlogger.Warn(r.Context(), "解析 channel-qiwei base URL 失败",
			"tag", "qiwei-admin-proxy", "baseURL", baseURL, "error", err.Error(),
		)
		apiErr(w, http.StatusBadGateway, "invalid channel-qiwei base URL")
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		if token := strings.TrimSpace(os.Getenv("QIWEI_ADMIN_TOKEN")); token != "" {
			req.Header.Set("X-Admin-Token", token)
		}
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, err error) {
		sharedlogger.Warn(r.Context(), "转发 channel-qiwei admin 请求失败",
			"tag", "qiwei-admin-proxy", "path", r.URL.Path, "error", err.Error(),
		)
		apiErr(rw, http.StatusBadGateway, "channel-qiwei unreachable: "+err.Error())
	}

	proxy.ServeHTTP(w, r)
}
