package main

import (
	"context"
	"strings"
	"sync"
	"time"

	logger "github.com/m-guo-2/ouroboros-agent/shared/logger"
)

const tagContactSync = "contact-sync"

type contactSyncTask struct {
	kind   string
	userID string
	roomID string
	reason string
}

type contactSyncRun struct {
	At     int64  `json:"at"`
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

type contactSyncStatus struct {
	LastFullSyncAt int64            `json:"lastFullSyncAt"`
	LastError      string           `json:"lastError,omitempty"`
	QueueDepth     int              `json:"queueDepth"`
	RecentRuns     []contactSyncRun `json:"recentRuns"`
}

type contactSyncState struct {
	fails          int
	nextAt         time.Time
	lastFullSyncAt int64
	lastError      string
	recentRuns     []contactSyncRun
}

type contactSyncer struct {
	app *app

	mu      sync.Mutex
	ctx     context.Context
	workers map[string]chan contactSyncTask
	states  map[string]*contactSyncState
}

func newContactSyncer(a *app) *contactSyncer {
	return &contactSyncer{
		app:     a,
		workers: map[string]chan contactSyncTask{},
		states:  map[string]*contactSyncState{},
	}
}

func (s *contactSyncer) Start(ctx context.Context) {
	s.mu.Lock()
	if s.ctx == nil {
		s.ctx = ctx
	}
	s.mu.Unlock()
	if !s.app.cfg.ContactSyncEnabled {
		return
	}
	go s.loop(ctx, s.app.cfg.ContactSyncInterval)
}

func (s *contactSyncer) EnqueueContact(rt *accountRuntime, userID string, reason string) {
	userID = strings.TrimSpace(userID)
	if rt == nil || userID == "" {
		return
	}
	s.enqueue(rt.AccountID(), contactSyncTask{kind: "contact", userID: userID, reason: reason})
}

func (s *contactSyncer) EnqueueRoom(rt *accountRuntime, roomID string, reason string) {
	roomID = strings.TrimSpace(roomID)
	if rt == nil || roomID == "" {
		return
	}
	s.enqueue(rt.AccountID(), contactSyncTask{kind: "room", roomID: roomID, reason: reason})
}

func (s *contactSyncer) EnqueueFull(rt *accountRuntime, reason string) {
	if rt == nil {
		return
	}
	s.enqueue(rt.AccountID(), contactSyncTask{kind: "full", reason: reason})
}

func (s *contactSyncer) enqueue(accountID string, task contactSyncTask) {
	ch := s.ensureWorker(accountID)
	select {
	case ch <- task:
	default:
		select {
		case <-ch:
		default:
		}
		logger.Warn(context.Background(), "contact sync queue full, dropping oldest task",
			"tag", tagContactSync,
			"accountId", accountID,
			"kind", task.kind,
		)
		ch <- task
	}
}

func (s *contactSyncer) ensureWorker(accountID string) chan contactSyncTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.workers[accountID]; ok {
		return ch
	}
	ch := make(chan contactSyncTask, 64)
	s.workers[accountID] = ch
	if _, ok := s.states[accountID]; !ok {
		s.states[accountID] = &contactSyncState{}
	}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	go s.worker(ctx, accountID, ch)
	return ch
}

func (s *contactSyncer) worker(ctx context.Context, accountID string, ch <-chan contactSyncTask) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-ch:
			s.runTask(ctx, accountID, task)
		}
	}
}

