package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	"agent/internal/config"
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

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	logger.Init(cfg.LogDir)
	defer logger.Flush()

	ctx := context.Background()

	if cfg.ConfigPath != "" {
		logger.Boundary(ctx, "已加载配置文件", "path", cfg.ConfigPath)
	}

	if err := storage.Init(cfg.DBPath); err != nil {
		logger.Error(ctx, "数据库初始化失败", "error", err.Error(), "path", cfg.DBPath)
		os.Exit(1)
	}

	if err := github.InitStore(cfg.GitHub); err != nil {
		logger.Error(ctx, "GitHub skill store 初始化失败", "error", err.Error())
		os.Exit(1)
	}

	schedulerCtx, cancelScheduler := context.WithCancel(ctx)
	go runner.StartDelayedTaskScheduler(schedulerCtx)

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
	api.Mount(mux, cfg.LogDir)

	adminDir := cfg.AdminDist
	if adminDir == "" {
		cwd, _ := os.Getwd()
		adminDir = filepath.Join(cwd, "admin", "dist")
	}
	if info, err := os.Stat(adminDir); err == nil && info.IsDir() {
		mux.Handle("/", spaHandler(adminDir, "/"))
		logger.Boundary(ctx, "Admin SPA 已挂载", "dir", adminDir)
	}

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: mux,
	}

	go func() {
		logger.Boundary(ctx, "Agent 启动中", "port", cfg.Port, "appId", cfg.ID, "version", cfg.Version)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, "服务启动失败", "error", err.Error())
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Boundary(ctx, "收到终止信号，开始优雅关闭")
	cancelScheduler()
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
