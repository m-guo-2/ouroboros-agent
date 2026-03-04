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
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      app.routes(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		fmt.Printf("QiWei Bot service started on :%s\n", cfg.Port)
		fmt.Printf("  Callback: http://localhost:%s/webhook/callback\n", cfg.Port)
		fmt.Printf("  Send API: http://localhost:%s/api/qiwei/send\n", cfg.Port)
		fmt.Printf("  Health:   http://localhost:%s/health\n", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("server error: %v\n", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
