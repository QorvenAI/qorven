// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package orchestrator_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/orchestrator"
	"github.com/qorvenai/qorven/internal/orchestrator/handlers"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/testutil"
)

// TestSweeper_ResumesWakeupRow simulates the gateway-restart scenario:
// approval resolved, wakeup row written, but the goroutine crashed.
// A fresh Sweeper.Run must pick the plan back up and drive it to done.
//
// Phase 3 (FU-030): uses NewIsolatedTenant + TenantScope so the
// sweeper only sees this test's rows. Zero quiesce patterns.
func TestSweeper_ResumesWakeupRow(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "sweeper-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	planner, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindPlanner, Title: "plan",
		Inputs: handlers.PlannerInputs{Description: "todo app"},
	})
	approve, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindHumanFeedback, Title: "approve",
	})
	builder, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "build",
		AssigneeSoul: "dev",
		Inputs:       handlers.AgentTaskInputs{AgentID: "dev", Instruction: "do it"},
	})
	_, _ = ps.AddEdge(ctx, p.ID, planner.ID, approve.ID, plans.CondAlways)
	_, _ = ps.AddEdge(ctx, p.ID, approve.ID, builder.ID, plans.CondApproved)

	runner := &scriptedAgent{responses: map[string]string{
		"todo app": `{"stack":"x","summary":"s","agents":[]}`,
		"do it":    "ok",
	}}
	svc := orchestrator.NewService(ps, as, runner, apievents.NewEmitter(), nil)
	if err := svc.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("initial ExecutePlan: %v", err)
	}

	// Resolve the approval outside the sweeper to mimic the HTTP flow,
	// and write a wakeup_requests row exactly as the approve handler does.
	appr, _ := as.ListByPlan(ctx, p.ID)
	if len(appr) == 0 {
		t.Fatalf("no approval pending")
	}
	if _, err := as.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: appr[0].ID, Next: approvals.StateApproved, ResolvedBy: "reviewer",
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := pool.Exec(ctx, `
        INSERT INTO wakeup_requests (agent_id, tenant_id, source, actor_type, cause, payload, plan_id)
        VALUES ('orchestrator', $2, 'test', 'user', 'approval_resolved', '{"approval_id":"`+appr[0].ID+`"}'::jsonb, $1)
    `, p.ID, tenantID); err != nil {
		t.Fatalf("insert wakeup: %v", err)
	}

	// Simulate a restart: brand-new runner + service; only the DB
	// carries state.
	runner2 := &scriptedAgent{responses: runner.responses}
	svc2 := orchestrator.NewService(ps, as, runner2, apievents.NewEmitter(), nil)
	sw := orchestrator.NewSweeper(pool, svc2, nil)
	sw.TenantScope = tenantID

	resumed, err := sw.Run(ctx)
	if err != nil {
		t.Fatalf("sweeper.Run: %v", err)
	}
	if resumed != 1 {
		t.Fatalf("expected exactly 1 plan resumed in tenant %s, got %d", tenantID, resumed)
	}
	if runner2.calls.Load() == 0 {
		t.Fatalf("sweeper did not invoke agent for our plan")
	}

	final, _ := ps.GetPlan(ctx, p.ID)
	if final.Status != plans.StatusDone {
		t.Fatalf("final plan status: %s", final.Status)
	}
	var unconsumed int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM wakeup_requests WHERE plan_id = $1 AND consumed_at IS NULL`, p.ID).Scan(&unconsumed); err != nil {
		t.Fatalf("count wakeups: %v", err)
	}
	if unconsumed != 0 {
		t.Fatalf("expected 0 unconsumed wakeup rows, got %d", unconsumed)
	}
}

// TestSweeper_SkipsTerminalPlans confirms the sweeper does not
// re-execute a plan that's already done; stray wakeup rows are still
// marked consumed so the loop doesn't retry forever.
func TestSweeper_SkipsTerminalPlans(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "terminal-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	for _, s := range []plans.PlanStatus{
		plans.StatusPendingApproval,
		plans.StatusApproved,
		plans.StatusRunning,
		plans.StatusDone,
	} {
		if _, err := ps.UpdatePlanStatus(ctx, p.ID, s); err != nil {
			t.Fatalf("→%s: %v", s, err)
		}
	}
	if _, err := pool.Exec(ctx, `
        INSERT INTO wakeup_requests (agent_id, tenant_id, source, actor_type, cause, payload, plan_id)
        VALUES ('orchestrator', $2, 'test', 'user', 'approval_resolved', '{}'::jsonb, $1)
    `, p.ID, tenantID); err != nil {
		t.Fatalf("insert wakeup: %v", err)
	}

	runner := &scriptedAgent{responses: map[string]string{}}
	svc := orchestrator.NewService(ps, as, runner, apievents.NewEmitter(), nil)
	sw := orchestrator.NewSweeper(pool, svc, nil)
	sw.TenantScope = tenantID
	if _, err := sw.Run(ctx); err != nil {
		t.Fatalf("sweeper.Run: %v", err)
	}
	if runner.calls.Load() != 0 {
		t.Fatalf("terminal plan must not re-execute (calls=%d)", runner.calls.Load())
	}
	var unconsumed int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM wakeup_requests WHERE plan_id = $1 AND consumed_at IS NULL`, p.ID).Scan(&unconsumed)
	if unconsumed != 0 {
		t.Fatalf("terminal plan's wakeup must be consumed (unconsumed=%d)", unconsumed)
	}
}

