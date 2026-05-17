// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"strings"
	"time"

	"github.com/adhocore/gronx"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RunJob manually triggers a job execution.
func (cs *Service) RunJob(jobID string, force bool) (bool, string, error) {
	cs.mu.Lock()
	var job *Job
	for i := range cs.store.Jobs {
		if cs.store.Jobs[i].ID == jobID {
			j := cs.store.Jobs[i]
			job = &j
			break
		}
	}
	handler := cs.onJob
	cs.mu.Unlock()

	if job == nil {
		return false, "", fmt.Errorf("job %s not found", jobID)
	}
	if handler == nil {
		return false, "", fmt.Errorf("no job handler configured")
	}
	if !force && (job.State.NextRunAtMS == nil || *job.State.NextRunAtMS > nowMS()) {
		return false, "not-due", nil
	}

	slog.Info("cron manual run", "id", job.ID, "name", job.Name, "force", force)
	result, _, err := ExecuteWithRetry(func() (string, error) { return handler(job) }, cs.retryCfg)

	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i := range cs.store.Jobs {
		if cs.store.Jobs[i].ID != jobID {
			continue
		}
		now := nowMS()
		cs.store.Jobs[i].State.LastRunAtMS = &now
		if err != nil {
			cs.store.Jobs[i].State.LastStatus = "error"
			cs.store.Jobs[i].State.LastError = err.Error()
		} else {
			cs.store.Jobs[i].State.LastStatus = "ok"
			cs.store.Jobs[i].State.LastError = ""
		}
		if cs.store.Jobs[i].DeleteAfterRun {
			cs.store.Jobs = append(cs.store.Jobs[:i], cs.store.Jobs[i+1:]...)
		} else {
			cs.store.Jobs[i].State.NextRunAtMS = cs.computeNextRun(&cs.store.Jobs[i].Schedule, now)
		}
		cs.saveUnsafe()
		break
	}
	cs.recordRunLocked(jobID, err, result)
	if err != nil {
		return true, "", err
	}
	return true, result, nil
}

// GetRunLog returns recent run log entries.
func (cs *Service) GetRunLog(jobID string, limit int) []RunLogEntry {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	result := []RunLogEntry{}
	for i := len(cs.runLog) - 1; i >= 0 && len(result) < limit; i-- {
		entry := cs.runLog[i]
		if jobID == "" || entry.JobID == jobID {
			result = append(result, entry)
		}
	}
	return result
}

func (cs *Service) recordRunLocked(jobID string, err error, resultText string) {
	entry := RunLogEntry{Ts: nowMS(), JobID: jobID}
	if err != nil {
		entry.Status = "error"
		entry.Error = err.Error()
	} else {
		entry.Status = "ok"
		entry.Summary = TruncateOutput(resultText)
	}
	cs.runLog = append(cs.runLog, entry)
	if len(cs.runLog) > 200 {
		cs.runLog = cs.runLog[len(cs.runLog)-200:]
	}
}

// --- Internal scheduling loop ---

func (cs *Service) runLoop(stopChan chan struct{}) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			cs.checkJobs()
		}
	}
}

func (cs *Service) checkJobs() {
	cs.mu.Lock()
	now := nowMS()
	dueJobIDs := []string{}
	for i := range cs.store.Jobs {
		job := &cs.store.Jobs[i]
		if job.Enabled && job.State.NextRunAtMS != nil && *job.State.NextRunAtMS <= now {
			dueJobIDs = append(dueJobIDs, job.ID)
		}
	}
	if len(dueJobIDs) == 0 {
		cs.mu.Unlock()
		return
	}
	dueMap := make(map[string]bool, len(dueJobIDs))
	for _, id := range dueJobIDs {
		dueMap[id] = true
	}
	for i := range cs.store.Jobs {
		if dueMap[cs.store.Jobs[i].ID] {
			cs.store.Jobs[i].State.NextRunAtMS = nil
		}
	}
	cs.saveUnsafe()
	cs.mu.Unlock()

	var wg sync.WaitGroup
	for _, jobID := range dueJobIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("cron job panic", "id", id, "panic", r)
				}
			}()
			cs.executeJobByID(id)
		}(jobID)
	}
	wg.Wait()
}

