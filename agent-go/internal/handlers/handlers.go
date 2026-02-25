package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"agent-go/internal/runner"
)

var (
	isDraining    atomic.Bool
	inflightCount atomic.Int32
	appVersion    = os.Getenv("AGENT_APP_VERSION")
	startTime     = time.Now()
)

type ProcessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

func ProcessHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if isDraining.Load() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Agent is shutting down",
		})
		return
	}

	var req runner.ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Invalid JSON payload",
		})
		return
	}

	if req.UserID == "" || req.AgentID == "" || req.Content == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Missing required fields: userId, agentId, content",
		})
		return
	}

	if req.Channel == "" || req.ChannelUserID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Missing required fields: channel, channelUserId",
		})
		return
	}

	// Immediate 202 Accepted
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(ProcessResponse{
		Success: true,
		Message: "Processing",
	})

	inflightCount.Add(1)

	// Background processing
	go func() {
		defer func() {
			if count := inflightCount.Add(-1); count <= 0 && isDraining.Load() {
				// Wait a bit, then exit
				go func() {
					time.Sleep(1 * time.Second)
					os.Exit(0)
				}()
			}
		}()

		// runner.EnqueueProcessRequest handles the queue mapping and async processing internally
		err := runner.EnqueueProcessRequest(context.Background(), req)
		if err != nil {
			// Log error (in a real system, use a structured logger)
			// fmt.Printf("[process] Unhandled error: %v\n", err)
		}
	}()
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := "ready"
	if isDraining.Load() {
		status = "draining"
	}

	if appVersion == "" {
		appVersion = "unknown"
	}

	resp := map[string]interface{}{
		"status":   status,
		"inflight": inflightCount.Load(),
		"version":  appVersion,
		"uptime":   time.Since(startTime).Seconds(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func DrainHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	isDraining.Store(true)

	// Instruct runner to graceful shutdown
	runner.GracefulShutdown()

	count := inflightCount.Load()
	resp := map[string]interface{}{
		"success":  true,
		"inflight": count,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

	if count <= 0 {
		go func() {
			time.Sleep(1 * time.Second)
			os.Exit(0)
		}()
	}
}
