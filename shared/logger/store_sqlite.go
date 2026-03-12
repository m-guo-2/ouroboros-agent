package logger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const createEventsSQL = `
CREATE TABLE IF NOT EXISTS events (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	trace_id   TEXT NOT NULL DEFAULT '',
	session_id TEXT NOT NULL DEFAULT '',
	agent_id   TEXT NOT NULL DEFAULT '',
	user_id    TEXT NOT NULL DEFAULT '',
	channel    TEXT NOT NULL DEFAULT '',
	level      TEXT NOT NULL DEFAULT '',
	event      TEXT NOT NULL DEFAULT '',
	severity   TEXT NOT NULL DEFAULT '',
	iteration  INTEGER NOT NULL DEFAULT 0,
	model      TEXT NOT NULL DEFAULT '',
	tool       TEXT NOT NULL DEFAULT '',
	msg        TEXT NOT NULL DEFAULT '',
	timestamp  TEXT NOT NULL DEFAULT '',
	data       TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_events_trace ON events(trace_id);
CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id);
CREATE INDEX IF NOT EXISTS idx_events_level_event ON events(level, event);
CREATE INDEX IF NOT EXISTS idx_events_user ON events(user_id) WHERE user_id != '';
CREATE INDEX IF NOT EXISTS idx_events_channel ON events(channel) WHERE channel != '';
CREATE INDEX IF NOT EXISTS idx_events_severity ON events(severity) WHERE severity != '';
`

const createLLMIOSQL = `
CREATE TABLE IF NOT EXISTS llm_io (
	ref       TEXT PRIMARY KEY,
	trace_id  TEXT NOT NULL DEFAULT '',
	iteration INTEGER NOT NULL DEFAULT 0,
	data      BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_llmio_trace ON llm_io(trace_id);
`

// SQLiteStore stores logs in per-day SQLite databases.
// It implements both LogWriter and LogReader.
type SQLiteStore struct {
	dir string

	mu         sync.Mutex
	writerDB   *sql.DB
	writerDate string
	readers    map[string]*readerEntry
}

type readerEntry struct {
	db       *sql.DB
	lastUsed time.Time
}

const maxReaders = 7

func NewSQLiteStore(dir string) (*SQLiteStore, error) {
	sqlDir := filepath.Join(dir, "sqlite")
	if err := os.MkdirAll(sqlDir, 0755); err != nil {
		return nil, fmt.Errorf("create sqlite dir: %w", err)
	}
	s := &SQLiteStore{
		dir:     sqlDir,
		readers: make(map[string]*readerEntry),
	}
	return s, nil
}

func (s *SQLiteStore) ensureDB(date string) (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writerDate == date && s.writerDB != nil {
		return s.writerDB, nil
	}

	// Day rollover: demote old writer to reader
	if s.writerDB != nil && s.writerDate != "" {
		s.readers[s.writerDate] = &readerEntry{db: s.writerDB, lastUsed: time.Now()}
		s.writerDB = nil
		s.writerDate = ""
		s.evictReadersLocked()
	}

	path := filepath.Join(s.dir, date+".db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", date, err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL %s: %w", date, err)
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set synchronous %s: %w", date, err)
	}
	if _, err := db.Exec(createEventsSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("create events table %s: %w", date, err)
	}
	if _, err := db.Exec(createLLMIOSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("create llm_io table %s: %w", date, err)
	}

	s.writerDB = db
	s.writerDate = date
	return db, nil
}

func (s *SQLiteStore) readerFor(date string) (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writerDate == date && s.writerDB != nil {
		return s.writerDB, nil
	}
	if r, ok := s.readers[date]; ok {
		r.lastUsed = time.Now()
		return r.db, nil
	}

	path := filepath.Join(s.dir, date+".db")
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}
	db, err := sql.Open("sqlite", path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s.readers[date] = &readerEntry{db: db, lastUsed: time.Now()}
	s.evictReadersLocked()
	return db, nil
}

func (s *SQLiteStore) evictReadersLocked() {
	for len(s.readers) > maxReaders {
		var oldest string
		var oldestTime time.Time
		for date, r := range s.readers {
			if oldest == "" || r.lastUsed.Before(oldestTime) {
				oldest = date
				oldestTime = r.lastUsed
			}
		}
		if oldest != "" {
			s.readers[oldest].db.Close()
			delete(s.readers, oldest)
		}
	}
}

func (s *SQLiteStore) availableDates() []string {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var dates []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".db") && !strings.HasSuffix(name, "-wal") && !strings.HasSuffix(name, "-shm") {
			dates = append(dates, strings.TrimSuffix(name, ".db"))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	return dates
}

// --- LogWriter ---

func strFromEntry(entry map[string]any, key string) string {
	v, _ := entry[key].(string)
	return v
}

