// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qorvenai/qorven/internal/byom"
	"github.com/qorvenai/qorven/internal/plans"
)

// Sweeper is the startup-and-periodic recovery loop that rescues plans
// whose approve-to-execute goroutine died (e.g. gateway restart).
//
// Two recovery sources:
//
//  1. wakeup_requests rows with cause='approval_resolved' and
//     consumed_at IS NULL. Created by the approve handler; consumed
//     here by invoking ExecutePlan and flipping consumed_at.
//  2. plans rows stuck in status='running' whose nodes have no
//     running/pending state changes in the last StalePlanAfter window.
//     These are plans whose orchestrator goroutine died but the plan
//     status never got reset. We resume them too.
//
// The sweeper runs once at startup (Run) and, optionally, on a
// periodic tick (RunBackground). Calls to ExecutePlan are fanned out
// through a bounded worker pool so a large backlog cannot spike
// concurrent DB/agent usage.
type Sweeper struct {
	pool        *pgxpool.Pool
	service     *Service
	logger      *slog.Logger

	// Concurrency cap for ExecutePlan calls during a sweep. Default 4.
	MaxWorkers int

	// StalePlanAfter is the window beyond which a running plan with no
	// activity is considered orphaned. Default 10 minutes.
	StalePlanAfter time.Duration

	// TickInterval is how often RunBackground wakes up to look for new
	// work. Default 30 seconds.
	TickInterval time.Duration

	// TenantScope restricts the sweep to a single tenant id. Empty
	// means "all tenants" — the production default in single-tenant
	// mode. Required (non-empty) when RequireTenantScope is true.
	// (Phase 3 FU-030 / multi-tenant prep.)
	TenantScope string

	// RequireTenantScope makes empty TenantScope a Run-time error.
	// Set to true by the gateway bootstrap when deployment.Config
	// reports IsMultiTenant — a single global sweeper scanning all
	// tenants at once violates multi-tenant boundaries. In that mode
	// the caller must spawn one Sweeper per active tenant with its
	// id set, or use TenantScope="*" (explicit opt-in to global scan
	// by an admin tool).
	RequireTenantScope bool

	// MaxAttempts is the retry budget per wakeup_request row. Once a
	// row has been attempted this many times without success it is
	// dead-lettered: consumed_at is set and dead_letter_reason records
	// the last error. Default 10.
	MaxAttempts int
}

// NewSweeper constructs a Sweeper from the pool + running Service.
// Returns nil when either is nil — orchestration is optional at boot.
func NewSweeper(pool *pgxpool.Pool, svc *Service, logger *slog.Logger) *Sweeper {
	if pool == nil || svc == nil {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	t := byom.Load()
	return &Sweeper{
		pool:           pool,
		service:        svc,
		logger:         logger,
		MaxWorkers:     4,
		MaxAttempts:    10,
		StalePlanAfter: t.StalePlanAfter,
		TickInterval:   t.SweeperTick,
	}
}

// Run performs a single sweep pass. Returns a count of plans resumed
// and the first error encountered (if any); later errors are logged
// but do not short-circuit the pass — one bad plan must not prevent
// recovery of the others.
func (s *Sweeper) Run(ctx context.Context) (int, error) {
	if s == nil {
		return 0, nil
	}

	// Multi-tenant strictness: refuse to scan globally from a sweeper
	// that has not been scoped. The caller must spawn per-tenant
	// sweepers or explicitly opt in via TenantScope="*".
	if s.RequireTenantScope && s.TenantScope == "" {
		return 0, errors.New(
			"sweeper: TenantScope is required in multi-tenant mode; " +
				"spawn one Sweeper per tenant or set TenantScope=\"*\" for a global admin scan")
	}

	// Collect plan ids from both sources. Dedupe before dispatching.
	ids, err := s.collectCandidates(ctx)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}

	s.logger.Info("orchestrator.sweeper: resuming plans",
		"count", len(ids), "max_workers", s.MaxWorkers)

	workers := s.MaxWorkers
	if workers <= 0 {
		workers = 4
	}
	if workers > len(ids) {
		workers = len(ids)
	}

	jobs := make(chan string, len(ids))
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)

	var wg sync.WaitGroup
	var firstErrMu sync.Mutex
	var firstErr error
	resumed := 0
	var resumedMu sync.Mutex

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for planID := range jobs {
				if ctx.Err() != nil {
					return
				}
				if err := s.resumeOne(ctx, planID); err != nil {
					s.logger.Warn("sweeper: resume failed",
						"plan_id", planID, "worker", w, "err", err)
					firstErrMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					firstErrMu.Unlock()
					continue
				}
				resumedMu.Lock()
				resumed++
				resumedMu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	return resumed, firstErr
}

