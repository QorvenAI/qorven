// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// Package cache provides a simple in-memory cache with TTL expiration.
//
// It is safe for concurrent use. Expired entries are lazily cleaned up on access
// and periodically via a background goroutine.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// defaultMaxEntries is the maximum number of items a cache will hold before
// evicting the oldest entry. This prevents unbounded memory growth in
// long-running processes.
const defaultMaxEntries = 1000

// cacheItem holds a single cached value with its expiration time.
type cacheItem struct {
	data       []byte
	expiresAt  time.Time
	insertedAt time.Time
}

// Cache is a concurrency-safe in-memory key-value store with TTL expiration
// and a maximum entry count. When the entry count exceeds maxEntries, the
// oldest entry (by insertion time) is evicted.
type Cache struct {
	mu         sync.RWMutex
	items      map[string]cacheItem
	maxEntries int
	stop       chan struct{}
}

// New creates a new Cache with the default max-entries limit and starts a
// background cleanup goroutine that removes expired entries every 60 seconds.
func New() *Cache {
	return NewWithMax(defaultMaxEntries)
}

// NewWithMax creates a new Cache with the given max-entries limit and starts
// a background cleanup goroutine. If maxEntries <= 0, defaultMaxEntries is used.
func NewWithMax(maxEntries int) *Cache {
	if maxEntries <= 0 {
		maxEntries = defaultMaxEntries
	}
	c := &Cache{
		items:      make(map[string]cacheItem),
		maxEntries: maxEntries,
		stop:       make(chan struct{}),
	}
	go c.janitor()
	return c
}

// Get retrieves a value from the cache. Returns the data and true if found
// and not expired; nil and false otherwise.
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}
	if time.Now().After(item.expiresAt) {
		// Lazily remove expired entry.
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil, false
	}
	return item.data, true
}

// Set stores a value in the cache with the given TTL. If the cache exceeds
// maxEntries, the oldest entry (by insertion time) is evicted.
func (c *Cache) Set(key string, data []byte, ttl time.Duration) {
	now := time.Now()
	c.mu.Lock()
	c.items[key] = cacheItem{
		data:       data,
		expiresAt:  now.Add(ttl),
		insertedAt: now,
	}
	if len(c.items) > c.maxEntries {
		c.evictOldestLocked()
	}
	c.mu.Unlock()
}

// evictOldestLocked removes the entry with the earliest insertion time.
// Caller must hold c.mu.
func (c *Cache) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for k, v := range c.items {
		if first || v.insertedAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.insertedAt
			first = false
		}
	}
	if !first {
		delete(c.items, oldestKey)
	}
}

// Len returns the number of items in the cache (including expired ones that
// have not yet been cleaned up).
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Close stops the background cleanup goroutine.
func (c *Cache) Close() {
	close(c.stop)
}

// janitor periodically removes expired entries.
func (c *Cache) janitor() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stop:
			return
		}
	}
}

// cleanup removes all expired entries from the cache.
func (c *Cache) cleanup() {
	now := time.Now()
	c.mu.Lock()
	for key, item := range c.items {
		if now.After(item.expiresAt) {
			delete(c.items, key)
		}
	}
	c.mu.Unlock()
}

// Key builds a cache key by hashing the endpoint and payload.
func Key(endpoint, payload string) string {
	h := sha256.Sum256([]byte(endpoint + "|" + payload))
	return hex.EncodeToString(h[:16]) // 128-bit is sufficient for cache keys
}
