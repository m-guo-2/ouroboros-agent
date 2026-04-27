package migrator

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseTextTimestampMs parses any of the SQLite TEXT timestamp formats
// that historically appeared in the agent DB and returns UTC epoch ms.
//
// Empty / NULL inputs map to 0 (the schema default). Anything else that
// fails to parse returns an error so the caller can abort the migration:
// silently dropping a date is worse than stopping.
func parseTextTimestampMs(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	// Numeric strings are common in the legacy DB: settings.updated_at,
	// models.created_at, etc. were declared TEXT but populated with
	// `time.Now().UnixMilli()` stringified. Treat those as already-ms
	// (or already-sec, autodetected by magnitude).
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return numericToMs(n), nil
	}
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, time.UTC); err == nil {
			return t.UnixMilli(), nil
		}
	}
	return 0, fmt.Errorf("unrecognized timestamp %q", s)
}

// numericToMs heuristically converts a raw integer (seconds vs ms) into
// epoch-ms. Anything below 1e12 is assumed to be seconds (any value past
// 2001-09-09 in seconds is < 1e10, but values past 2286 in ms also clear
// 1e13, so 1e12 cleanly separates the two regimes).
func numericToMs(n int64) int64 {
	if n == 0 {
		return 0
	}
	if n < 1_000_000_000_000 {
		return n * 1000
	}
	return n
}

// nullableString unwraps a *string-style sql.NullString-like value held
// in an interface{}, returning ("", false) on NULL.
func nullableString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	}
	return ""
}

func nullableInt64(v interface{}) int64 {
	if v == nil {
		return 0
	}
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	}
	return 0
}

// jsonArrayCount reports how many elements a JSON array column holds, for
// child-row verification. Empty / NULL / non-array inputs return 0.
func jsonArrayCount(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" || s == "[]" {
		return 0
	}
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return 0
	}
	return int64(len(arr))
}

// jsonObjectCount reports the number of top-level keys in a JSON object,
// for child-row verification of `subagent_*` columns.
func jsonObjectCount(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" || s == "{}" {
		return 0
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return 0
	}
	return int64(len(m))
}

// jsonObjectFlatCount sums len(arr) for each value if the value is a
// JSON array; used by `subagent_skills` whose layout is
// {key: [skillID, ...]}.
func jsonObjectFlatCount(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" || s == "{}" {
		return 0
	}
	var m map[string][]json.RawMessage
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return 0
	}
	var n int64
	for _, v := range m {
		n += int64(len(v))
	}
	return n
}

// defaultJSONObject coerces empty / NULL / blank inputs into "{}" so the
// MySQL JSON column does not reject them. Anything that already looks
// like JSON is returned untouched.
func defaultJSONObject(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return "{}"
	}
	return s
}

// countToolUseBlocks counts how many "tool_use" blocks exist in a
// messages.tool_calls JSON array. Mirrors the filter used by
// insertMessageToolCalls: blocks without `type` are also counted as a
// tool_use (the legacy storage occasionally omitted the field).
func countToolUseBlocks(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return 0
	}
	var blocks []map[string]any
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		return 0
	}
	var n int64
	for _, b := range blocks {
		t, _ := b["type"].(string)
		if t == "" || t == "tool_use" {
			n++
		}
	}
	return n
}
