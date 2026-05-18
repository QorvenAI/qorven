// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/memory"
)

// Cortex is the agent's background inner monologue.
// It sees across all sessions, generates memory bulletins,
// runs decay, and detects patterns.
type Cortex struct {
	agentID  string
	tenantID string
	memStore *memory.Store
	interval time.Duration
	stop     chan struct{}
}

func NewCortex(agentID, tenantID string, memStore *memory.Store, interval time.Duration) *Cortex {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &Cortex{
		agentID:  agentID,
		tenantID: tenantID,
		memStore: memStore,
		interval: interval,
		stop:     make(chan struct{}),
	}
}

// Start begins the cortex background loop.
func (c *Cortex) Start(ctx context.Context) {
	go c.run(ctx)
	slog.Info("cortex started", "agent", c.agentID, "interval", c.interval)
}

// Stop signals the cortex to shut down.
func (c *Cortex) Stop() {
	close(c.stop)
}

func (c *Cortex) run(ctx context.Context) {
	// Generate initial bulletin on startup
	c.tick(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.tick(ctx)
		case <-c.stop:
			slog.Info("cortex stopped", "agent", c.agentID)
			return
		case <-ctx.Done():
			return
		}
	}
}

func (c *Cortex) tick(ctx context.Context) {
	if c.memStore == nil {
		return
	}

	start := time.Now()

	// 1. Generate memory bulletin
	bulletin, err := c.memStore.GenerateBulletin(ctx, c.tenantID, c.agentID)
	if err != nil {
		slog.Warn("cortex: bulletin generation failed", "agent", c.agentID, "error", err)
	} else if bulletin != "" {
		slog.Debug("cortex: bulletin generated", "agent", c.agentID, "length", len(bulletin))
	}

	// 2. Run memory decay (reduce importance of old unused memories)
	decayed, err := c.memStore.Decay(ctx, c.agentID)
	if err != nil {
		slog.Warn("cortex: decay failed", "agent", c.agentID, "error", err)
	} else if decayed > 0 {
		slog.Info("cortex: memories decayed", "agent", c.agentID, "count", decayed)
	}

	dur := time.Since(start)
	slog.Debug("cortex: tick complete", "agent", c.agentID, "duration_ms", dur.Milliseconds())
}
