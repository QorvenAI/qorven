// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// QueryRewriter rewrites user messages into optimized search queries.
// Mentor: only for Research intent, use gemini-2.0-flash, last 3 messages, cache 60s.
type QueryRewriter struct {
	provider providers.Provider
	model    string
	cache    sync.Map // hash → cachedQueries
}

type cachedQueries struct {
	queries   []string
	expiresAt time.Time
}

func NewQueryRewriter(provider providers.Provider, model string) *QueryRewriter {
	return &QueryRewriter{provider: provider, model: model}
}

// Rewrite generates 1-3 optimized search queries from the user message + recent history.
func (qr *QueryRewriter) Rewrite(ctx context.Context, userMessage string, recentHistory []providers.Message) []string {
	// Build cache key from last 3 messages + current
	key := qr.cacheKey(userMessage, recentHistory)
	if cached, ok := qr.cache.Load(key); ok {
		c := cached.(*cachedQueries)
		if time.Now().Before(c.expiresAt) {
			return c.queries
		}
		qr.cache.Delete(key)
	}

	// Build context from last 3 messages (mentor: not 6)
	var historyText strings.Builder
	start := len(recentHistory) - 6 // 3 pairs
	if start < 0 { start = 0 }
	for _, m := range recentHistory[start:] {
		historyText.WriteString(fmt.Sprintf("%s: %s\n", m.Role, truncate(m.Content, 200)))
	}

	prompt := fmt.Sprintf(`Analyze the conversation and generate 1-3 search queries to find relevant information.
Today's date: %s

Output ONLY JSON: {"queries": ["query1", "query2"]}

If no search is needed, return: {"queries": []}

Recent conversation:
%s
Current message: %s`, time.Now().Format("January 2, 2006"), historyText.String(), userMessage)

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	resp, err := qr.provider.Chat(ctx, providers.ChatRequest{
		Model:    qr.model,
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options:  map[string]any{"max_tokens": 100, "temperature": 0.3},
	})
	if err != nil {
		slog.Warn("query_rewrite.failed", "error", err)
		return []string{userMessage} // fallback to original
	}

	queries := extractJSONStringArray(resp.Content, "queries")
	if len(queries) == 0 {
		queries = []string{userMessage}
	}

	// Cache for 60s
	qr.cache.Store(key, &cachedQueries{queries: queries, expiresAt: time.Now().Add(60 * time.Second)})
	slog.Info("query_rewrite.done", "original", userMessage, "queries", queries)
	return queries
}

func (qr *QueryRewriter) cacheKey(msg string, history []providers.Message) string {
	h := sha256.New()
	start := len(history) - 6
	if start < 0 { start = 0 }
	for _, m := range history[start:] {
		h.Write([]byte(m.Content))
	}
	h.Write([]byte(msg))
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// RewriteIfResearch only rewrites for research intent, returns original for others.
func RewriteIfResearch(ctx context.Context, qr *QueryRewriter, intent ChatIntent, userMessage string, history []providers.Message) []string {
	if qr == nil || intent != IntentResearch {
		return nil // no rewriting needed
	}
	return qr.Rewrite(ctx, userMessage, history)
}

// Ensure extractJSONStringArray is available (defined in background_tasks.go)
// No duplicate needed — it's in the same package.