func (cs *Service) executeJobByID(jobID string) {
	cs.mu.Lock()
	var job *Job
	for i := range cs.store.Jobs {
		if cs.store.Jobs[i].ID == jobID {
			j := cs.store.Jobs[i]
			job = &j
			break
		}
	}
	handler := cs.onJob
	cs.mu.Unlock()

	if job == nil || handler == nil {
		return
	}

	slog.Info("cron executing job", "id", job.ID, "name", job.Name)
	result, attempts, err := ExecuteWithRetry(func() (string, error) { return handler(job) }, cs.retryCfg)
	if attempts > 1 {
		slog.Info("cron job retried", "id", job.ID, "attempts", attempts, "success", err == nil)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i := range cs.store.Jobs {
		if cs.store.Jobs[i].ID != jobID {
			continue
		}
		now := nowMS()
		cs.store.Jobs[i].State.LastRunAtMS = &now
		if err != nil {
			cs.store.Jobs[i].State.LastStatus = "error"
			cs.store.Jobs[i].State.LastError = err.Error()
			slog.Error("cron job failed", "id", jobID, "error", err)
		} else {
			cs.store.Jobs[i].State.LastStatus = "ok"
			cs.store.Jobs[i].State.LastError = ""
			slog.Info("cron job completed", "id", jobID)
		}
		if cs.store.Jobs[i].DeleteAfterRun {
			cs.store.Jobs = append(cs.store.Jobs[:i], cs.store.Jobs[i+1:]...)
		} else {
			next := cs.computeNextRun(&cs.store.Jobs[i].Schedule, now)
			cs.store.Jobs[i].State.NextRunAtMS = next
			if next == nil {
				cs.store.Jobs[i].Enabled = false
			}
		}
		break
	}
	cs.recordRunLocked(jobID, err, result)
	cs.saveUnsafe()
}

// --- Schedule computation ---

func (cs *Service) computeNextRun(schedule *Schedule, now int64) *int64 {
	switch schedule.Kind {
	case "at":
		if schedule.AtMS != nil && *schedule.AtMS > now {
			return schedule.AtMS
		}
		return nil
	case "every":
		if schedule.EveryMS == nil || *schedule.EveryMS <= 0 {
			return nil
		}
		next := now + *schedule.EveryMS
		return &next
	case "cron":
		if schedule.Expr == "" {
			return nil
		}
		nowTime := time.UnixMilli(now)
		if schedule.TZ != "" {
			if loc, err := time.LoadLocation(schedule.TZ); err == nil {
				nowTime = nowTime.In(loc)
			}
		}
		nextTime, err := gronx.NextTickAfter(schedule.Expr, nowTime, false)
		if err != nil {
			slog.Error("cron: failed to compute next run", "expr", schedule.Expr, "error", err)
			return nil
		}
		nextMS := nextTime.UnixMilli()
		return &nextMS
	default:
		return nil
	}
}

func (cs *Service) validateSchedule(schedule *Schedule) error {
	switch schedule.Kind {
	case "at":
		if schedule.AtMS == nil {
			return fmt.Errorf("at schedule requires atMs")
		}
	case "every":
		if schedule.EveryMS == nil || *schedule.EveryMS <= 0 {
			return fmt.Errorf("every schedule requires positive everyMs")
		}
	case "cron":
		if schedule.Expr == "" {
			return fmt.Errorf("cron schedule requires expr")
		}
		if !gronx.New().IsValid(schedule.Expr) {
			return fmt.Errorf("invalid cron expression: %s", schedule.Expr)
		}
		if schedule.TZ != "" {
			if _, err := time.LoadLocation(schedule.TZ); err != nil {
				return fmt.Errorf("invalid timezone: %s", schedule.TZ)
			}
		}
	default:
		return fmt.Errorf("unknown schedule kind: %s", schedule.Kind)
	}
	return nil
}

func (cs *Service) getNextWakeMS() *int64 {
	var earliest *int64
	for _, job := range cs.store.Jobs {
		if job.Enabled && job.State.NextRunAtMS != nil {
			if earliest == nil || *job.State.NextRunAtMS < *earliest {
				earliest = job.State.NextRunAtMS
			}
		}
	}
	return earliest
}

// --- Persistence ---

func (cs *Service) loadUnsafe() error {
	data, err := os.ReadFile(cs.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &cs.store)
}

func (cs *Service) saveUnsafe() error {
	dir := filepath.Dir(cs.storePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cs.store, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write — cron store holds scheduled-job metadata; a torn
	// save after crash would re-queue jobs or silently drop schedules
	// on next boot.
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(cs.storePath)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, werr := tmp.Write(data); werr != nil {
		tmp.Close(); os.Remove(tmpPath); return werr
	}
	if serr := tmp.Sync(); serr != nil {
		tmp.Close(); os.Remove(tmpPath); return serr
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpPath); return cerr
	}
	if rerr := os.Rename(tmpPath, cs.storePath); rerr != nil {
		os.Remove(tmpPath); return rerr
	}
	return nil
}

// --- DB-backed Runner (for gateway integration) ---

// DBRunHandler is the callback for DB-backed cron execution.
type DBRunHandler func(ctx context.Context, jobName, payload, agentID string)

// Runner is a DB-backed cron runner for gateway integration.
type Runner struct {
	pool    *pgxpool.Pool
	handler DBRunHandler
	cancel  context.CancelFunc
}

// NewRunner creates a DB-backed cron runner.
func NewRunner(pool *pgxpool.Pool, handler DBRunHandler) *Runner {
	return &Runner{pool: pool, handler: handler}
}

// Start begins the cron runner loop.
func (r *Runner) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				r.tick(ctx)
			}
		}
	}()
	slog.Info("cron runner started", "interval", "30s")
}

