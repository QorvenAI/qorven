// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package orchestrator_test

import (
	"context"
	"testing"
	"time"

	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/byom"
	"github.com/qorvenai/qorven/internal/orchestrator"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/testutil"
)

// TestSweeperManager_SpawnsPerTenant asserts the manager discovers
// tenants with active work and stands up one sweeper per tenant — the
// Phase 4 replacement for the global TenantScope="*" loop. A single
// sweeper seeing every tenant's rows was Gap #1 from Step 3; this is
// the closing tripwire.
func TestSweeperManager_SpawnsPerTenant(t *testing.T) {
	pool, tenantA := testutil.NewIsolatedTenant(t)
	_, tenantB := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	// Seed one running plan in each tenant so both land in the
	// "active tenants" set.
	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)
	seedRunningPlan := func(tenant string) string {
		p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
			TenantID: tenant, Title: "mgr-" + testutil.TempID("p"),
		})
		if err != nil {
			t.Fatalf("CreatePlan: %v", err)
		}
		for _, st := range []plans.PlanStatus{
			plans.StatusPendingApproval, plans.StatusApproved, plans.StatusRunning,
		} {
			if _, err := ps.UpdatePlanStatus(ctx, p.ID, st); err != nil {
				t.Fatalf("→%s: %v", st, err)
			}
		}
		return p.ID
	}
	seedRunningPlan(tenantA)
	seedRunningPlan(tenantB)

	// Runner is a no-op — the manager test is about supervision, not
	// plan execution. The per-tenant Sweeper still invokes it; we
	// don't care how many times.
	runner := &callCountRunner{}
	svc := orchestrator.NewService(ps, as, runner, apievents.NewEmitter(), nil)

	mgr := orchestrator.NewSweeperManager(pool, svc, nil)
	if mgr == nil {
		t.Fatalf("NewSweeperManager returned nil")
	}
	mgr.TickInterval = 50 * time.Millisecond
	mgr.IdleTicksBeforeStop = 2

	seen, err := mgr.ReconcileNow(ctx)
	if err != nil {
		t.Fatalf("ReconcileNow: %v", err)
	}

	// The manager is shared across all tests (same pool) — other
	// active tenants leaking in from prior tests would be a test
	// hygiene issue. We only assert that OUR two tenants both appear.
	foundA, foundB := false, false
	for _, id := range seen {
		if id == tenantA {
			foundA = true
		}
		if id == tenantB {
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Fatalf("manager missed a tenant: foundA=%v foundB=%v seen=%v", foundA, foundB, seen)
	}

	// Each tenant MUST have its own sweeper — not one shared sweeper
	// walking both. The count assertion here is the "boundary" proof.
	if mgr.ActiveTenantCount() < 2 {
		t.Fatalf("expected at least 2 per-tenant sweepers, got %d", mgr.ActiveTenantCount())
	}
}

// TestSweeperManager_RetiresIdleTenants asserts that when a tenant
// goes idle (no active plans, no unconsumed wakeups), the manager
// spins its sweeper down after IdleTicksBeforeStop reconcile passes.
// Without this, every tenant that ever created a plan holds a
// goroutine forever — an obvious BYOM resource leak.
func TestSweeperManager_RetiresIdleTenants(t *testing.T) {
	pool, tenantA := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	// Create a plan, drive to running, then immediately terminalize it.
	// After termination, tenantA has no active work.
	p, _ := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantA, Title: "retire-" + testutil.TempID("p"),
	})
	for _, st := range []plans.PlanStatus{
		plans.StatusPendingApproval, plans.StatusApproved, plans.StatusRunning,
	} {
		if _, err := ps.UpdatePlanStatus(ctx, p.ID, st); err != nil {
			t.Fatalf("→%s: %v", st, err)
		}
	}
	// Manually spawn tenantA's sweeper by reconciling with active work
	// present:
	runner := &callCountRunner{}
	svc := orchestrator.NewService(ps, as, runner, apievents.NewEmitter(), nil)
	mgr := orchestrator.NewSweeperManager(pool, svc, nil)
	mgr.TickInterval = 25 * time.Millisecond
	mgr.IdleTicksBeforeStop = 2

	if _, err := mgr.ReconcileNow(ctx); err != nil {
		t.Fatalf("ReconcileNow 1: %v", err)
	}

	// Confirm sweeper is up for tenantA.
	hasA := false
	for _, id := range mgr.ActiveTenants() {
		if id == tenantA {
			hasA = true
		}
	}
	if !hasA {
		t.Fatalf("tenantA sweeper did not spawn on initial reconcile; have=%v", mgr.ActiveTenants())
	}

	// Now terminalize — tenantA is idle from this point.
	if _, err := ps.UpdatePlanStatus(ctx, p.ID, plans.StatusDone); err != nil {
		t.Fatalf("→done: %v", err)
	}

	// Reconcile IdleTicksBeforeStop times. After that many, the
	// sweeper should be retired.
	for i := 0; i < mgr.IdleTicksBeforeStop; i++ {
		if _, err := mgr.ReconcileNow(ctx); err != nil {
			t.Fatalf("ReconcileNow idle %d: %v", i, err)
		}
	}

	for _, id := range mgr.ActiveTenants() {
		if id == tenantA {
			t.Fatalf("tenantA sweeper still running after %d idle reconciles — goroutine leak",
				mgr.IdleTicksBeforeStop)
		}
	}
}

