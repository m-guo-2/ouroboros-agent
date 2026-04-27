// Package timeutil centralises the "wall-clock now" helper used by all
// persistence code in channel-qiwei. Storing timestamps as UTC epoch
// milliseconds keeps schema, wire, and code uniform; time-zone concerns
// belong at the presentation boundary.
package timeutil

import "time"

// CST is kept for display formatting only; never used for persistence.
var CST = time.FixedZone("CST", 8*3600)

// NowMs returns the current wall clock as UTC epoch milliseconds. This
// is the canonical "now" for any *_at column stored as BIGINT.
func NowMs() int64 {
	return time.Now().UTC().UnixMilli()
}

// NowSec returns the current wall clock as UTC epoch seconds. Prefer
// NowMs for new code; this helper exists only for legacy call sites
// that still accept second-precision timestamps.
func NowSec() int64 {
	return time.Now().UTC().Unix()
}

// FormatCST formats a UTC epoch-ms into the Asia/Shanghai local
// representation. Empty / zero timestamps render as an empty string.
func FormatCST(ms int64) string {
	if ms == 0 {
		return ""
	}
	return time.UnixMilli(ms).In(CST).Format("2006-01-02 15:04:05")
}
