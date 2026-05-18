// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package graph_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/orchestrator/graph"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/testutil"
)

func seedPlan(t *testing.T) (*plans.Store, *approvals.Store, *plans.Plan) {
	t.Helper()
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)
	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)
	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID,
		Title:    "graph-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	return ps, as, p
}

func TestGraph_LinearPlannerToAgentTask(t *testing.T) {
	ps, as, p := seedPlan(t)
	ctx := testutil.Ctx(t)

	planner, _ := ps.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindPlanner, Title: "plan"})
	agent, _ := ps.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindAgentTask, Title: "build", AssigneeSoul: "builder"})
	_, _ = ps.AddEdge(ctx, p.ID, planner.ID, agent.ID, plans.CondAlways)

	var plannerRan, agentRan atomic.Int32
	reg := graph.NewRegistry().
		Register(plans.KindPlanner, func(_ context.Context, h *graph.HandlerContext) (graph.Outcome, any, error) {
			plannerRan.Add(1)
			return graph.OutcomeSuccess, map[string]any{"files": 3}, nil
		}).
		Register(plans.KindAgentTask, func(_ context.Context, h *graph.HandlerContext) (graph.Outcome, any, error) {
			agentRan.Add(1)
			h.Emit("progress", map[string]any{"step": "writing"})
			return graph.OutcomeSuccess, map[string]any{"wrote": "app.go"}, nil
		})

	rt := graph.NewRuntime(graph.Config{
		Plans: ps, Approvals: as, Registry: reg,
	})
	if err := rt.Run(ctx, p.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if plannerRan.Load() != 1 {
		t.Fatalf("planner ran %d times", plannerRan.Load())
	}
	if agentRan.Load() != 1 {
		t.Fatalf("agent ran %d times", agentRan.Load())
	}

	got, _ := ps.GetPlan(ctx, p.ID)
	if got.Status != plans.StatusDone {
		t.Fatalf("plan status: %s", got.Status)
	}
}

func TestGraph_HumanFeedbackPausesAndResumes(t *testing.T) {
	ps, as, p := seedPlan(t)
	ctx := testutil.Ctx(t)

	planner, _ := ps.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindPlanner, Title: "plan"})
	feedback, _ := ps.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindHumanFeedback, Title: "approve"})
	agent, _ := ps.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindAgentTask, Title: "build"})

	_, _ = ps.AddEdge(ctx, p.ID, planner.ID, feedback.ID, plans.CondAlways)
	_, _ = ps.AddEdge(ctx, p.ID, feedback.ID, agent.ID, plans.CondApproved)

	// feedback handler pauses on first invocation; on second it reads
	// the approval state (which the test flips before resuming) and
	// returns success with outcome=approved recorded via artifacts.
	var agentRan atomic.Int32
	reg := graph.NewRegistry().
		Register(plans.KindPlanner, func(_ context.Context, _ *graph.HandlerContext) (graph.Outcome, any, error) {
			return graph.OutcomeSuccess, nil, nil
		}).
		Register(plans.KindHumanFeedback, func(ctx context.Context, h *graph.HandlerContext) (graph.Outcome, any, error) {
			appr, err := h.Approvals.Request(ctx, h.Plan.ID, h.Node.ID, "system", nil)
			if err != nil {
				return "", nil, err
			}
			// Re-read the latest state to tolerate resume.
			fresh, err := h.Approvals.Get(ctx, appr.ID)
			if err != nil {
				return "", nil, err
			}
			switch fresh.State {
			case approvals.StatePending:
				return "", nil, &graph.PauseSignal{Reason: "awaiting approval"}
			case approvals.StateApproved:
				return graph.OutcomeApproved, map[string]any{"approval": "approved"}, nil
			case approvals.StateRejected:
				return graph.OutcomeRejected, map[string]any{"approval": "rejected"}, nil
			case approvals.StateRevisionRequested:
				return graph.OutcomeRevision, map[string]any{"approval": "revision_requested"}, nil
			}
			return "", nil, errors.New("unknown approval state")
		}).
		Register(plans.KindAgentTask, func(_ context.Context, _ *graph.HandlerContext) (graph.Outcome, any, error) {
			agentRan.Add(1)
			return graph.OutcomeSuccess, nil, nil
		})

	rt := graph.NewRuntime(graph.Config{Plans: ps, Approvals: as, Registry: reg})

	err := rt.Run(ctx, p.ID)
	if !errors.Is(err, graph.ErrPaused) {
		t.Fatalf("expected paused, got %v", err)
	}
	if agentRan.Load() != 0 {
		t.Fatalf("agent must not run while paused")
	}

	// Resolve approval.
	appr, _ := as.ListByPlan(ctx, p.ID)
	if len(appr) == 0 {
		t.Fatalf("no approval requested")
	}
	if _, err := as.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: appr[0].ID, Next: approvals.StateApproved, ResolvedBy: "reviewer",
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// Resume.
	if err := rt.Run(ctx, p.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if agentRan.Load() != 1 {
		t.Fatalf("agent must run after approve, got %d", agentRan.Load())
	}
	got, _ := ps.GetPlan(ctx, p.ID)
	if got.Status != plans.StatusDone {
		t.Fatalf("plan status after resume: %s", got.Status)
	}
}

