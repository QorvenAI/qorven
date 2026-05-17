// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// BackgroundProcess represents a long-running task that notifies the agent on completion.
// Start a build, test suite, deployment, or training run — the agent gets notified
// when it finishes without polling.
type BackgroundProcess struct {
	ID        string
	AgentID   string
	SessionID string
	Name      string
	Command   string
	StartedAt time.Time
	Status    string // "running", "completed", "failed"
	Result    string
	ExitCode  int
	Duration  time.Duration
}

// ProcessNotifier manages background processes and delivers completion notifications.
type ProcessNotifier struct {
	mu        sync.Mutex
	processes map[string]*BackgroundProcess
	onNotify  func(agentID, sessionID, message string) // callback to deliver notification
}

func NewProcessNotifier(onNotify func(agentID, sessionID, message string)) *ProcessNotifier {
	return &ProcessNotifier{
		processes: make(map[string]*BackgroundProcess),
		onNotify:  onNotify,
	}
}

// Start registers a background process and runs it asynchronously.
// The provided runFn executes the actual work; on completion, the agent is notified.
func (pn *ProcessNotifier) Start(ctx context.Context, agentID, sessionID, name string, runFn func(ctx context.Context) (string, error)) string {
	id := fmt.Sprintf("bg_%s_%d", agentID[:8], time.Now().UnixMilli())

	proc := &BackgroundProcess{
		ID: id, AgentID: agentID, SessionID: sessionID,
		Name: name, StartedAt: time.Now(), Status: "running",
	}

	pn.mu.Lock()
	pn.processes[id] = proc
	pn.mu.Unlock()

	go func() {
		result, err := runFn(ctx)
		proc.Duration = time.Since(proc.StartedAt)

		pn.mu.Lock()
		if err != nil {
			proc.Status = "failed"
			proc.Result = err.Error()
			proc.ExitCode = 1
		} else {
			proc.Status = "completed"
			proc.Result = result
		}
		pn.mu.Unlock()

		// Notify the agent
		msg := fmt.Sprintf("🔔 Background task **%s** %s (took %s)", name, proc.Status, proc.Duration.Round(time.Second))
		if proc.Result != "" {
			preview := proc.Result
			if len(preview) > 500 { preview = preview[:500] + "..." }
			msg += "\n\n```\n" + preview + "\n```"
		}

		if pn.onNotify != nil {
			pn.onNotify(agentID, sessionID, msg)
		}
		slog.Info("background.complete", "id", id, "name", name, "status", proc.Status, "duration", proc.Duration)
	}()

	return id
}

// Get returns a background process by ID.
func (pn *ProcessNotifier) Get(id string) (*BackgroundProcess, bool) {
	pn.mu.Lock()
	defer pn.mu.Unlock()
	p, ok := pn.processes[id]
	return p, ok
}

// List returns all processes for an agent.
func (pn *ProcessNotifier) List(agentID string) []*BackgroundProcess {
	pn.mu.Lock()
	defer pn.mu.Unlock()
	var out []*BackgroundProcess
	for _, p := range pn.processes {
		if p.AgentID == agentID { out = append(out, p) }
	}
	return out
}

// Cleanup removes completed processes older than maxAge.
func (pn *ProcessNotifier) Cleanup(maxAge time.Duration) int {
	pn.mu.Lock()
	defer pn.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for id, p := range pn.processes {
		if p.Status != "running" && p.StartedAt.Before(cutoff) {
			delete(pn.processes, id)
			removed++
		}
	}
	return removed
}
