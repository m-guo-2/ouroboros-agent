package runner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"agent/internal/eventlog"
	"agent/internal/logger"
	"agent/internal/sandbox"
	"agent/internal/storage"
	"agent/internal/types"
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
	MessageID             int64                    `json:"messageId"`
	SessionID             string                   `json:"sessionId,omitempty"`
	TraceID               string                   `json:"traceId,omitempty"`
}

type SessionWorker struct {
	SessionID      string
	SessionKey     string
	WorkDir        string
	Mode           types.SessionMode
	EventLog       *eventlog.EventLog
	Processing     bool
	CancelFunc     context.CancelFunc
	LastActivityAt int64
	IdleTimer      *time.Timer
}

var (
	SessionIdleTimeoutMs = 10 * 60 * 1000
	sessionWorkers       = make(map[string]*SessionWorker)
	workerMutex          sync.Mutex
	shuttingDown         = false
	sandboxMgr           = sandbox.NewManager("/tmp/agent-sandboxes")
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

	go func() { _ = sandboxMgr.Destroy(sessionID) }()
}

func drainWorker(worker *SessionWorker) {
	for {
		baseCtx, cancel := context.WithCancel(context.Background())
		ctx := logger.WithTrace(baseCtx, fmt.Sprintf("drain-%d", time.Now().UnixNano()), worker.SessionID)

		logger.Business(ctx, "开始处理会话事件")
		_ = storage.SaveLifecycleEvent(map[string]any{
			"sessionId": worker.SessionID,
			"stage":     "worker_started",
			"summary":   "会话 worker 开始处理事件",
		})

		_ = storage.UpdateSession(worker.SessionID, map[string]interface{}{
			"executionStatus": "processing",
		})

		workerMutex.Lock()
		worker.CancelFunc = cancel
		workerMutex.Unlock()

		err := processSession(ctx, worker)

		workerMutex.Lock()
		worker.CancelFunc = nil
		workerMutex.Unlock()

		if err != nil {
			logger.Error(ctx, "处理会话失败", "error", err.Error())
			_ = storage.UpdateSession(worker.SessionID, map[string]interface{}{
				"executionStatus": "interrupted",
			})
		} else {
			_ = storage.UpdateSession(worker.SessionID, map[string]interface{}{
				"executionStatus": "completed",
			})
		}
		logger.Business(ctx, "会话处理完成")

		// Double-check under mutex before exiting.
		workerMutex.Lock()
		has, _ := worker.EventLog.HasNew()
		if !has {
			worker.Processing = false
			resetIdleTimer(worker)
			workerMutex.Unlock()
			return
		}
		workerMutex.Unlock()
	}
}

// EnqueueProcessRequest signals the worker for a session that new events are
// available. The actual event data has already been persisted to
// session_events by the dispatcher. This function only manages worker
// lifecycle: it creates a SessionWorker (with an EventLog) if none exists
// and starts a drainWorker goroutine when needed.
func EnqueueProcessRequest(ctx context.Context, req ProcessRequest) error {
	sessionKey := resolveSessionKey(req.Channel, req.ChannelUserID, req.ChannelConversationID)

	var sessionID string
	var workDir string
	var eventCursor int64

	if req.SessionID != "" {
		sd, _ := storage.GetSession(req.SessionID)
		if sd != nil {
			sessionID = sd.ID
			workDir = sd.WorkDir
			eventCursor = sd.EventCursor
		}
	}

	if sessionID == "" {
		sd, _ := storage.FindSessionByKey(req.AgentID, sessionKey)
		if sd != nil {
			sessionID = sd.ID
			workDir = sd.WorkDir
			eventCursor = sd.EventCursor
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
		eventCursor = 0
	}

	traceID := req.TraceID
	if traceID == "" {
		traceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
	}

	traceCtx := logger.WithTrace(ctx, traceID, sessionID)
	logger.Boundary(traceCtx, "事件通知", "agentId", req.AgentID, "channel", req.Channel)
	if req.MessageID > 0 {
		_ = storage.SaveLifecycleEvent(map[string]any{
			"sessionId":        sessionID,
			"messageId":        req.MessageID,
			"traceId":          traceID,
			"channelMessageId": req.ChannelMessageID,
			"stage":            "worker_notified",
			"summary":          "runner 已接收处理通知",
		})
	}

	logger.Business(traceCtx, "trace 开始",
		"traceEvent", "start", "agentId", req.AgentID, "userId", req.UserID, "channel", req.Channel)

	workerMutex.Lock()
	if shuttingDown {
		workerMutex.Unlock()
		return fmt.Errorf("agent is shutting down")
	}
	worker, ok := sessionWorkers[sessionID]
	if !ok {
		worker = &SessionWorker{
			SessionID:      sessionID,
			SessionKey:     sessionKey,
			WorkDir:        workDir,
			EventLog:       eventlog.New(sessionID, eventCursor),
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

	if !worker.Processing {
		worker.Processing = true
		go drainWorker(worker)
	}
	workerMutex.Unlock()

	return nil
}

// RecoverSession creates a SessionWorker with a pre-initialized EventLog
// and starts a drainWorker for it. Used at startup to resume sessions that
// were interrupted by a previous crash.
func RecoverSession(ctx context.Context, sd storage.SessionData, el *eventlog.EventLog) error {
	sessionKey := ""
	if sd.SessionKey != "" {
		sessionKey = sd.SessionKey
	}

	workerMutex.Lock()
	defer workerMutex.Unlock()

	if shuttingDown {
		return fmt.Errorf("agent is shutting down")
	}
	if _, exists := sessionWorkers[sd.ID]; exists {
		return nil
	}

	worker := &SessionWorker{
		SessionID:      sd.ID,
		SessionKey:     sessionKey,
		WorkDir:        sd.WorkDir,
		EventLog:       el,
		Processing:     true,
		LastActivityAt: time.Now().UnixMilli(),
	}
	sessionWorkers[sd.ID] = worker
	go drainWorker(worker)
	return nil
}

func GracefulShutdown() {
	workerMutex.Lock()
	if shuttingDown {
		workerMutex.Unlock()
		return
	}
	shuttingDown = true

	var processingSessions []string
	for sessionID, worker := range sessionWorkers {
		if worker.IdleTimer != nil {
			worker.IdleTimer.Stop()
		}
		if worker.CancelFunc != nil {
			worker.CancelFunc()
		}
		if worker.Processing {
			processingSessions = append(processingSessions, sessionID)
		}
	}
	sessionWorkers = make(map[string]*SessionWorker)
	workerMutex.Unlock()

	for _, sessionID := range processingSessions {
		_ = storage.UpdateSession(sessionID, map[string]interface{}{
			"executionStatus": "interrupted",
		})
	}

	sandboxMgr.Shutdown()
}
