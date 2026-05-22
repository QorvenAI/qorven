// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Embedder generates a vector embedding for a text string.
// Implemented by memory.EmbeddingClient; kept as an interface here to
// avoid an import cycle between gateway/llm and internal/memory.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// CacheLayer is the two-tier LLM response cache.
//
// Tier 1: in-memory LRU (max 1000 entries, 1h TTL) — ~0.1ms lookup.
// Tier 2: pgvector cosine-similarity search (threshold ≥ 0.95) — ~10ms.
//
// Safety rule: semantic cache is skipped when any message in the request
// has role "tool" (active tool context — file content, code, API responses).
// Exact cache is always checked since identical tool results → identical
// response is safe to return.
//
// Tier 2 is a no-op when db or embedder are nil (graceful degradation).
type CacheLayer struct {
	// Tier 1 — in-memory LRU
	mu       sync.Mutex
	entries  map[string]*list.Element
	lru      *list.List
	maxItems int
	ttl      time.Duration

	// Tier 2 — pgvector semantic cache
	db        *pgxpool.Pool
	embedder  Embedder
	tenantID  string
	threshold float64 // cosine similarity threshold (default 0.95)
}

type cacheEntry struct {
	key      string
	resp     *GatewayResponse
	expireAt time.Time
}

// defaultSemanticThreshold is the minimum cosine similarity for a semantic hit.
// 0.95 means the cached prompt must be ≥95% directionally similar to the query.
const defaultSemanticThreshold = 0.95

// NewCacheLayer creates an in-memory LRU cache with the given size and TTL.
// Pass maxItems=0 to use the default of 1000. Pass ttl=0 for 1h default.
// Tier 2 (semantic) is disabled until WithSemanticBackend is called.
func NewCacheLayer(maxItems int, ttl time.Duration) *CacheLayer {
	if maxItems <= 0 {
		maxItems = 1000
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &CacheLayer{
		entries:   make(map[string]*list.Element, maxItems),
		lru:       list.New(),
		maxItems:  maxItems,
		ttl:       ttl,
		threshold: defaultSemanticThreshold,
	}
}

// WithSemanticBackend enables the pgvector semantic cache tier.
func (c *CacheLayer) WithSemanticBackend(db *pgxpool.Pool, embedder Embedder, tenantID string) *CacheLayer {
	c.db = db
	c.embedder = embedder
	c.tenantID = tenantID
	return c
}

// WithThreshold overrides the cosine similarity threshold (0.0–1.0).
func (c *CacheLayer) WithThreshold(t float64) *CacheLayer {
	c.threshold = t
	return c
}

// Lookup checks both cache tiers for req.
//
// Order:
//  1. Tier 1 exact LRU — always checked.
//  2. Tier 2 semantic pgvector — only when no tool messages present.
//
// Returns the cached response and true on any hit.
func (c *CacheLayer) Lookup(ctx context.Context, req GatewayRequest) (*GatewayResponse, bool) {
	// Tier 1: exact match
	if resp, ok := c.lruLookup(req); ok {
		return resp, true
	}

	// Tier 2: semantic — skip when tool messages are present
	if c.db != nil && c.embedder != nil && !hasToolResults(req) {
		if resp, ok := c.semanticLookup(ctx, req); ok {
			// Warm tier 1 so subsequent identical calls are instant.
			go c.lruStore(req, resp)
			return resp, true
		}
	}

	return nil, false
}

// Store saves resp in both cache tiers (tier 2 asynchronously).
func (c *CacheLayer) Store(ctx context.Context, req GatewayRequest, resp *GatewayResponse) {
	if resp == nil || resp.ChatResponse == nil || resp.CacheHit {
		return
	}
	c.lruStore(req, resp)
	if c.db != nil && c.embedder != nil {
		go c.semanticStore(ctx, req, resp)
	}
}

// Stats returns current tier-1 cache size and capacity.
func (c *CacheLayer) Stats() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return map[string]int{
		"size":     c.lru.Len(),
		"capacity": c.maxItems,
	}
}

// Flush clears all tier-1 entries. Tier-2 entries expire naturally via expires_at.
func (c *CacheLayer) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*list.Element, c.maxItems)
	c.lru.Init()
}

// --- Tier 1: in-memory LRU ---

func (c *CacheLayer) lruLookup(req GatewayRequest) (*GatewayResponse, bool) {
	key := cacheKey(req)
	if key == "" {
		return nil, false
	}
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
	c.lru.MoveToFront(el)
	return e.resp, true
}

func (c *CacheLayer) lruStore(req GatewayRequest, resp *GatewayResponse) {
	key := cacheKey(req)
	if key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		c.lru.MoveToFront(el)
		el.Value.(*cacheEntry).resp = resp
		el.Value.(*cacheEntry).expireAt = time.Now().Add(c.ttl)
		return
	}
	if c.lru.Len() >= c.maxItems {
		if oldest := c.lru.Back(); oldest != nil {
			c.lru.Remove(oldest)
			delete(c.entries, oldest.Value.(*cacheEntry).key)
		}
	}
	entry := &cacheEntry{key: key, resp: resp, expireAt: time.Now().Add(c.ttl)}
	c.entries[key] = c.lru.PushFront(entry)
}

// --- Tier 2: pgvector semantic cache ---

// semanticText builds the text that is embedded to represent this request.
// We embed the last user message content (the query), prefixed with the model
// name so embeddings from different models don't cross-pollinate.
func semanticText(req GatewayRequest) string {
	var userContent string
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			userContent = req.Messages[i].Content
			break
		}
	}
	if userContent == "" {
		return ""
	}
	// Cap at 8000 chars — embedders have token limits.
	if len(userContent) > 8000 {
		userContent = userContent[:8000]
	}
	return req.Model + " " + userContent
}

