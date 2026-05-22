// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"context"
	"fmt"
)

// PriorityQueue is a three-tier concurrency limiter. Each tier has an
// independent semaphore so interactive requests can never be blocked by
// background or batch work.
//
// Capacities (configurable via WithCapacities):
//
//	Interactive: 200 concurrent requests
//	Background:  500 concurrent requests
//	Batch:       300 concurrent requests
type PriorityQueue struct {
	interactive chan struct{}
	background  chan struct{}
	batch       chan struct{}
}

// DefaultCapacities are used when no explicit configuration is provided.
const (
	DefaultInteractiveCapacity = 200
	DefaultBackgroundCapacity  = 500
	DefaultBatchCapacity       = 300
)

// PriorityQueueConfig holds per-tier concurrency limits.
type PriorityQueueConfig struct {
	InteractiveCapacity int
	BackgroundCapacity  int
	BatchCapacity       int
}

// NewPriorityQueue creates a queue with default capacities.
func NewPriorityQueue() *PriorityQueue {
	return NewPriorityQueueWithConfig(PriorityQueueConfig{
		InteractiveCapacity: DefaultInteractiveCapacity,
		BackgroundCapacity:  DefaultBackgroundCapacity,
		BatchCapacity:       DefaultBatchCapacity,
	})
}

// NewPriorityQueueWithConfig creates a queue with the given capacities.
// A zero capacity in any field falls back to the corresponding default.
func NewPriorityQueueWithConfig(cfg PriorityQueueConfig) *PriorityQueue {
	if cfg.InteractiveCapacity <= 0 {
		cfg.InteractiveCapacity = DefaultInteractiveCapacity
	}
	if cfg.BackgroundCapacity <= 0 {
		cfg.BackgroundCapacity = DefaultBackgroundCapacity
	}
	if cfg.BatchCapacity <= 0 {
		cfg.BatchCapacity = DefaultBatchCapacity
	}
	return &PriorityQueue{
		interactive: make(chan struct{}, cfg.InteractiveCapacity),
		background:  make(chan struct{}, cfg.BackgroundCapacity),
		batch:       make(chan struct{}, cfg.BatchCapacity),
	}
}

// Acquire claims a slot in the priority tier. It blocks until a slot is
// available or ctx is cancelled. Each successful Acquire must be paired
// with a Release on the same tier.
func (q *PriorityQueue) Acquire(ctx context.Context, p Priority) error {
	sem := q.semFor(p)
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release frees the slot held by this request.
func (q *PriorityQueue) Release(p Priority) {
	sem := q.semFor(p)
	select {
	case <-sem:
	default:
	}
}

// Depths returns the current queue depth (occupied slots) for all tiers.
func (q *PriorityQueue) Depths() map[string]int {
	return map[string]int{
		"interactive": len(q.interactive),
		"background":  len(q.background),
		"batch":       len(q.batch),
	}
}

// Capacities returns the maximum capacity for all tiers.
func (q *PriorityQueue) Capacities() map[string]int {
	return map[string]int{
		"interactive": cap(q.interactive),
		"background":  cap(q.background),
		"batch":       cap(q.batch),
	}
}

func (q *PriorityQueue) semFor(p Priority) chan struct{} {
	switch p {
	case PriorityInteractive:
		return q.interactive
	case PriorityBackground:
		return q.background
	case PriorityBatch:
		return q.batch
	default:
		panic(fmt.Sprintf("gateway/llm: unknown priority %d", p))
	}
}
