package googlecalendar

import (
	"sync"
	"time"
)

type listCache struct {
	mu      sync.RWMutex
	now     func() time.Time
	entries map[string]cacheEntry
}

type cacheEntry struct {
	expiresAt time.Time
	events    []Event
}

func newListCache(now func() time.Time) *listCache {
	if now == nil {
		now = time.Now
	}
	return &listCache{
		now:     now,
		entries: map[string]cacheEntry{},
	}
}

func (c *listCache) Get(key string) ([]Event, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if !entry.expiresAt.After(c.now()) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}
	return cloneEvents(entry.events), true
}

func (c *listCache) Set(key string, events []Event, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	c.entries[key] = cacheEntry{
		expiresAt: c.now().Add(ttl),
		events:    cloneEvents(events),
	}
	c.mu.Unlock()
}

func (c *listCache) ClearAll() {
	c.mu.Lock()
	c.entries = map[string]cacheEntry{}
	c.mu.Unlock()
}

func cloneEvents(events []Event) []Event {
	if len(events) == 0 {
		return nil
	}
	out := make([]Event, len(events))
	copy(out, events)
	return out
}
