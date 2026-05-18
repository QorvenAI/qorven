// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scraper

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// CircuitBreaker tracks per-domain failure rates and auto-pauses scraping.
// After maxFails consecutive failures on a domain, it enters "open" state
// and rejects requests for cooldownDuration. This prevents burning proxies
// and IP reputation on domains that are actively blocking us.
type CircuitBreaker struct {
	mu       sync.Mutex
	states   map[string]*circuitState
	maxFails int
	cooldown time.Duration
}

type circuitState struct {
	consecutiveFails int
	openUntil        time.Time
	lastFail         time.Time
}

// NewCircuitBreaker creates a breaker that opens after 3 consecutive failures
// and stays open for 10 minutes per domain.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		states:   make(map[string]*circuitState),
		maxFails: 3,
		cooldown: 10 * time.Minute,
	}
}

// Allow checks if a domain is currently allowed.
// Returns error if the circuit is open.
func (cb *CircuitBreaker) Allow(domain string) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state, exists := cb.states[domain]
	if !exists {
		return nil
	}

	if time.Now().Before(state.openUntil) {
		remaining := time.Until(state.openUntil).Round(time.Second)
		return fmt.Errorf("circuit open for %s — %s consecutive failures, retry in %s", domain, fmt.Sprint(state.consecutiveFails), remaining)
	}

	// Circuit was open but cooldown expired — reset to half-open
	if state.consecutiveFails >= cb.maxFails {
		state.consecutiveFails = cb.maxFails - 1 // allow one retry
	}
	return nil
}

// RecordSuccess resets the failure counter for a domain.
func (cb *CircuitBreaker) RecordSuccess(domain string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.states, domain)
}

// RecordFailure increments the failure counter. Opens the circuit after maxFails.
func (cb *CircuitBreaker) RecordFailure(domain string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state, exists := cb.states[domain]
	if !exists {
		state = &circuitState{}
		cb.states[domain] = state
	}

	state.consecutiveFails++
	state.lastFail = time.Now()

	if state.consecutiveFails >= cb.maxFails {
		state.openUntil = time.Now().Add(cb.cooldown)
		slog.Warn("circuit.opened", "domain", domain, "fails", state.consecutiveFails, "cooldown", cb.cooldown)
	}
}

// RequestDedup prevents duplicate fetches of the same URL within a TTL window.
type RequestDedup struct {
	mu    sync.Mutex
	cache map[string]time.Time
	ttl   time.Duration
}

// NewRequestDedup creates a deduplicator with a 5-minute TTL.
func NewRequestDedup() *RequestDedup {
	return &RequestDedup{
		cache: make(map[string]time.Time),
		ttl:   5 * time.Minute,
	}
}

// IsDuplicate returns true if this URL was fetched recently.
func (rd *RequestDedup) IsDuplicate(url string) bool {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	// Cleanup expired entries periodically
	if len(rd.cache) > 1000 {
		now := time.Now()
		for k, t := range rd.cache {
			if now.Sub(t) > rd.ttl {
				delete(rd.cache, k)
			}
		}
	}

	if t, exists := rd.cache[url]; exists {
		if time.Since(t) < rd.ttl {
			return true
		}
	}
	rd.cache[url] = time.Now()
	return false
}
