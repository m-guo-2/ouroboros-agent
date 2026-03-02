package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level represents the three-tier log level matching the Server's design.
type Level string

const (
	LBoundary Level = "boundary" // Service lifecycle, HTTP in/out
	LBusiness Level = "business" // Trace events, LLM calls, tool calls, decisions
	LDetail   Level = "detail"   // Full LLM I/O, tool raw data, error stacks
)

type ctxKey struct{}

type traceCtx struct {
	traceID   string
	sessionID string
}

// WithTrace injects traceId / sessionId into context for automatic log enrichment.
func WithTrace(ctx context.Context, traceID, sessionID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, traceCtx{traceID: traceID, sessionID: sessionID})
}

// GetTrace extracts traceId/sessionId from context (returns empty strings if not set).
func GetTrace(ctx context.Context) (traceID, sessionID string) {
	if tc, ok := ctx.Value(ctxKey{}).(traceCtx); ok {
		return tc.traceID, tc.sessionID
	}
	return "", ""
}

var (
	logDir string
	mu     sync.Mutex
	files  = make(map[string]*os.File) // key: "{level}/{date}"

	retentionDays = map[Level]int{
		LBoundary: 30,
		LBusiness: 14,
		LDetail:   7,
	}
)

// Init configures three-level logging. logDir is the root directory for log files.
// Structure: {logDir}/boundary/{date}.jsonl, {logDir}/business/{date}.jsonl, etc.
func Init(dir string) {
	logDir = dir
	for _, level := range []Level{LBoundary, LBusiness, LDetail} {
		_ = os.MkdirAll(filepath.Join(dir, string(level)), 0755)
	}
	_ = os.MkdirAll(filepath.Join(dir, "detail", "llm-io"), 0755)

	go cleanupLoop()
}

// Boundary logs at L1 boundary level (service lifecycle, HTTP requests).
func Boundary(ctx context.Context, msg string, args ...any) {
	writeLog(ctx, LBoundary, msg, args...)
}

// Business logs at L2 business level (trace events, decisions).
func Business(ctx context.Context, msg string, args ...any) {
	writeLog(ctx, LBusiness, msg, args...)
}

// Detail logs at L3 detail level (full I/O payloads, raw data).
func Detail(ctx context.Context, msg string, args ...any) {
	writeLog(ctx, LDetail, msg, args...)
}

// Error logs at business level with "ERROR" severity for important failures.
func Error(ctx context.Context, msg string, args ...any) {
	args = append(args, "severity", "ERROR")
	writeLog(ctx, LBusiness, msg, args...)
}

// Warn logs at business level with "WARN" severity.
func Warn(ctx context.Context, msg string, args ...any) {
	args = append(args, "severity", "WARN")
	writeLog(ctx, LBusiness, msg, args...)
}

func writeLog(ctx context.Context, level Level, msg string, args ...any) {
	now := time.Now()
	entry := map[string]any{
		"time":  now.Format(time.RFC3339Nano),
		"level": string(level),
		"msg":   msg,
	}

	if tc, ok := ctx.Value(ctxKey{}).(traceCtx); ok {
		if tc.traceID != "" {
			entry["traceId"] = tc.traceID
		}
		if tc.sessionID != "" {
			entry["sessionId"] = tc.sessionID
		}
	}

	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		entry[key] = args[i+1]
	}

	writeToFile(level, now, entry)
	writeToConsole(level, now, msg, entry)
}

func writeToFile(level Level, t time.Time, entry map[string]any) {
	if logDir == "" {
		return
	}

	date := t.Format("2006-01-02")
	key := string(level) + "/" + date
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	f, ok := files[key]
	if !ok {
		path := filepath.Join(logDir, key+".jsonl")
		f, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		files[key] = f
	}

	_, _ = f.Write(data)
	_, _ = f.Write([]byte("\n"))
}