func TestGraph_HumanFeedbackRejectionDoesNotRunDownstream(t *testing.T) {
	ps, as, p := seedPlan(t)
	ctx := testutil.Ctx(t)

	feedback, _ := ps.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindHumanFeedback, Title: "approve"})
	agent, _ := ps.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindAgentTask, Title: "build"})
	_, _ = ps.AddEdge(ctx, p.ID, feedback.ID, agent.ID, plans.CondApproved)

	var agentRan atomic.Int32
	reg := graph.NewRegistry().
		Register(plans.KindHumanFeedback, func(ctx context.Context, h *graph.HandlerContext) (graph.Outcome, any, error) {
			appr, _ := h.Approvals.Request(ctx, h.Plan.ID, h.Node.ID, "s", nil)
			fresh, _ := h.Approvals.Get(ctx, appr.ID)
			if fresh.State == approvals.StatePending {
				return "", nil, &graph.PauseSignal{Reason: "wait"}
			}
			if fresh.State == approvals.StateRejected {
				return graph.OutcomeRejected, map[string]any{"approval": "rejected"}, nil
			}
			return graph.OutcomeApproved, map[string]any{"approval": "approved"}, nil
		}).
		Register(plans.KindAgentTask, func(_ context.Context, _ *graph.HandlerContext) (graph.Outcome, any, error) {
			agentRan.Add(1)
			return graph.OutcomeSuccess, nil, nil
		})

	rt := graph.NewRuntime(graph.Config{Plans: ps, Approvals: as, Registry: reg})
	_ = rt.Run(ctx, p.ID) // pauses
	appr, _ := as.ListByPlan(ctx, p.ID)
	_, _ = as.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: appr[0].ID, Next: approvals.StateRejected, ResolvedBy: "r",
	})
	if err := rt.Run(ctx, p.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if agentRan.Load() != 0 {
		t.Fatalf("agent ran despite rejection")
	}
}

func TestGraph_HandlerFailureMarksPlanFailed(t *testing.T) {
	ps, as, p := seedPlan(t)
	ctx := testutil.Ctx(t)

	doomed, _ := ps.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindAgentTask, Title: "doomed"})
	_ = doomed

	reg := graph.NewRegistry().
		Register(plans.KindAgentTask, func(_ context.Context, _ *graph.HandlerContext) (graph.Outcome, any, error) {
			return graph.OutcomeError, nil, errors.New("provider down")
		})
	rt := graph.NewRuntime(graph.Config{Plans: ps, Approvals: as, Registry: reg})

	err := rt.Run(ctx, p.ID)
	if err == nil {
		t.Fatalf("expected failure error")
	}
	got, _ := ps.GetPlan(ctx, p.ID)
	if got.Status != plans.StatusFailed {
		t.Fatalf("plan status: %s", got.Status)
	}
}

func TestGraph_Cancellation(t *testing.T) {
	ps, as, p := seedPlan(t)

	long, _ := ps.AppendNode(ctx(t), plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindAgentTask, Title: "long"})
	_ = long

	reg := graph.NewRegistry().
		Register(plans.KindAgentTask, func(ctx context.Context, _ *graph.HandlerContext) (graph.Outcome, any, error) {
			<-ctx.Done()
			return graph.OutcomeError, nil, ctx.Err()
		})
	rt := graph.NewRuntime(graph.Config{Plans: ps, Approvals: as, Registry: reg})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- rt.Run(ctx, p.ID) }()
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestGraph_NoHandlerForKindFails(t *testing.T) {
	ps, as, p := seedPlan(t)
	ctx := testutil.Ctx(t)

	_, _ = ps.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindReview, Title: "review"})

	reg := graph.NewRegistry()
	rt := graph.NewRuntime(graph.Config{Plans: ps, Approvals: as, Registry: reg})
	err := rt.Run(ctx, p.ID)
	if err == nil || !contains(err.Error(), "no handler for node kind") {
		t.Fatalf("expected missing-handler error, got %v", err)
	}
}

