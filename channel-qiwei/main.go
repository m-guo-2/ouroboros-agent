package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

func main() {
	cfg := LoadConfig()
	if err := cfg.Validate(); err != nil {
		fmt.Println("Missing required QiWei config:", err.Error())
		os.Exit(1)
	}

	ctx := context.Background()

	db, err := OpenDB(cfg.DBPath)
	if err != nil {
		fmt.Println("open qiwei.db failed:", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	seeded, err := seedFromYAML(ctx, db, cfg)
	if err != nil {
		fmt.Println("seed default account failed:", err.Error())
		os.Exit(1)
	}

	app := newApp(cfg, db)

	if err := app.reloadRegistry(ctx); err != nil {
		fmt.Println("load account registry failed:", err.Error())
		os.Exit(1)
	}

	// Legacy single-file known_rooms.txt → qiwei_known_rooms migration.
	// Safe to run every boot: file-absent is a no-op, and after the first
	// successful run the file is renamed to *.migrated.{unix}.
	migratedRooms := 0
	if rt, ok := app.currentRegistry().Default(); ok {
		n, err := migrateKnownRoomsFile(ctx, db, rt.AccountID(), cfg.DataDir)
		if err != nil {
			logger.Warn(ctx, "迁移 known_rooms.txt 失败", "error", err.Error())
		}
		migratedRooms = n
	}

	logger.Boundary(ctx, "账号注册表加载完成",
		"accounts", app.currentRegistry().Count(),
		"seeded", seeded,
		"migratedRooms", migratedRooms,
	)

	go app.preloadKnownRooms(ctx)
	for _, rt := range app.currentRegistry().All() {
		go app.loadSelfUserID(ctx, rt)
	}

	syncer := newProfileSyncer(app)
	go syncer.runOnce(ctx)
	syncCtx, syncCancel := context.WithCancel(ctx)
	defer syncCancel()
	go syncer.loop(syncCtx, cfg.ProfileSyncInterval)
	app.contactSync.Start(syncCtx)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      app.routes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Boundary(ctx, "服务启动",
			"port", cfg.Port,
			"logLevel", cfg.LogLevel,
			"agentEnabled", cfg.AgentEnabled,
			"agentServer", cfg.AgentServer,
			"adminEnabled", cfg.AdminToken != "",
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(ctx, "服务启动失败", "error", err.Error())
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	logger.Boundary(ctx, "收到终止信号，开始关闭")
	syncCancel()
	shutdownCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	logger.Flush()
}
