// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

import (
	"sync"
	"time"
)

const (
	maxTrackedKeys   = 4096
	rateLimitWindow  = 60 * time.Second
	rateLimitMaxHits = 30
)

type rateLimitEntry struct {
	windowStart time.Time
	count       int
}

// WebhookRateLimiter bounds the number of tracked rate-limit keys.
type WebhookRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateLimitEntry
}

// NewWebhookRateLimiter creates a bounded webhook rate limiter.
func NewWebhookRateLimiter() *WebhookRateLimiter {
	return &WebhookRateLimiter{entries: make(map[string]*rateLimitEntry)}
}

// Allow returns true if the key is within rate limits.
func (r *WebhookRateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// Prune stale entries when approaching the cap
	if len(r.entries) >= maxTrackedKeys {
		for k, e := range r.entries {
			if now.Sub(e.windowStart) >= rateLimitWindow {
				delete(r.entries, k)
			}
		}
		for len(r.entries) >= maxTrackedKeys {
			for k := range r.entries {
				delete(r.entries, k)
				break
			}
		}
	}

	e, ok := r.entries[key]
	if !ok || now.Sub(e.windowStart) >= rateLimitWindow {
		r.entries[key] = &rateLimitEntry{windowStart: now, count: 1}
		return true
	}

	e.count++
	return e.count <= rateLimitMaxHits
}
