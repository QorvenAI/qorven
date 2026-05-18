// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package daemon

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// store is the DB-persistence layer for daemon plans and tasks.
// All methods are best-effort — errors are logged but never propagate
// to callers because the in-memory state is authoritative at runtime.
// The DB is used for reload-on-restart and audit; it is not the hot path.
type store struct {
	pool     *pgxpool.Pool
	tenantID string
}

func (s *store) savePlan(ctx context.Context, p *Plan) {
	if s == nil || s.pool == nil {
		return
	}
	tasksJSON, _ := json.Marshal(p.Tasks)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO daemon_plans (id, tenant_id, title, description, proposed_by, status, tasks, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
		ON CONFLICT (id) DO UPDATE SET
			status     = EXCLUDED.status,
			tasks      = EXCLUDED.tasks,
			updated_at = NOW()
	`, p.ID, s.tenantID, p.Title, p.Description, p.ProposedBy, string(p.Status), tasksJSON, p.CreatedAt)
	if err != nil {
		slog.Warn("daemon.store.save_plan", "id", p.ID, "err", err)
	}
}

func (s *store) updatePlanStatus(ctx context.Context, planID string, status PlanStatus, decidedBy, modifications, rejectReason string) {
	if s == nil || s.pool == nil {
		return
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE daemon_plans
		   SET status        = $2,
		       decided_by    = NULLIF($3, ''),
		       decided_at    = NOW(),
		       modifications = NULLIF($4, ''),
		       reject_reason = NULLIF($5, ''),
		       updated_at    = NOW()
		 WHERE id = $1
	`, planID, string(status), decidedBy, modifications, rejectReason)
	if err != nil {
		slog.Warn("daemon.store.update_plan_status", "id", planID, "err", err)
	}
}


func (s *store) saveTask(ctx context.Context, t *Task) {
	if s == nil || s.pool == nil {
		return
	}
	ctxJSON, _ := json.Marshal(t.Context)
	planID := ""
	if t.Context != nil {
		planID, _ = t.Context["plan_id"].(string)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO daemon_tasks (id, tenant_id, title, description, owner, priority, status,
		                          plan_id, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), $6, $7, NULLIF($8,''), $9, $10, $10)
		ON CONFLICT (id) DO NOTHING
	`, t.ID, s.tenantID, t.Title, t.Description, t.Owner, t.Priority, string(t.Status),
		planID, t.CreatedBy, t.CreatedAt)
	if err != nil {
		slog.Warn("daemon.store.save_task", "id", t.ID, "err", err)
	}
	_ = ctxJSON
}

func (s *store) updateTaskStatus(ctx context.Context, taskID string, status TaskStatus, summary, errMsg string, completedAt *time.Time) {
	if s == nil || s.pool == nil {
		return
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE daemon_tasks
		   SET status       = $2,
		       summary      = NULLIF($3, ''),
		       error        = NULLIF($4, ''),
		       completed_at = $5,
		       updated_at   = NOW()
		 WHERE id = $1
	`, taskID, string(status), summary, errMsg, completedAt)
	if err != nil {
		slog.Warn("daemon.store.update_task_status", "id", taskID, "err", err)
	}
}

// LoadState restores pending plans and unfinished tasks from the DB into
// the registry's in-memory maps. Called once at startup after SetPool.
func (reg *Registry) LoadState(ctx context.Context) {
	if reg.db == nil || reg.db.pool == nil {
		return
	}
	s := reg.db

	// Load non-terminal plans (pending | approved | executing).
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, description, proposed_by, status,
		       COALESCE(modifications,''), COALESCE(reject_reason,''),
		       COALESCE(tasks,'[]'::jsonb), created_at,
		       decided_at, COALESCE(decided_by,'')
		  FROM daemon_plans
		 WHERE tenant_id = $1
		   AND status IN ('pending','approved','executing')
		 ORDER BY created_at ASC
	`, s.tenantID)
	if err != nil {
		slog.Warn("daemon.store.load_plans", "err", err)
		return
	}
	defer rows.Close()

	var planIDs []string
	reg.mu.Lock()
	for rows.Next() {
		var p Plan
		var status, mods, reject, decidedBy string
		var tasksJSON []byte
		if err := rows.Scan(&p.ID, &p.Title, &p.Description, &p.ProposedBy,
			&status, &mods, &reject, &tasksJSON, &p.CreatedAt,
			&p.DecidedAt, &decidedBy); err != nil {
			continue
		}
		p.Status = PlanStatus(status)
		p.Modifications = mods
		p.DecidedBy = decidedBy
		if len(tasksJSON) > 0 {
			json.Unmarshal(tasksJSON, &p.Tasks) //nolint:errcheck
		}
		reg.plans[p.ID] = &p
		planIDs = append(planIDs, p.ID)
	}
	reg.mu.Unlock()

	if len(planIDs) == 0 {
		return
	}

	// Load plan tasks from daemon_plans_tasks virtual join: tasks with a plan_id
	// that is still active.
	taskRows, err := s.pool.Query(ctx, `
		SELECT id, title, description, COALESCE(owner,''), priority, status,
		       COALESCE(plan_id,''), created_by, created_at
		  FROM daemon_tasks
		 WHERE tenant_id = $1
		   AND status IN ('queued','in_progress')
		 ORDER BY created_at ASC
	`, s.tenantID)
	if err != nil {
		slog.Warn("daemon.store.load_tasks", "err", err)
		return
	}
	defer taskRows.Close()

	reg.mu.Lock()
	for taskRows.Next() {
		var t Task
		var status, planID string
		if err := taskRows.Scan(&t.ID, &t.Title, &t.Description, &t.Owner,
			&t.Priority, &status, &planID, &t.CreatedBy, &t.CreatedAt); err != nil {
			continue
		}
		t.Status = TaskStatus(status)
		if planID != "" {
			if t.Context == nil {
				t.Context = make(map[string]any)
			}
			t.Context["plan_id"] = planID
		}
		// Tasks restored from DB that were in_progress are re-queued since
		// the agent that held them is no longer connected.
		if t.Status == TaskInProgress {
			t.Status = TaskQueued
			t.Owner = ""
		}
		reg.tasks[t.ID] = &t
	}
	reg.mu.Unlock()

	slog.Info("daemon.store.loaded", "plans", len(planIDs), "tasks", len(reg.tasks))
}
