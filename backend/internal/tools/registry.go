// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Registry manages tool instances with execution, scrubbing, and rate limiting.
type Registry struct {
	mu          sync.RWMutex
	tools       map[string]Tool
	aliases     map[string]string // alias → canonical name
	disabled    map[string]bool   // disabled tools (still registered but not offered to LLM)
	scrubbing   bool
	rateLimiter *RateLimiter
	metrics     *ToolMetrics
}

func NewRegistry() *Registry {
	return &Registry{
		tools:    make(map[string]Tool),
		aliases:  make(map[string]string),
		disabled: make(map[string]bool),
		metrics:  NewToolMetrics(),
	}
}

func (r *Registry) Register(t Tool)    { r.mu.Lock(); r.tools[t.Name()] = t; r.mu.Unlock() }
func (r *Registry) Unregister(name string) { r.mu.Lock(); delete(r.tools, name); r.mu.Unlock() }

// Disable marks a tool as disabled (still registered but hidden from LLM tool lists).
func (r *Registry) Disable(name string) { r.mu.Lock(); r.disabled[name] = true; r.mu.Unlock() }

// Enable re-enables a previously disabled tool.
func (r *Registry) Enable(name string) { r.mu.Lock(); delete(r.disabled, name); r.mu.Unlock() }

// IsDisabled returns true if the tool is disabled.
func (r *Registry) IsDisabled(name string) bool { r.mu.RLock(); defer r.mu.RUnlock(); return r.disabled[name] }

func (r *Registry) RegisterAlias(alias, canonical string) {
	r.mu.Lock(); r.aliases[alias] = canonical; r.mu.Unlock()
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock(); defer r.mu.RUnlock()
	if t, ok := r.tools[name]; ok { return t, true }
	if canonical, ok := r.aliases[name]; ok { return r.tools[canonical], true }
	return nil, false
}

// Execute runs a tool with rate limiting, credential scrubbing, and logging.
func (r *Registry) Execute(ctx context.Context, name string, args map[string]any) *Result {
	t, ok := r.Get(name)
	if !ok {
		return ErrorResult("unknown tool: " + name)
	}

	// Sandbox enforcement — check before execution
	if err := EnforceSandbox(ctx, name, args); err != nil {
		return ErrorResult(err.Error())
	}

	// Rate limit check
	if r.rateLimiter != nil {
		sessionKey := SessionIDFromCtx(ctx)
		if !r.rateLimiter.Allow(sessionKey) {
			return ErrorResult("rate limit exceeded — try again later")
		}
	}

	// Per-tool timeout: 60 seconds max
	toolCtx, toolCancel := context.WithTimeout(ctx, 60*time.Second)
	defer toolCancel()
	start := time.Now()
	result := t.Execute(toolCtx, args)
	dur := time.Since(start)

	// Record metrics
	if r.metrics != nil {
		r.metrics.Record(name, dur, !result.IsError)
	}

	// Credential scrubbing — defense against CVE-style credential leaks
	if r.scrubbing {
		result.ForLLM = ScrubCredentials(result.ForLLM)
		result.ForUser = ScrubCredentials(result.ForUser)
	}

	slog.Debug("tool executed", "tool", name, "duration_ms", dur.Milliseconds(), "is_error", result.IsError)
	return result
}

// List returns all registered canonical tool names.
func (r *Registry) List() []string {
	r.mu.RLock(); defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools { names = append(names, name) }
	return names
}

