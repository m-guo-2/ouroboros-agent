package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agent-go/internal/handlers"
	"agent-go/internal/runner"
	"agent-go/internal/serverclient"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "1996"
	}

	appVersion := os.Getenv("AGENT_APP_VERSION")
	if appVersion == "" {
		appVersion = "1.0.0"
	}

	appID := os.Getenv("AGENT_ID")
	if appID == "" {
		appID = "agent-go-instance"
	}

	publicURL := os.Getenv("AGENT_PUBLIC_URL")
	if publicURL == "" {
		publicURL = fmt.Sprintf("http://localhost:%s", port)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/process", handlers.ProcessHandler)
	mux.HandleFunc("/process-event", handlers.ProcessHandler) // alias
	mux.HandleFunc("/health", handlers.HealthHandler)
	mux.HandleFunc("/drain", handlers.DrainHandler)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Register with control plane
	serverClient := serverclient.NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := serverClient.Register(ctx, appID, publicURL, appVersion)
	if err != nil {
		log.Printf("Failed to register with server: %v", err)
	} else {
		log.Printf("Registered successfully with server (ID: %s)", appID)
	}

	// Start heartbeat
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := serverClient.Heartbeat(ctx, appID)
			cancel()
			if err != nil {
				log.Printf("Heartbeat failed: %v", err)
			}
		}
	}()

	// Start server
	go func() {
		log.Printf("Agent-go starting on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Println("Received termination signal, starting graceful shutdown...")

	runner.GracefulShutdown()

	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := server.Shutdown(ctxShutdown); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Graceful shutdown complete. Exiting.")
}
