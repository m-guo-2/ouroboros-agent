package main

import (
	"sync"
	"time"
)

type ttlSet struct {
	mu    sync.Mutex
	items map[string]time.Time
	ttl   time.Duration
}

func newTTLSet(ttl time.Duration) *ttlSet {
	return &ttlSet{
		items: make(map[string]time.Time),
		ttl:   ttl,
	}
}

func (s *ttlSet) Seen(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for k, t := range s.items {
		if now.After(t) {
			delete(s.items, k)
		}
	}

	if exp, ok := s.items[key]; ok && now.Before(exp) {
		return true
	}
	s.items[key] = now.Add(s.ttl)
	return false
}
