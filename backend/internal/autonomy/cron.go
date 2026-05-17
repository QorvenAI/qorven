// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package autonomy

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/adhocore/gronx"
	"github.com/google/uuid"
)

// CronJob defines a scheduled recurring task for an agent.
type CronJob struct {
	ID          string    `json:"id"`
	TenantID    string    `json:"tenant_id"`
	AgentID     string    `json:"agent_id"`
	Name        string    `json:"name"`
	Prompt      string    `json:"prompt"`
	Schedule    string    `json:"schedule"` // cron expression or interval
	Timezone    string    `json:"timezone"`
	Enabled     bool      `json:"enabled"`
	NextRunAt   time.Time `json:"next_run_at"`
	LastRunAt   *time.Time `json:"last_run_at,omitempty"`
	RunCount    int       `json:"run_count"`
	DeliverTo   string    `json:"deliver_to,omitempty"` // "origin", "telegram:123", etc.
	CreatedByAgent string `json:"created_by_agent,omitempty"` // agents can create routines
}

// CronRunResult is the outcome of a cron execution.
type CronRunResult struct {
	JobID     string
	AgentID   string
	Output    string
	Success   bool
	Duration  time.Duration
	Tokens    int
}

// CronScheduler manages scheduled agent tasks.
type CronScheduler struct {
	jobs    map[string]*CronJob
	mu      sync.Mutex
	running bool
	stopCh  chan struct{}

	// OnRun is called when a job should execute. Injected by the application.
	OnRun func(ctx context.Context, job *CronJob) (*CronRunResult, error)
}

// NewCronScheduler creates a scheduler.
func NewCronScheduler(onRun func(ctx context.Context, job *CronJob) (*CronRunResult, error)) *CronScheduler {
	return &CronScheduler{
		jobs:   make(map[string]*CronJob),
		stopCh: make(chan struct{}),
		OnRun:  onRun,
	}
}

// Add registers a new cron job.
func (s *CronScheduler) Add(job CronJob) (string, error) {
	if job.Prompt == "" {
		return "", fmt.Errorf("prompt required")
	}
	if job.AgentID == "" {
		return "", fmt.Errorf("agent_id required")
	}
	if len(job.Prompt) > 10000 {
		return "", fmt.Errorf("prompt exceeds 10,000 char limit")
	}

	if job.ID == "" {
		job.ID = uuid.New().String()[:12]
	}
	job.Enabled = true
	job.NextRunAt = s.computeNextRun(job.Schedule, job.Timezone)

	s.mu.Lock()
	s.jobs[job.ID] = &job
	s.mu.Unlock()

	slog.Info("cron.added", "id", job.ID, "name", job.Name, "agent", job.AgentID, "next", job.NextRunAt)
	return job.ID, nil
}

// Remove deletes a cron job.
func (s *CronScheduler) Remove(jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[jobID]; !ok {
		return fmt.Errorf("job %s not found", jobID)
	}
	delete(s.jobs, jobID)
	return nil
}

// List returns all jobs, optionally filtered.
func (s *CronScheduler) List(includeDisabled bool) []*CronJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := []*CronJob{}
	for _, j := range s.jobs {
		if !includeDisabled && !j.Enabled {
			continue
		}
		result = append(result, j)
	}
	return result
}

// Start begins the scheduler tick loop.
func (s *CronScheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	go s.tickLoop()
	slog.Info("cron.started")
}

// Stop halts the scheduler.
func (s *CronScheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()
	close(s.stopCh)
	slog.Info("cron.stopped")
}

func (s *CronScheduler) tickLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.tick(now)
		}
	}
}

func (s *CronScheduler) tick(now time.Time) {
	s.mu.Lock()
	due := []*CronJob{}
	for _, job := range s.jobs {
		if job.Enabled && now.After(job.NextRunAt) {
			due = append(due, job)
		}
	}
	s.mu.Unlock()

	for _, job := range due {
		go s.executeJob(job)
	}
}

func (s *CronScheduler) executeJob(job *CronJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	slog.Info("cron.executing", "id", job.ID, "name", job.Name, "agent", job.AgentID)

	start := time.Now()
	result, err := s.OnRun(ctx, job)

	s.mu.Lock()
	now := time.Now()
	job.LastRunAt = &now
	job.RunCount++
	job.NextRunAt = s.computeNextRun(job.Schedule, job.Timezone)
	s.mu.Unlock()

	if err != nil {
		slog.Error("cron.failed", "id", job.ID, "error", err, "duration", time.Since(start))
		return
	}

	slog.Info("cron.completed", "id", job.ID, "duration", time.Since(start),
		"output_len", len(result.Output), "tokens", result.Tokens)
}

// computeNextRun parses schedule and returns next run time.
// Delegates to computeNextRunFrom with the current wall time.
func (s *CronScheduler) computeNextRun(schedule, timezone string) time.Time {
	return s.computeNextRunFrom(schedule, timezone, time.Now())
}

// computeNextRunFrom parses schedule and returns next run time relative to now.
// Supports:
//   - "every:30m", "every:1h", "every:1d" — fixed intervals
//   - "at:HH:MM+Zone" — once per day at a wall-clock time, e.g. "at:08:00+Asia/Shanghai"
func (s *CronScheduler) computeNextRunFrom(schedule, timezone string, now time.Time) time.Time {
	// "at:HH:MM+Zone" — once per day at wall-clock time
	if strings.HasPrefix(schedule, "at:") {
		rest := schedule[3:]
		zonePart := timezone
		timePart := rest
		if idx := strings.Index(rest, "+"); idx > 0 {
			timePart = rest[:idx]
			zonePart = rest[idx+1:]
		}
		var hour, min int
		if n, _ := fmt.Sscanf(timePart, "%d:%d", &hour, &min); n == 2 {
			loc := time.UTC
			if zonePart != "" {
				if l, err := time.LoadLocation(zonePart); err == nil {
					loc = l
				}
			}
			nowLocal := now.In(loc)
			target := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), hour, min, 0, 0, loc)
			if !target.After(now) {
				target = target.Add(24 * time.Hour)
			}
			return target
		}
	}

	// Parse interval format: "every:Xm", "every:Xh", "every:Xd"
	if strings.HasPrefix(schedule, "every:") {
		interval := schedule[6:]
		if d, err := time.ParseDuration(interval); err == nil {
			return now.Add(d)
		}
		// Handle "Xd" (days) — not supported by ParseDuration
		if len(interval) > 1 && interval[len(interval)-1] == 'd' {
			if days := parseInt(interval[:len(interval)-1]); days > 0 {
				return now.Add(time.Duration(days) * 24 * time.Hour)
			}
		}
	}

	// Standard cron expression (5 or 6 fields) — use gronx to compute next run
	if gronx.IsValid(schedule) {
		loc := time.UTC
		if timezone != "" {
			if l, err := time.LoadLocation(timezone); err == nil {
				loc = l
			}
		}
		start := now.In(loc).Truncate(time.Minute)
		if t, err := gronx.NextTickAfter(schedule, start, false); err == nil {
			return t
		}
	}

	// Default: 1 hour from now
	return now.Add(1 * time.Hour)
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			return 0
		}
	}
	return n
}
