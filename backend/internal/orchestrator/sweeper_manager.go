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
)

// SweeperManager owns a set of per-tenant Sweeper goroutines. It exists
// to close Gap #1 from Phase 3 Step 3: the multi-tenant gateway used
// to run one Sweeper with TenantScope="*" which walks every tenant's
// rows in one pass — a boundary violation on its face.
//
// The manager reconciles on a tick. On every tick it:
//
//  1. Queries the DB for "tenants with active plans or fresh wakeups."
//     A tenant is "active" when it has either (a) a plan in a
//     non-terminal status, OR (b) an unconsumed wakeup_request.
//  2. For each active tenant that has no running sweeper, it starts
//     one (TenantScope=tenantID, RequireTenantScope=true).
//  3. For each sweeper whose tenant has been idle for
//     IdleTicksBeforeStop consecutive ticks, it cancels the sweeper
//     and removes it. This prevents goroutine leak on long-lived
//     managers when tenants come and go.
//
// Intentionally simple: no Raft, no Redis locks. A single-Postgres
// deployment with one gateway binary (the BYOM default) reconciles
// idempotently — running two managers against the same DB is fine
// because each per-tenant sweeper's queries are idempotent and the
// ExecutePlan path is goroutine-safe. Operators that scale beyond one
// binary already need to think about who owns recovery; that's a
// Phase 5 problem.
//
// Lifecycle: call Start(ctx) once at gateway boot. The manager runs
// until ctx is cancelled. All child sweepers are stopped on shutdown.
type SweeperManager struct {
	pool    *pgxpool.Pool
	service *Service
	logger  *slog.Logger

	// TickInterval is how often the manager reconciles its set of
	// per-tenant sweepers. Defaults to byom.Load().SweeperTick * 2 —
	// the reconcile loop is strictly slower than the sweepers it
	// supervises, because it's the supervisor, not the worker.
	TickInterval time.Duration

	// IdleTicksBeforeStop is how many consecutive reconcile ticks a
	// tenant may be absent from the "active" set before we retire its
	// sweeper goroutine. Default 3 — enough to absorb a momentary
	// lull during approval flight without thrashing.
	IdleTicksBeforeStop int

	// MaxWorkersPerSweeper caps worker-pool size for each spawned
	// Sweeper. Default 4 (same as per-tenant sweeper default).
	MaxWorkersPerSweeper int

	mu       sync.Mutex
	sweepers map[string]*managedSweeper // tenant_id → running sweeper
}

// managedSweeper bundles a running Sweeper with its cancel handle and
// idle-tick counter.
type managedSweeper struct {
	sweeper   *Sweeper
	cancel    context.CancelFunc
	done      chan struct{}
	idleTicks int
}

// NewSweeperManager constructs a manager from the pool + Service. nil
// inputs return nil — orchestration is optional at boot and the manager
// is composed into the gateway via a nil-friendly pattern.
func NewSweeperManager(pool *pgxpool.Pool, svc *Service, logger *slog.Logger) *SweeperManager {
	if pool == nil || svc == nil {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	t := byom.Load()
	return &SweeperManager{
		pool:                 pool,
		service:              svc,
		logger:               logger,
		TickInterval:         t.SweeperTick * 2,
		IdleTicksBeforeStop:  3,
		MaxWorkersPerSweeper: 4,
		sweepers:             make(map[string]*managedSweeper),
	}
}

// Start runs the reconcile loop until ctx is cancelled. Safe to call
// once; calling twice is a bug.
func (m *SweeperManager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	interval := m.TickInterval
	if interval <= 0 {
		interval = 60 * time.Second
	}
	// Initial reconcile before the ticker fires — a gateway that just
	// booted should pick up existing active tenants immediately, not
	// after one interval of silence.
	m.reconcile(ctx)

	t := time.NewTicker(interval)
	defer t.Stop()
	defer m.stopAll()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.reconcile(ctx)
		}
	}
}