func (s *contactSyncer) runTask(ctx context.Context, accountID string, task contactSyncTask) {
	rt, ok := s.app.currentRegistry().GetByID(accountID)
	if !ok || rt.gateway == nil {
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var err error
	switch task.kind {
	case "contact":
		err = s.syncContact(callCtx, rt, task.userID)
	case "room":
		err = s.syncRoom(callCtx, rt, task.roomID)
	case "full":
		err = s.runFull(callCtx, rt)
		s.recordFullResult(accountID, err)
	default:
		return
	}
	if err != nil {
		logger.Warn(callCtx, "contact sync task failed",
			"tag", tagContactSync,
			"accountId", accountID,
			"kind", task.kind,
			"reason", task.reason,
			"error", err.Error(),
		)
	}
}

func (s *contactSyncer) syncContact(ctx context.Context, rt *accountRuntime, userID string) error {
	if rt.gateway == nil || rt.gateway.src == nil {
		return nil
	}
	contacts, err := rt.gateway.src.BatchGetUserInfo(ctx, []string{userID})
	if err != nil {
		return err
	}
	for _, snapshot := range contacts {
		if strings.TrimSpace(snapshot.UserID) == userID {
			return rt.gateway.persistContact(ctx, snapshot)
		}
	}
	return nil
}

func (s *contactSyncer) syncRoom(ctx context.Context, rt *accountRuntime, roomID string) error {
	if rt.gateway == nil || rt.gateway.src == nil {
		return nil
	}
	rooms, err := rt.gateway.src.BatchGetRoomDetail(ctx, []string{roomID})
	if err != nil {
		return err
	}
	for _, snapshot := range rooms {
		if strings.TrimSpace(snapshot.RoomID) == roomID {
			return rt.gateway.persistRoom(ctx, snapshot)
		}
	}
	return nil
}

func (s *contactSyncer) runFull(ctx context.Context, rt *accountRuntime) error {
	if rt.gateway == nil || rt.gateway.src == nil {
		return nil
	}
	external, err := rt.gateway.src.ListExternalContacts(ctx)
	if err != nil {
		return err
	}
	internal, err := rt.gateway.src.ListInternalContacts(ctx)
	if err != nil {
		return err
	}
	for _, snapshot := range append(external, internal...) {
		if err := rt.gateway.persistContact(ctx, snapshot); err != nil {
			return err
		}
	}

	rooms, err := rt.gateway.src.ListRooms(ctx)
	if err != nil {
		return err
	}
	if len(rooms) == 0 {
		return nil
	}
	roomIDs := make([]string, 0, len(rooms))
	for _, room := range rooms {
		roomIDs = append(roomIDs, room.RoomID)
		if err := rt.gateway.persistRoom(ctx, room); err != nil {
			return err
		}
	}
	details, err := rt.gateway.src.BatchGetRoomDetail(ctx, roomIDs)
	if err != nil {
		return err
	}
	for _, room := range details {
		if err := rt.gateway.persistRoom(ctx, room); err != nil {
			return err
		}
	}
	return nil
}

func (s *contactSyncer) loop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			for _, rt := range s.app.currentRegistry().All() {
				if !s.due(rt.AccountID(), now) {
					continue
				}
				s.EnqueueFull(rt, "periodic")
			}
		}
	}
}

func (s *contactSyncer) due(accountID string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.stateLocked(accountID)
	return !now.Before(state.nextAt)
}

func (s *contactSyncer) recordFullResult(accountID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.stateLocked(accountID)
	run := contactSyncRun{At: time.Now().Unix()}
	if err != nil {
		state.fails++
		state.nextAt = time.Now().Add(backoff(state.fails))
		state.lastError = err.Error()
		run.Result = "failed"
		run.Error = err.Error()
	} else {
		state.fails = 0
		state.nextAt = time.Time{}
		state.lastError = ""
		state.lastFullSyncAt = time.Now().Unix()
		run.Result = "ok"
	}
	state.recentRuns = append([]contactSyncRun{run}, state.recentRuns...)
	if len(state.recentRuns) > 10 {
		state.recentRuns = state.recentRuns[:10]
	}
}

func (s *contactSyncer) Status(accountID string) contactSyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.stateLocked(accountID)
	queueDepth := 0
	if ch, ok := s.workers[accountID]; ok {
		queueDepth = len(ch)
	}
	recent := make([]contactSyncRun, len(state.recentRuns))
	copy(recent, state.recentRuns)
	return contactSyncStatus{
		LastFullSyncAt: state.lastFullSyncAt,
		LastError:      state.lastError,
		QueueDepth:     queueDepth,
		RecentRuns:     recent,
	}
}

func (s *contactSyncer) stateLocked(accountID string) *contactSyncState {
	state := s.states[accountID]
	if state == nil {
		state = &contactSyncState{}
		s.states[accountID] = state
	}
	return state
}
