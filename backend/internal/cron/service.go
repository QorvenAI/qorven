// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cron

import (
	"fmt"
	"log/slog"
	"sync"
)

// Service manages cron jobs with persistence, scheduling, and execution.
type Service struct {
	storePath string
	store     Store
	onJob     JobHandler
	running   bool
	stopChan  chan struct{}
	mu        sync.Mutex
	runLog    []RunLogEntry // in-memory run history (last 200 entries)
	retryCfg  RetryConfig
}

// NewService creates a new cron service.
func NewService(storePath string, onJob JobHandler) *Service {
	return &Service{storePath: storePath, store: Store{Version: 1}, onJob: onJob, retryCfg: DefaultRetryConfig()}
}

func (cs *Service) SetRetryConfig(cfg RetryConfig) { cs.mu.Lock(); cs.retryCfg = cfg; cs.mu.Unlock() }
func (cs *Service) SetOnJob(handler JobHandler)    { cs.mu.Lock(); cs.onJob = handler; cs.mu.Unlock() }

// Start loads persisted jobs and begins the scheduling loop.
func (cs *Service) Start() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.running {
		return nil
	}
	if err := cs.loadUnsafe(); err != nil {
		slog.Warn("cron: failed to load store, starting fresh", "error", err)
		cs.store = Store{Version: 1}
	}
	now := nowMS()
	for i := range cs.store.Jobs {
		job := &cs.store.Jobs[i]
		if job.Enabled && job.State.NextRunAtMS == nil {
			job.State.NextRunAtMS = cs.computeNextRun(&job.Schedule, now)
		}
	}
	cs.saveUnsafe()
	cs.stopChan = make(chan struct{})
	cs.running = true
	go cs.runLoop(cs.stopChan)
	slog.Info("cron service started", "jobs", len(cs.store.Jobs))
	return nil
}

// Stop halts the scheduling loop.
func (cs *Service) Stop() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if !cs.running {
		return
	}
	close(cs.stopChan)
	cs.running = false
	slog.Info("cron service stopped")
}

// AddJob creates and registers a new cron job.
func (cs *Service) AddJob(name string, schedule Schedule, message string, deliver bool, channel, to, agentID string) (*Job, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if err := cs.validateSchedule(&schedule); err != nil {
		return nil, fmt.Errorf("invalid schedule: %w", err)
	}
	now := nowMS()
	job := Job{
		ID: generateID(), Name: name, AgentID: agentID, Enabled: true, Schedule: schedule,
		Payload: Payload{Kind: "agent_turn", Message: message},
		Deliver: deliver, DeliverChannel: channel, DeliverTo: to,
		CreatedAtMS: now, UpdatedAtMS: now, DeleteAfterRun: schedule.Kind == "at",
	}
	job.State.NextRunAtMS = cs.computeNextRun(&job.Schedule, now)
	cs.store.Jobs = append(cs.store.Jobs, job)
	cs.saveUnsafe()
	slog.Info("cron job added", "id", job.ID, "name", name, "kind", schedule.Kind)
	return &job, nil
}

// RemoveJob deletes a job by ID.
func (cs *Service) RemoveJob(jobID string) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i, job := range cs.store.Jobs {
		if job.ID == jobID {
			cs.store.Jobs = append(cs.store.Jobs[:i], cs.store.Jobs[i+1:]...)
			cs.saveUnsafe()
			slog.Info("cron job removed", "id", jobID)
			return nil
		}
	}
	return fmt.Errorf("job %s not found", jobID)
}

// EnableJob toggles a job's enabled state.
func (cs *Service) EnableJob(jobID string, enabled bool) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i := range cs.store.Jobs {
		if cs.store.Jobs[i].ID == jobID {
			cs.store.Jobs[i].Enabled = enabled
			cs.store.Jobs[i].UpdatedAtMS = nowMS()
			if enabled {
				cs.store.Jobs[i].State.NextRunAtMS = cs.computeNextRun(&cs.store.Jobs[i].Schedule, nowMS())
			} else {
				cs.store.Jobs[i].State.NextRunAtMS = nil
			}
			cs.saveUnsafe()
			slog.Info("cron job toggled", "id", jobID, "enabled", enabled)
			return nil
		}
	}
	return fmt.Errorf("job %s not found", jobID)
}

// ListJobs returns all jobs, optionally including disabled ones.
func (cs *Service) ListJobs(includeDisabled bool) []Job {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	result := []Job{}
	for _, job := range cs.store.Jobs {
		if includeDisabled || job.Enabled {
			result = append(result, job)
		}
	}
	return result
}

// GetJob returns a job by ID.
func (cs *Service) GetJob(jobID string) (*Job, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i, job := range cs.store.Jobs {
		if job.ID == jobID {
			return &cs.store.Jobs[i], true
		}
	}
	return nil, false
}

// UpdateJob patches an existing job's fields.
func (cs *Service) UpdateJob(jobID string, patch JobPatch) (*Job, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i := range cs.store.Jobs {
		if cs.store.Jobs[i].ID != jobID {
			continue
		}
		job := &cs.store.Jobs[i]
		if patch.Name != "" {
			job.Name = patch.Name
		}
		if patch.AgentID != nil {
			job.AgentID = *patch.AgentID
		}
		if patch.Enabled != nil {
			job.Enabled = *patch.Enabled
		}
		if patch.Schedule != nil {
			if err := cs.validateSchedule(patch.Schedule); err != nil {
				return nil, fmt.Errorf("invalid schedule: %w", err)
			}
			job.Schedule = *patch.Schedule
		}
		if patch.Message != "" {
			job.Payload.Message = patch.Message
		}
		if patch.Deliver != nil {
			job.Deliver = *patch.Deliver
		}
		if patch.DeliverChannel != nil {
			job.DeliverChannel = *patch.DeliverChannel
		}
		if patch.DeliverTo != nil {
			job.DeliverTo = *patch.DeliverTo
		}
		if patch.WakeHeartbeat != nil {
			job.WakeHeartbeat = *patch.WakeHeartbeat
		}
		if patch.Stateless != nil {
			job.Stateless = *patch.Stateless
		}
		if patch.DeleteAfterRun != nil {
			job.DeleteAfterRun = *patch.DeleteAfterRun
		}
		job.UpdatedAtMS = nowMS()
		if job.Enabled {
			job.State.NextRunAtMS = cs.computeNextRun(&job.Schedule, nowMS())
		} else {
			job.State.NextRunAtMS = nil
		}
		cs.saveUnsafe()
		slog.Info("cron job updated", "id", jobID)
		result := cs.store.Jobs[i]
		return &result, nil
	}
	return nil, fmt.Errorf("job %s not found", jobID)
}

// Status returns the service status.
func (cs *Service) Status() map[string]any {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return map[string]any{"enabled": cs.running, "jobs": len(cs.store.Jobs), "nextWakeAtMs": cs.getNextWakeMS()}
}
