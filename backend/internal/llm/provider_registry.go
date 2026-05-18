// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package llm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Registry manages all configured LLM providers with fallback.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	order     []string // fallback order
	primary   string
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

func (r *Registry) Register(name string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = p
	r.order = append(r.order, name)
	if r.primary == "" { r.primary = name }
}

func (r *Registry) SetPrimary(name string) { r.mu.Lock(); r.primary = name; r.mu.Unlock() }

func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

func (r *Registry) Primary() (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[r.primary]
	return p, ok
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]string{}, r.order...)
}

// ChatWithFallback tries primary, then falls back through other providers.
func (r *Registry) ChatWithFallback(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	r.mu.RLock()
	order := append([]string{r.primary}, r.order...)
	r.mu.RUnlock()

	seen := map[string]bool{}
	var lastErr error
	for _, name := range order {
		if seen[name] { continue }
		seen[name] = true
		p, ok := r.Get(name)
		if !ok { continue }

		resp, err := p.Chat(ctx, req)
		if err == nil { return resp, nil }
		lastErr = err
		slog.Warn("llm.fallback", "provider", name, "error", err)
	}
	return nil, fmt.Errorf("all providers failed, last: %w", lastErr)
}

// StreamWithFallback tries primary, then falls back.
func (r *Registry) StreamWithFallback(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	r.mu.RLock()
	order := append([]string{r.primary}, r.order...)
	r.mu.RUnlock()

	seen := map[string]bool{}
	var lastErr error
	for _, name := range order {
		if seen[name] { continue }
		seen[name] = true
		p, ok := r.Get(name)
		if !ok { continue }

		resp, err := p.ChatStream(ctx, req, onChunk)
		if err == nil { return resp, nil }
		lastErr = err
		slog.Warn("llm.stream.fallback", "provider", name, "error", err)
	}
	return nil, fmt.Errorf("all providers failed, last: %w", lastErr)
}
