// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package bus

import (
	"sync"
	"time"
)

// DedupeCache is a TTL-based deduplication cache for inbound messages.
type DedupeCache struct {
	mu      sync.Mutex
	entries map[string]int64 // key → unix millis
	ttl     time.Duration
	maxSize int
}

// NewDedupeCache creates a new dedup cache.
func NewDedupeCache(ttl time.Duration, maxSize int) *DedupeCache {
	return &DedupeCache{
		entries: make(map[string]int64, 256),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// IsDuplicate returns true if key was already seen within the TTL window.
func (d *DedupeCache) IsDuplicate(key string) bool {
	now := time.Now().UnixMilli()
	cutoff := now - d.ttl.Milliseconds()

	d.mu.Lock()
	defer d.mu.Unlock()

	if ts, ok := d.entries[key]; ok && ts >= cutoff {
		return true
	}

	d.cleanup(cutoff)
	d.entries[key] = now
	return false
}

func (d *DedupeCache) cleanup(cutoff int64) {
	for k, ts := range d.entries {
		if ts < cutoff {
			delete(d.entries, k)
		}
	}

	if d.maxSize > 0 && len(d.entries) >= d.maxSize {
		excess := len(d.entries) - d.maxSize + 1
		for k := range d.entries {
			if excess <= 0 {
				break
			}
			delete(d.entries, k)
			excess--
		}
	}
}
