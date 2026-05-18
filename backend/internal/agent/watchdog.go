// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"log/slog"
	"time"
)

// Watchdog monitors agent health and auto-recovers on failure.
type Watchdog struct {
	store    *Store
	interval time.Duration
	onRestart func(agentID string)
}

func NewWatchdog(store *Store, interval time.Duration, onRestart func(string)) *Watchdog {
	return &Watchdog{store: store, interval: interval, onRestart: onRestart}
}

// Start begins the watchdog loop. Checks all agents with role=supervisor.
func (w *Watchdog) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.check(ctx)
		}
	}
}

func (w *Watchdog) check(ctx context.Context) {
	if w.store == nil { return }
	agents, err := w.store.List(ctx, "")
	if err != nil { return }
	for _, a := range agents {
		if a.Role != nil && *a.Role == "supervisor" && a.Status == "crashed" {
			slog.Warn("watchdog.restart", "agent", a.ID[:8], "name", a.DisplayName)
			w.store.pool.Exec(ctx, `UPDATE agents SET status = 'active' WHERE id = $1`, a.ID)
			if w.onRestart != nil { w.onRestart(a.ID) }
		}
	}
}

// RetryDB wraps a DB operation with retry logic for transient errors.
func RetryDB(ctx context.Context, maxRetries int, fn func() error) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil { return nil }
		if i < maxRetries-1 {
			slog.Warn("db.retry", "attempt", i+1, "error", err)
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
		}
	}
	return err
}
