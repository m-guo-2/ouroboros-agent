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
	"agent/internal/cardrender"
	"agent/internal/channels"
	"agent/internal/config"
	"agent/internal/dispatcher"
	"agent/internal/eventlog"
	"agent/internal/github"
	"agent/internal/logger"
	"agent/internal/runner"
	"agent/internal/storage"

	sharedlogger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

var (
	isDraining    atomic.Bool
	inflightCount atomic.Int32
	startTime     = time.Now()
)

func main() {
	bootAt := time.Now()
	emitBootstrap("启动 agent 进程", "pid", os.Getpid(), "args", os.Args)

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	emitBootstrap("配置加载完成",
		"configPath", displayValue(cfg.ConfigPath, "<default-or-env>"),
		"logDir", cfg.LogDir,
		"dbPath", cfg.DBPath,
		"port", cfg.Port,
		"adminDist", displayValue(cfg.AdminDist, "<auto-detect>"),
	)

	logger.Init(cfg.LogDir)
	defer logger.Flush()

	ctx := context.Background()
	logger.Boundary(ctx, "启动配置摘要",
		"pid", os.Getpid(),
		"configPath", displayValue(cfg.ConfigPath, "<default-or-env>"),
		"logDir", cfg.LogDir,
		"dbPath", cfg.DBPath,
		"port", cfg.Port,
		"adminDist", displayValue(cfg.AdminDist, "<auto-detect>"),
		"githubRepo", cfg.GitHub.SkillsRepo,
		"githubBranch", displayValue(cfg.GitHub.Branch, "<default>"),
		"githubSkillsPath", displayValue(cfg.GitHub.SkillsPath, "<default>"),
		"githubLocalDir", displayValue(cfg.GitHub.SkillsLocalDir, "<default>"),
		"githubSyncInterval", cfg.GitHub.ParseSyncInterval().String(),
	)

	if cfg.ConfigPath != "" {
		logger.Boundary(ctx, "已加载配置文件", "path", cfg.ConfigPath)
	}

	if ossCfg := cfg.OSS.ToSharedConfig(); ossCfg.Endpoint != "" {
		if err := cardrender.InitWith(ossCfg); err != nil {
			logger.Warn(ctx, "OSS/cardrender 初始化失败（媒体中转将不可用）", "error", err.Error())
		} else {
			logger.Boundary(ctx, "OSS/cardrender 初始化完成", "endpoint", ossCfg.Endpoint, "bucket", ossCfg.Bucket)
		}
	}

	stepAt := time.Now()
	logger.Boundary(ctx, "开始初始化数据库", "path", cfg.DBPath)
	if err := storage.Init(cfg.DBPath); err != nil {
		logger.Error(ctx, "数据库初始化失败", "error", err.Error(), "path", cfg.DBPath)
		os.Exit(1)
	}
	logger.Boundary(ctx, "数据库初始化完成", "path", cfg.DBPath, "elapsed", time.Since(stepAt).String())

	// --- Session recovery: resume sessions interrupted by previous crash ---
	stepAt = time.Now()
	logger.Boundary(ctx, "开始恢复中断会话")
	recoverSessions(ctx)
	logger.Boundary(ctx, "会话恢复扫描完成", "elapsed", time.Since(stepAt).String())

	// --- Fast init: no network, all local ---
	stepAt = time.Now()
	logger.Boundary(ctx, "开始初始化 GitHub skill store")
	if err := github.NewStore(cfg.GitHub); err != nil {
		logger.Error(ctx, "GitHub skill store 初始化失败", "error", err.Error())
		os.Exit(1)
	}
	logger.Boundary(ctx, "GitHub skill store 初始化完成",
		"skillsRepo", cfg.GitHub.SkillsRepo,
		"skillsPath", displayValue(cfg.GitHub.SkillsPath, "<default>"),
		"localDir", displayValue(cfg.GitHub.SkillsLocalDir, "<default>"),
		"elapsed", time.Since(stepAt).String(),
	)

	stepAt = time.Now()
	logger.Boundary(ctx, "开始初始化内置 channel adapters")
	channels.InitBuiltinAdapters(func(key string) string {
		v, _ := storage.GetSettingValue(key)
		return v
	})
	logger.Boundary(ctx, "内置 channel adapters 初始化完成", "elapsed", time.Since(stepAt).String())

	mux := http.NewServeMux()

	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/drain", drainHandler)

	mux.HandleFunc("/api/channels/incoming", dispatcher.HandleIncoming)
	mux.HandleFunc("/api/channels/group-event", dispatcher.HandleGroupEvent)
	mux.HandleFunc("/api/data/channels/send", handleChannelSend)

	api.Mount(mux, sharedlogger.GetReader())

	adminDir := cfg.AdminDist
	if adminDir == "" {
		cwd, _ := os.Getwd()
		adminDir = filepath.Join(cwd, "admin", "dist")
		logger.Detail(ctx, "未显式配置 adminDist，使用默认路径", "cwd", cwd, "dir", adminDir)
	}
	if info, err := os.Stat(adminDir); err == nil && info.IsDir() {
		mux.Handle("/", spaHandler(adminDir, "/"))
		logger.Boundary(ctx, "Admin SPA 已挂载", "dir", adminDir)
	} else if err != nil {
		logger.Warn(ctx, "Admin SPA 未挂载", "dir", adminDir, "error", err.Error())
	} else {
		logger.Warn(ctx, "Admin SPA 未挂载：路径不是目录", "dir", adminDir)
	}

	logMiddleware := sharedlogger.Middleware(sharedlogger.MiddlewareOptions{
		SkipPaths: map[string]bool{"/health": true},
	})

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: logMiddleware(mux),
	}

	// --- Start HTTP server immediately so health check is available ---
	go func() {
		logger.Boundary(ctx, "HTTP 服务开始监听", "addr", srv.Addr, "appId", cfg.ID, "version", cfg.Version)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, "服务启动失败", "error", err.Error())
			logger.Flush()
			os.Exit(1)
		}
	}()

	// --- Slow init: network calls in background goroutines ---

	go func() {
		loadAt := time.Now()
		logger.Boundary(ctx, "开始后台加载 GitHub skills 缓存")
		if err := github.DefaultStore.LoadCache(); err != nil {
			logger.Error(ctx, "GitHub skill 缓存加载失败（后台重试将继续）", "error", err.Error())
		} else {
			logger.Boundary(ctx, "GitHub skills 缓存加载完成", "elapsed", time.Since(loadAt).String())
		}
		interval := cfg.GitHub.ParseSyncInterval()
		logger.Boundary(ctx, "启动 GitHub skills 周期同步", "interval", interval.String())
		github.DefaultStore.StartSync(interval)
	}()

	schedulerCtx, cancelScheduler := context.WithCancel(ctx)
	logger.Boundary(ctx, "启动延迟任务调度器")
	go runner.StartDelayedTaskScheduler(schedulerCtx)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	logger.Boundary(ctx, "Agent 启动完成，进入运行态",
		"elapsed", time.Since(bootAt).String(),
		"health", "/health",
		"drain", "/drain",
	)
	<-stop

	logger.Boundary(ctx, "收到终止信号，开始优雅关闭")
	github.DefaultStore.StopSync()
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
	status := "starting"
	if isDraining.Load() {
		status = "draining"
	} else if github.DefaultStore != nil && github.DefaultStore.Ready() {
		status = "ready"
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

// recoverSessions scans for sessions that were in "processing" state when
// the previous process exited. Sessions with pending events are resumed;
// sessions with no pending events are marked "interrupted".
func recoverSessions(ctx context.Context) {
	sessions, err := storage.GetProcessingSessionsWithPendingEvents()
	if err != nil {
		logger.Error(ctx, "会话恢复扫描失败", "error", err.Error())
		return
	}
	if len(sessions) == 0 {
		return
	}

	logger.Boundary(ctx, "发现需恢复的会话", "count", len(sessions))

	for _, sd := range sessions {
		has, err := storage.HasSessionEventsAfter(sd.ID, sd.EventCursor)
		if err != nil {
			logger.Error(ctx, "检查会话待处理事件失败",
				"sessionId", sd.ID, "error", err.Error())
			continue
		}

		if !has {
			_ = storage.UpdateSession(sd.ID, map[string]interface{}{
				"executionStatus": "interrupted",
			})
			logger.Business(ctx, "会话无待处理事件，标记为中断",
				"sessionId", sd.ID)
			continue
		}

		el := eventlog.New(sd.ID, sd.EventCursor)
		_ = runner.RecoverSession(ctx, sd, el)
		logger.Business(ctx, "会话已恢复",
			"sessionId", sd.ID, "eventCursor", sd.EventCursor)
	}
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

func emitBootstrap(msg string, args ...any) {
	parts := make([]string, 0, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		parts = append(parts, fmt.Sprintf("%v=%v", args[i], args[i+1]))
	}
	if len(parts) == 0 {
		fmt.Fprintf(os.Stderr, "[bootstrap] %s\n", msg)
		return
	}
	fmt.Fprintf(os.Stderr, "[bootstrap] %s %s\n", msg, strings.Join(parts, " "))
}

func displayValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
