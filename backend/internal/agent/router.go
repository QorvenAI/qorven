// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const defaultRouterTTL = 10 * time.Minute

// agentEntry wraps a cached Loop with a timestamp for TTL-based expiration.
type agentEntry struct {
	loop     *Loop
	cachedAt time.Time
}

// ActivityStatus tracks the current phase of a running agent.
type ActivityStatus struct {
	RunID     string
	Phase     string // "thinking", "tool_exec", "compacting"
	Tool      string // current tool name (when Phase == "tool_exec")
	Iteration int
	StartedAt time.Time
}

// Router manages multiple agent Loop instances.
// Cached Loops expire after TTL (safety net for multi-instance).
type Router struct {
	agents        map[string]*agentEntry
	mu            sync.RWMutex
	activeRuns    sync.Map // runID → *ActiveRun
	sessionRuns   sync.Map // sessionKey → runID
	agentActivity sync.Map // sessionKey → *ActivityStatus
	resolver      ResolverFunc
	ttl           time.Duration
}

// NewRouter creates a new agent router.
func NewRouter() *Router {
	return &Router{
		agents: make(map[string]*agentEntry),
		ttl:    defaultRouterTTL,
	}
}

// SetResolver sets a resolver function for lazy agent creation.
func (r *Router) SetResolver(fn ResolverFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolver = fn
}

// Register adds an agent loop to the router.
func (r *Router) Register(id string, loop *Loop) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[id] = &agentEntry{loop: loop, cachedAt: time.Now()}
}

// Get returns an agent loop by ID. Lazy-creates from DB via resolver if needed.
func (r *Router) Get(ctx context.Context, agentID string) (*Loop, error) {
	cacheKey := agentCacheKey(ctx, agentID)

	r.mu.RLock()
	entry, ok := r.agents[cacheKey]
	resolver := r.resolver
	r.mu.RUnlock()

	if ok && (r.ttl == 0 || time.Since(entry.cachedAt) < r.ttl) {
		return entry.loop, nil
	}

	// TTL expired → remove stale entry
	if ok {
		r.mu.Lock()
		delete(r.agents, cacheKey)
		r.mu.Unlock()
	}

	// Try resolver
	if resolver != nil {
		loop, err := resolver(ctx, agentID)
		if err != nil {
			return nil, err
		}
		r.mu.Lock()
		if existing, ok := r.agents[cacheKey]; ok {
			r.mu.Unlock()
			return existing.loop, nil
		}
		r.agents[cacheKey] = &agentEntry{loop: loop, cachedAt: time.Now()}
		r.mu.Unlock()
		return loop, nil
	}

	return nil, fmt.Errorf("agent not found: %s", agentID)
}

// agentCacheKey builds a tenant-scoped cache key.
func agentCacheKey(ctx context.Context, agentID string) string {
	tid := TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		return agentID
	}
	return tid.String() + ":" + agentID
}

// TenantIDFromContext extracts tenant ID from context.
func TenantIDFromContext(ctx context.Context) uuid.UUID {
	if v := ctx.Value(ctxTenantIDKey{}); v != nil {
		if tid, ok := v.(uuid.UUID); ok {
			return tid
		}
	}
	return uuid.Nil
}

type ctxTenantIDKey struct{}

// WithTenantID adds tenant ID to context.
func WithTenantID(ctx context.Context, tenantID uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxTenantIDKey{}, tenantID)
}

// Remove removes an agent from the router.
func (r *Router) Remove(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	suffix := ":" + agentID
	for key := range r.agents {
		if key == agentID || strings.HasSuffix(key, suffix) {
			delete(r.agents, key)
		}
	}
}

// List returns all registered agent IDs.
func (r *Router) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	return ids
}

// AgentInfo is lightweight metadata about an agent.
type AgentInfo struct {
	ID        string `json:"id"`
	Model     string `json:"model"`
	IsRunning bool   `json:"isRunning"`
}

// ListInfo returns metadata for all agents.
func (r *Router) ListInfo() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]AgentInfo, 0, len(r.agents))
	for id := range r.agents {
		infos = append(infos, AgentInfo{
			ID:        id,
			Model:     "",
			IsRunning: r.IsRunning(id),
		})
	}
	return infos
}

