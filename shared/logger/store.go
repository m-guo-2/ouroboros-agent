package logger

// LogWriter is the write-side interface for log storage backends.
// FileStore and SQLiteStore both implement this.
type LogWriter interface {
	Append(level Level, entry map[string]any) error
	WriteLLMIO(ref string, traceID string, iteration int, data []byte) error
	Close() error
}

// LogReader is the read-side interface for log query backends.
// Only SQLiteStore implements this; file-based reading is removed.
type LogReader interface {
	ListTraces(filter TraceFilter) ([]TraceSummary, error)
	ReadTraceEvents(traceID string) ([]TraceEvent, error)
	ReadLLMIO(ref string) ([]byte, error)
	ListLLMIORefs(traceID string) ([]string, error)
}

type TraceFilter struct {
	SessionID string
	Limit     int
	Days      int
}

type TraceSummary struct {
	ID        string
	SessionID string
	StartedAt string
}

type TraceEvent struct {
	Raw map[string]any
}