func TestGraph_EmitFiresForProgress(t *testing.T) {
	ps, as, p := seedPlan(t)
	ctx := testutil.Ctx(t)

	_, _ = ps.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindAgentTask, Title: "t"})

	var emitted []string
	reg := graph.NewRegistry().
		Register(plans.KindAgentTask, func(_ context.Context, h *graph.HandlerContext) (graph.Outcome, any, error) {
			h.Emit("progress", map[string]any{"x": 1})
			return graph.OutcomeSuccess, nil, nil
		})

	rt := graph.NewRuntime(graph.Config{
		Plans: ps, Approvals: as, Registry: reg,
		Emit: func(_ context.Context, _, _, kind string, _ map[string]any) {
			emitted = append(emitted, kind)
		},
	})
	if err := rt.Run(ctx, p.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Expect: node.started, progress, node.completed.
	found := map[string]bool{}
	for _, k := range emitted {
		found[k] = true
	}
	for _, k := range []string{"node.started", "progress", "node.completed"} {
		if !found[k] {
			t.Fatalf("missing emit %q (got %v)", k, emitted)
		}
	}
}

func TestGraph_MaxHopsGuard(t *testing.T) {
	ps, as, p := seedPlan(t)
	ctx := testutil.Ctx(t)

	// Single always-runnable node whose handler keeps returning success
	// but the runtime must terminate because the node goes to done and
	// there are no outgoing edges. Max-hops guard is tested by a
	// pathological config: we lower the bound and prove it raises.
	for i := 0; i < 5; i++ {
		_, _ = ps.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindAgentTask, Title: "n"})
	}
	reg := graph.NewRegistry().
		Register(plans.KindAgentTask, func(_ context.Context, _ *graph.HandlerContext) (graph.Outcome, any, error) {
			return graph.OutcomeSuccess, nil, nil
		})
	rt := graph.NewRuntime(graph.Config{Plans: ps, Approvals: as, Registry: reg, MaxHopsPerRun: 2})
	err := rt.Run(ctx, p.ID)
	if err == nil || !contains(err.Error(), "exceeded max hops") {
		t.Fatalf("expected max-hops error, got %v", err)
	}
}

// TestGraph_OutcomeErrorAuditArtifact verifies FU-027: when a handler
// returns OutcomeError and an on_error edge routes execution, the
// failing node's artifacts contain "outcome":"on_error" as the audit
// record.
func TestGraph_OutcomeErrorAuditArtifact(t *testing.T) {
	ps, as, p := seedPlan(t)
	ctx := testutil.Ctx(t)

	source, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "source",
	})
	handler, _ := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindReview, Title: "error-handler",
	})
	if _, err := ps.AddEdge(ctx, p.ID, source.ID, handler.ID, plans.CondOnError); err != nil {
		t.Fatalf("AddEdge on_error: %v", err)
	}

	var handlerRan atomic.Int32
	reg := graph.NewRegistry().
		Register(plans.KindAgentTask, func(_ context.Context, _ *graph.HandlerContext) (graph.Outcome, any, error) {
			return graph.OutcomeError, map[string]any{"detail": "timeout"}, errors.New("upstream timeout")
		}).
		Register(plans.KindReview, func(_ context.Context, _ *graph.HandlerContext) (graph.Outcome, any, error) {
			handlerRan.Add(1)
			return graph.OutcomeSuccess, nil, nil
		})
	rt := graph.NewRuntime(graph.Config{Plans: ps, Approvals: as, Registry: reg})

	if err := rt.Run(ctx, p.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Error-handler node must have run.
	if handlerRan.Load() != 1 {
		t.Fatalf("error-handler ran %d times, want 1", handlerRan.Load())
	}

	// Source node must be marked failed with "outcome":"on_error" in artifacts.
	src, err := ps.GetNode(ctx, source.ID)
	if err != nil {
		t.Fatalf("GetNode source: %v", err)
	}
	if src.State != plans.NodeFailed {
		t.Fatalf("source node state: want failed, got %s", src.State)
	}
	var arts map[string]any
	if err := json.Unmarshal(src.Artifacts, &arts); err != nil {
		t.Fatalf("unmarshal artifacts: %v (raw: %s)", err, src.Artifacts)
	}
	if got, ok := arts["outcome"]; !ok || got != "on_error" {
		t.Errorf("source artifacts[outcome]: want %q, got %v (full: %v)", "on_error", got, arts)
	}
	// Handler-provided artifact must also survive the merge.
	if got, ok := arts["detail"]; !ok || got != "timeout" {
		t.Errorf("source artifacts[detail]: want %q, got %v", "timeout", got)
	}
}

// contains is a tiny local helper that avoids pulling in strings for
// what is a single substring check per test.
func contains(s, sub string) bool {
	return indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func ctx(t *testing.T) context.Context { return testutil.Ctx(t) }

// Ensure json is used so imports don't go stale.
var _ = json.RawMessage(nil)
