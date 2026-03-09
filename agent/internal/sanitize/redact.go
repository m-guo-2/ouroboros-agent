package sanitize

import "regexp"

var (
	figmaPATPattern = regexp.MustCompile(`figd_[A-Za-z0-9_-]{10,}`)

	// Redact common CLI/env secret assignments, e.g.:
	// --figma-api-key=xxx, api_key=xxx, token xxx, access_token:xxx
	assignmentPattern = regexp.MustCompile(`(?i)\b(figma-api-key|api[-_]?key|access[-_]?token|token|secret)\b(\s*[:=]\s*|\s+)([^\s"'\},]+)`)
)

// RedactSecrets replaces known secret-like substrings with a fixed marker.
func RedactSecrets(input string) string {
	if input == "" {
		return input
	}
	out := figmaPATPattern.ReplaceAllString(input, "[REDACTED]")
	out = assignmentPattern.ReplaceAllString(out, `${1}${2}[REDACTED]`)
	return out
}
