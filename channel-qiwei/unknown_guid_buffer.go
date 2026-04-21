package main

import (
	"sync"
)

// unknownGuidEvent is a single webhook payload that arrived for a guid that
// the running process doesn't know about (not registered, or disabled).
// Operators see these via the admin API to diagnose "forgot to register"
// situations without trawling through the log stream.
type unknownGuidEvent struct {
	GUID     string `json:"guid"`
	Cmd      int    `json:"cmd"`
	MsgType  int    `json:"msgType"`
	MsgSvrID string `json:"msgSvrId,omitempty"`
	At       int64  `json:"at"`
}

// unknownGuidBuffer is a fixed-size ring buffer of recent unknown-guid
// webhook hits. It is intentionally small and bounded; we don't want a
// malicious or misconfigured caller to spike memory.
type unknownGuidBuffer struct {
	mu    sync.Mutex
	cap   int
	head  int
	count int
	data  []unknownGuidEvent
}

func newUnknownGuidBuffer(cap int) *unknownGuidBuffer {
	if cap <= 0 {
		cap = 128
	}
	return &unknownGuidBuffer{
		cap:  cap,
		data: make([]unknownGuidEvent, cap),
	}
}

// Record adds an event, overwriting the oldest entry when full.
func (b *unknownGuidBuffer) Record(ev unknownGuidEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[b.head] = ev
	b.head = (b.head + 1) % b.cap
	if b.count < b.cap {
		b.count++
	}
}

// Snapshot returns the buffered events in newest-to-oldest order.
func (b *unknownGuidBuffer) Snapshot() []unknownGuidEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]unknownGuidEvent, b.count)
	// head points to the next slot; the most recent is head-1.
	for i := 0; i < b.count; i++ {
		idx := (b.head - 1 - i + b.cap) % b.cap
		out[i] = b.data[idx]
	}
	return out
}
