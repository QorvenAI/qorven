// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package briefing

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/autonomy"
)

// Scheduler registers per-agent daily briefing cron jobs.
type Scheduler struct {
	cron    *autonomy.CronScheduler
	builder *Builder
	pool    *pgxpool.Pool

	mu     sync.Mutex
	jobIDs map[string]string // agentID -> cron jobID
}

// NewScheduler creates a briefing scheduler.
func NewScheduler(cron *autonomy.CronScheduler, builder *Builder, pool *pgxpool.Pool) *Scheduler {
	return &Scheduler{
		cron:   cron,
		builder: builder,
		pool:   pool,
		jobIDs: make(map[string]string),
	}
}

// RegisterAgent adds or replaces a daily briefing cron job for an agent.
func (s *Scheduler) RegisterAgent(ctx context.Context, agentID, tenantID, briefingTime, briefingTimezone string) {
	s.mu.Lock()
	// Remove existing job for this agent if present.
	if existing, ok := s.jobIDs[agentID]; ok {
		_ = s.cron.Remove(existing) // ignore "not found" errors
		delete(s.jobIDs, agentID)
	}
	s.mu.Unlock()

	schedule := fmt.Sprintf("at:%s+%s", briefingTime, briefingTimezone)
	jobID, err := s.cron.Add(autonomy.CronJob{
		TenantID: tenantID,
		AgentID:  agentID,
		Name:     "Daily Briefing",
		Prompt:   "[BRIEFING]",
		Schedule: schedule,
		Timezone: briefingTimezone,
		Enabled:  true,
	})
	if err != nil {
		slog.Error("briefing.register_failed", "agent", agentID, "err", err)
		return
	}

	s.mu.Lock()
	s.jobIDs[agentID] = jobID
	s.mu.Unlock()

	slog.Info("briefing.scheduled", "agent", agentID, "job", jobID, "schedule", schedule)
}

// SyncAll loads all agents with briefing_enabled=true and registers their jobs.
func (s *Scheduler) SyncAll(ctx context.Context) error {
	rows, err := s.pool.Query(ctx,
		`SELECT agent_id, tenant_id::text, briefing_time, briefing_timezone
		 FROM inbound_agent_config WHERE briefing_enabled = true`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var agentID, tenantID, bt, btz string
		if err := rows.Scan(&agentID, &tenantID, &bt, &btz); err != nil {
			continue
		}
		s.RegisterAgent(ctx, agentID, tenantID, bt, btz)
	}
	return nil
}
