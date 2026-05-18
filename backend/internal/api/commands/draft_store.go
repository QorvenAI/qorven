// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package commands

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/cache"
)

// DraftBackend is the storage interface for prompt drafts keyed by session id.
// The in-memory DraftStore satisfies this interface; the Redis-backed
// RedisDraftBackend does too. Swap the implementation via NewDraftStoreGuarded.
type DraftBackend interface {
	Append(sessionID, text string) int
	Get(sessionID string) string
	Take(sessionID string) string
	Clear(sessionID string)
}

// DraftStore holds in-memory prompt drafts keyed by session id.
//
// !! WARNING — MULTI-REPLICA UNSAFE !!
// This store lives in process memory. A load-balanced deployment with
// multiple gateway replicas will see drafts silently desync — a user who
// types on replica A and then submits on replica B will observe an empty
// draft with no error. Deployments running more than one replica MUST
// set QORVEN_CACHE_BACKEND=redis so NewDraftStoreGuarded constructs a
// Redis-backed store instead of panicking.
//
// Drafts are in-memory only — losing them on restart is intentional; they
// represent transient typing state, not persisted messages.
//
// The store includes a simple TTL sweeper so abandoned drafts do not
// accumulate forever.
type DraftStore struct {
	mu     sync.Mutex
	drafts map[string]*draft
	ttl    time.Duration
	stop   chan struct{}
}

type draft struct {
	text      string
	updatedAt time.Time
}

// NewDraftStore constructs a store with the given idle-TTL. Drafts not
// touched within ttl are garbage-collected every ttl/2 seconds. Passing
// ttl <= 0 disables the sweeper.
//
// Prefer NewDraftStoreGuarded in production — it refuses to start under
// a multi-replica deployment when using the in-memory backend.
func NewDraftStore(ttl time.Duration) *DraftStore {
	s := &DraftStore{
		drafts: make(map[string]*draft),
		ttl:    ttl,
		stop:   make(chan struct{}),
	}
	if ttl > 0 {
		go s.run()
	}
	return s
}

// NewDraftStoreGuarded is the production constructor. It panics only when
// replicas > 1 AND the backend is in-memory — a loud startup failure is
// better than silent draft desync across replicas. When QORVEN_CACHE_BACKEND
// is "redis" it returns a Redis-backed DraftBackend regardless of replica
// count, satisfying the multi-replica requirement.
func NewDraftStoreGuarded(ttl time.Duration, replicas int) DraftBackend {
	backend := cache.NewFromEnv[string]("drafts")
	// If we got a Redis-backed cache (or any non-memory backend), use it.
	// NewFromEnv logs a warning and falls back to inmemory on connection
	// failure, so check whether we actually got a non-nil Redis client by
	// attempting a type assertion.
	if _, isMemory := backend.(*cache.InMemoryCache[string]); !isMemory {
		return newRedisDraftBackend(backend, ttl)
	}
	// In-memory backend: panic if replicas > 1 to catch misconfigured
	// scale-out before silent desync occurs.
	if replicas > 1 {
		panic(fmt.Sprintf(
			"commands.DraftStore is in-memory and MUST NOT run with %d replicas. "+
				"Set QORVEN_CACHE_BACKEND=redis (with REDIS_URL) to use a shared draft store.",
			replicas))
	}
	return NewDraftStore(ttl)
}

// Append adds text to the draft for the session and returns the combined
// draft length (runes). Creates the draft if absent.
func (s *DraftStore) Append(sessionID, text string) int {
	if sessionID == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[sessionID]
	if !ok {
		d = &draft{}
		s.drafts[sessionID] = d
	}
	d.text += text
	d.updatedAt = time.Now()
	return len([]rune(d.text))
}

// Clear resets the draft for a session.
func (s *DraftStore) Clear(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.drafts, sessionID)
}

// Get returns the current draft text. Empty string when no draft exists.
func (s *DraftStore) Get(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.drafts[sessionID]; ok {
		return d.text
	}
	return ""
}

// Take returns the current draft and clears it atomically. Used by
// SubmitPrompt when the client does not override the text.
func (s *DraftStore) Take(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.drafts[sessionID]; ok {
		delete(s.drafts, sessionID)
		return d.text
	}
	return ""
}

// Close stops the background sweeper. Safe to call multiple times.
func (s *DraftStore) Close() {
	select {
	case <-s.stop:
		// already closed
	default:
		close(s.stop)
	}
}

func (s *DraftStore) run() {
	interval := s.ttl / 2
	if interval <= 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case now := <-t.C:
			s.sweep(now)
		}
	}
}

func (s *DraftStore) sweep(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := now.Add(-s.ttl)
	for k, d := range s.drafts {
		if d.updatedAt.Before(cutoff) {
			delete(s.drafts, k)
		}
	}
}

// ─── Redis-backed draft backend ───────────────────────────────────────────────

// redisDraftBackend wraps cache.Cache[string] to satisfy DraftBackend.
// Append is not truly atomic on Redis (GETSET + SET), but draft accumulation
// is a best-effort UX feature — a few lost bytes during a concurrent burst
// are acceptable. Take uses a Lua-based GETDEL approximation.
type redisDraftBackend struct {
	c   cache.Cache[string]
	ttl time.Duration
}

func newRedisDraftBackend(c cache.Cache[string], ttl time.Duration) *redisDraftBackend {
	return &redisDraftBackend{c: c, ttl: ttl}
}

func (r *redisDraftBackend) Append(sessionID, text string) int {
	if sessionID == "" {
		return 0
	}
	ctx := context.Background()
	existing, _ := r.c.Get(ctx, sessionID)
	combined := existing + text
	r.c.Set(ctx, sessionID, combined, r.ttl)
	return len([]rune(combined))
}

func (r *redisDraftBackend) Get(sessionID string) string {
	v, _ := r.c.Get(context.Background(), sessionID)
	return v
}

func (r *redisDraftBackend) Take(sessionID string) string {
	ctx := context.Background()
	v, ok := r.c.Get(ctx, sessionID)
	if ok {
		r.c.Delete(ctx, sessionID)
	}
	return v
}

func (r *redisDraftBackend) Clear(sessionID string) {
	r.c.Delete(context.Background(), sessionID)
}