// RunBackground calls Run on startup and then on a ticker until ctx
// is cancelled. Errors are logged; the loop continues.
func (s *Sweeper) RunBackground(ctx context.Context) {
	if s == nil {
		return
	}
	interval := s.TickInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	// Initial sweep.
	if _, err := s.Run(ctx); err != nil {
		s.logger.Warn("sweeper: initial run had errors", "err", err)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := s.Run(ctx); err != nil {
				s.logger.Warn("sweeper: tick had errors", "err", err)
			}
		}
	}
}

// collectCandidates returns deduped plan ids from the two recovery
// sources. Order matters for test determinism: wakeup_requests first
// (explicit intent), then stale-running plans (implicit recovery).
//
// TenantScope restricts both queries to the configured tenant when
// set — used by isolated tests and as foundational multi-tenant
// scaffolding. Empty scope preserves the production default of
// scanning all tenants.
func (s *Sweeper) collectCandidates(ctx context.Context) ([]string, error) {
	seen := make(map[string]struct{})
	var ids []string

	// Source 1: unconsumed approval_resolved wakeup requests.
	// When TenantScope is set to a concrete id, we join plans via the
	// wakeup's plan_id and filter by plans.tenant_id. TenantScope="*"
	// is the explicit global-scan admin override — treat as unscoped.
	// Empty scope (single-tenant default) uses the cheap path.
	scopeTenant := s.TenantScope
	if scopeTenant == "*" {
		scopeTenant = ""
	}
	wakeupSQL := `
        SELECT plan_id::text FROM wakeup_requests
        WHERE cause = 'approval_resolved'
          AND consumed_at IS NULL
          AND dead_letter_reason IS NULL
          AND plan_id IS NOT NULL
        ORDER BY created_at ASC
        LIMIT 100`
	wakeupArgs := []any{}
	if scopeTenant != "" {
		wakeupSQL = `
            SELECT w.plan_id::text
              FROM wakeup_requests w
              JOIN plans p ON p.id = w.plan_id
             WHERE w.cause = 'approval_resolved'
               AND w.consumed_at IS NULL
               AND w.dead_letter_reason IS NULL
               AND w.plan_id IS NOT NULL
               AND p.tenant_id::text = $1
             ORDER BY w.created_at ASC
             LIMIT 100`
		wakeupArgs = append(wakeupArgs, scopeTenant)
	}
	rows, err := s.pool.Query(ctx, wakeupSQL, wakeupArgs...)
	if err != nil {
		return nil, fmt.Errorf("sweeper: query wakeups: %w", err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	// Source 2: running plans with no recent activity.
	cutoff := time.Now().Add(-s.StalePlanAfter)
	staleSQL := `
        SELECT id::text FROM plans
        WHERE status = 'running'
          AND updated_at < $1
        ORDER BY updated_at ASC
        LIMIT 100`
	staleArgs := []any{cutoff}
	if scopeTenant != "" {
		staleSQL = `
            SELECT id::text FROM plans
            WHERE status = 'running'
              AND updated_at < $1
              AND tenant_id::text = $2
            ORDER BY updated_at ASC
            LIMIT 100`
		staleArgs = append(staleArgs, scopeTenant)
	}
	rows, err = s.pool.Query(ctx, staleSQL, staleArgs...)
	if err != nil {
		return nil, fmt.Errorf("sweeper: query stale: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

// resumeOne resumes a single plan, increments the attempt counter on
// every try, and dead-letters rows that exceed MaxAttempts.
func (s *Sweeper) resumeOne(ctx context.Context, planID string) error {
	maxAttempts := s.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 10
	}

	// Increment attempts for every unconsumed wakeup pointing at this plan.
	// We do this before ExecutePlan so a crash mid-run still counts.
	// Two-step: UPDATE then SELECT MAX so we can aggregate.
	_, err := s.pool.Exec(ctx, `
        UPDATE wakeup_requests
           SET attempts = attempts + 1
         WHERE plan_id = $1::uuid
           AND consumed_at IS NULL
           AND dead_letter_reason IS NULL
    `, planID)
	if err != nil {
		s.logger.Warn("sweeper: increment attempts failed", "plan_id", planID, "err", err)
	}
	var maxSeen int
	if scanErr := s.pool.QueryRow(ctx, `
        SELECT COALESCE(MAX(attempts), 0) FROM wakeup_requests
         WHERE plan_id = $1::uuid
           AND consumed_at IS NULL
           AND dead_letter_reason IS NULL
    `, planID).Scan(&maxSeen); scanErr != nil {
		s.logger.Debug("sweeper: no active wakeups for plan", "plan_id", planID)
	}

	// If any row has hit the budget, dead-letter the whole group for this plan.
	if maxSeen >= maxAttempts {
		reason := fmt.Sprintf("exceeded %d attempts", maxAttempts)
		s.deadLetter(ctx, planID, reason)
		s.logger.Warn("sweeper: dead-lettered wakeups", "plan_id", planID, "attempts", maxSeen)
		return fmt.Errorf("sweeper: %s for plan %s", reason, planID)
	}

	// Skip plans that have already reached a terminal state between
	// collection and dispatch — prevents double-work and unnecessary
	// errors in the logs.
	p, err := s.service.plans.GetPlan(ctx, planID)
	if err != nil {
		if errors.Is(err, plans.ErrNotFound) {
			s.markConsumed(ctx, planID)
			return nil
		}
		return err
	}
	switch p.Status {
	case plans.StatusDone, plans.StatusFailed, plans.StatusCancelled, plans.StatusRejected:
		s.markConsumed(ctx, planID)
		return nil
	}

	// Bound each per-plan resume with a timeout so a wedged plan cannot
	// stall the whole sweep.
	planCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	// Enrich with the human actor who triggered this resume (FU-023).
	if s.service.plans != nil {
		if actor := s.latestApprovalActor(ctx, planID); actor != "" {
			planCtx = WithActor(planCtx, actor)
		}
	}
	if err := s.service.ExecutePlan(planCtx, planID); err != nil {
		s.logger.Warn("sweeper: ExecutePlan failed", "plan_id", planID, "err", err)
		// Do not mark consumed — let the next sweep retry (up to MaxAttempts).
		return err
	}
	s.markConsumed(ctx, planID)
	return nil
}

// latestApprovalActor returns the resolved_by value from the most
// recently resolved approval for the plan, or "" if none exists.
func (s *Sweeper) latestApprovalActor(ctx context.Context, planID string) string {
	var actor string
	_ = s.pool.QueryRow(ctx, `
        SELECT COALESCE(resolved_by, '') FROM approvals
         WHERE plan_id = $1::uuid
           AND state IN ('approved', 'revision_requested')
         ORDER BY updated_at DESC
         LIMIT 1
    `, planID).Scan(&actor)
	return actor
}

// markConsumed flips consumed_at for every unconsumed wakeup_request
// pointing at the plan.
func (s *Sweeper) markConsumed(ctx context.Context, planID string) {
	_, err := s.pool.Exec(ctx, `
        UPDATE wakeup_requests
           SET consumed_at = NOW()
         WHERE plan_id = $1::uuid
           AND consumed_at IS NULL
           AND dead_letter_reason IS NULL
    `, planID)
	if err != nil {
		s.logger.Warn("sweeper: mark consumed failed", "plan_id", planID, "err", err)
	}
}

// deadLetter marks rows as permanently failed: sets consumed_at + dead_letter_reason.
func (s *Sweeper) deadLetter(ctx context.Context, planID, reason string) {
	_, err := s.pool.Exec(ctx, `
        UPDATE wakeup_requests
           SET consumed_at = NOW(),
               dead_letter_reason = $2
         WHERE plan_id = $1::uuid
           AND consumed_at IS NULL
    `, planID, reason)
	if err != nil {
		s.logger.Warn("sweeper: dead-letter failed", "plan_id", planID, "err", err)
	}
}
