package main

import (
	"encoding/base64"
	"testing"
)

func TestDecodeMaybeBase64RepairsInvalidUTF8(t *testing.T) {
	raw := base64.StdEncoding.EncodeToString([]byte{0x89, 'b'})

	got := decodeMaybeBase64(raw)
	if got != "\uFFFDb" {
		t.Fatalf("expected invalid UTF-8 to be repaired, got %q", got)
	}
}
