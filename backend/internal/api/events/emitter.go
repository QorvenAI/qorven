// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/ssestream"
)

// Emitter is a gateway-level helper that writes an event simultaneously to:
//
//   - one or more SSE consumers (the per-request Emitter chain), and
//   - the realtime.Hub (so WebSocket clients subscribed to the session,
//     project, or room also observe it).
//
// Not every event belongs on every channel; callers use Sink to direct
// the emission. A zero Sink (Sinks=None) is a no-op so tests can use a
// bare Emitter without any wiring.
//
// Concurrency: safe for use from any goroutine. Each individual SSE
// stream is serialized internally by ssestream.Emitter.
type Emitter struct {
	mu        sync.RWMutex
	sse       map[string]*ssestream.Emitter // keyed by subscriber id
	hub       *realtime.Hub
	logger    *slog.Logger
	idCounter atomic.Int64
}

// Sink flags where a single emission is delivered.
type Sink uint8

const (
	// SinkNone delivers nowhere — useful for dry-runs and tests.
	SinkNone Sink = 0
	// SinkSSE delivers to every attached SSE subscriber.
	SinkSSE Sink = 1 << iota
	// SinkHub delivers to the realtime WebSocket hub.
	SinkHub
	// SinkLog writes a slog entry at debug level.
	SinkLog

	// SinkAll fans out to every sink.
	SinkAll = SinkSSE | SinkHub | SinkLog
)

// Option customizes an Emitter.
type Option func(*Emitter)

// WithHub wires the realtime hub so events tagged SinkHub broadcast to WS.
func WithHub(h *realtime.Hub) Option { return func(e *Emitter) { e.hub = h } }

// WithLogger overrides the slog logger (default: slog.Default).
func WithLogger(l *slog.Logger) Option { return func(e *Emitter) { e.logger = l } }

// NewEmitter constructs an Emitter. Both hub and logger are optional.
func NewEmitter(opts ...Option) *Emitter {
	e := &Emitter{
		sse:    make(map[string]*ssestream.Emitter),
		logger: slog.Default(),
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Attach registers an SSE emitter under an opaque subscriber id. The id
// must be unique per call site; typically a request UUID. Returns a detach
// function — call it from the HTTP handler's defer so the emitter is
// cleaned up when the connection closes.
func (e *Emitter) Attach(id string, sse *ssestream.Emitter) func() {
	if e == nil {
		return func() {}
	}
	e.mu.Lock()
	e.sse[id] = sse
	e.mu.Unlock()
	return func() {
		e.mu.Lock()
		delete(e.sse, id)
		e.mu.Unlock()
	}
}

// Emit sends a typed event to the selected sinks. Properties must marshal
// to valid JSON. Unknown types log a warning but still emit — we don't
// block production on a typo in a dev handler.
func (e *Emitter) Emit(ctx context.Context, sinks Sink, t Type, props any) error {
	if e == nil || sinks == SinkNone {
		return nil
	}
	env, err := NewEnvelope(t, props)
	if err != nil {
		return err
	}
	env.ID = e.nextID()
	env.EmittedAtMS = time.Now().UnixMilli()

	if !IsKnown(t) {
		e.logger.Warn("events.emit: unknown type", "type", string(t))
	}

	var combined error

	if sinks&SinkLog != 0 {
		e.logger.Debug("events.emit",
			"type", string(t),
			"id", env.ID,
			"ts_ms", env.EmittedAtMS,
		)
	}

	if sinks&SinkSSE != 0 {
		e.mu.RLock()
		targets := make([]*ssestream.Emitter, 0, len(e.sse))
		for _, s := range e.sse {
			targets = append(targets, s)
		}
		e.mu.RUnlock()
		for _, s := range targets {
			if err := s.Send("", env); err != nil {
				// One broken SSE shouldn't fail the entire emission. Log
				// and continue — the detach function will clean up when
				// the request context terminates.
				e.logger.Warn("events.emit: sse send failed",
					"type", string(t), "err", err)
				combined = errors.Join(combined, err)
			}
		}
	}

	if sinks&SinkHub != 0 && e.hub != nil {
		e.hub.Broadcast(realtime.Event{
			Type:      "event",
			Data:      env,
			Timestamp: env.EmittedAtMS,
		})
	}

	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return errors.Join(combined, ctxErr)
		}
	}
	return combined
}

// MustEmit is like Emit but panics on marshal errors. Use only when the
// payload is a compile-time literal that can't possibly fail to marshal.
func (e *Emitter) MustEmit(ctx context.Context, sinks Sink, t Type, props any) {
	if err := e.Emit(ctx, sinks, t, props); err != nil {
		panic(fmt.Sprintf("events.MustEmit(%s): %v", t, err))
	}
}

// nextID returns a monotonically-increasing event ID scoped to this emitter.
// Format: "evt-<nanosecond-sequence>". Opaque to clients.
func (e *Emitter) nextID() string {
	return fmt.Sprintf("evt-%d", e.idCounter.Add(1))
}