func intFromEntry(entry map[string]any, key string) int {
	switch v := entry[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

func (s *SQLiteStore) Append(level Level, entry map[string]any) error {
	ts := strFromEntry(entry, "time")
	date := time.Now().Format("2006-01-02")
	if len(ts) >= 10 {
		date = ts[:10]
	}

	db, err := s.ensureDB(date)
	if err != nil {
		return err
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	_, err = db.Exec(
		`INSERT INTO events (trace_id, session_id, agent_id, user_id, channel, level, event, severity, iteration, model, tool, msg, timestamp, data)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strFromEntry(entry, "traceId"),
		strFromEntry(entry, "sessionId"),
		strFromEntry(entry, "agentId"),
		strFromEntry(entry, "userId"),
		strFromEntry(entry, "channel"),
		string(level),
		strFromEntry(entry, "traceEvent"),
		strFromEntry(entry, "severity"),
		intFromEntry(entry, "iteration"),
		strFromEntry(entry, "model"),
		strFromEntry(entry, "tool"),
		strFromEntry(entry, "msg"),
		ts,
		string(data),
	)
	return err
}

func (s *SQLiteStore) WriteLLMIO(ref string, traceID string, iteration int, data []byte) error {
	date := time.Now().Format("2006-01-02")
	db, err := s.ensureDB(date)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT OR REPLACE INTO llm_io (ref, trace_id, iteration, data) VALUES (?, ?, ?, ?)`,
		ref, traceID, iteration, data,
	)
	return err
}

// --- LogReader ---

func (s *SQLiteStore) ListTraces(filter TraceFilter) ([]TraceSummary, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	days := filter.Days
	if days <= 0 {
		days = 14
	}

	dates := s.availableDates()
	if len(dates) == 0 {
		today := time.Now().Format("2006-01-02")
		dates = []string{today}
	}

	seen := make(map[string]TraceSummary)
	for _, date := range dates {
		if len(seen) >= limit {
			break
		}
		if days <= 0 {
			break
		}
		days--

		db, err := s.readerFor(date)
		if err != nil || db == nil {
			continue
		}

		query := `SELECT trace_id, session_id, MIN(timestamp) as started_at
			FROM events WHERE event = 'start' AND trace_id != ''`
		args := []any{}

		if filter.SessionID != "" {
			query += ` AND session_id = ?`
			args = append(args, filter.SessionID)
		}
		query += ` GROUP BY trace_id ORDER BY started_at DESC`

		rows, err := db.Query(query, args...)
		if err != nil {
			continue
		}

		for rows.Next() {
			var ts TraceSummary
			if err := rows.Scan(&ts.ID, &ts.SessionID, &ts.StartedAt); err != nil {
				continue
			}
			if _, exists := seen[ts.ID]; !exists {
				seen[ts.ID] = ts
			}
		}
		rows.Close()
	}

	result := make([]TraceSummary, 0, len(seen))
	for _, ts := range seen {
		result = append(result, ts)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].StartedAt > result[j].StartedAt
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (s *SQLiteStore) ReadTraceEvents(traceID string) ([]TraceEvent, error) {
	dates := s.availableDates()
	var events []TraceEvent

	for _, date := range dates {
		db, err := s.readerFor(date)
		if err != nil || db == nil {
			continue
		}

		rows, err := db.Query(
			`SELECT data FROM events WHERE trace_id = ? ORDER BY timestamp, id`,
			traceID,
		)
		if err != nil {
			continue
		}

		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				continue
			}
			events = append(events, TraceEvent{Raw: m})
		}
		rows.Close()
	}

	return events, nil
}

func (s *SQLiteStore) ReadLLMIO(ref string) ([]byte, error) {
	dates := s.availableDates()

	for _, date := range dates {
		db, err := s.readerFor(date)
		if err != nil || db == nil {
			continue
		}

		var data []byte
		err = db.QueryRow(`SELECT data FROM llm_io WHERE ref = ?`, ref).Scan(&data)
		if err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("llm-io ref not found: %s", ref)
}

func (s *SQLiteStore) ListLLMIORefs(traceID string) ([]string, error) {
	dates := s.availableDates()
	var refs []string

	for _, date := range dates {
		db, err := s.readerFor(date)
		if err != nil || db == nil {
			continue
		}

		rows, err := db.Query(
			`SELECT ref FROM llm_io WHERE trace_id = ? ORDER BY ref`,
			traceID,
		)
		if err != nil {
			continue
		}

		for rows.Next() {
			var ref string
			if err := rows.Scan(&ref); err != nil {
				continue
			}
			refs = append(refs, ref)
		}
		rows.Close()
	}

	sort.Strings(refs)
	return refs, nil
}

// --- Lifecycle ---

func (s *SQLiteStore) Cleanup(retentionDays int) error {
	if retentionDays <= 0 {
		retentionDays = 14
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Format("2006-01-02")

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".db") {
			continue
		}
		date := strings.TrimSuffix(name, ".db")
		if date >= cutoff {
			continue
		}

		if r, ok := s.readers[date]; ok {
			r.db.Close()
			delete(s.readers, date)
		}
		if s.writerDate == date && s.writerDB != nil {
			s.writerDB.Close()
			s.writerDB = nil
			s.writerDate = ""
		}

		_ = os.Remove(filepath.Join(s.dir, name))
		_ = os.Remove(filepath.Join(s.dir, name+"-wal"))
		_ = os.Remove(filepath.Join(s.dir, name+"-shm"))
	}
	return nil
}

func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writerDB != nil {
		s.writerDB.Close()
		s.writerDB = nil
		s.writerDate = ""
	}
	for date, r := range s.readers {
		r.db.Close()
		delete(s.readers, date)
	}
	return nil
}
