package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg := LoadConfig()
	if err := cfg.Validate(); err != nil {
		fmt.Println("Missing required QiWei config: QIWEI_API_BASE_URL, QIWEI_TOKEN, QIWEI_GUID")
		os.Exit(1)
	}

	app := newApp(cfg)
	log := app.log

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      app.routes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("service started",
			"port", cfg.Port,
			"logLevel", cfg.LogLevel,
			"agentEnabled", cfg.AgentEnabled,
			"agentServer", cfg.AgentServer,
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
