// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// Package sessioncancel tracks per-session context.CancelFunc so a user
// or admin abort can actually stop a running agent turn. Without this
// registry the gateway could emit a session.cancelled event while the
// agent goroutine kept burning tokens.
//
// Contract: one active context per session_id. If a new Submit arrives
// while one is already running, the old one is cancelled first
// (Cancel-and-Replace semantics) — concurrent turns on the same session
// are not supported by the agent loop today and would double-charge the
// cost meter anyway.
package sessioncancel

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// CancelCode tags the reason a session was cancelled. Callers read it
// from the event emitted at abort time.
type CancelCode string

const (
	CodeUserAbort  CancelCode = "user_abort"
	CodeAdminAbort CancelCode = "admin_abort"
	CodeTimeout    CancelCode = "timeout"
	CodeShutdown   CancelCode = "shutdown"
	CodePreempted  CancelCode = "preempted" // replaced by a newer submit
)

// Entry holds a single session's cancel state. Exposed for tests that
// want to assert on the tag fields; callers normally interact via
// Registry.
type Entry struct {
	Cancel   context.CancelFunc
	Actor    string
	Reason   string
	Code     CancelCode
	Deadline time.Time
	// cancelled is flipped to 1 on first Cancel call; prevents double-invoke.
	cancelled atomic.Bool
}

// Registry is a thread-safe map of session_id → cancel entry. Zero value
// is valid and ready for use.
type Registry struct {
	mu      sync.Mutex
	entries map[string]*Entry
}

// NewRegistry is a convenience constructor. A zero Registry works just
// as well; use the constructor for intent-readability in wiring code.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]*Entry)}
}

// Register installs a new cancel func for the session. If a previous
// entry exists it is cancelled first and its code is recorded as
// "preempted" for observability. The returned release func must be
// invoked by the caller (typically via defer) when the goroutine exits
// — it removes the entry if (and only if) this caller's entry is still
// the one installed.
func (r *Registry) Register(sessionID string, cancel context.CancelFunc, deadline time.Time) (release func()) {
	if sessionID == "" || cancel == nil {
		// Defensive: never panic in production from caller mistake.
		return func() {}
	}
	r.lazyInit()

	entry := &Entry{
		Cancel:   cancel,
		Deadline: deadline,
	}

	r.mu.Lock()
	if prev, ok := r.entries[sessionID]; ok {
		// Preempt the previous run. Do not hold the mutex while calling
		// cancel — the goroutine on the other end may call back into the
		// registry on its unwind path.
		r.mu.Unlock()
		prev.cancelOnce(CodePreempted, "", "replaced by new submit")
		r.mu.Lock()
	}
	r.entries[sessionID] = entry
	r.mu.Unlock()

	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		// Only delete if this caller is still the installed entry —
		// Cancel-and-Replace leaves the new entry in place.
		if r.entries[sessionID] == entry {
			delete(r.entries, sessionID)
		}
	}
}

// Cancel invokes the cancel func for sessionID with the given tags. It
// does NOT remove the entry — the goroutine's release func does that on
// its unwind. Returns true if an active entry was found and cancelled.
//
// Cancel is idempotent: a second call for the same sessionID is a no-op
// and returns false.
func (r *Registry) Cancel(sessionID string, code CancelCode, actor, reason string) bool {
	if sessionID == "" {
		return false
	}
	r.mu.Lock()
	entry, ok := r.entries[sessionID]
	r.mu.Unlock()
	if !ok {
		return false
	}
	return entry.cancelOnce(code, actor, reason)
}

// Active returns the number of currently-registered sessions. Intended
// for metrics and admin introspection.
func (r *Registry) Active() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

// Lookup returns a shallow copy of the entry's tag fields, or
// (Entry{}, false) if no entry exists. The CancelFunc is intentionally
// NOT returned — callers wanting to cancel must go through Cancel().
func (r *Registry) Lookup(sessionID string) (Entry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[sessionID]
	if !ok {
		return Entry{}, false
	}
	// Return a tag-only copy (CancelFunc zeroed) so the caller can't
	// cancel out-of-band.
	return Entry{
		Actor:    e.Actor,
		Reason:   e.Reason,
		Code:     e.Code,
		Deadline: e.Deadline,
	}, true
}

// CancelAll is invoked on graceful shutdown. All active entries are
// cancelled with code="shutdown". Returns the number of entries cancelled.
func (r *Registry) CancelAll(actor string) int {
	r.mu.Lock()
	entries := make([]*Entry, 0, len(r.entries))
	for _, e := range r.entries {
		entries = append(entries, e)
	}
	r.mu.Unlock()
	for _, e := range entries {
		e.cancelOnce(CodeShutdown, actor, "server shutdown")
	}
	return len(entries)
}

// cancelOnce is the internal single-shot cancel path. Safe to call
// concurrently; only the first call actually invokes the CancelFunc.
func (e *Entry) cancelOnce(code CancelCode, actor, reason string) bool {
	if !e.cancelled.CompareAndSwap(false, true) {
		return false
	}
	e.Code = code
	e.Actor = actor
	e.Reason = reason
	if e.Cancel != nil {
		e.Cancel()
	}
	return true
}

func (r *Registry) lazyInit() {
	r.mu.Lock()
	if r.entries == nil {
		r.entries = make(map[string]*Entry)
	}
	r.mu.Unlock()
}
