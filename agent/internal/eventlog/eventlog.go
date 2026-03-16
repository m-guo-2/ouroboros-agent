package eventlog

import (
	"sync"

	"agent/internal/storage"
)

type Event struct {
	Seq     int64
	Message *storage.MessageData
}

type EventLog struct {
	mu        sync.Mutex
	sessionID string
	cursor    int64
}

func New(sessionID string, initialCursor int64) *EventLog {
	return &EventLog{
		sessionID: sessionID,
		cursor:    initialCursor,
	}
}

// DrainNew returns all events with seq > cursor, then advances the in-memory
// cursor to the max seq returned. Returns nil if no new events.
// The cursor is only persisted to DB when the caller explicitly saves it
// alongside the session context.
func (l *EventLog) DrainNew() ([]Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	rows, err := storage.GetSessionEventsAfter(l.sessionID, l.cursor)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		msg, err := storage.GetMessageByID(row.MessageID)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			continue
		}
		events = append(events, Event{Seq: row.Seq, Message: msg})
	}

	if len(rows) > 0 {
		l.cursor = rows[len(rows)-1].Seq
	}

	return events, nil
}

// HasNew returns true if there are events with seq > cursor, without
// advancing the cursor.
func (l *EventLog) HasNew() (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return storage.HasSessionEventsAfter(l.sessionID, l.cursor)
}

// Cursor returns the current in-memory cursor value. The caller persists
// this to DB alongside the session context to ensure atomic recovery.
func (l *EventLog) Cursor() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cursor
}

// SessionID returns the session ID this EventLog is bound to.
func (l *EventLog) SessionID() string {
	return l.sessionID
}
