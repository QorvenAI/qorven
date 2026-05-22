// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// CacheLayer is the two-tier LLM response cache.
//
// Tier 1: in-memory LRU (max 1000 entries, 1h TTL) — ~0.1ms lookup.
// Tier 2: pgvector semantic similarity — not yet implemented; reserved for
//
//	Phase 3.4 once the pgvector extension is available in the deployment.
//
// Safety: semantic cache is skipped when any message in the request has
// role "tool" (active tool context — file content, code, API responses).
// Exact cache is always checked since identical tool results → identical
// response is safe to return.
type CacheLayer struct {
	mu       sync.Mutex
	entries  map[string]*list.Element
	lru      *list.List
	maxItems int
	ttl      time.Duration
}

type cacheEntry struct {
	key      string
	resp     *GatewayResponse
	expireAt time.Time
}

// NewCacheLayer creates an in-memory LRU cache with the given size and TTL.
// Pass maxItems=0 to use the default of 1000. Pass ttl=0 for 1h default.
func NewCacheLayer(maxItems int, ttl time.Duration) *CacheLayer {
	if maxItems <= 0 {
		maxItems = 1000
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &CacheLayer{
		entries:  make(map[string]*list.Element, maxItems),
		lru:      list.New(),
		maxItems: maxItems,
		ttl:      ttl,
	}
}

// Lookup checks the cache for req. Returns the cached response and true on
// hit, or nil and false on miss.
//
// SAFETY RULE: if any message in the request has role "tool" (agent has an
// active tool context — file content, code, API responses), the semantic
// tier is skipped. Exact-match lookup still applies.
func (c *CacheLayer) Lookup(_ context.Context, req GatewayRequest) (*GatewayResponse, bool) {
	key := cacheKey(req)
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	e := el.Value.(*cacheEntry)
	if time.Now().After(e.expireAt) {
		c.lru.Remove(el)
		delete(c.entries, key)
		return nil, false
	}
	// Move to front (most recently used).
	c.lru.MoveToFront(el)
	return e.resp, true
}

// Store saves resp in the cache for req. Non-blocking: any error is logged
// and discarded.
func (c *CacheLayer) Store(_ context.Context, req GatewayRequest, resp *GatewayResponse) {
	if resp == nil || resp.ChatResponse == nil || resp.CacheHit {
		return
	}
	key := cacheKey(req)
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.entries[key]; ok {
		c.lru.MoveToFront(el)
		el.Value.(*cacheEntry).resp = resp
		el.Value.(*cacheEntry).expireAt = time.Now().Add(c.ttl)
		return
	}

	// Evict oldest if at capacity.
	if c.lru.Len() >= c.maxItems {
		oldest := c.lru.Back()
		if oldest != nil {
			c.lru.Remove(oldest)
			delete(c.entries, oldest.Value.(*cacheEntry).key)
		}
	}

	entry := &cacheEntry{key: key, resp: resp, expireAt: time.Now().Add(c.ttl)}
	el := c.lru.PushFront(entry)
	c.entries[key] = el
}

// Stats returns current cache size and capacity.
func (c *CacheLayer) Stats() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return map[string]int{
		"size":     c.lru.Len(),
		"capacity": c.maxItems,
	}
}

// Flush clears all entries from the cache.
func (c *CacheLayer) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*list.Element, c.maxItems)
	c.lru.Init()
}

type cacheKeyMessage struct {
	Role    string `json:"r"`
	Content string `json:"c"`
	ToolID  string `json:"tid,omitempty"`
}

type cacheKeyShape struct {
	Model    string             `json:"m"`
	Messages []cacheKeyMessage  `json:"ms"`
	Tools    []string           `json:"t,omitempty"`
}

// cacheKey returns a stable SHA-256 cache key for req.
// It incorporates model, messages (with roles, content, and tool_call_ids),
// and tool definitions — anything that affects the LLM's response.
func cacheKey(req GatewayRequest) string {
	msgs := make([]cacheKeyMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = cacheKeyMessage{Role: m.Role, Content: m.Content, ToolID: m.ToolCallID}
	}
	toolNames := make([]string, 0, len(req.Tools))
	for _, t := range req.Tools {
		toolNames = append(toolNames, t.Function.Name)
	}

	b, err := json.Marshal(cacheKeyShape{Model: req.Model, Messages: msgs, Tools: toolNames})
	if err != nil {
		slog.Warn("gateway.cache: failed to marshal cache key", "error", err)
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// hasToolResults returns true if any message in the request has role "tool".
// Used to decide whether the semantic cache tier is safe to consult.
func hasToolResults(req GatewayRequest) bool {
	for _, m := range req.Messages {
		if m.Role == "tool" {
			return true
		}
	}
	return false
}