// Stop halts the cron runner.
func (r *Runner) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *Runner) tick(ctx context.Context) {
	rows, err := r.pool.Query(ctx, `SELECT id, agent_id, name, payload, cron_expression, next_run_at
		FROM cron_jobs WHERE enabled = true AND next_run_at IS NOT NULL AND next_run_at <= NOW()
		ORDER BY next_run_at LIMIT 10`)
	if err != nil {
		return
	}
	defer rows.Close()

	type dueJob struct {
		id, agentID, name, payload, expr string
		nextRun                          time.Time
	}
	jobs := []dueJob{}
	for rows.Next() {
		var j dueJob
		if err := rows.Scan(&j.id, &j.agentID, &j.name, &j.payload, &j.expr, &j.nextRun); err != nil {
			continue
		}
		jobs = append(jobs, j)
	}
	rows.Close()

	for _, j := range jobs {
		// Detect missed runs (next_run is more than 2 intervals behind)
		missedDur := time.Since(j.nextRun)
		if missedDur > 2*time.Hour {
			slog.Warn("cron.missed_runs", "job", j.name, "missed_duration", missedDur.Round(time.Minute))
		}

		go func(job dueJob) {
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("cron runner panic", "job", job.name, "panic", rec)
				}
			}()

			slog.Info("cron.execute", "job", job.name, "agent", job.agentID)
			r.handler(ctx, job.name, job.payload, job.agentID)

			// Compute next run from cron expression, not fixed interval
			nextRun := computeNextRunFromExpr(job.expr)
			r.pool.Exec(ctx,
				`UPDATE cron_jobs SET last_run_at=NOW(), last_status='ok', run_count=COALESCE(run_count,0)+1, next_run_at=$1 WHERE id=$2`,
				nextRun, job.id)
		}(j)
	}
}

// NextRunFromExpr parses a cron expression and returns the next run time.
func NextRunFromExpr(expr string) time.Time {
	return computeNextRunFromExpr(expr)
}

// computeNextRunFromExpr parses a cron expression and returns the next run time.
func computeNextRunFromExpr(expr string) time.Time {
	now := time.Now()
	parts := strings.Fields(expr)
	if len(parts) < 5 {
		return now.Add(time.Hour) // fallback
	}

	// Handle common patterns directly
	switch {
	case expr == "* * * * *":
		return now.Add(time.Minute).Truncate(time.Minute)
	case parts[0] == "0" && parts[1] == "*":
		return now.Add(time.Hour).Truncate(time.Hour)
	case parts[0] == "0" && parts[1] == "0":
		next := now.AddDate(0, 0, 1)
		return time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, now.Location())
	}

	// Parse minute and hour fields for simple schedules
	minute := parseField(parts[0], 0, 59)
	hour := parseField(parts[1], 0, 23)

	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func parseField(field string, min, max int) int {
	if field == "*" { return min }
	parts := strings.Split(field, "/")
	v := 0
	fmt.Sscanf(parts[0], "%d", &v)
	if v < min { v = min }
	if v > max { v = max }
	return v
}

// FormatNextRun formats a next run time for display.
func FormatNextRun(t *time.Time) string {
	if t == nil {
		return "not scheduled"
	}
	return t.Format(time.RFC3339)
}

// FormatNextRunExpr formats a cron expression's next run for display.
func FormatNextRunExpr(expr string) string {
	if expr == "" {
		return "not scheduled"
	}
	nextTime, err := gronx.NextTickAfter(expr, time.Now(), false)
	if err != nil {
		return "invalid expression"
	}
	return nextTime.Format(time.RFC3339)
}

// RestartJob restarts a cron job by ID (recomputes next run).
func (r *Runner) RestartJob(ctx context.Context, jobID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE cron_jobs SET next_run_at = NOW() + interval '1 minute' WHERE id = $1`, jobID)
	return err
}
