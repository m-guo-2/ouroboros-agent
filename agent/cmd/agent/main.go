package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"agent/internal/api"
	"agent/internal/channels"
	"agent/internal/dispatcher"
	"agent/internal/github"
	"agent/internal/logger"
	"agent/internal/runner"
	"agent/internal/storage"
)

var (
	isDraining    atomic.Bool
	inflightCount atomic.Int32
	startTime     = time.Now()
)

// loadEnv reads a .env file and sets any vars not already in the environment.
func loadEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		if idx := strings.Index(line, " #"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if os.Getenv(k) == "" {
			os.Setenv(k, strings.TrimSpace(v))
		}
	}
}

func main() {
	loadEnv(".env")
	loadEnv("../.env")

	port := os.Getenv("PORT")
	if port == "" {
		port = "1997"
	}

	appVersion := os.Getenv("AGENT_APP_VERSION")
	if appVersion == "" {
		appVersion = "1.0.0"
	}

	appID := os.Getenv("AGENT_ID")
	if appID == "" {
		appID = "agent-instance"
	}

	logDir := os.Getenv("LOG_DIR")
	if logDir == "" {
		logDir = filepath.Join("data", "logs")
	}
	logger.Init(logDir)
	defer logger.Flush()

	// Resolve database path (env override or default relative to cwd).
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join("data", "config.db")
	}

	ctx := context.Background()

	if err := storage.Init(dbPath); err != nil {
		logger.Error(ctx, "数据库初始化失败", "error", err.Error(), "path", dbPath)
		os.Exit(1)
	}

	if err := github.InitStore(); err != nil {
		logger.Error(ctx, "GitHub skill store 初始化失败", "error", err.Error())
		os.Exit(1)
	}

	// Initialise channel adapters (feishu, qiwei, webui).
	channels.InitBuiltinAdapters(func(key string) string {
		v, _ := storage.GetSettingValue(key)
		return v
	})

	mux := http.NewServeMux()

	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/drain", drainHandler)

	// New unified channel ingress — channel adapters POST here.
	mux.HandleFunc("/api/channels/incoming", dispatcher.HandleIncoming)

	// Outbound channel send — called by engine's send_channel_message tool.
	mux.HandleFunc("/api/data/channels/send", handleChannelSend)

	// Admin API routes (sessions, settings, agents, skills, users, traces).
	api.Mount(mux, logDir)

	// Serve admin SPA from admin/dist if available.
	// In Go's ServeMux, "/" is the catch-all that fires when no other pattern matches,
	// so it safely sits behind all /api/* routes.
	adminDir := os.Getenv("ADMIN_DIST")
	if adminDir == "" {
		// Resolve relative to cwd (agent/ runs from repo root by default).
		cwd, _ := os.Getwd()
		adminDir = filepath.Join(cwd, "admin", "dist")
	}
	if info, err := os.Stat(adminDir); err == nil && info.IsDir() {
		mux.Handle("/", spaHandler(adminDir, "/"))
		logger.Boundary(ctx, "Admin SPA 已挂载", "dir", adminDir)
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		logger.Boundary(ctx, "Agent 启动中", "port", port, "appId", appID, "version", appVersion)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, "服务启动失败", "error", err.Error())
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Boundary(ctx, "收到终止信号，开始优雅关闭")
	runner.GracefulShutdown()

	ctxShutdown, cancelShutdown := context.WithTimeout(ctx, 5*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(ctxShutdown); err != nil {
		logger.Error(ctx, "服务关闭出错", "error", err.Error())
	}
	logger.Boundary(ctx, "优雅关闭完成")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := "ready"
	if isDraining.Load() {
		status = "draining"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   status,
		"inflight": inflightCount.Load(),
		"uptime":   time.Since(startTime).Seconds(),
	})
}

func drainHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	isDraining.Store(true)
	runner.GracefulShutdown()
	count := inflightCount.Load()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"inflight": count,
	})
	if count <= 0 {
		go func() {
			time.Sleep(time.Second)
			os.Exit(0)
		}()
	}
}

// spaHandler serves a built React SPA from dir.
// All paths that do not match a real file on disk fall back to index.html
// for client-side routing.
func spaHandler(dir, _ string) http.Handler {
	fsys := os.DirFS(dir)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(fsys, p); err != nil {
			p = "index.html"
		}
		http.ServeFileFS(w, r, fsys, p)
	})
}

// handleChannelSend provides a local HTTP facade for the send_channel_message tool
// when it needs to call over HTTP (e.g. from tests or external callers).
// In normal operation the engine calls channels.SendToChannel directly.
func handleChannelSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg channels.OutgoingMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "invalid JSON"})
		return
	}

	if msg.Channel == "" || msg.ChannelUserID == "" || msg.Content == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "missing required fields: channel, channelUserId, content",
		})
		return
	}

	if err := channels.SendToChannel(msg); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}
