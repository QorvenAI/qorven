// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// Semaphore limits concurrent LLM calls across all Souls.
// Prevents rate limiting when many Souls work simultaneously.
type Semaphore struct {
	ch      chan struct{}
	max     int
	active  atomic.Int64
	queued  atomic.Int64
}

// NewSemaphore creates a concurrency limiter.
// max = maximum concurrent LLM calls (e.g., 5 for Bedrock)
func NewSemaphore(max int) *Semaphore {
	if max <= 0 {
		max = 5
	}
	return &Semaphore{
		ch:  make(chan struct{}, max),
		max: max,
	}
}

// Acquire blocks until a slot is available or context is cancelled.
func (s *Semaphore) Acquire(ctx context.Context) error {
	s.queued.Add(1)
	select {
	case s.ch <- struct{}{}:
		s.queued.Add(-1)
		s.active.Add(1)
		return nil
	case <-ctx.Done():
		s.queued.Add(-1)
		return ctx.Err()
	}
}

// Release frees a slot.
func (s *Semaphore) Release() {
	s.active.Add(-1)
	<-s.ch
}

// Stats returns current usage.
func (s *Semaphore) Stats() (active, queued, max int) {
	return int(s.active.Load()), int(s.queued.Load()), s.max
}

// --- Wrap Provider with Semaphore ---

// RateLimitedProvider wraps a Provider with a semaphore.
type RateLimitedProvider struct {
	inner Provider
	sem   *Semaphore
}

func NewRateLimitedProvider(inner Provider, sem *Semaphore) *RateLimitedProvider {
	return &RateLimitedProvider{inner: inner, sem: sem}
}

func (p *RateLimitedProvider) Name() string        { return p.inner.Name() }
func (p *RateLimitedProvider) DefaultModel() string { return p.inner.DefaultModel() }

func (p *RateLimitedProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if err := p.sem.Acquire(ctx); err != nil {
		return nil, err
	}
	defer p.sem.Release()
	active, queued, max := p.sem.Stats()
	slog.Debug("llm.call", "provider", p.inner.Name(), "active", active, "queued", queued, "max", max)
	return p.inner.Chat(ctx, req)
}

func (p *RateLimitedProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	if err := p.sem.Acquire(ctx); err != nil {
		return nil, err
	}
	defer p.sem.Release()
	return p.inner.ChatStream(ctx, req, onChunk)
}

// ListModels forwards the optional interface through the wrapper so the
// gateway's per-provider /v1/providers/{id}/models endpoint sees the full
// curated model list. Without this passthrough a type assertion against
// *RateLimitedProvider silently fails and the handler falls back to
// [DefaultModel()] — returning just one model for Bedrock.
func (p *RateLimitedProvider) ListModels(ctx context.Context) ([]string, error) {
	type modelLister interface {
		ListModels(ctx context.Context) ([]string, error)
	}
	if ml, ok := p.inner.(modelLister); ok {
		return ml.ListModels(ctx)
	}
	return []string{p.inner.DefaultModel()}, nil
}

// Unwrap returns the wrapped provider, useful for tests and for handlers
// that need to reach non-interface methods on the underlying provider.
func (p *RateLimitedProvider) Unwrap() Provider { return p.inner }
