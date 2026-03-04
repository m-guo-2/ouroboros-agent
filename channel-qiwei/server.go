package main

import (
	"net/http"
	"strings"
	"time"

	"channel-qiwei/internal/modules"
)

type app struct {
	cfg      Config
	client   *qiweiClient
	http     *http.Client
	registry modules.Registry
	dedupe   *ttlSet
}

func newApp(cfg Config) *app {
	return &app{
		cfg:      cfg,
		client:   newQiweiClient(cfg),
		http:     &http.Client{Timeout: time.Duration(cfg.RequestTimout) * time.Second},
		registry: modules.BuildRegistry(),
		dedupe:   newTTLSet(5 * time.Minute),
	}
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/api/health", a.handleHealth)
	mux.HandleFunc("/webhook/callback", a.handleWebhookCallback)
	mux.HandleFunc("/api/qiwei/send", a.handleSend)
	mux.HandleFunc("/api/qiwei/do", a.handleDoAPI)
	mux.HandleFunc("/api/qiwei/", a.handleModuleAction)
	return withJSONMiddleware(mux)
}

func withJSONMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"service":   "channel-qiwei",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
