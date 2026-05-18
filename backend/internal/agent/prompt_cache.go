// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// PromptCache caches built system prompts per agent+session to avoid rebuilding every turn.
// Qorven pattern: system prompt is stable within a session, only changes on config/skill updates.
// This also enables Anthropic prompt caching (cache_control) for massive cost reduction.
type PromptCache struct {
	mu      sync.RWMutex
	entries map[string]*promptCacheEntry
	maxAge  time.Duration
}

type promptCacheEntry struct {
	prompt    string
	hash      string
	createdAt time.Time
	hitCount  int
}

// NewPromptCache creates a prompt cache with the given TTL.
func NewPromptCache(maxAge time.Duration) *PromptCache {
	return &PromptCache{
		entries: make(map[string]*promptCacheEntry),
		maxAge:  maxAge,
	}
}

// Get returns a cached prompt if it exists and hasn't expired.
func (pc *PromptCache) Get(agentID, sessionID string) (string, bool) {
	key := agentID + ":" + sessionID
	pc.mu.RLock()
	entry, ok := pc.entries[key]
	pc.mu.RUnlock()
	if !ok || time.Since(entry.createdAt) > pc.maxAge {
		return "", false
	}
	pc.mu.Lock()
	entry.hitCount++
	pc.mu.Unlock()
	return entry.prompt, true
}

// Set stores a built prompt in the cache.
func (pc *PromptCache) Set(agentID, sessionID, prompt string) {
	key := agentID + ":" + sessionID
	h := sha256.Sum256([]byte(prompt))
	pc.mu.Lock()
	pc.entries[key] = &promptCacheEntry{
		prompt:    prompt,
		hash:      fmt.Sprintf("%x", h[:8]),
		createdAt: time.Now(),
	}
	pc.mu.Unlock()
}

// Invalidate removes a cached prompt (e.g. when agent config changes).
func (pc *PromptCache) Invalidate(agentID string) {
	pc.mu.Lock()
	for key := range pc.entries {
		if len(key) > len(agentID) && key[:len(agentID)] == agentID {
			delete(pc.entries, key)
		}
	}
	pc.mu.Unlock()
}

// Prune removes expired entries. Call periodically.
func (pc *PromptCache) Prune() int {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pruned := 0
	for key, entry := range pc.entries {
		if time.Since(entry.createdAt) > pc.maxAge {
			delete(pc.entries, key)
			pruned++
		}
	}
	return pruned
}

// Stats returns cache statistics.
func (pc *PromptCache) Stats() map[string]any {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	totalHits := 0
	for _, e := range pc.entries {
		totalHits += e.hitCount
	}
	return map[string]any{
		"entries":    len(pc.entries),
		"total_hits": totalHits,
	}
}
