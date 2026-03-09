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

type ttlCache struct {
	mu   sync.Mutex
	data map[string]cacheEntry
	ttl  time.Duration
}

type cacheEntry struct {
	value   string
	expires time.Time
}

func newTTLCache(ttl time.Duration) *ttlCache {
	return &ttlCache{
		data: make(map[string]cacheEntry),
		ttl:  ttl,
	}
}

func (c *ttlCache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.data[key]
	if !ok || time.Now().After(entry.expires) {
		delete(c.data, key)
		return "", false
	}
	return entry.value, true
}

func (c *ttlCache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = cacheEntry{value: value, expires: time.Now().Add(c.ttl)}
}
