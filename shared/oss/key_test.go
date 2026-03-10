package oss

import (
	"bytes"
	"testing"
	"time"
)

func TestGenerateObjectKey(t *testing.T) {
	key, err := generateObjectKey(
		"/agent/uploads/",
		"My File (Final).TXT",
		time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
		bytes.NewReader([]byte{1, 2, 3, 4, 5, 6}),
	)
	if err != nil {
		t.Fatalf("generateObjectKey() error = %v", err)
	}

	want := "agent/uploads/2026/03/10/my-file-final-010203040506.txt"
	if key != want {
		t.Fatalf("generateObjectKey() = %q, want %q", key, want)
	}
}

func TestNormalizeObjectKey(t *testing.T) {
	got := normalizeObjectKey("team/uploads", `\nested\..\avatar.png`)
	want := "team/uploads/avatar.png"
	if got != want {
		t.Fatalf("normalizeObjectKey() = %q, want %q", got, want)
	}
}
