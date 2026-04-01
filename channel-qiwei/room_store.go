package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// roomStore is a thread-safe, file-backed set of room IDs.
// Used to detect whether the bot has seen a group before.
type roomStore struct {
	mu       sync.Mutex
	rooms    map[string]bool
	filePath string
}

func newRoomStore(filePath string) *roomStore {
	rs := &roomStore{
		rooms:    make(map[string]bool),
		filePath: filePath,
	}
	rs.loadFromFile()
	return rs
}

// Add records a room ID. Returns true if the room was new (not previously known).
func (rs *roomStore) Add(roomID string) bool {
	roomID = strings.TrimSpace(roomID)
	if roomID == "" {
		return false
	}

	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.rooms[roomID] {
		return false
	}
	rs.rooms[roomID] = true
	rs.appendToFile(roomID)
	return true
}

// Merge adds multiple room IDs in batch. New IDs are persisted to the file.
func (rs *roomStore) Merge(roomIDs []string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	var newIDs []string
	for _, id := range roomIDs {
		id = strings.TrimSpace(id)
		if id == "" || rs.rooms[id] {
			continue
		}
		rs.rooms[id] = true
		newIDs = append(newIDs, id)
	}
	if len(newIDs) > 0 {
		rs.appendLinesToFile(newIDs)
	}
}

func (rs *roomStore) loadFromFile() {
	f, err := os.Open(rs.filePath)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			rs.rooms[line] = true
		}
	}
}

func (rs *roomStore) appendToFile(roomID string) {
	rs.appendLinesToFile([]string{roomID})
}

func (rs *roomStore) appendLinesToFile(ids []string) {
	if err := os.MkdirAll(filepath.Dir(rs.filePath), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(rs.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	for _, id := range ids {
		_, _ = f.WriteString(id + "\n")
	}
}
