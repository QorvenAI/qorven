// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
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

// TestChaos_MidPlanCrashRecovery_NoDoubleDestructive is the Phase 8
// RC1 chaos test. It simulates the worst-case failure pattern for a
// multi-tenant orchestrator: the gateway process dies while a plan
// is mid-execution. A fresh gateway instance — simulated by a fresh
// orchestrator.Service bound to the same DB — must resume the plan
// via the sweeper path and run exactly the steps that had NOT yet
// completed, without re-firing any destructive action.
//
// ## Why this is the hardest version of the test
//
// Previous tests (P3E-04) verified that completed nodes don't
// re-execute. That's table stakes. This test adds:
//
//   1. The destructive tool is a stand-in for a real GH push / file
//      mutation — the test asserts the tool's call counter stays at
//      exactly 1 even across gateway "restarts".
//
//   2. The crash happens at the worst moment: the plan is paused on
//      an approval gate. This is the longest-lived intermediate
//      state in the system — a real operator could have the plan
//      sitting here for hours or days before approving. The sweeper
//      must handle resumption cleanly regardless of how long the
//      gap was.
//
//   3. The "fresh gateway" uses a DIFFERENT runner instance + a
//      DIFFERENT in-memory state than the pre-crash gateway. Any
//      cross-instance state sharing would be cheating — we only
//      allow the DB to carry state.
//
// If this test ever regresses, treat it as a P0: the sweeper's
// idempotency invariants have broken and real users could get
// duplicated destructive actions on recovery.
func TestChaos_MidPlanCrashRecovery_NoDoubleDestructive(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	// Shared stores — the "DB" that survives both gateway instances.
	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	// ──────────────── Build the plan graph ────────────────
	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "chaos-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	planner, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindPlanner, Title: "plan",
		Inputs: handlers.PlannerInputs{Description: "ship a feature"},
	})
	approve, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindHumanFeedback, Title: "approve-destructive-work",
	})
	destructive, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "ship_code_to_prod",
		AssigneeSoul: "shipper",
		Inputs: handlers.AgentTaskInputs{
			AgentID:     "shipper",
			Instruction: "gh_push_file path=main.go sha=HEAD",
		},
	})
	if _, err := ps.AddEdge(ctx, p.ID, planner.ID, approve.ID, plans.CondAlways); err != nil {
		t.Fatalf("edge plan→approve: %v", err)
	}
	if _, err := ps.AddEdge(ctx, p.ID, approve.ID, destructive.ID, plans.CondApproved); err != nil {
		t.Fatalf("edge approve→destructive: %v", err)
	}

	// ──────────────── Pre-crash gateway: Service #1 ────────────────
	//
	// Runner A simulates the first gateway's LLM. The key property:
	// it tracks per-node invocation counts via a shared tracker so
	// the post-crash runner can see what A had already done. The
	// runner keys the tracker by matching the instruction text
	// back to the concrete node IDs we created above — that's how
	// the assertions' .count(planner.ID) / .count(destructive.ID)
	// resolve.
	tracker := &nodeInvocationTracker{calls: map[string]*atomic.Int32{}}
	resolveNodeID := func(instruction string) string {
		switch {
		case containsAny(instruction, "ship a feature"):
			return planner.ID
		case containsAny(instruction, "gh_push_file"):
			return destructive.ID
		}
		return ""
	}
	runnerA := &destructiveTrackingRunner{
		tracker:      tracker,
		resolveNode:  resolveNodeID,
		plannerReply: `{"stack":"go","summary":"ship it","agents":[]}`,
	}

	svcA := orchestrator.NewService(ps, as, runnerA, apievents.NewEmitter(), nil)
	if err := svcA.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("pre-crash ExecutePlan: %v", err)
	}
	// After this run the planner node must be done; the approval
	// node must be blocked on human_feedback; the destructive node
	// must still be pending.
	if got := tracker.count(planner.ID); got != 1 {
		t.Fatalf("pre-crash: planner expected 1 call, got %d", got)
	}
	if got := tracker.count(destructive.ID); got != 0 {
		t.Fatalf("pre-crash: destructive must NOT have fired yet; got %d calls", got)
	}

	// Confirm an approval was requested.
	pending, err := as.ListByPlan(ctx, p.ID)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(pending) == 0 {
		t.Fatalf("expected a pending approval after pre-crash run")
	}

	// ──────────────── Simulate the crash ────────────────
	//
	// We discard runnerA + svcA entirely. The only state that
	// survives is in the DB (via pool). No in-memory bridge.
	runnerA = nil
	svcA = nil
	_ = runnerA
	_ = svcA

	// Simulate "some time passes" — a real operator might wait
	// minutes or days before approving. The sweeper must tolerate
	// arbitrarily long gaps; we use a short sleep just to prove
	// we're not coupling recovery to any wall-clock assumption.
	time.Sleep(50 * time.Millisecond)

	// User approves via the HTTP flow (we replicate the DB write
	// the approve handler does: resolve the approval + insert a
	// wakeup_request row).
	if _, err := as.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: pending[0].ID,
		Next:       approvals.StateApproved,
		ResolvedBy: "chaos-user",
	}); err != nil {
		t.Fatalf("resolve approval: %v", err)
	}
	if _, err := pool.Exec(ctx, `
        INSERT INTO wakeup_requests
            (agent_id, tenant_id, source, actor_type, cause, payload, plan_id)
        VALUES ('orchestrator', $2, 'chaos-test', 'user', 'approval_resolved',
                '{"approval_id":"`+pending[0].ID+`"}'::jsonb, $1)
    `, p.ID, tenantID); err != nil {
		t.Fatalf("insert wakeup: %v", err)
	}

	// ──────────────── Post-crash gateway: Service #2 ────────────────
	//
	// Brand-new runner bound to the SAME tracker. A fresh
	// in-memory service. The sweeper is responsible for picking up
	// the plan + calling ExecutePlan. If the sweeper replays the
	// planner (which is already done) OR fires the destructive
	// twice, tracker.count will expose the bug.
	runnerB := &destructiveTrackingRunner{
		tracker:      tracker,
		resolveNode:  resolveNodeID, // same closure — keeps trackers unified
		plannerReply: `{"stack":"go","summary":"ship it","agents":[]}`,
	}
	svcB := orchestrator.NewService(ps, as, runnerB, apievents.NewEmitter(), nil)

	sw := orchestrator.NewSweeper(pool, svcB, nil)
	sw.TenantScope = tenantID // isolate — prevent sibling tests from being swept

	resumed, err := sw.Run(ctx)
	if err != nil {
		t.Fatalf("post-crash sweeper.Run: %v", err)
	}
	if resumed != 1 {
		t.Fatalf("expected exactly 1 plan resumed, got %d", resumed)
	}

	// ──────────────── Assertions ────────────────

	// 1. Planner must NOT have been re-run by runnerB.
	if got := tracker.count(planner.ID); got != 1 {
		t.Fatalf("planner double-invocation: total calls=%d (pre-crash already did it)", got)
	}

	// 2. Destructive tool fired EXACTLY once.
	if got := tracker.count(destructive.ID); got != 1 {
		t.Fatalf("destructive tool fired %d times — CRITICAL duplication bug", got)
	}

	// 3. Plan reached done state.
	final, err := ps.GetPlan(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPlan final: %v", err)
	}
	if final.Status != plans.StatusDone {
		t.Fatalf("plan not done after recovery: status=%s", final.Status)
	}

	// 4. Wakeup request is consumed (no resurrection loops).
	var unconsumed int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM wakeup_requests WHERE plan_id = $1 AND consumed_at IS NULL`,
		p.ID,
	).Scan(&unconsumed); err != nil {
		t.Fatalf("count unconsumed wakeups: %v", err)
	}
	if unconsumed != 0 {
		t.Fatalf("wakeup_request left unconsumed after recovery: count=%d", unconsumed)
	}

	// 5. Re-running the sweeper on a done plan is a no-op — the
	// destructive tool count stays at 1. Guards against a future
	// regression where sweeper's idempotency check drifts.
	if _, err := sw.Run(ctx); err != nil {
		t.Fatalf("second sweeper run: %v", err)
	}
	if got := tracker.count(destructive.ID); got != 1 {
		t.Fatalf("destructive re-fired on idle sweep: count=%d", got)
	}
}

// nodeInvocationTracker counts how many times each node_id was
// invoked by an agent runner. Shared across pre-crash + post-crash
// runners so the test can assert no duplication across restart.
type nodeInvocationTracker struct {
	mu    sync.Mutex
	calls map[string]*atomic.Int32
}

func (t *nodeInvocationTracker) record(nodeID string) {
	t.mu.Lock()
	c, ok := t.calls[nodeID]
	if !ok {
		c = &atomic.Int32{}
		t.calls[nodeID] = c
	}
	t.mu.Unlock()
	c.Add(1)
}

func (t *nodeInvocationTracker) count(nodeID string) int32 {
	t.mu.Lock()
	c, ok := t.calls[nodeID]
	t.mu.Unlock()
	if !ok {
		return 0
	}
	return c.Load()
}

// destructiveTrackingRunner is an AgentRunner that:
//   • records every call against the shared tracker by node_id
//   • replies with a valid planner JSON blob when the request is a
//     planner invocation (detected by the "stack" / "agents" hints
//     in the instruction — this matches how scripts work in the
//     existing integration tests)
//   • replies with a short "destructive-tool done" text for
//     agent-task invocations.
//
// The runner does NOT actually call any tool — the point is to
// simulate an LLM that decided to invoke a destructive action. The
// DB-visible side effect (tracker) stands in for the real tool's
// mutation.
type destructiveTrackingRunner struct {
	tracker      *nodeInvocationTracker
	resolveNode  func(instruction string) string // maps req text → node_id
	plannerReply string
	calls        atomic.Int32
}

func (r *destructiveTrackingRunner) Run(ctx context.Context, req agent.RunRequest, onEvent func(agent.StreamEvent)) (*agent.RunResult, error) {
	r.calls.Add(1)

	// Map the request's instruction text back to the concrete
	// node_id created by the test. The runner never sees node_id
	// directly (that would require extending agent.RunRequest);
	// the closure supplied by the test owns the mapping.
	msg := req.UserMessage
	if r.resolveNode != nil {
		if nodeID := r.resolveNode(msg); nodeID != "" {
			r.tracker.record(nodeID)
		}
	}

	reply := "shipped"
	if containsAny(msg, "ship a feature", "todo app", "description", "Build") {
		reply = r.plannerReply
	}
	onEvent(agent.StreamEvent{Type: "text_delta", Delta: reply})
	return &agent.RunResult{Content: reply}, nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

// indexOf is a local Copy of strings.Index to avoid a separate
// import in this single-file test module.
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
