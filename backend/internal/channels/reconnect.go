// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"
)

// Reconnector manages auto-reconnect with exponential backoff for any channel.
type Reconnector struct {
	mu          sync.Mutex
	attempts    int
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	connectFn   func(ctx context.Context) error
	stopCh      chan struct{}
	running     bool
}

func NewReconnector(maxAttempts int, connectFn func(ctx context.Context) error) *Reconnector {
	return &Reconnector{
		maxAttempts: maxAttempts,
		baseDelay:   2 * time.Second,
		maxDelay:    60 * time.Second,
		connectFn:   connectFn,
		stopCh:      make(chan struct{}),
	}
}

// OnDisconnect starts the reconnection loop in background.
func (r *Reconnector) OnDisconnect(channelName string) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.attempts = 0
	r.mu.Unlock()

	go func() {
		for {
			r.mu.Lock()
			r.attempts++
			attempt := r.attempts
			if attempt > r.maxAttempts {
				r.running = false
				r.mu.Unlock()
				slog.Error("channel.reconnect.max_attempts", "channel", channelName, "attempts", attempt)
				return
			}
			r.mu.Unlock()

			delay := time.Duration(math.Min(
				float64(r.baseDelay)*math.Pow(2, float64(attempt-1)),
				float64(r.maxDelay),
			))
			slog.Info("channel.reconnect", "channel", channelName, "attempt", attempt, "delay", delay)

			select {
			case <-time.After(delay):
			case <-r.stopCh:
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := r.connectFn(ctx)
			cancel()

			if err == nil {
				r.mu.Lock()
				r.running = false
				r.attempts = 0
				r.mu.Unlock()
				slog.Info("channel.reconnected", "channel", channelName, "after_attempts", attempt)
				return
			}
			slog.Warn("channel.reconnect.failed", "channel", channelName, "attempt", attempt, "error", err)
		}
	}()
}

func (r *Reconnector) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		close(r.stopCh)
		r.running = false
	}
}
