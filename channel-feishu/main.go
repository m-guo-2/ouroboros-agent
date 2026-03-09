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
		fmt.Println("❌ Missing required Feishu config: FEISHU_APP_ID, FEISHU_APP_SECRET")
		os.Exit(1)
	}

	app := newApp(cfg)
	app.initEventBridge()

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      app.routes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app.startWS(ctx)

	go func() {
		fmt.Printf("🤖 Feishu Bot service started on :%s\n", cfg.Port)
		fmt.Printf("   API:      http://localhost:%s/api/feishu\n", cfg.Port)
		fmt.Printf("   Webhook:  http://localhost:%s/webhook/event\n", cfg.Port)
		fmt.Printf("   Health:   http://localhost:%s/health\n", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("❌ server error: %v\n", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}
