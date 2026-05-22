// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package llm

import (
	"errors"
	"sync"
	"time"

	"github.com/sony/gobreaker"
)

// CircuitBreakerBank maintains one circuit breaker per API key.
// When a key has 5 consecutive failures within 60 seconds the breaker
// opens; the key is skipped for 30 seconds, then a single probe request
// is allowed through.
type CircuitBreakerBank struct {
	mu       sync.RWMutex
	breakers map[string]*gobreaker.CircuitBreaker
}

// NewCircuitBreakerBank creates an empty bank. Breakers are created on
// first use so there's no startup cost for providers with many keys.
func NewCircuitBreakerBank() *CircuitBreakerBank {
	return &CircuitBreakerBank{breakers: make(map[string]*gobreaker.CircuitBreaker)}
}

// Execute runs fn inside the circuit breaker for keyID. If the breaker is
// open the call is short-circuited and gobreaker.ErrOpenState is returned;
// callers should treat that as a signal to try the next available key.
func (b *CircuitBreakerBank) Execute(keyID string, fn func() error) error {
	_, err := b.get(keyID).Execute(func() (any, error) {
		return nil, fn()
	})
	return err
}

// State returns the current state of the breaker for a key. Returns
// gobreaker.StateClosed if the key has never been seen.
func (b *CircuitBreakerBank) State(keyID string) gobreaker.State {
	b.mu.RLock()
	cb, ok := b.breakers[keyID]
	b.mu.RUnlock()
	if !ok {
		return gobreaker.StateClosed
	}
	return cb.State()
}

// Stats returns a snapshot of counts for the given key. Zero values if
// the key has no breaker yet.
func (b *CircuitBreakerBank) Stats(keyID string) gobreaker.Counts {
	b.mu.RLock()
	cb, ok := b.breakers[keyID]
	b.mu.RUnlock()
	if !ok {
		return gobreaker.Counts{}
	}
	return cb.Counts()
}

// AllStates returns a snapshot of (keyID → state) for all tracked keys.
func (b *CircuitBreakerBank) AllStates() map[string]gobreaker.State {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make(map[string]gobreaker.State, len(b.breakers))
	for k, cb := range b.breakers {
		out[k] = cb.State()
	}
	return out
}

// CircuitStateName converts a gobreaker.State to its string representation.
func CircuitStateName(s gobreaker.State) string {
	switch s {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateHalfOpen:
		return "half_open"
	case gobreaker.StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// isCircuitOpenError returns true if the error is a gobreaker open-state error.
func isCircuitOpenError(err error) bool {
	return errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests)
}

func (b *CircuitBreakerBank) get(keyID string) *gobreaker.CircuitBreaker {
	b.mu.RLock()
	cb, ok := b.breakers[keyID]
	b.mu.RUnlock()
	if ok {
		return cb
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	// Double-check under write lock.
	if cb, ok = b.breakers[keyID]; ok {
		return cb
	}

	cb = gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name: keyID,
		// Reset failure counter every 60 s so a brief bad period
		// doesn't trip the breaker for the rest of time.
		Interval: 60 * time.Second,
		// After opening, try one probe request after 30 s.
		Timeout: 30 * time.Second,
		// Only one probe allowed in half-open state before fully closing.
		MaxRequests: 1,
		// Open after 5 consecutive failures within the interval.
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	})
	b.breakers[keyID] = cb
	return cb
}
