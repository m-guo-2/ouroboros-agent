package main

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"channel-qiwei/internal/modules"
	sharedoss "github.com/m-guo-2/ouroboros-agent/shared/oss"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

// app is the long-lived process state. Everything that depends on a specific
// qiwei account lives on *accountRuntime; app keeps only per-process
// resources (HTTP clients, storage, module registry, DB, account registry).
type app struct {
	cfg           Config
	http          *http.Client
	recognizer    recognizer
	storage       sharedoss.Storage
	storageConfig sharedoss.Config
	modules       modules.Registry

	db *sql.DB

	// registry holds the current *accountRegistry; we swap the pointer on
	// hot reload so the running request handlers always observe a
	// consistent snapshot.
	registry atomic.Pointer[accountRegistry]

	// unknownGuids buffers callback events for guids that are not (yet)
	// in the registry; the admin API exposes this for operators to
	// discover "forgot to register" situations.
	unknownGuids *unknownGuidBuffer

	contactSync *contactSyncer

	gatewayRefreshLimiter *ttlSet
}

func newApp(cfg Config, db *sql.DB) *app {
	logger.Init(cfg.LogDir, "channel-qiwei")
	storageRuntime := newObjectStorage(cfg.OSS)
	a := &app{
		cfg:                   cfg,
		http:                  logger.NewClient("http-download", time.Duration(cfg.RequestTimout)*time.Second),
		recognizer:            newVolcengineRecognizer(cfg),
		storage:               storageRuntime.store,
		storageConfig:         storageRuntime.cfg,
		modules:               modules.BuildRegistry(),
		db:                    db,
		unknownGuids:          newUnknownGuidBuffer(200),
		gatewayRefreshLimiter: newTTLSet(30 * time.Second),
	}
	a.contactSync = newContactSyncer(a)
	a.registry.Store(newAccountRegistry())
	return a
}

// currentRegistry returns the currently active registry snapshot. Hot paths
// call this on every request; it's cheap because of atomic.Pointer.
func (a *app) currentRegistry() *accountRegistry {
	return a.registry.Load()
}

// reloadRegistry rebuilds the registry from the DB and atomically swaps the
// pointer. Safe to call concurrently with in-flight requests.
func (a *app) reloadRegistry(ctx context.Context) error {
	reg, err := loadRegistry(ctx, a.db, a.cfg)
	if err != nil {
		return err
	}
	a.registry.Store(reg)
	return nil
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
	mux.HandleFunc("/api/qiwei/get_group_detail", a.handleGetGroupDetail)
	mux.HandleFunc("/api/qiwei/get_contact_detail", a.handleGetContactDetail)

	// Admin endpoints are mounted unconditionally. The auth middleware
	// decides how to treat requests: with a configured X-Admin-Token it
	// enforces the token; with an empty token it runs in debug mode and
	// logs a warning. Registering here (even without a token) also keeps
	// the /api/qiwei/ catch-all below from swallowing admin paths.
	a.registerAdminRoutes(mux)
	a.registerGatewayRoutes(mux)

	// Module action catch-all must be registered last so the specific
	// handlers above take priority.
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

// preloadKnownRooms hydrates each account's roomStore from the qiwei
// platform so restarts don't re-emit group_joined for groups the bot has
// already been in.
func (a *app) preloadKnownRooms(ctx context.Context) {
	for _, rt := range a.currentRegistry().All() {
		a.preloadKnownRoomsFor(ctx, rt)
	}
}

// preloadKnownRoomsFor pulls the current room list for one account and
// merges it into that account's roomStore. Failures are logged and
// swallowed: preload is an optimization, not a correctness requirement.
func (a *app) preloadKnownRoomsFor(ctx context.Context, rt *accountRuntime) {
	items, err := a.listGroups(ctx, rt)
	if err != nil {
		logger.Warn(ctx, "预加载群列表失败",
			"tag", tagCallback,
			"accountId", rt.AccountID(),
			"error", err.Error(),
		)
		return
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if id := firstNonEmpty(anyToString(item["roomId"]), anyToString(item["id"])); id != "" {
			ids = append(ids, id)
		}
	}
	rt.roomStore.Merge(ids)
	logger.Business(ctx, "预加载已知群",
		"tag", tagCallback,
		"accountId", rt.AccountID(),
		"count", len(ids),
	)
}

func (a *app) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"service":   "channel-qiwei",
		"timestamp": time.Now().Format(time.RFC3339),
		"accounts":  a.currentRegistry().Count(),
	})
}
