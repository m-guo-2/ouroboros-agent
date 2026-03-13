package timeutil

import (
	"strconv"
	"time"
)

var CST = time.FixedZone("CST", 8*3600)

func NowMs() int64 {
	return time.Now().UnixMilli()
}

func FormatCST(ms int64) string {
	if ms == 0 {
		return ""
	}
	return time.UnixMilli(ms).In(CST).Format("2006-01-02 15:04:05")
}

func FormatCSTCompact(ms int64) string {
	if ms == 0 {
		return ""
	}
	return time.UnixMilli(ms).In(CST).Format("01-02 15:04")
}

// ParseToMs parses a time string to Unix milliseconds.
// Bare strings without timezone are treated as CST (UTC+8).
func ParseToMs(input string) (int64, bool) {
	if n, err := strconv.ParseInt(input, 10, 64); err == nil {
		if n > 1e15 {
			return n, true
		}
		if n > 1e12 {
			return n, true
		}
		if n > 1e9 {
			return n * 1000, true
		}
	}

	formats := []struct {
		layout string
		loc    *time.Location
	}{
		{time.RFC3339, nil},
		{time.RFC3339Nano, nil},
		{"2006-01-02T15:04:05", CST},
		{"2006-01-02 15:04:05", CST},
		{"2006-01-02T15:04", CST},
		{"2006-01-02 15:04", CST},
	}
	for _, f := range formats {
		var t time.Time
		var err error
		if f.loc != nil {
			t, err = time.ParseInLocation(f.layout, input, f.loc)
		} else {
			t, err = time.Parse(f.layout, input)
		}
		if err == nil {
			return t.UnixMilli(), true
		}
	}
	return 0, false
}
