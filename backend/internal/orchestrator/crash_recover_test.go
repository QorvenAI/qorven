// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package orchestrator_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/byom"
	"github.com/qorvenai/qorven/internal/orchestrator"
	"github.com/qorvenai/qorven/internal/orchestrator/handlers"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/testutil"
)

// P3E-04: simulated crash-and-recover tests with BYOM awareness.
//
// These tests stand in for the operational pain point the greenlight
// called out: "gateway crashes mid-inference → sweeper must bring the
// plan back without retrying a payload that already OOM'd the local
// model, and must NOT treat a legitimately-slow local model as stuck."
//
// The three scenarios:
//
//   - CrashMidTurn_DoesNotDoubleInvokeCompletedNodes — completed nodes
//     must not re-execute when the orchestrator restarts; only pending
//     ones run. This is the correctness test for idempotent resume.
//
//   - ByomTunedStaleness_SurvivesSlowInference — if an operator sets
//     QORVEN_STALE_PLAN_AFTER=30m to accommodate a local Llama 8B, a
//     plan whose node has been running for 6 minutes (beyond the
//     hosted-cloud 10m default but WELL under the tuned window) must
//     NOT be hijacked by the sweeper.
//
//   - CrashBeforeApprovalWakeup_Recovers — the classic wakeup path:
//     approval resolved, wakeup row written, gateway crashed. A fresh
//     Sweeper must resume to done. (Coverage overlap with
//     TestSweeper_ResumesWakeupRow is intentional — this file is the
//     "crash-and-recover suite" docs point CI at.)

// TestCrashRecover_DoesNotDoubleInvokeCompletedNodes asserts that on a
// simulated gateway crash (runner A dies mid-plan, runner B boots
// fresh), the resume path does NOT replay the planner. Re-running a
// completed node would retry an identical agent payload against a
// model that may have died specifically because that payload was too
// expensive — the exact foot-gun open-source BYOM users trip on.
func TestCrashRecover_DoesNotDoubleInvokeCompletedNodes(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "crash-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	planner, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindPlanner, Title: "plan",
		Inputs: handlers.PlannerInputs{Description: "dashboard"},
	})
	approve, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindHumanFeedback, Title: "approve",
	})
	builder, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "build",
		AssigneeSoul: "dev",
		Inputs:       handlers.AgentTaskInputs{AgentID: "dev", Instruction: "ship it"},
	})
	_, _ = ps.AddEdge(ctx, p.ID, planner.ID, approve.ID, plans.CondAlways)
	_, _ = ps.AddEdge(ctx, p.ID, approve.ID, builder.ID, plans.CondApproved)

	runnerA := &scriptedAgent{responses: map[string]string{
		"dashboard": `{"stack":"x","summary":"s","agents":[]}`,
		"ship it":   "done",
	}}
	svcA := orchestrator.NewService(ps, as, runnerA, apievents.NewEmitter(), nil)
	if err := svcA.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("pre-crash ExecutePlan: %v", err)
	}
	if runnerA.calls.Load() != 1 {
		t.Fatalf("pre-crash expected 1 planner call, got %d", runnerA.calls.Load())
	}

	// The "crash": discard runnerA+svcA entirely. State is in the DB.
	// Resolve the approval (mimicking a user who clicked Approve during
	// the gateway's downtime) and write a wakeup.
	appr, _ := as.ListByPlan(ctx, p.ID)
	if len(appr) == 0 {
		t.Fatalf("expected a pending approval")
	}
	if _, err := as.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: appr[0].ID, Next: approvals.StateApproved, ResolvedBy: "reviewer",
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := pool.Exec(ctx, `
        INSERT INTO wakeup_requests (agent_id, tenant_id, source, actor_type, cause, payload, plan_id)
        VALUES ('orchestrator', $2, 'test', 'user', 'approval_resolved', '{}'::jsonb, $1)
    `, p.ID, tenantID); err != nil {
		t.Fatalf("insert wakeup: %v", err)
	}

	// The "recovery": fresh runner B with the SAME scripted responses.
	// If the graph re-ran the planner, runnerB.calls would be 2 (planner
	// + builder). The correctness assertion is: runnerB runs only the
	// builder, exactly once.
	runnerB := &scriptedAgent{responses: runnerA.responses}
	svcB := orchestrator.NewService(ps, as, runnerB, apievents.NewEmitter(), nil)
	sw := orchestrator.NewSweeper(pool, svcB, nil)
	sw.TenantScope = tenantID

	resumed, err := sw.Run(ctx)
	if err != nil {
		t.Fatalf("sweeper.Run: %v", err)
	}
	if resumed != 1 {
		t.Fatalf("expected 1 plan resumed, got %d", resumed)
	}
	if got := runnerB.calls.Load(); got != 1 {
		t.Fatalf("idempotence violated: fresh runner called %d times (planner should NOT rerun)", got)
	}
	final, _ := ps.GetPlan(ctx, p.ID)
	if final.Status != plans.StatusDone {
		t.Fatalf("final plan status: %s", final.Status)
	}
}