// IsRunning checks if a specific agent is currently running.
func (r *Router) IsRunning(agentID string) bool {
	_, ok := r.sessionRuns.Load(agentID)
	return ok
}

// --- Active Run Tracking ---

// ActiveRun tracks a running agent invocation.
type ActiveRun struct {
	RunID      string
	SessionKey string
	AgentID    string
	Cancel     context.CancelFunc
	StartedAt  time.Time
	InjectCh   chan InjectedMessage
}

const injectBufferSize = 8

// RegisterRun records an active run.
func (r *Router) RegisterRun(runID, sessionKey, agentID string, cancel context.CancelFunc) <-chan InjectedMessage {
	injectCh := make(chan InjectedMessage, injectBufferSize)
	r.activeRuns.Store(runID, &ActiveRun{
		RunID:      runID,
		SessionKey: sessionKey,
		AgentID:    agentID,
		Cancel:     cancel,
		StartedAt:  time.Now(),
		InjectCh:   injectCh,
	})
	r.sessionRuns.Store(sessionKey, runID)
	return injectCh
}

// UnregisterRun removes a completed run.
func (r *Router) UnregisterRun(runID string) {
	if val, ok := r.activeRuns.Load(runID); ok {
		run := val.(*ActiveRun)
		r.sessionRuns.Delete(run.SessionKey)
	}
	r.activeRuns.Delete(runID)
}

// AbortRun cancels a run by ID.
func (r *Router) AbortRun(runID, sessionKey string) bool {
	val, ok := r.activeRuns.Load(runID)
	if !ok {
		return false
	}
	run := val.(*ActiveRun)

	if sessionKey != "" && run.SessionKey != sessionKey {
		return false
	}

	run.Cancel()
	r.sessionRuns.Delete(run.SessionKey)
	r.activeRuns.Delete(runID)
	return true
}

// InjectMessage sends a user message to the running loop.
func (r *Router) InjectMessage(sessionKey string, msg InjectedMessage) bool {
	runIDVal, ok := r.sessionRuns.Load(sessionKey)
	if !ok {
		return false
	}
	runVal, ok := r.activeRuns.Load(runIDVal)
	if !ok {
		return false
	}
	run := runVal.(*ActiveRun)
	select {
	case run.InjectCh <- msg:
		return true
	default:
		return false
	}
}

// IsSessionBusy returns true if there's an active run for the session.
func (r *Router) IsSessionBusy(sessionKey string) bool {
	_, ok := r.sessionRuns.Load(sessionKey)
	return ok
}

// SessionRunID returns the active run ID for a session.
func (r *Router) SessionRunID(sessionKey string) (string, bool) {
	val, ok := r.sessionRuns.Load(sessionKey)
	if !ok {
		return "", false
	}
	return val.(string), true
}

// AbortRunsForSession cancels all active runs for a session.
func (r *Router) AbortRunsForSession(sessionKey string) []string {
	var aborted []string
	r.activeRuns.Range(func(key, val any) bool {
		run := val.(*ActiveRun)
		if run.SessionKey == sessionKey {
			run.Cancel()
			r.activeRuns.Delete(key)
			r.sessionRuns.Delete(sessionKey)
			aborted = append(aborted, run.RunID)
		}
		return true
	})
	return aborted
}

// UpdateActivity records the current phase of a running agent.
func (r *Router) UpdateActivity(sessionKey, runID, phase, tool string, iteration int) {
	r.agentActivity.Store(sessionKey, &ActivityStatus{
		RunID:     runID,
		Phase:     phase,
		Tool:      tool,
		Iteration: iteration,
		StartedAt: time.Now(),
	})
}

// ClearActivity removes the activity status for a session.
func (r *Router) ClearActivity(sessionKey string) {
	r.agentActivity.Delete(sessionKey)
}

// GetActivity returns the current activity status for a session.
func (r *Router) GetActivity(sessionKey string) *ActivityStatus {
	val, ok := r.agentActivity.Load(sessionKey)
	if !ok {
		return nil
	}
	return val.(*ActivityStatus)
}
