package logger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileReader implements LogReader by scanning JSONL files.
// It serves as a fallback when SQLiteStore has no data (e.g. historical data
// that predates the SQLite migration).
type FileReader struct {
	dir string
}

func NewFileReader(dir string) *FileReader {
	return &FileReader{dir: dir}
}

func (r *FileReader) ListTraces(filter TraceFilter) ([]TraceSummary, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	days := filter.Days
	if days <= 0 {
		days = 14
	}

	dates := r.availableDates(days)
	seen := make(map[string]TraceSummary)

	for _, date := range dates {
		if len(seen) >= limit {
			break
		}
		r.scanFileForTraces(filepath.Join(r.dir, "business", date+".jsonl"), seen)
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

func (r *FileReader) scanFileForTraces(path string, seen map[string]TraceSummary) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 2*1024*1024)

	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		ev, _ := m["traceEvent"].(string)
		tid, _ := m["traceId"].(string)
		if ev != "start" || tid == "" {
			continue
		}
		if _, exists := seen[tid]; exists {
			continue
		}
		sid, _ := m["sessionId"].(string)
		ts, _ := m["time"].(string)
		seen[tid] = TraceSummary{ID: tid, SessionID: sid, StartedAt: ts}
	}
}

func (r *FileReader) ReadTraceEvents(traceID string) ([]TraceEvent, error) {
	dates := r.availableDates(14)
	var events []TraceEvent

	for _, date := range dates {
		path := filepath.Join(r.dir, "business", date+".jsonl")
		evts := r.scanFileForEvents(path, traceID)
		events = append(events, evts...)
	}
	return events, nil
}

func (r *FileReader) scanFileForEvents(path, traceID string) []TraceEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 2*1024*1024)

	var events []TraceEvent
	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		tid, _ := m["traceId"].(string)
		if tid != traceID {
			continue
		}
		events = append(events, TraceEvent{Raw: m})
	}
	return events
}

func (r *FileReader) ReadLLMIO(ref string) ([]byte, error) {
	path := filepath.Join(r.dir, "detail", "llm-io", ref+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("llm-io ref not found: %s", ref)
	}
	return data, nil
}

func (r *FileReader) ListLLMIORefs(traceID string) ([]string, error) {
	dir := filepath.Join(r.dir, "detail", "llm-io")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}

	prefix := traceID + "_"
	var refs []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".json") {
			refs = append(refs, strings.TrimSuffix(name, ".json"))
		}
	}
	sort.Strings(refs)
	return refs, nil
}

func (r *FileReader) availableDates(days int) []string {
	dir := filepath.Join(r.dir, "business")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	var dates []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		date := strings.TrimSuffix(name, ".jsonl")
		if date < cutoff {
			continue
		}
		dates = append(dates, date)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	return dates
}