// Definitions returns ToolDefinitions for all registered tools (for LLM API).
func (r *Registry) Definitions() []ToolDefinition {
	r.mu.RLock(); defer r.mu.RUnlock()
	defs := make([]ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools { defs = append(defs, ToDefinition(t)) }
	return defs
}

func (r *Registry) Count() int { r.mu.RLock(); defer r.mu.RUnlock(); return len(r.tools) }
func (r *Registry) Aliases() map[string]string { r.mu.RLock(); defer r.mu.RUnlock(); return r.aliases }
func (r *Registry) SetScrubbing(v bool) { r.scrubbing = v }
func (r *Registry) SetRateLimiter(rl *RateLimiter) { r.rateLimiter = rl }
func (r *Registry) Metrics() *ToolMetrics { return r.metrics }

// --- Credential Scrubbing (defense against Qorven credential leak CVEs) ---

var scrubPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{20,})`),                          // OpenAI keys
	regexp.MustCompile(`(?i)(ghp_[a-zA-Z0-9]{36,})`),                         // GitHub PATs
	regexp.MustCompile(`(?i)(gho_[a-zA-Z0-9]{36,})`),                         // GitHub OAuth
	regexp.MustCompile(`(?i)(glpat-[a-zA-Z0-9\-]{20,})`),                     // GitLab PATs
	regexp.MustCompile(`(?i)(xoxb-[a-zA-Z0-9\-]+)`),                          // Slack bot tokens
	regexp.MustCompile(`(?i)(xoxp-[a-zA-Z0-9\-]+)`),                          // Slack user tokens
	regexp.MustCompile(`(?i)(Bearer\s+[a-zA-Z0-9\-_.~+/]+=*)`),               // Bearer tokens
	regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16})`),                             // AWS access keys
	regexp.MustCompile(`(?i)(postgres://[^\s]+)`),                             // Connection strings
	regexp.MustCompile(`(?i)(mysql://[^\s]+)`),                                // MySQL DSN
	regexp.MustCompile(`(?i)(mongodb(\+srv)?://[^\s]+)`),                      // MongoDB DSN
	regexp.MustCompile(`(?i)(redis://[^\s]+)`),                                // Redis DSN
	regexp.MustCompile(`(?i)([a-zA-Z_]*(SECRET|TOKEN|PASSWORD|CREDENTIAL|API_KEY)[a-zA-Z_]*\s*=\s*\S+)`), // env var patterns
	regexp.MustCompile(`[0-9a-f]{64}`),                                        // 64-char hex (encryption keys)
}

var (
	dynamicScrubMu     sync.RWMutex
	dynamicScrubValues []string
)

// AddDynamicScrubValues registers runtime secrets for scrubbing (e.g., server IPs).
func AddDynamicScrubValues(values ...string) {
	dynamicScrubMu.Lock()
	dynamicScrubValues = append(dynamicScrubValues, values...)
	dynamicScrubMu.Unlock()
}

// ScrubCredentials replaces sensitive patterns with [REDACTED].
func ScrubCredentials(s string) string {
	if s == "" { return s }
	for _, p := range scrubPatterns {
		s = p.ReplaceAllString(s, "[REDACTED]")
	}
	dynamicScrubMu.RLock()
	for _, v := range dynamicScrubValues {
		if v != "" && len(v) > 4 {
			s = strings.ReplaceAll(s, v, "[REDACTED]")
		}
	}
	dynamicScrubMu.RUnlock()
	return s
}

// --- Rate Limiter (defense against ClawJacked brute-force) ---

type RateLimiter struct {
	mu       sync.Mutex
	counters map[string]*rateBucket
	limit    int
}

type rateBucket struct {
	count    int
	resetAt  time.Time
}

func NewRateLimiter(perHour int) *RateLimiter {
	return &RateLimiter{counters: make(map[string]*rateBucket), limit: perHour}
}

func (rl *RateLimiter) Allow(key string) bool {
	if key == "" { return true }
	rl.mu.Lock(); defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.counters[key]
	if !ok || now.After(b.resetAt) {
		rl.counters[key] = &rateBucket{count: 1, resetAt: now.Add(time.Hour)}
		return true
	}
	b.count++
	return b.count <= rl.limit
}

// DescribeAll returns "name: description" for all registered tools.
func (r *Registry) DescribeAll() []string {
	r.mu.RLock(); defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Name() + ": " + t.Description())
	}
	return out
}
