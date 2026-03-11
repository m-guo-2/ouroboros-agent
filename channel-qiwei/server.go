package main

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"channel-qiwei/internal/modules"
	sharedoss "github.com/m-guo-2/ouroboros-agent/shared/oss"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

type app struct {
	cfg           Config
	client        *qiweiClient
	http          *http.Client
	recognizer    recognizer
	storage       sharedoss.Storage
	storageConfig sharedoss.Config
	registry      modules.Registry
	dedupe        *ttlSet
	nameCache     *ttlCache

	contactsMu       sync.Mutex
	contactsLoadedAt time.Time
}

func newApp(cfg Config) *app {
	logger.Init(cfg.LogDir, "channel-qiwei")
	storageRuntime := newObjectStorage(cfg.OSS)
	return &app{
		cfg:           cfg,
		client:        newQiweiClient(cfg),
		http:          logger.NewClient("http-download", time.Duration(cfg.RequestTimout)*time.Second),
		recognizer:    newVolcengineRecognizer(cfg),
		storage:       storageRuntime.store,
		storageConfig: storageRuntime.cfg,
		registry:      modules.BuildRegistry(),
		dedupe:        newTTLSet(5 * time.Minute),
		nameCache:     newTTLCache(10 * time.Minute),
	}
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/api/health", a.handleHealth)
	mux.HandleFunc("/webhook/callback", a.handleWebhookCallback)
	mux.HandleFunc("/api/qiwei/search_targets", a.handleSearchTargets)
	mux.HandleFunc("/api/qiwei/list_or_get_conversations", a.handleListOrGetConversations)
	mux.HandleFunc("/api/qiwei/parse_message", a.handleParseMessage)
	mux.HandleFunc("/api/qiwei/send_message", a.handleFacadeSendMessage)
	mux.HandleFunc("/api/qiwei/send", a.handleSend)
	mux.HandleFunc("/api/qiwei/do", a.handleDoAPI)
	mux.HandleFunc("/api/qiwei/", a.handleModuleAction)

	logMiddleware := logger.Middleware(logger.MiddlewareOptions{
		SkipPaths: map[string]bool{"/health": true, "/api/health": true},
	})
	return logMiddleware(withJSONMiddleware(mux))
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