// TestSweeperManager_StartStop exercises the full Start/cancel path:
// the manager must cleanly return from Start when its context is
// cancelled, and every child sweeper goroutine must exit. Flaky or
// hanging shutdown is the worst class of bug for a long-running
// supervisor — catch it here.
func TestSweeperManager_StartStop(t *testing.T) {
	pool, tenantA := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	// One running plan so the manager has something to spawn.
	p, _ := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantA, Title: "startstop-" + testutil.TempID("p"),
	})
	for _, st := range []plans.PlanStatus{
		plans.StatusPendingApproval, plans.StatusApproved, plans.StatusRunning,
	} {
		if _, err := ps.UpdatePlanStatus(ctx, p.ID, st); err != nil {
			t.Fatalf("→%s: %v", st, err)
		}
	}

	runner := &callCountRunner{}
	svc := orchestrator.NewService(ps, as, runner, apievents.NewEmitter(), nil)
	mgr := orchestrator.NewSweeperManager(pool, svc, nil)
	mgr.TickInterval = 20 * time.Millisecond

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		mgr.Start(runCtx)
		close(done)
	}()

	// Wait for first reconcile to take effect.
	time.Sleep(60 * time.Millisecond)
	if mgr.ActiveTenantCount() < 1 {
		t.Fatalf("manager never spawned any sweepers")
	}

	cancel()
	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatalf("manager.Start did not return within 2s of context cancel — shutdown is hanging")
	}

	// Post-shutdown, every child sweeper must be gone.
	if mgr.ActiveTenantCount() != 0 {
		t.Fatalf("leaked sweepers after Start returned: count=%d", mgr.ActiveTenantCount())
	}
}

// TestSweeperManager_HonorsByomTick proves the manager's default tick
// is driven by byom.Load() — an operator tuning QORVEN_SWEEPER_TICK
// up for slow local inference gets a strictly-slower supervisor too,
// not a supervisor that pegs DB load while the workers it supervises
// are in 5-minute tool-calls.
func TestSweeperManager_HonorsByomTick(t *testing.T) {
	restore := byom.SetForTests(byom.Timeouts{
		SubmitHardTimeout: 30 * time.Minute,
		PermissionTimeout: 2 * time.Minute,
		StalePlanAfter:    30 * time.Minute,
		SweeperTick:       4 * time.Minute, // slow BYOM knob
		GraphMaxHops:      256,
	})
	defer restore()

	pool, _ := testutil.NewIsolatedTenant(t)
	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)
	runner := &callCountRunner{}
	svc := orchestrator.NewService(ps, as, runner, apievents.NewEmitter(), nil)

	mgr := orchestrator.NewSweeperManager(pool, svc, nil)
	// Manager tick should be strictly slower than the per-sweeper
	// tick. 4m × 2 = 8m.
	if mgr.TickInterval != 8*time.Minute {
		t.Fatalf("manager TickInterval=%v, want 8m (2×BYOM SweeperTick)", mgr.TickInterval)
	}
}