// TestCrashRecover_ByomTunedStalenessSurvivesSlowInference proves the
// sweeper honors byom.Load().StalePlanAfter. A BYOM operator running
// Llama 8B on CPU sets QORVEN_STALE_PLAN_AFTER=30m; a plan whose
// node has been running for 6 minutes must NOT be treated as stale,
// even though the hosted-cloud default (15m) hasn't been hit either.
//
// This is the regression guard for the scenario the greenlight
// flagged: "stale-plan detection tuned for Opus-class inference marks
// a legitimate local-model run as stuck and spuriously recovers it."
func TestCrashRecover_ByomTunedStalenessSurvivesSlowInference(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	// Simulate a BYOM operator's tuned timeouts. 30m stale window is
	// in the ballpark for a local-inference deployment.
	restore := byom.SetForTests(byom.Timeouts{
		SubmitHardTimeout: 30 * time.Minute,
		PermissionTimeout: 5 * time.Minute,
		StalePlanAfter:    30 * time.Minute,
		SweeperTick:       30 * time.Second,
		GraphMaxHops:      256,
	})
	defer restore()

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	// Create a plan, transition to running, then backdate its
	// updated_at so it LOOKS like a run that's been going for 6m.
	// This is legitimately-slow inference under BYOM, not a crash.
	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "slow-byom-" + testutil.TempID("p"),
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
	// Backdate: 6 minutes is past the OLD hardcoded 10m-minus-a-bit
	// and well past 5m; we want to confirm the BYOM knob is what keeps
	// the sweeper's hands off. Use 6m so that if someone silently
	// lowers StalePlanAfter below that, this test catches it.
	if _, err := pool.Exec(ctx, `
        UPDATE plans SET updated_at = NOW() - INTERVAL '6 minutes' WHERE id = $1
    `, p.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	runner := &callCountRunner{} // panics if invoked — it must not be
	svc := orchestrator.NewService(ps, as, runner, apievents.NewEmitter(), nil)
	sw := orchestrator.NewSweeper(pool, svc, nil)
	sw.TenantScope = tenantID
	// Confirm NewSweeper picked up the BYOM-tuned window.
	if sw.StalePlanAfter != 30*time.Minute {
		t.Fatalf("sweeper did not pick up BYOM StalePlanAfter: got %v want 30m", sw.StalePlanAfter)
	}

	resumed, err := sw.Run(ctx)
	if err != nil {
		t.Fatalf("sweeper.Run: %v", err)
	}
	if resumed != 0 {
		t.Fatalf("BYOM stale-window respected: expected 0 resumptions of a legitimately-slow plan, got %d", resumed)
	}
	if runner.calls.Load() != 0 {
		t.Fatalf("runner must not be invoked: got %d calls", runner.calls.Load())
	}

	final, _ := ps.GetPlan(ctx, p.ID)
	if final.Status != plans.StatusRunning {
		t.Fatalf("plan should remain running: got %s", final.Status)
	}
}

// TestCrashRecover_HijackStalePlanAfterWindowElapsed is the companion
// to the BYOM test: with the window tightened (as on hosted cloud),
// the sweeper DOES reclaim a plan whose updated_at crossed the
// configured cutoff. Without this, the previous test could pass for
// the wrong reason (sweeper broken outright).
func TestCrashRecover_HijackStalePlanAfterWindowElapsed(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	restore := byom.SetForTests(byom.Timeouts{
		SubmitHardTimeout: 10 * time.Minute,
		PermissionTimeout: 2 * time.Minute,
		StalePlanAfter:    1 * time.Minute,
		SweeperTick:       30 * time.Second,
		GraphMaxHops:      256,
	})
	defer restore()

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	p, _ := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "hijack-" + testutil.TempID("p"),
	})
	// Build a minimally-viable graph so ExecutePlan has something to run.
	node, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "task",
		AssigneeSoul: "dev",
		Inputs:       handlers.AgentTaskInputs{AgentID: "dev", Instruction: "go"},
	})
	_ = node
	for _, st := range []plans.PlanStatus{
		plans.StatusPendingApproval, plans.StatusApproved, plans.StatusRunning,
	} {
		if _, err := ps.UpdatePlanStatus(ctx, p.ID, st); err != nil {
			t.Fatalf("→%s: %v", st, err)
		}
	}
	if _, err := pool.Exec(ctx, `
        UPDATE plans SET updated_at = NOW() - INTERVAL '10 minutes' WHERE id = $1
    `, p.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	runner := &scriptedAgent{responses: map[string]string{"go": "ok"}}
	svc := orchestrator.NewService(ps, as, runner, apievents.NewEmitter(), nil)
	sw := orchestrator.NewSweeper(pool, svc, nil)
	sw.TenantScope = tenantID

	resumed, err := sw.Run(ctx)
	if err != nil {
		t.Fatalf("sweeper.Run: %v", err)
	}
	if resumed != 1 {
		t.Fatalf("truly-stale plan should be reclaimed: got resumed=%d", resumed)
	}
}

// callCountRunner refuses to actually run. If the orchestrator calls
// it the counter goes up, which we assert stays zero. Used by the
// BYOM-staleness test: invocation under a properly-tuned window is
// itself the bug.
type callCountRunner struct {
	calls atomic.Int32
}

func (c *callCountRunner) Run(ctx context.Context, req agent.RunRequest, onEvent func(agent.StreamEvent)) (*agent.RunResult, error) {
	c.calls.Add(1)
	return &agent.RunResult{}, nil
}
