package logger

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Level string

const (
	LBoundary Level = "boundary"
	LBusiness Level = "business"
	LDetail   Level = "detail"
)

type ctxKey struct{}

type traceCtx struct {
	traceID   string
	sessionID string
	requestID string
}

var (
	logDir  string
	service string

	writer      *asyncWriter
	sqliteStore *SQLiteStore

	retentionDays = map[Level]int{
		LBoundary: 30,
		LBusiness: 14,
		LDetail:   7,
	}
)

func Init(dir, svc string) {
	logDir = dir
	service = svc
	if dir == "" {
		return
	}

	var backends []LogWriter

	fileStore, err := NewFileStore(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: failed to init file store: %v\n", err)
	} else {
		backends = append(backends, fileStore)
	}

	ss, err := NewSQLiteStore(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: failed to init sqlite store: %v\n", err)
	} else {
		sqliteStore = ss
		backends = append(backends, ss)
	}

	if len(backends) > 0 {
		mw := newMultiWriter(backends...)
		writer = newAsyncWriter(mw)
	}

	go cleanupLoop()
}

func GetReader() LogReader {
	return sqliteStore
}

func WithTrace(ctx context.Context, traceID, sessionID string) context.Context {
	tc := extractCtx(ctx)
	tc.traceID = traceID
	tc.sessionID = sessionID
	return context.WithValue(ctx, ctxKey{}, tc)
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	tc := extractCtx(ctx)
	tc.requestID = requestID
	return context.WithValue(ctx, ctxKey{}, tc)
}

func GetTrace(ctx context.Context) (traceID, sessionID string) {
	tc := extractCtx(ctx)
	return tc.traceID, tc.sessionID
}

func GetRequestID(ctx context.Context) string {
	return extractCtx(ctx).requestID
}

func extractCtx(ctx context.Context) traceCtx {
	if tc, ok := ctx.Value(ctxKey{}).(traceCtx); ok {
		return tc
	}
	return traceCtx{}
}

func GenerateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("req-%x", b)
}

func Boundary(ctx context.Context, msg string, args ...any) {
	writeLog(ctx, LBoundary, msg, args...)
}

func Business(ctx context.Context, msg string, args ...any) {
	writeLog(ctx, LBusiness, msg, args...)
}

func Detail(ctx context.Context, msg string, args ...any) {
	writeLog(ctx, LDetail, msg, args...)
}

func Error(ctx context.Context, msg string, args ...any) {
	args = append(args, "severity", "ERROR")
	writeLog(ctx, LBusiness, msg, args...)
}

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
	if service != "" {
		entry["service"] = service
	}

	tc := extractCtx(ctx)
	if tc.traceID != "" {
		entry["traceId"] = tc.traceID
	}
	if tc.sessionID != "" {
		entry["sessionId"] = tc.sessionID
	}
	if tc.requestID != "" {
		entry["requestId"] = tc.requestID
	}

	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		entry[key] = args[i+1]
	}

	if writer != nil {
		writer.Append(level, entry)
	}
	writeToConsole(level, now, msg, entry)
}

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
	requestID, _ := entry["requestId"].(string)

	var severityTag string
	if severity == "ERROR" {
		severityTag = " " + colorRed + "ERROR" + colorReset
	} else if severity == "WARN" {
		severityTag = " " + colorYellow + "WARN" + colorReset
	}

	var idTag string
	if traceID != "" {
		short := traceID
		if len(short) > 12 {
			short = short[len(short)-8:]
		}
		idTag = colorDim + "[" + short + "]" + colorReset + " "
	} else if requestID != "" {
		short := requestID
		if len(short) > 12 {
			short = short[len(short)-8:]
		}
		idTag = colorDim + "[" + short + "]" + colorReset + " "
	}

	var svcTag string
	if svc, _ := entry["service"].(string); svc != "" {
		svcTag = colorDim + "(" + svc + ")" + colorReset + " "
	}

	extra := compactExtra(entry)

	line := fmt.Sprintf("%s%s%s %s%s%s %s%s%s%s%s",
		colorDim, timeStr, colorReset,
		style.color, style.label, colorReset,
		svcTag,
		idTag,
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
		"requestId": true, "service": true,
	}

	var parts []string
	for k, v := range entry {
		if skip[k] {
			continue
		}
		switch val := v.(type) {
		case string:
			if len(val) > 200 {
				parts = append(parts, fmt.Sprintf("%s=[%d chars]", k, len(val)))
			} else if val != "" {
				parts = append(parts, fmt.Sprintf("%s=%s", k, val))
			}
		case int, int64, float64, bool:
			parts = append(parts, fmt.Sprintf("%s=%v", k, val))
		default:
			s, _ := json.Marshal(v)
			if len(s) > 200 {
				parts = append(parts, fmt.Sprintf("%s=[%d bytes]", k, len(s)))
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

func WriteLLMIO(ctx context.Context, iteration int, request, response any) string {
	if writer == nil {
		return ""
	}
	traceID, _ := GetTrace(ctx)
	if traceID == "" {
		traceID = "unknown"
	}
	ref := fmt.Sprintf("%s_iter%d", traceID, iteration)

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

	writer.WriteLLMIO(ref, traceID, iteration, data)
	return ref
}

func ReadLLMIO(ref string) ([]byte, error) {
	if sqliteStore != nil {
		return sqliteStore.ReadLLMIO(ref)
	}
	if logDir == "" {
		return nil, fmt.Errorf("log dir not initialized")
	}
	path := filepath.Join(logDir, "detail", "llm-io", ref+".json")
	return os.ReadFile(path)
}

func Flush() {
	if writer != nil {
		writer.Flush()
		writer = nil
	}
}

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
			fileDate := name[:10]
			if fileDate < cutoff {
				_ = os.Remove(filepath.Join(dir, name))
			}
		}
	}

	llmDir := filepath.Join(logDir, "detail", "llm-io")
	entries, err := os.ReadDir(llmDir)
	if err == nil {
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

	if sqliteStore != nil {
		_ = sqliteStore.Cleanup(retentionDays[LBusiness])
	}
}
