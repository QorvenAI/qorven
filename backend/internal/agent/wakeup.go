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

	"github.com/google/uuid"
)

// WakeupRequest triggers an agent to start working.
// Agents don't poll — they're woken up by requests from humans, other agents, or the system.
type WakeupRequest struct {
	ID        string       `json:"id"`
	AgentID   string       `json:"agent_id"`
	Source    WakeupSource `json:"source"`
	ActorType ActorType    `json:"actor_type"`
	ActorID   string       `json:"actor_id,omitempty"`
	Reason    string       `json:"reason,omitempty"`
	Context   map[string]any `json:"context,omitempty"` // issueId, prompt, taskId, etc.
	Priority  int          `json:"priority"`
	Status    WakeupStatus `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
}

// WakeupSource identifies what triggered the wakeup.
type WakeupSource string

const (
	WakeupAssignment     WakeupSource = "assignment"      // issue/task assigned
	WakeupRoutine        WakeupSource = "routine"         // scheduled routine fired
	WakeupManual         WakeupSource = "manual"          // human triggered directly
	WakeupAgent          WakeupSource = "agent"           // another agent triggered
	WakeupHeartbeat      WakeupSource = "heartbeat"       // periodic self-check
	WakeupWebhook        WakeupSource = "webhook"         // external webhook
	WakeupChannelMessage WakeupSource = "channel_message" // inbound channel msg (Telegram/email/Slack)
	WakeupSynthesis      WakeupSource = "synthesis"       // all subtasks of a parent completed
)

// ActorType identifies who triggered the wakeup.
type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorAgent  ActorType = "agent"
	ActorSystem ActorType = "system"
)

// WakeupStatus tracks the request lifecycle.
type WakeupStatus string

const (
	WakeupPending   WakeupStatus = "pending"
	WakeupRunning   WakeupStatus = "running"
	WakeupCompleted WakeupStatus = "completed"
	WakeupFailed    WakeupStatus = "failed"
)

// WakeupQueue manages pending wakeup requests per agent.
type WakeupQueue struct {
	requests map[string][]*WakeupRequest // agentID → pending requests
	mu       sync.Mutex
	onWakeup func(req *WakeupRequest) // callback when agent should wake
}

// NewWakeupQueue creates a queue with a wakeup handler.
func NewWakeupQueue(onWakeup func(req *WakeupRequest)) *WakeupQueue {
	return &WakeupQueue{
		requests: make(map[string][]*WakeupRequest),
		onWakeup: onWakeup,
	}
}

// Enqueue adds a wakeup request. Skips if agent has no actionable work.
func (q *WakeupQueue) Enqueue(ctx context.Context, req WakeupRequest) (string, error) {
	if req.AgentID == "" {
		return "", fmt.Errorf("agent_id required")
	}

	req.ID = uuid.New().String()
	req.Status = WakeupPending
	req.CreatedAt = time.Now()

	q.mu.Lock()
	q.requests[req.AgentID] = append(q.requests[req.AgentID], &req)
	q.mu.Unlock()

	slog.Info("wakeup.enqueued",
		"id", req.ID, "agent", req.AgentID,
		"source", req.Source, "actor_type", req.ActorType,
		"actor_id", req.ActorID, "reason", req.Reason)

	// Trigger the wakeup handler
	if q.onWakeup != nil {
		go q.onWakeup(&req)
	}

	return req.ID, nil
}

// WakeAgent is a convenience for the common case: wake an agent for an assigned task.
func (q *WakeupQueue) WakeAgent(ctx context.Context, agentID string, opts WakeupOpts) error {
	_, err := q.Enqueue(ctx, WakeupRequest{
		AgentID:   agentID,
		Source:    opts.Source,
		ActorType: opts.ActorType,
		ActorID:   opts.ActorID,
		Reason:    opts.Reason,
		Context:   opts.Context,
		Priority:  opts.Priority,
	})
	return err
}

// WakeupOpts are options for WakeAgent.
type WakeupOpts struct {
	Source    WakeupSource
	ActorType ActorType
	ActorID   string
	Reason    string
	Context   map[string]any
	Priority  int
}

// Pending returns pending requests for an agent, sorted by priority.
func (q *WakeupQueue) Pending(agentID string) []*WakeupRequest {
	q.mu.Lock()
	defer q.mu.Unlock()

	pending := q.requests[agentID]
	var result []*WakeupRequest
	for _, r := range pending {
		if r.Status == WakeupPending {
			result = append(result, r)
		}
	}
	return result
}

// MarkRunning marks a request as being processed.
func (q *WakeupQueue) MarkRunning(requestID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, reqs := range q.requests {
		for _, r := range reqs {
			if r.ID == requestID {
				r.Status = WakeupRunning
				return
			}
		}
	}
}

// MarkDone marks a request as completed or failed.
func (q *WakeupQueue) MarkDone(requestID string, failed bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, reqs := range q.requests {
		for _, r := range reqs {
			if r.ID == requestID {
				if failed {
					r.Status = WakeupFailed
				} else {
					r.Status = WakeupCompleted
				}
				return
			}
		}
	}
}

// Cleanup removes completed/failed requests older than maxAge.
func (q *WakeupQueue) Cleanup(maxAge time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for agentID, reqs := range q.requests {
		var kept []*WakeupRequest
		for _, r := range reqs {
			if (r.Status == WakeupCompleted || r.Status == WakeupFailed) && r.CreatedAt.Before(cutoff) {
				continue
			}
			kept = append(kept, r)
		}
		q.requests[agentID] = kept
	}
}
