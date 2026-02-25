package runner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"agent-go/internal/serverclient"
	"agent-go/internal/types"
)

type ProcessRequest struct {
	UserID                string `json:"userId"`
	AgentID               string `json:"agentId"`
	Content               string `json:"content"`
	Channel               string `json:"channel"`
	ChannelUserID         string `json:"channelUserId"`
	ChannelConversationID string `json:"channelConversationId,omitempty"`
	ChannelMessageID      string `json:"channelMessageId,omitempty"`
	SenderName            string `json:"senderName,omitempty"`
	MessageID             string `json:"messageId"`
	SessionID             string `json:"sessionId,omitempty"`
	TraceID               string `json:"traceId,omitempty"`
}

type QueuedRequest struct {
	ProcessRequest
	TraceStarted bool
}

type SessionWorker struct {
	SessionID      string
	SessionKey     string
	WorkDir        string
	Queue          []QueuedRequest
	Processing     bool
	CancelFunc     context.CancelFunc
	LastActivityAt int64
	IdleTimer      *time.Timer
}

var (
	SessionIdleTimeoutMs = 10 * 60 * 1000 // 10 minutes
	sessionWorkers       = make(map[string]*SessionWorker)
	workerMutex          sync.Mutex
	shuttingDown         = false
)

func resolveSessionKey(channel, channelUserId, channelConversationId string) string {
	if channelConversationId != "" {
		return fmt.Sprintf("%s:%s", channel, channelConversationId)
	}
	return fmt.Sprintf("%s:%s", channel, channelUserId)
}

func resetIdleTimer(worker *SessionWorker) {
	if worker.IdleTimer != nil {
		worker.IdleTimer.Stop()
	}
	worker.LastActivityAt = time.Now().UnixMilli()
	worker.IdleTimer = time.AfterFunc(time.Duration(SessionIdleTimeoutMs)*time.Millisecond, func() {
		evictSession(worker.SessionID)
	})
}

func evictSession(sessionID string) {
	workerMutex.Lock()
	defer workerMutex.Unlock()

	worker, ok := sessionWorkers[sessionID]
	if !ok {
		return
	}
	if worker.Processing {
		resetIdleTimer(worker)
		return
	}
	
	if worker.IdleTimer != nil {
		worker.IdleTimer.Stop()
	}
	delete(sessionWorkers, sessionID)
}

func drainWorker(worker *SessionWorker) {
	server := serverclient.NewClient()

	for {
		workerMutex.Lock()
		if len(worker.Queue) == 0 {
			worker.Processing = false
			resetIdleTimer(worker)
			workerMutex.Unlock()
			return
		}
		req := worker.Queue[0]
		worker.Queue = worker.Queue[1:]
		workerMutex.Unlock()

		_ = server.UpdateSession(context.Background(), worker.SessionID, map[string]interface{}{
			"executionStatus": "processing",
		})

		ctx, cancel := context.WithCancel(context.Background())
		workerMutex.Lock()
		worker.CancelFunc = cancel
		workerMutex.Unlock()

		err := processOneEvent(ctx, worker, req, server)
		if err != nil {
			_ = server.UpdateSession(context.Background(), worker.SessionID, map[string]interface{}{
				"executionStatus": "interrupted",
			})
		}

		workerMutex.Lock()
		worker.CancelFunc = nil
		workerMutex.Unlock()

		_ = server.UpdateSession(context.Background(), worker.SessionID, map[string]interface{}{
			"executionStatus": "completed",
		})
	}
}

func EnqueueProcessRequest(ctx context.Context, req ProcessRequest) error {
	workerMutex.Lock()
	if shuttingDown {
		workerMutex.Unlock()
		return fmt.Errorf("agent is shutting down")
	}
	workerMutex.Unlock()

	server := serverclient.NewClient()
	sessionKey := resolveSessionKey(req.Channel, req.ChannelUserID, req.ChannelConversationID)

	var sessionID string
	var workDir string

	if req.SessionID != "" {
		sd, _ := server.GetSession(ctx, req.SessionID)
		if sd != nil {
			sessionID = sd.ID
			workDir = sd.WorkDir
		}
	}
	
	if sessionID == "" {
		sd, _ := server.FindSessionByKey(ctx, req.AgentID, sessionKey)
		if sd != nil {
			sessionID = sd.ID
			workDir = sd.WorkDir
		}
	}

	// Wait, I need a UUID generator for missing session/trace IDs.
	// Assume missing trace ID is handled by the caller or we can generate a pseudo-random one if needed.
	// For now, let's just make sure they are strings.
	if sessionID == "" {
		sessionID = req.SessionID // should generate UUID in real implementation
		if sessionID == "" {
			sessionID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
		}
		workDir = "/tmp/agent-sessions/" + sessionID
		title := req.Content
		if len(title) > 30 {
			title = title[:30] + "..."
		}
		
		_, _ = server.CreateSession(ctx, map[string]interface{}{
			"id": sessionID,
			"agentId": req.AgentID,
			"userId": req.UserID,
			"channel": req.Channel,
			"sessionKey": sessionKey,
			"channelConversationId": req.ChannelConversationID,
			"workDir": workDir,
			"title": title,
		})
	}

	traceID := req.TraceID
	if traceID == "" {
		traceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}

	queuedReq := QueuedRequest{
		ProcessRequest: req,
		TraceStarted:   false,
	}
	queuedReq.SessionID = sessionID
	queuedReq.TraceID = traceID

	_ = server.ReportTraceEventSync(ctx, types.TraceEventPayload{
		TraceID: traceID,
		SessionID: sessionID,
		AgentID: req.AgentID,
		UserID: req.UserID,
		Channel: req.Channel,
		AgentEvent: types.AgentEvent{
			Type: "start",
			Timestamp: time.Now().UnixMilli(),
		},
	})
	queuedReq.TraceStarted = true

	workerMutex.Lock()
	worker, ok := sessionWorkers[sessionID]
	if !ok {
		worker = &SessionWorker{
			SessionID: sessionID,
			SessionKey: sessionKey,
			WorkDir: workDir,
			Queue: []QueuedRequest{},
			Processing: false,
			LastActivityAt: time.Now().UnixMilli(),
		}
		sessionWorkers[sessionID] = worker
	} else {
		worker.LastActivityAt = time.Now().UnixMilli()
		if worker.IdleTimer != nil {
			worker.IdleTimer.Stop()
			worker.IdleTimer = nil
		}
	}

	worker.Queue = append(worker.Queue, queuedReq)

	if !worker.Processing {
		worker.Processing = true
		go drainWorker(worker)
	}
	workerMutex.Unlock()

	return nil
}

func GracefulShutdown() {
	workerMutex.Lock()
	if shuttingDown {
		workerMutex.Unlock()
		return
	}
	shuttingDown = true
	
	for _, worker := range sessionWorkers {
		if worker.IdleTimer != nil {
			worker.IdleTimer.Stop()
		}
		if worker.CancelFunc != nil {
			worker.CancelFunc()
		}
		worker.Queue = nil
	}
	workerMutex.Unlock()

	server := serverclient.NewClient()
	for sessionID, worker := range sessionWorkers {
		if worker.Processing {
			_ = server.UpdateSession(context.Background(), sessionID, map[string]interface{}{
				"executionStatus": "interrupted",
			})
		}
	}
	
	workerMutex.Lock()
	sessionWorkers = make(map[string]*SessionWorker)
	workerMutex.Unlock()
}
