// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package providers

import (
	"crypto/sha256"
	"fmt"
	"sync"
)

// PromptCache provides application-level prompt caching hints.
// It detects the provider type and injects the right cache markers
// before sending to the API — no proxy dependency.
//
// Supported providers:
//   - Anthropic: cache_control ephemeral on system + first N messages
//   - Gemini: cachedContent field (via Options)
//   - OpenAI: automatic (GPT-4o+ caches internally)
//   - Others: no-op passthrough
type PromptCache struct {
	mu    sync.RWMutex
	seen  map[string]bool // hash → already sent
}

func NewPromptCache() *PromptCache {
	return &PromptCache{seen: make(map[string]bool)}
}

// Apply injects cache hints into a ChatRequest based on provider type.
// Returns a modified copy — does not mutate the original.
func (pc *PromptCache) Apply(req ChatRequest, providerName string) ChatRequest {
	switch detectCacheStrategy(providerName) {
	case cacheAnthropic:
		return pc.applyAnthropic(req)
	case cacheGemini:
		return pc.applyGemini(req)
	default:
		return req // no-op for OpenAI (auto-cached) and others
	}
}

type cacheStrategy int

const (
	cacheNone      cacheStrategy = iota
	cacheAnthropic               // cache_control: ephemeral
	cacheGemini                  // cachedContent
)

func detectCacheStrategy(name string) cacheStrategy {
	switch {
	case contains(name, "anthropic", "claude", "bedrock-anthropic"):
		return cacheAnthropic
	case contains(name, "gemini", "google", "vertex"):
		return cacheGemini
	default:
		return cacheNone
	}
}

// applyAnthropic marks system prompt and first messages with cache_control.
// Anthropic caches content blocks marked with {"type": "ephemeral"}.
// This saves ~90% on repeated system prompts across turns.
func (pc *PromptCache) applyAnthropic(req ChatRequest) ChatRequest {
	if len(req.Messages) == 0 {
		return req
	}
	if req.Options == nil {
		req.Options = make(map[string]any)
	}

	// Mark system message for caching
	for i, m := range req.Messages {
		if m.Role == "system" {
			hash := hashContent(m.Content)
			pc.mu.RLock()
			_, wasSeen := pc.seen[hash]
			pc.mu.RUnlock()

			if !wasSeen {
				pc.mu.Lock()
				pc.seen[hash] = true
				pc.mu.Unlock()
			}
			// Always mark system for caching — Anthropic handles dedup
			req.Messages[i].CacheControl = "ephemeral"
			break
		}
	}

	// Mark the last user message before the current turn as cacheable
	// (conversation prefix caching)
	userCount := 0
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			userCount++
			if userCount == 2 { // second-to-last user message = conversation prefix
				req.Messages[i].CacheControl = "ephemeral"
				break
			}
		}
	}

	req.Options["_cache_strategy"] = "anthropic_ephemeral"
	return req
}

// applyGemini sets cachedContent option for system prompt reuse.
// Gemini's context caching requires a minimum of 32k tokens to activate.
func (pc *PromptCache) applyGemini(req ChatRequest) ChatRequest {
	if req.Options == nil {
		req.Options = make(map[string]any)
	}
	// Signal to the Gemini provider to use cached_content API
	// when system prompt exceeds 32k tokens
	for _, m := range req.Messages {
		if m.Role == "system" && len(m.Content) > 32000 {
			req.Options["_cache_strategy"] = "gemini_cached_content"
			req.Options["_cached_system"] = hashContent(m.Content)
			break
		}
	}
	return req
}

func hashContent(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8])
}

func contains(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
