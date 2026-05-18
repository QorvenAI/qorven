// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scheduler

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
)

// RunFunc executes an agent run. Called by the scheduler when it's the request's turn.
type RunFunc func(ctx context.Context, req agent.RunRequest) (*agent.RunResult, error)

// TokenEstimateFunc returns token estimate and context window for a session.
type TokenEstimateFunc func(sessionKey string) (tokens int, contextWindow int)

// RunOutcome is the result of a scheduled agent run.
type RunOutcome struct {
	Result *agent.RunResult
	Err    error
}

// QueueConfig configures per-session message queuing.
type QueueConfig struct {
	Cap           int `json:"cap"`
	MaxConcurrent int `json:"max_concurrent"`
}

// DefaultQueueConfig returns sensible defaults.
func DefaultQueueConfig() QueueConfig {
	return QueueConfig{Cap: 10, MaxConcurrent: 1}
}

// PendingRequest is a queued agent run awaiting execution.
type PendingRequest struct {
	Req        agent.RunRequest
	ResultCh   chan RunOutcome
	EnqueuedAt time.Time
}

// SessionQueue manages agent runs for a single session key.
type SessionQueue struct {
	key           string
	config        QueueConfig
	runFn         RunFunc
	laneMgr       *LaneManager
	lane          string
	mu            sync.Mutex
	queue         []*PendingRequest
	activeCount   int
	maxConcurrent int
	tokenEstimateFn TokenEstimateFunc
}

// NewSessionQueue creates a queue for a specific session.
func NewSessionQueue(key, lane string, cfg QueueConfig, laneMgr *LaneManager, runFn RunFunc) *SessionQueue {
	maxC := cfg.MaxConcurrent
	if maxC <= 0 {
		maxC = 1
	}
	return &SessionQueue{
		key: key, config: cfg, runFn: runFn,
		laneMgr: laneMgr, lane: lane, maxConcurrent: maxC,
	}
}

// Enqueue adds a run request to the session queue.
func (sq *SessionQueue) Enqueue(ctx context.Context, req agent.RunRequest) <-chan RunOutcome {
	ch := make(chan RunOutcome, 1)
	sq.mu.Lock()

	// Drop oldest if queue full
	if len(sq.queue) >= sq.config.Cap {
		old := sq.queue[0]
		sq.queue = sq.queue[1:]
		old.ResultCh <- RunOutcome{Err: ErrQueueDropped}
		close(old.ResultCh)
	}

	pending := &PendingRequest{Req: req, ResultCh: ch, EnqueuedAt: time.Now()}
	sq.queue = append(sq.queue, pending)
	sq.mu.Unlock()

	sq.tryStart(ctx)
	return ch
}

// tryStart starts queued runs if capacity allows.
func (sq *SessionQueue) tryStart(ctx context.Context) {
	sq.mu.Lock()
	maxC := sq.maxConcurrent
	if sq.tokenEstimateFn != nil {
		tokens, cw := sq.tokenEstimateFn(sq.key)
		if cw > 0 && float64(tokens)/float64(cw) >= 0.6 {
			maxC = 1 // near context limit → serialize
		}
	}

	for sq.activeCount < maxC && len(sq.queue) > 0 {
		pending := sq.queue[0]
		sq.queue = sq.queue[1:]
		sq.activeCount++
		sq.mu.Unlock()

		lane := sq.laneMgr.Get(sq.lane)
		if lane == nil {
			pending.ResultCh <- RunOutcome{Err: ErrLaneCleared}
			close(pending.ResultCh)
			sq.mu.Lock()
			sq.activeCount--
			continue
		}

		p := pending
		lane.Submit(ctx, func() {
			result, err := sq.runFn(ctx, p.Req)
			p.ResultCh <- RunOutcome{Result: result, Err: err}
			close(p.ResultCh)

			sq.mu.Lock()
			sq.activeCount--
			sq.mu.Unlock()
			sq.tryStart(ctx) // start next queued
		})

		sq.mu.Lock()
	}
	sq.mu.Unlock()
}

// IsActive returns true if any run is executing.
func (sq *SessionQueue) IsActive() bool {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	return sq.activeCount > 0
}

// CancelAll drains the queue.
func (sq *SessionQueue) CancelAll() bool {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	had := len(sq.queue) > 0
	for _, p := range sq.queue {
		p.ResultCh <- RunOutcome{Err: ErrLaneCleared}
		close(p.ResultCh)
	}
	sq.queue = nil
	return had
}

// Scheduler is the top-level coordinator managing lanes and session queues.
type Scheduler struct {
	lanes           *LaneManager
	sessions        map[string]*SessionQueue
	config          QueueConfig
	runFn           RunFunc
	mu              sync.RWMutex
	draining        atomic.Bool
	tokenEstimateFn TokenEstimateFunc
}

// NewScheduler creates a scheduler with the given lane and queue config.
func NewScheduler(laneConfigs []LaneConfig, queueCfg QueueConfig, runFn RunFunc) *Scheduler {
	if laneConfigs == nil {
		laneConfigs = DefaultLanes()
	}
	return &Scheduler{
		lanes: NewLaneManager(laneConfigs), sessions: make(map[string]*SessionQueue),
		config: queueCfg, runFn: runFn,
	}
}

// SetTokenEstimateFunc sets the callback for adaptive throttle.
func (s *Scheduler) SetTokenEstimateFunc(fn TokenEstimateFunc) {
	s.tokenEstimateFn = fn
}

// MarkDraining rejects new requests during shutdown.
func (s *Scheduler) MarkDraining() {
	s.draining.Store(true)
	slog.Info("scheduler: draining, new requests rejected")
}

// Schedule submits a run request to the appropriate session queue.
func (s *Scheduler) Schedule(ctx context.Context, lane string, req agent.RunRequest) <-chan RunOutcome {
	if s.draining.Load() {
		ch := make(chan RunOutcome, 1)
		ch <- RunOutcome{Err: ErrGatewayDraining}
		close(ch)
		return ch
	}
	sq := s.getOrCreateSession(req.SessionKey, lane)
	return sq.Enqueue(ctx, req)
}

func (s *Scheduler) getOrCreateSession(sessionKey, lane string) *SessionQueue {
	s.mu.RLock()
	sq, ok := s.sessions[sessionKey]
	s.mu.RUnlock()
	if ok {
		return sq
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if sq, ok := s.sessions[sessionKey]; ok {
		return sq
	}
	sq = NewSessionQueue(sessionKey, lane, s.config, s.lanes, s.runFn)
	if s.tokenEstimateFn != nil {
		sq.tokenEstimateFn = s.tokenEstimateFn
	}
	s.sessions[sessionKey] = sq
	return sq
}

// CancelSession cancels all pending runs for a session.
func (s *Scheduler) CancelSession(sessionKey string) bool {
	s.mu.RLock()
	sq, ok := s.sessions[sessionKey]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	return sq.CancelAll()
}

// Stop shuts down all lanes.
func (s *Scheduler) Stop() {
	s.MarkDraining()
	s.lanes.StopAll()
}

// HasActiveSessionsForAgent checks if any session for the agent has active runs.
func (s *Scheduler) HasActiveSessionsForAgent(agentID string) bool {
	prefix := "agent:" + agentID + ":"
	s.mu.RLock()
	defer s.mu.RUnlock()
	for key, sq := range s.sessions {
		if strings.HasPrefix(key, prefix) && sq.IsActive() {
			return true
		}
	}
	return false
}

// LaneStats returns utilization metrics for all lanes.
func (s *Scheduler) LaneStats() []LaneStats { return s.lanes.AllStats() }