// ANSI colors
const (
	colorReset  = "\033[0m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

var levelStyle = map[Level]struct{ color, label string }{
	LBoundary: {colorCyan, "BND"},
	LBusiness: {colorYellow, "BIZ"},
	LDetail:   {colorGray, "DTL"},
}

func writeToConsole(level Level, t time.Time, msg string, entry map[string]any) {
	style := levelStyle[level]
	timeStr := t.Format("15:04:05.000")

	severity, _ := entry["severity"].(string)
	traceID, _ := entry["traceId"].(string)

	var severityTag string
	if severity == "ERROR" {
		severityTag = " " + colorRed + "ERROR" + colorReset
	} else if severity == "WARN" {
		severityTag = " " + colorYellow + "WARN" + colorReset
	}

	var traceTag string
	if traceID != "" {
		short := traceID
		if len(short) > 12 {
			short = short[len(short)-8:]
		}
		traceTag = colorDim + "[" + short + "]" + colorReset + " "
	}

	// Compact extra fields
	extra := compactExtra(entry)

	line := fmt.Sprintf("%s%s%s %s%s%s %s%s%s%s",
		colorDim, timeStr, colorReset,
		style.color, style.label, colorReset,
		traceTag,
		msg,
		severityTag,
		extra,
	)

	fmt.Fprintln(os.Stdout, line)
}

func compactExtra(entry map[string]any) string {
	skip := map[string]bool{
		"time": true, "level": true, "msg": true,
		"traceId": true, "sessionId": true, "severity": true,
	}

	var parts []string
	for k, v := range entry {
		if skip[k] {
			continue
		}
		switch val := v.(type) {
		case string:
			if len(val) > 80 {
				parts = append(parts, fmt.Sprintf("%s=[%d chars]", k, len(val)))
			} else if val != "" {
				parts = append(parts, fmt.Sprintf("%s=%s", k, val))
			}
		case int, int64, float64, bool:
			parts = append(parts, fmt.Sprintf("%s=%v", k, val))
		default:
			s, _ := json.Marshal(v)
			if len(s) > 60 {
				parts = append(parts, fmt.Sprintf("%s={...}", k))
			} else {
				parts = append(parts, fmt.Sprintf("%s=%s", k, s))
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}

	result := " " + colorDim
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	result += colorReset
	return result
}

// WriteLLMIO writes full LLM request/response to a dedicated file.
// Returns the reference path (relative to logDir) for linking from business-level logs.
func WriteLLMIO(ctx context.Context, iteration int, request, response any) string {
	if logDir == "" {
		return ""
	}

	traceID, _ := GetTrace(ctx)
	if traceID == "" {
		traceID = "unknown"
	}

	ref := fmt.Sprintf("%s_iter%d", traceID, iteration)
	dir := filepath.Join(logDir, "detail", "llm-io")
	path := filepath.Join(dir, ref+".json")

	payload := map[string]any{
		"traceId":   traceID,
		"iteration": iteration,
		"time":      time.Now().Format(time.RFC3339Nano),
		"request":   request,
		"response":  response,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return ""
	}

	return ref
}

// ReadLLMIO reads a previously written LLM I/O file by reference.
func ReadLLMIO(ref string) ([]byte, error) {
	if logDir == "" {
		return nil, fmt.Errorf("log dir not initialized")
	}
	path := filepath.Join(logDir, "detail", "llm-io", ref+".json")
	return os.ReadFile(path)
}

// Flush closes all open file handles (call on shutdown).
func Flush() {
	mu.Lock()
	defer mu.Unlock()
	for k, f := range files {
		_ = f.Close()
		delete(files, k)
	}
}

// cleanupLoop periodically removes old log files based on retention policy.
func cleanupLoop() {
	for {
		time.Sleep(1 * time.Hour)
		cleanupOldFiles()
	}
}

func cleanupOldFiles() {
	if logDir == "" {
		return
	}

	now := time.Now()
	for level, days := range retentionDays {
		dir := filepath.Join(logDir, string(level))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		cutoff := now.AddDate(0, 0, -days).Format("2006-01-02")
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if len(name) < 10 {
				continue
			}
			fileDate := name[:10] // "2026-02-27"
			if fileDate < cutoff {
				_ = os.Remove(filepath.Join(dir, name))
			}
		}
	}

	// LLM I/O files: clean up based on modification time (7 days)
	llmDir := filepath.Join(logDir, "detail", "llm-io")
	entries, err := os.ReadDir(llmDir)
	if err != nil {
		return
	}
	cutoff := now.AddDate(0, 0, -7)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(llmDir, e.Name()))
		}
	}
}
