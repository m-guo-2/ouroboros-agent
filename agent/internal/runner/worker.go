package runner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"agent/internal/logger"
	"agent/internal/storage"
)

type ProcessRequest struct {
	UserID                string                   `json:"userId"`
	AgentID               string                   `json:"agentId"`
	Content               string                   `json:"content"`
	Channel               string                   `json:"channel"`
	ChannelUserID         string                   `json:"channelUserId"`
	ChannelConversationID string                   `json:"channelConversationId,omitempty"`
	ChannelMessageID      string                   `json:"channelMessageId,omitempty"`
	SenderName            string                   `json:"senderName,omitempty"`
	MessageType           string                   `json:"messageType,omitempty"`
	Attachments           []storage.AttachmentData `json:"attachments,omitempty"`
	ChannelMeta           map[string]any           `json:"channelMeta,omitempty"`
	MessageID             string                   `json:"messageId"`
	SessionID             string                   `json:"sessionId,omitempty"`
	TraceID               string                   `json:"traceId,omitempty"`
}

type QueuedRequest struct {
	ProcessRequest
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
	SessionIdleTimeoutMs = 10 * 60 * 1000
	MaxAbsorbRounds      = 5
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

func popAllPending(worker *SessionWorker) []QueuedRequest {
	workerMutex.Lock()
	defer workerMutex.Unlock()
	if len(worker.Queue) == 0 {
		return nil
	}
	pending := make([]QueuedRequest, len(worker.Queue))
	copy(pending, worker.Queue)
	worker.Queue = worker.Queue[:0]
	return pending
}

func drainWorker(worker *SessionWorker) {
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

		baseCtx, cancel := context.WithCancel(context.Background())
		traceID, sessionID := req.TraceID, worker.SessionID
		ctx := logger.WithTrace(baseCtx, traceID, sessionID)

		logger.Business(ctx, "出队开始处理", "queueRemaining", len(worker.Queue))

		_ = storage.UpdateSession(worker.SessionID, map[string]interface{}{
			"executionStatus": "processing",
		})

		workerMutex.Lock()
		worker.CancelFunc = cancel
		workerMutex.Unlock()

		err := processOneEvent(ctx, worker, req)
		if err != nil {
			logger.Error(ctx, "处理请求失败", "error", err.Error())
			_ = storage.UpdateSession(worker.SessionID, map[string]interface{}{
				"executionStatus": "interrupted",
			})
		}

		workerMutex.Lock()
		worker.CancelFunc = nil
		workerMutex.Unlock()

		_ = storage.UpdateSession(worker.SessionID, map[string]interface{}{
			"executionStatus": "completed",
		})
		logger.Business(ctx, "请求处理完成")
	}
}

func EnqueueProcessRequest(ctx context.Context, req ProcessRequest) error {
	workerMutex.Lock()
	if shuttingDown {
		workerMutex.Unlock()
		return fmt.Errorf("agent is shutting down")
	}
	workerMutex.Unlock()

	sessionKey := resolveSessionKey(req.Channel, req.ChannelUserID, req.ChannelConversationID)

	var sessionID string
	var workDir string

	if req.SessionID != "" {
		sd, _ := storage.GetSession(req.SessionID)
		if sd != nil {
			sessionID = sd.ID
			workDir = sd.WorkDir
		}
	}

	if sessionID == "" {
		sd, _ := storage.FindSessionByKey(req.AgentID, sessionKey)
		if sd != nil {
			sessionID = sd.ID
			workDir = sd.WorkDir
		}
	}

	if sessionID == "" {
		sessionID = req.SessionID
		if sessionID == "" {
			sessionID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
		}
		workDir = "/tmp/agent-sessions/" + sessionID
		title := req.Content
		if len(title) > 30 {
			title = title[:30] + "..."
		}

		_, _ = storage.CreateSession(map[string]interface{}{
			"id":                    sessionID,
			"agentId":               req.AgentID,
			"userId":                req.UserID,
			"channel":               req.Channel,
			"sessionKey":            sessionKey,
			"channelConversationId": req.ChannelConversationID,
			"workDir":               workDir,
			"title":                 title,
		})
	}

	traceID := req.TraceID
	if traceID == "" {
		traceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}

	traceCtx := logger.WithTrace(ctx, traceID, sessionID)
	logger.Boundary(traceCtx, "请求入队", "agentId", req.AgentID, "channel", req.Channel)

	queuedReq := QueuedRequest{ProcessRequest: req}
	queuedReq.SessionID = sessionID
	queuedReq.TraceID = traceID

	logger.Business(traceCtx, "trace 开始",
		"traceEvent", "start", "agentId", req.AgentID, "userId", req.UserID, "channel", req.Channel)

	workerMutex.Lock()
	worker, ok := sessionWorkers[sessionID]
	if !ok {
		worker = &SessionWorker{
			SessionID:      sessionID,
			SessionKey:     sessionKey,
			WorkDir:        workDir,
			Queue:          []QueuedRequest{},
			Processing:     false,
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

	for sessionID, worker := range sessionWorkers {
		if worker.Processing {
			_ = storage.UpdateSession(sessionID, map[string]interface{}{
				"executionStatus": "interrupted",
			})
		}
	}

	workerMutex.Lock()
	sessionWorkers = make(map[string]*SessionWorker)
	workerMutex.Unlock()
}
