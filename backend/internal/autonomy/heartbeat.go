// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package autonomy

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// HeartbeatRunner periodically checks on agents and triggers autonomous work.
// The heartbeat is the "pulse" of an autonomous agent — it wakes up, checks
// for pending work, executes it, and goes back to sleep.
type HeartbeatRunner struct {
	agents   map[string]*HeartbeatAgent
	mu       sync.Mutex
	running  bool
	stopCh   chan struct{}
	interval time.Duration

	// OnHeartbeat is called for each agent on each tick.
	OnHeartbeat func(ctx context.Context, agentID string) error
}

// HeartbeatAgent tracks per-agent heartbeat state.
type HeartbeatAgent struct {
	AgentID       string
	Enabled       bool
	Interval      time.Duration // per-agent override (0 = use default)
	LastBeat      time.Time
	LastError     string
	ConsecutiveFails int
}

// NewHeartbeatRunner creates a runner with default 5-minute interval.
func NewHeartbeatRunner(onHeartbeat func(ctx context.Context, agentID string) error) *HeartbeatRunner {
	return &HeartbeatRunner{
		agents:      make(map[string]*HeartbeatAgent),
		stopCh:      make(chan struct{}),
		interval:    5 * time.Minute,
		OnHeartbeat: onHeartbeat,
	}
}

// Register adds an agent to the heartbeat schedule.
func (h *HeartbeatRunner) Register(agentID string, interval time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.agents[agentID] = &HeartbeatAgent{
		AgentID:  agentID,
		Enabled:  true,
		Interval: interval,
	}
}

// Unregister removes an agent from heartbeat.
func (h *HeartbeatRunner) Unregister(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.agents, agentID)
}

// Start begins the heartbeat loop.
func (h *HeartbeatRunner) Start() {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.mu.Unlock()

	go h.loop()
	slog.Info("heartbeat.started", "interval", h.interval)
}

// Stop halts the heartbeat.
func (h *HeartbeatRunner) Stop() {
	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return
	}
	h.running = false
	h.mu.Unlock()
	close(h.stopCh)
}

func (h *HeartbeatRunner) loop() {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopCh:
			return
		case now := <-ticker.C:
			h.beat(now)
		}
	}
}

func (h *HeartbeatRunner) beat(now time.Time) {
	h.mu.Lock()
	var due []string
	for id, agent := range h.agents {
		if !agent.Enabled {
			continue
		}
		interval := agent.Interval
		if interval <= 0 {
			interval = h.interval
		}
		if now.Sub(agent.LastBeat) >= interval {
			due = append(due, id)
		}
	}
	h.mu.Unlock()

	for _, agentID := range due {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		err := h.OnHeartbeat(ctx, agentID)
		cancel()

		h.mu.Lock()
		agent := h.agents[agentID]
		if agent != nil {
			agent.LastBeat = time.Now()
			if err != nil {
				agent.LastError = err.Error()
				agent.ConsecutiveFails++
				slog.Warn("heartbeat.failed", "agent", agentID, "fails", agent.ConsecutiveFails, "error", err)
				// Auto-disable after 5 consecutive failures
				if agent.ConsecutiveFails >= 5 {
					agent.Enabled = false
					slog.Error("heartbeat.disabled", "agent", agentID, "reason", "5 consecutive failures")
				}
			} else {
				agent.LastError = ""
				agent.ConsecutiveFails = 0
			}
		}
		h.mu.Unlock()
	}
}
