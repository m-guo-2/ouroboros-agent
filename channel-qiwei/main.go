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
		fmt.Println("Missing required QiWei config: QIWEI_API_BASE_URL, QIWEI_TOKEN, QIWEI_GUID")
		os.Exit(1)
	}

	app := newApp(cfg)

	ctx := context.Background()

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
	shutdownCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	logger.Flush()
}