// TestSweeper_BoundedConcurrency proves MaxWorkers caps concurrent
// ExecutePlan calls during a sweep. Uses NewIsolatedTenant so the
// sweeper's "unconsumed wakeups" and "stale plans" scans only see
// this test's rows — no cross-test pollution possible.
func TestSweeper_BoundedConcurrency(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	var in, peak, total atomic.Int32
	var peakMu sync.Mutex
	observePeak := func() {
		peakMu.Lock()
		if cur := in.Load(); cur > peak.Load() {
			peak.Store(cur)
		}
		peakMu.Unlock()
	}
	runner := &slowCountingAgent{
		onStart: func() { in.Add(1); observePeak() },
		onEnd:   func() { in.Add(-1); total.Add(1) },
		delay:   30 * time.Millisecond,
		reply:   `{"stack":"x"}`,
	}

	const n = 8
	for i := 0; i < n; i++ {
		p, _ := ps.CreatePlan(ctx, plans.CreatePlanInput{
			TenantID: tenantID, Title: "bounded-" + testutil.TempID("p"),
		})
		_, _ = ps.AppendNode(ctx, plans.AppendNodeInput{
			PlanID: p.ID, Kind: plans.KindPlanner, Title: "plan",
			Inputs: handlers.PlannerInputs{Description: "x"},
		})
		if _, err := pool.Exec(ctx, `
            INSERT INTO wakeup_requests (agent_id, tenant_id, source, actor_type, cause, payload, plan_id)
            VALUES ('orchestrator', $2, 'test', 'user', 'approval_resolved', '{}'::jsonb, $1)
        `, p.ID, tenantID); err != nil {
			t.Fatalf("insert wakeup: %v", err)
		}
	}

	svc := orchestrator.NewService(ps, as, runner, apievents.NewEmitter(), nil)
	sw := orchestrator.NewSweeper(pool, svc, nil)
	sw.MaxWorkers = 3
	sw.TenantScope = tenantID // P3-04: filter to this test's tenant only.

	if _, err := sw.Run(ctx); err != nil {
		t.Fatalf("sweeper.Run: %v", err)
	}
	if total.Load() != n {
		t.Fatalf("expected %d plan runs, got %d", n, total.Load())
	}
	if peak.Load() > 3 {
		t.Fatalf("concurrency cap violated: peak=%d max=3", peak.Load())
	}
}

// slowCountingAgent sleeps inside Run to let the bounded-concurrency
// test observe overlap.
type slowCountingAgent struct {
	onStart func()
	onEnd   func()
	delay   time.Duration
	reply   string
	calls   atomic.Int32
}

func (s *slowCountingAgent) Run(ctx context.Context, req agent.RunRequest, onEvent func(agent.StreamEvent)) (*agent.RunResult, error) {
	s.calls.Add(1)
	if s.onStart != nil {
		s.onStart()
	}
	defer func() {
		if s.onEnd != nil {
			s.onEnd()
		}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.delay):
	}
	onEvent(agent.StreamEvent{Type: "text_delta", Delta: s.reply})
	return &agent.RunResult{}, nil
}
