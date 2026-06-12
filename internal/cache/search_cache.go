package cache

import (
	"sync"
	"time"

	"autofilterbot/internal/autofilter"
)

type cacheEntry struct {
	results   []autofilter.Files
	expiresAt time.Time
}

type MemorySearchCache struct {
	mu       sync.RWMutex
	store    map[string]cacheEntry
	duration time.Duration
}

func NewMemorySearchCache(duration time.Duration) *MemorySearchCache {
	c := &MemorySearchCache{
		store:    make(map[string]cacheEntry),
		duration: duration,
	}
	// Run cleanup in background every minute
	go c.cleanupLoop()
	return c
}

func (c *MemorySearchCache) Get(query string) ([]autofilter.Files, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.store[query]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.results, true
}

func (c *MemorySearchCache) Set(query string, results []autofilter.Files) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.store[query] = cacheEntry{
		results:   results,
		expiresAt: time.Now().Add(c.duration),
	}
}

func (c *MemorySearchCache) Delete(query string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, query)
}

func (c *MemorySearchCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = make(map[string]cacheEntry)
}

func (c *MemorySearchCache) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, v := range c.store {
			if now.After(v.expiresAt) {
				delete(c.store, k)
			}
		}
		c.mu.Unlock()
	}
}