// reconcile performs one pass: discover active tenants, spawn missing
// sweepers, retire idle ones.
func (m *SweeperManager) reconcile(ctx context.Context) {
	active, err := m.activeTenants(ctx)
	if err != nil {
		m.logger.Warn("sweeper_manager: active-tenant query failed", "err", err)
		return
	}

	activeSet := make(map[string]struct{}, len(active))
	for _, id := range active {
		activeSet[id] = struct{}{}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Spawn missing.
	for tenantID := range activeSet {
		if _, running := m.sweepers[tenantID]; running {
			// Reset idle counter — tenant is active again.
			m.sweepers[tenantID].idleTicks = 0
			continue
		}
		m.spawnLocked(ctx, tenantID)
	}

	// Retire idle.
	for tenantID, ms := range m.sweepers {
		if _, stillActive := activeSet[tenantID]; stillActive {
			continue
		}
		ms.idleTicks++
		if ms.idleTicks >= m.IdleTicksBeforeStop {
			m.logger.Info("sweeper_manager: retiring idle tenant sweeper",
				"tenant_id", tenantID, "idle_ticks", ms.idleTicks)
			ms.cancel()
			<-ms.done
			delete(m.sweepers, tenantID)
		}
	}
}

// spawnLocked starts a per-tenant Sweeper. Caller holds m.mu.
func (m *SweeperManager) spawnLocked(parentCtx context.Context, tenantID string) {
	sweeper := NewSweeper(m.pool, m.service, m.logger.With("tenant_id", tenantID))
	if sweeper == nil {
		return
	}
	sweeper.TenantScope = tenantID
	sweeper.RequireTenantScope = true
	sweeper.MaxWorkers = m.MaxWorkersPerSweeper

	childCtx, cancel := context.WithCancel(parentCtx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		sweeper.RunBackground(childCtx)
	}()
	m.sweepers[tenantID] = &managedSweeper{
		sweeper: sweeper,
		cancel:  cancel,
		done:    done,
	}
	m.logger.Info("sweeper_manager: spawned per-tenant sweeper",
		"tenant_id", tenantID,
		"tick", sweeper.TickInterval,
		"stale_after", sweeper.StalePlanAfter,
	)
}

// stopAll cancels every child sweeper. Called from Start's defer on
// shutdown.
func (m *SweeperManager) stopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for tenantID, ms := range m.sweepers {
		ms.cancel()
		<-ms.done
		delete(m.sweepers, tenantID)
	}
}

// ActiveTenantCount returns the number of currently-running per-tenant
// sweepers. Exposed for tests and the /v1/orchestrator/status endpoint.
func (m *SweeperManager) ActiveTenantCount() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sweepers)
}

// ActiveTenants returns a sorted (by map iteration — not guaranteed)
// slice of tenant ids currently being swept. Used only in tests.
func (m *SweeperManager) ActiveTenants() []string {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.sweepers))
	for id := range m.sweepers {
		out = append(out, id)
	}
	return out
}

// ReconcileNow forces an immediate reconciliation pass, bypassing the
// ticker. For tests and admin endpoints that want deterministic
// progression. Returns the set of active tenants seen by this pass.
func (m *SweeperManager) ReconcileNow(ctx context.Context) ([]string, error) {
	if m == nil {
		return nil, errors.New("sweeper_manager: nil manager")
	}
	m.reconcile(ctx)
	return m.ActiveTenants(), nil
}

// activeTenants returns the set of tenant ids that currently have
// work a sweeper could recover: either a non-terminal plan, or an
// unconsumed wakeup_request. The UNION + DISTINCT is a single
// round-trip — cheaper than two queries plus de-dup.
func (m *SweeperManager) activeTenants(ctx context.Context) ([]string, error) {
	const q = `
        SELECT DISTINCT tenant_id::text FROM (
            SELECT tenant_id FROM plans
             WHERE status NOT IN ('done','failed','cancelled','rejected')
            UNION
            SELECT p.tenant_id
              FROM wakeup_requests w
              JOIN plans p ON p.id = w.plan_id
             WHERE w.consumed_at IS NULL
        ) s
        WHERE tenant_id IS NOT NULL
    `
	rows, err := m.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("sweeper_manager: active tenants: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
