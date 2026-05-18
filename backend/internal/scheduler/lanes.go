// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scheduler

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
)

// Lane name constants.
const (
	LaneMain     = "main"
	LaneSubagent = "subagent"
	LaneTeam     = "team"
	LaneCron     = "cron"
)

// LaneConfig configures a single lane.
type LaneConfig struct {
	Name        string `json:"name"`
	Concurrency int    `json:"concurrency"`
}

// LaneStats is a snapshot of lane utilization.
type LaneStats struct {
	Name        string `json:"name"`
	Concurrency int    `json:"concurrency"`
	Active      int    `json:"active"`
	Pending     int    `json:"pending"`
}

// Lane is a named worker pool with bounded concurrency.
type Lane struct {
	name        string
	concurrency int
	sem         chan struct{}
	pending     atomic.Int64
	active      atomic.Int64
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewLane creates a lane with the given concurrency limit.
func NewLane(name string, concurrency int) *Lane {
	if concurrency <= 0 {
		concurrency = 2
	}
	ctx, cancel := context.WithCancel(context.Background())
	l := &Lane{
		name: name, concurrency: concurrency,
		sem: make(chan struct{}, concurrency),
		ctx: ctx, cancel: cancel,
	}
	for i := 0; i < concurrency; i++ {
		l.sem <- struct{}{}
	}
	return l
}

// Submit runs fn in the lane, blocking until a worker slot is available.
func (l *Lane) Submit(ctx context.Context, fn func()) error {
	l.pending.Add(1)
	defer l.pending.Add(-1)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.ctx.Done():
		return context.Canceled
	case token, ok := <-l.sem:
		if !ok {
			return context.Canceled
		}
		l.active.Add(1)
		l.wg.Add(1)
		go func() {
			defer func() {
				l.active.Add(-1)
				l.wg.Done()
				l.sem <- token
			}()
			fn()
		}()
		return nil
	}
}

// Stop drains the lane and waits for active work.
func (l *Lane) Stop() {
	l.cancel()
	l.wg.Wait()
}

// Stats returns lane utilization metrics.
func (l *Lane) Stats() LaneStats {
	return LaneStats{
		Name: l.name, Concurrency: l.concurrency,
		Active: int(l.active.Load()), Pending: int(l.pending.Load()),
	}
}

// LaneManager manages named lanes.
type LaneManager struct {
	lanes map[string]*Lane
	mu    sync.RWMutex
}

// NewLaneManager creates a lane manager with preconfigured lanes.
func NewLaneManager(configs []LaneConfig) *LaneManager {
	lm := &LaneManager{lanes: make(map[string]*Lane)}
	for _, cfg := range configs {
		lm.lanes[cfg.Name] = NewLane(cfg.Name, cfg.Concurrency)
		slog.Info("lane created", "name", cfg.Name, "concurrency", cfg.Concurrency)
	}
	return lm
}

// DefaultLanes returns the standard lane configuration.
func DefaultLanes() []LaneConfig {
	return []LaneConfig{
		{Name: LaneMain, Concurrency: laneEnv("QORVEN_LANE_MAIN", 30)},
		{Name: LaneSubagent, Concurrency: laneEnv("QORVEN_LANE_SUBAGENT", 50)},
		{Name: LaneTeam, Concurrency: laneEnv("QORVEN_LANE_TEAM", 100)},
		{Name: LaneCron, Concurrency: laneEnv("QORVEN_LANE_CRON", 30)},
	}
}

func laneEnv(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultVal
}

// Get returns a lane by name, falling back to main.
func (lm *LaneManager) Get(name string) *Lane {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	if lane, ok := lm.lanes[name]; ok {
		return lane
	}
	if lane, ok := lm.lanes[LaneMain]; ok {
		return lane
	}
	return nil
}

// StopAll stops all lanes.
func (lm *LaneManager) StopAll() {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	for name, lane := range lm.lanes {
		slog.Info("stopping lane", "name", name)
		lane.Stop()
	}
}

// AllStats returns utilization for all lanes.
func (lm *LaneManager) AllStats() []LaneStats {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	stats := make([]LaneStats, 0, len(lm.lanes))
	for _, lane := range lm.lanes {
		stats = append(stats, lane.Stats())
	}
	return stats
}
