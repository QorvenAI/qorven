// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// ResponseDedup prevents duplicate responses from being sent to clients.
// Handles two cases:
// 1. Identical requests arriving within a short window (webhook retries, double-clicks)
// 2. Partial stream guard — if a stream is already active for a session, reject new requests
type ResponseDedup struct {
	mu       sync.Mutex
	recent   map[string]*dedupEntry
	streams  map[string]bool // sessionID → stream active
	ttl      time.Duration
	maxItems int
}

type dedupEntry struct {
	hash      string
	response  string // cached response for non-streaming
	createdAt time.Time
}

func NewResponseDedup(ttl time.Duration, maxItems int) *ResponseDedup {
	d := &ResponseDedup{
		recent:   make(map[string]*dedupEntry),
		streams:  make(map[string]bool),
		ttl:      ttl,
		maxItems: maxItems,
	}
	go d.cleanupLoop()
	return d
}

// CheckRequest returns (cachedResponse, isDuplicate).
// If the same request was seen within TTL, returns the cached response.
func (d *ResponseDedup) CheckRequest(sessionID, userMessage string) (string, bool) {
	hash := hashRequest(sessionID, userMessage)

	d.mu.Lock()
	defer d.mu.Unlock()

	if entry, ok := d.recent[hash]; ok && time.Since(entry.createdAt) < d.ttl {
		return entry.response, true
	}
	return "", false
}

// RecordRequest records a request hash and its response for dedup.
func (d *ResponseDedup) RecordRequest(sessionID, userMessage, response string) {
	hash := hashRequest(sessionID, userMessage)

	d.mu.Lock()
	defer d.mu.Unlock()

	d.recent[hash] = &dedupEntry{hash: hash, response: response, createdAt: time.Now()}
}

// AcquireStream tries to acquire a stream lock for a session.
// Returns false if a stream is already active (prevents duplicate streaming).
func (d *ResponseDedup) AcquireStream(sessionID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.streams[sessionID] {
		return false // stream already active
	}
	d.streams[sessionID] = true
	return true
}

// ReleaseStream releases the stream lock for a session.
func (d *ResponseDedup) ReleaseStream(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.streams, sessionID)
}

func hashRequest(sessionID, message string) string {
	h := sha256.Sum256([]byte(sessionID + "|" + message))
	return hex.EncodeToString(h[:16])
}

func (d *ResponseDedup) cleanupLoop() {
	ticker := time.NewTicker(d.ttl)
	defer ticker.Stop()
	for range ticker.C {
		d.mu.Lock()
		now := time.Now()
		for k, v := range d.recent {
			if now.Sub(v.createdAt) > d.ttl { delete(d.recent, k) }
		}
		if len(d.recent) > d.maxItems {
			// Evict oldest
			var oldest string
			var oldestTime time.Time
			for k, v := range d.recent {
				if oldest == "" || v.createdAt.Before(oldestTime) {
					oldest = k
					oldestTime = v.createdAt
				}
			}
			if oldest != "" { delete(d.recent, oldest) }
		}
		d.mu.Unlock()
	}
}