func (c *CacheLayer) semanticLookup(ctx context.Context, req GatewayRequest) (*GatewayResponse, bool) {
	text := semanticText(req)
	if text == "" {
		return nil, false
	}

	vec, err := c.embedder.Embed(ctx, text)
	if err != nil {
		slog.Debug("gateway.cache.semantic: embed failed", "error", err)
		return nil, false
	}

	// Build pgvector literal: '[0.1,0.2,...]'
	vecLiteral := float32SliceToLiteral(vec)

	var responseJSON []byte
	var tokensSaved int
	err = c.db.QueryRow(ctx, `
		SELECT response, tokens_saved
		FROM   llm_cache
		WHERE  tenant_id = $1
		  AND  embedding IS NOT NULL
		  AND  (expires_at IS NULL OR expires_at > now())
		  AND  1 - (embedding <=> $2::vector) >= $3
		ORDER BY embedding <=> $2::vector
		LIMIT 1
	`, c.tenantID, vecLiteral, c.threshold).Scan(&responseJSON, &tokensSaved)
	if err != nil {
		// pgx returns pgx.ErrNoRows — that's a normal miss, not an error.
		return nil, false
	}

	// Increment hit count asynchronously — non-blocking.
	go func() {
		c.db.Exec(context.Background(), `
			UPDATE llm_cache
			SET    hit_count = hit_count + 1
			WHERE  tenant_id = $1
			  AND  embedding IS NOT NULL
			  AND  1 - (embedding <=> $2::vector) >= $3
			ORDER BY embedding <=> $2::vector
			LIMIT 1
		`, c.tenantID, vecLiteral, c.threshold) //nolint:errcheck
	}()

	// Deserialise cached response.
	var cached cachedResponseShape
	if err := json.Unmarshal(responseJSON, &cached); err != nil {
		slog.Warn("gateway.cache.semantic: failed to unmarshal cached response", "error", err)
		return nil, false
	}

	resp := cached.toGatewayResponse()
	resp.CacheHit = true
	slog.Debug("gateway.cache.semantic: hit", "tokens_saved", tokensSaved)
	return resp, true
}

func (c *CacheLayer) semanticStore(ctx context.Context, req GatewayRequest, resp *GatewayResponse) {
	text := semanticText(req)
	if text == "" {
		return
	}

	vec, err := c.embedder.Embed(ctx, text)
	if err != nil {
		slog.Debug("gateway.cache.semantic: embed for store failed", "error", err)
		return
	}
	vecLiteral := float32SliceToLiteral(vec)

	shape := newCachedResponseShape(resp)
	responseJSON, err := json.Marshal(shape)
	if err != nil {
		return
	}

	key := cacheKey(req)
	tokensIn := 0
	tokensOut := 0
	if resp.Usage != nil {
		tokensIn = resp.Usage.PromptTokens
		tokensOut = resp.Usage.CompletionTokens
	}

	expiresAt := time.Now().Add(c.ttl)
	_, err = c.db.Exec(ctx, `
		INSERT INTO llm_cache
		    (tenant_id, cache_key, prompt_hash, response, model, tokens_saved, embedding, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::vector, $8)
		ON CONFLICT (tenant_id, cache_key) DO UPDATE
		    SET response    = EXCLUDED.response,
		        embedding   = EXCLUDED.embedding,
		        tokens_saved = EXCLUDED.tokens_saved,
		        expires_at  = EXCLUDED.expires_at,
		        hit_count   = llm_cache.hit_count
	`, c.tenantID, key, key, responseJSON, req.Model, tokensIn+tokensOut, vecLiteral, expiresAt)
	if err != nil {
		slog.Debug("gateway.cache.semantic: store failed", "error", err)
	}
}

// --- Serialisation helpers ---

// cachedResponseShape is the JSONB shape stored in llm_cache.response.
type cachedResponseShape struct {
	Content    string `json:"content"`
	Model      string `json:"model,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	KeyID      string `json:"key_id,omitempty"`
}

func newCachedResponseShape(resp *GatewayResponse) cachedResponseShape {
	content := ""
	if resp.ChatResponse != nil {
		content = resp.Content
	}
	return cachedResponseShape{
		Content:    content,
		Model:      resp.ModelResolved,
		ProviderID: resp.ProviderID,
		KeyID:      resp.KeyID,
	}
}

func (s cachedResponseShape) toGatewayResponse() *GatewayResponse {
	return buildCachedGatewayResponse(s.Content, s.Model, s.ProviderID, s.KeyID)
}

// float32SliceToLiteral converts a float32 slice to a pgvector literal string.
// Format: '[0.1,0.2,0.3,...]'
func float32SliceToLiteral(vec []float32) string {
	if len(vec) == 0 {
		return "[]"
	}
	var sb strings.Builder
	sb.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(fmt.Sprintf("%.8f", v))
	}
	sb.WriteByte(']')
	return sb.String()
}

// --- Shared helpers ---

type cacheKeyMessage struct {
	Role    string `json:"r"`
	Content string `json:"c"`
	ToolID  string `json:"tid,omitempty"`
}

type cacheKeyShape struct {
	Model    string            `json:"m"`
	Messages []cacheKeyMessage `json:"ms"`
	Tools    []string          `json:"t,omitempty"`
}

// cacheKey returns a stable SHA-256 cache key for req.
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
func hasToolResults(req GatewayRequest) bool {
	for _, m := range req.Messages {
		if m.Role == "tool" {
			return true
		}
	}
	return false
}
