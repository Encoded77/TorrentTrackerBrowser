package core

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// ttlCache is a bounded in-memory map with per-entry expiry, shared by the
// result and payload caches.
type ttlCache[T any] struct {
	mu      sync.Mutex
	ttl     time.Duration
	cap     int
	prefix  string
	entries map[string]ttlEntry[T]
	order   []string // insertion order, for eviction when full
	now     func() time.Time
}

type ttlEntry[T any] struct {
	val     T
	expires time.Time
}

func newTTLCache[T any](prefix string, ttl time.Duration, capacity int) *ttlCache[T] {
	if capacity <= 0 {
		capacity = 10000
	}
	return &ttlCache[T]{ttl: ttl, cap: capacity, prefix: prefix, entries: map[string]ttlEntry[T]{}, now: time.Now}
}

// put stores the value built by mk under a fresh id and returns that id.
func (c *ttlCache[T]) put(mk func(id string) T) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeLocked()
	for len(c.order) >= c.cap {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.entries, oldest)
	}
	id := c.prefix + randomID(8)
	c.entries[id] = ttlEntry[T]{val: mk(id), expires: c.now().Add(c.ttl)}
	c.order = append(c.order, id)
	return id
}

// get returns the value for id when present and not expired.
func (c *ttlCache[T]) get(id string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[id]
	if !ok || !c.now().Before(e.expires) {
		var zero T
		return zero, false
	}
	return e.val, true
}

func (c *ttlCache[T]) purgeLocked() {
	if len(c.entries) == 0 {
		return
	}
	now := c.now()
	kept := c.order[:0]
	for _, id := range c.order {
		if e, ok := c.entries[id]; ok && now.Before(e.expires) {
			kept = append(kept, id)
		} else {
			delete(c.entries, id)
		}
	}
	c.order = kept
}

// randomID returns n random bytes as hex.
func randomID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
