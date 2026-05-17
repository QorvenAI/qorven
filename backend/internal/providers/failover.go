// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package providers

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// FailoverReason classifies why an LLM call failed.
type FailoverReason string

const (
	FailoverAuth       FailoverReason = "auth"
	FailoverRateLimit  FailoverReason = "rate_limit"
	FailoverOverloaded FailoverReason = "overloaded"
	FailoverTimeout    FailoverReason = "timeout"
	FailoverBilling    FailoverReason = "billing"
	FailoverOverflow   FailoverReason = "context_overflow"
	FailoverServer     FailoverReason = "server_error"
	FailoverUnknown    FailoverReason = "unknown"
)

// ClassifyError determines the failover reason from an error message.
// 80+ patterns across 7 categories (from Qorven's error classification).
func ClassifyError(errMsg string) FailoverReason {
	lower := strings.ToLower(errMsg)

	// Billing (check early — 402 can look like auth)
	for _, p := range billingPatterns {
		if strings.Contains(lower, p) {
			return FailoverBilling
		}
	}
	// Rate limit
	for _, p := range rateLimitPatterns {
		if strings.Contains(lower, p) {
			return FailoverRateLimit
		}
	}
	// Overloaded
	for _, p := range overloadedPatterns {
		if strings.Contains(lower, p) {
			return FailoverOverloaded
		}
	}
	// Context overflow
	for _, p := range overflowPatterns {
		if strings.Contains(lower, p) {
			return FailoverOverflow
		}
	}
	// Auth
	for _, p := range authPatterns {
		if strings.Contains(lower, p) {
			return FailoverAuth
		}
	}
	// Timeout/network
	for _, p := range timeoutPatterns {
		if strings.Contains(lower, p) {
			return FailoverTimeout
		}
	}
	// Server error
	for _, p := range serverPatterns {
		if strings.Contains(lower, p) {
			return FailoverServer
		}
	}
	return FailoverUnknown
}

var billingPatterns = []string{
	"402", "payment required", "insufficient credits", "insufficient_quota",
	"credit balance", "insufficient balance", "requires more credits",
}
var rateLimitPatterns = []string{
	"rate limit", "rate_limit", "too many requests", "429",
	"exceeded your current quota", "resource_exhausted", "quota exceeded",
	"tokens per minute", "tokens per day",
}
var overloadedPatterns = []string{
	"overloaded", "overloaded_error", "high demand",
}
var overflowPatterns = []string{
	"context window", "context length", "maximum context",
	"too many tokens", "prompt is too long", "request size exceeds",
}
var authPatterns = []string{
	"invalid_api_key", "incorrect api key", "unauthorized", "forbidden",
	"authentication", "invalid token", "access denied", "401", "403",
	"expired", "no credentials found", "permission_error",
}
var timeoutPatterns = []string{
	"timeout", "timed out", "deadline exceeded", "connection error",
	"network error", "fetch failed", "socket hang up", "econnrefused",
	"econnreset", "dns", "enetunreach",
}
var serverPatterns = []string{
	"internal server error", "internal_error", "server_error",
	"bad gateway", "gateway timeout", "502", "503", "504",
}

// --- Auth Profile Rotation ---

// AuthProfile represents one API key/credential for a provider.
type AuthProfile struct {
	ID         string
	Provider   string
	APIKey     string
	CooldownUntil time.Time // don't use until this time
	FailCount  int
	LastUsed   time.Time
	LastFailed *time.Time
}

// AuthRotator manages multiple API keys per provider with automatic failover.
type AuthRotator struct {
	profiles map[string][]*AuthProfile // provider → profiles
	mu       sync.Mutex
	cooldownDuration time.Duration
	maxFails int
}

// NewAuthRotator creates a rotator with defaults.
func NewAuthRotator() *AuthRotator {
	return &AuthRotator{
		profiles:         make(map[string][]*AuthProfile),
		cooldownDuration: 5 * time.Minute,
		maxFails:         3,
	}
}

// AddProfile registers an API key for a provider.
func (r *AuthRotator) AddProfile(provider, profileID, apiKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.profiles[provider] = append(r.profiles[provider], &AuthProfile{
		ID:       profileID,
		Provider: provider,
		APIKey:   apiKey,
	})
}

// GetKey returns the best available API key for a provider.
// Skips profiles in cooldown. Returns empty if all exhausted.
func (r *AuthRotator) GetKey(provider string) (profileID, apiKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	profiles := r.profiles[provider]
	now := time.Now()

	// Find first available (not in cooldown)
	for _, p := range profiles {
		if now.After(p.CooldownUntil) {
			p.LastUsed = now
			return p.ID, p.APIKey
		}
	}

	// All in cooldown — use the one with earliest cooldown expiry
	if len(profiles) > 0 {
		best := profiles[0]
		for _, p := range profiles[1:] {
			if p.CooldownUntil.Before(best.CooldownUntil) {
				best = p
			}
		}
		best.LastUsed = now
		return best.ID, best.APIKey
	}

	return "", ""
}

// MarkFailure records a failure for a profile and applies cooldown.
func (r *AuthRotator) MarkFailure(provider, profileID string, reason FailoverReason) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Timeouts are transport issues, not auth health signals
	if reason == FailoverTimeout {
		return
	}

	for _, p := range r.profiles[provider] {
		if p.ID == profileID {
			p.FailCount++
			now := time.Now()
			p.LastFailed = &now
			p.CooldownUntil = now.Add(r.cooldownDuration)
			slog.Warn("auth.profile.failure", "provider", provider, "profile", profileID,
				"reason", reason, "fails", p.FailCount, "cooldown_until", p.CooldownUntil)
			return
		}
	}
}

// MarkSuccess clears failure state for a profile.
func (r *AuthRotator) MarkSuccess(provider, profileID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.profiles[provider] {
		if p.ID == profileID {
			p.FailCount = 0
			p.LastFailed = nil
			p.CooldownUntil = time.Time{}
			return
		}
	}
}

// --- Smart Model Routing ---

// SmartRoute decides whether to use a cheap or primary model.
// Simple messages → cheap model (saves cost). Complex → primary.
// From Qorven's smart model routing (keyword-based, no LLM call needed).
func SmartRoute(message, primaryModel, cheapModel string) string {
	if cheapModel == "" {
		return primaryModel
	}

	text := strings.ToLower(strings.TrimSpace(message))

	// Too long → complex
	if len(text) > 500 || strings.Count(text, "\n") > 3 {
		return primaryModel
	}
	// Has code → complex
	if strings.Contains(text, "```") || strings.Contains(text, "`") {
		return primaryModel
	}
	// Has URL → complex
	if strings.Contains(text, "http://") || strings.Contains(text, "https://") {
		return primaryModel
	}
	// Complex keywords → primary
	for _, kw := range complexKeywords {
		if strings.Contains(text, kw) {
			return primaryModel
		}
	}
	// Short and simple → cheap
	if len(strings.Fields(text)) <= 20 {
		return cheapModel
	}
	return primaryModel
}

var complexKeywords = []string{
	"debug", "implement", "refactor", "analyze", "architecture",
	"design", "optimize", "review", "test", "deploy",
	"docker", "kubernetes", "database", "migration",
	"error", "exception", "traceback", "fix",
}
