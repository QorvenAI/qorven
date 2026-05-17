// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package orchestrator_test

// FU-022: golden event sequence for the graph-only build path.
//
// These tests assert the canonical ordering of graph.node_* events
// across a full plan lifecycle, proving the graph runtime is correct
// in isolation now that the legacy orchestrateBuild path is retired.
//
// Each test defines a "golden sequence" — the ordered list of (kind,
// node_id) pairs expected on the SSE stream — and fails if the
// observed stream diverges.

import (
	"testing"

	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/orchestrator"
	"github.com/qorvenai/qorven/internal/orchestrator/handlers"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/ssestream"
	"github.com/qorvenai/qorven/internal/testutil"
)

// seqEntry is one step in a golden sequence.
type seqEntry struct {
	evType apievents.Type
	nodeID string // "" means "any node"
}

// collectGraphNodeEvents parses a captureBuf and returns, in emission
// order, every graph.node_* canonical event (started / completed /
// paused / failed).
func collectGraphNodeEvents(buf *captureBuf) []seqEntry {
	var out []seqEntry
	for _, env := range buf.parse() {
		switch env.Type {
		case apievents.TypeGraphNodeStarted:
			var p apievents.GraphNodeStartedProps
			if err := env.Decode(&p); err == nil {
				out = append(out, seqEntry{evType: env.Type, nodeID: p.NodeID})
			}
		case apievents.TypeGraphNodeCompleted:
			var p apievents.GraphNodeCompletedProps
			if err := env.Decode(&p); err == nil {
				out = append(out, seqEntry{evType: env.Type, nodeID: p.NodeID})
			}
		case apievents.TypeGraphNodePaused:
			var p apievents.GraphNodePausedProps
			if err := env.Decode(&p); err == nil {
				out = append(out, seqEntry{evType: env.Type, nodeID: p.NodeID})
			}
		case apievents.TypeGraphNodeFailed:
			var p apievents.GraphNodeFailedProps
			if err := env.Decode(&p); err == nil {
				out = append(out, seqEntry{evType: env.Type, nodeID: p.NodeID})
			}
		}
	}
	return out
}

// matchesGolden checks that observed matches the golden sequence.
// nodeID "" in the golden entry means "don't check node identity".
func matchesGolden(t *testing.T, golden, observed []seqEntry) {
	t.Helper()
	if len(observed) != len(golden) {
		t.Fatalf("event count mismatch: want %d, got %d\ngolden:   %v\nobserved: %v",
			len(golden), len(observed), golden, observed)
	}
	for i, want := range golden {
		got := observed[i]
		if got.evType != want.evType {
			t.Errorf("event[%d]: type want %q got %q", i, want.evType, got.evType)
		}
		if want.nodeID != "" && got.nodeID != want.nodeID {
			t.Errorf("event[%d]: node_id want %q got %q", i, want.nodeID, got.nodeID)
		}
	}
}

// TestGolden_SingleAgentTask: simplest possible plan — one agent_task node,
// no edges. Golden sequence: started → completed.
func TestGolden_SingleAgentTask(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "golden-single-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	node, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "do it",
		AssigneeSoul: "dev",
		Inputs:       handlers.AgentTaskInputs{AgentID: "dev", Instruction: "write tests"},
	})
	if err != nil {
		t.Fatalf("AppendNode: %v", err)
	}

	buf := &captureBuf{}
	sse := ssestream.NewEmitterWriter(buf, nil)
	emitter := apievents.NewEmitter()
	defer emitter.Attach("golden-single", sse)()

	runner := &scriptedAgent{responses: map[string]string{"write tests": "done"}}
	svc := orchestrator.NewService(ps, as, runner, emitter, nil)

	if err := svc.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	golden := []seqEntry{
		{evType: apievents.TypeGraphNodeStarted, nodeID: node.ID},
		{evType: apievents.TypeGraphNodeCompleted, nodeID: node.ID},
	}
	matchesGolden(t, golden, collectGraphNodeEvents(buf))

	// Plan must be terminal.
	final, _ := ps.GetPlan(ctx, p.ID)
	if final.Status != plans.StatusDone {
		t.Fatalf("plan status: want done, got %s", final.Status)
	}
}

// TestGolden_LinearChain: planner → agent_task with always edge.
// First and only run completes both nodes.
// Golden: started(planner), completed(planner), started(builder), completed(builder).
func TestGolden_LinearChain(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "golden-chain-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	planner, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindPlanner, Title: "plan",
		Inputs: handlers.PlannerInputs{Description: "widget"},
	})
	if err != nil {
		t.Fatalf("AppendNode planner: %v", err)
	}
	builder, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "build",
		AssigneeSoul: "dev",
		Inputs:       handlers.AgentTaskInputs{AgentID: "dev", Instruction: "scaffold widget"},
	})
	if err != nil {
		t.Fatalf("AppendNode builder: %v", err)
	}
	if _, err := ps.AddEdge(ctx, p.ID, planner.ID, builder.ID, plans.CondAlways); err != nil {
		t.Fatalf("AddEdge: %v", err)
	}

	buf := &captureBuf{}
	sse := ssestream.NewEmitterWriter(buf, nil)
	emitter := apievents.NewEmitter()
	defer emitter.Attach("golden-chain", sse)()

	runner := &scriptedAgent{responses: map[string]string{
		"widget":          `{"stack":"go","summary":"widget service","agents":[]}`,
		"scaffold widget": "files created",
	}}
	svc := orchestrator.NewService(ps, as, runner, emitter, nil)

	if err := svc.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("ExecutePlan: %v", err)
	}

	golden := []seqEntry{
		{evType: apievents.TypeGraphNodeStarted, nodeID: planner.ID},
		{evType: apievents.TypeGraphNodeCompleted, nodeID: planner.ID},
		{evType: apievents.TypeGraphNodeStarted, nodeID: builder.ID},
		{evType: apievents.TypeGraphNodeCompleted, nodeID: builder.ID},
	}
	matchesGolden(t, golden, collectGraphNodeEvents(buf))

	final, _ := ps.GetPlan(ctx, p.ID)
	if final.Status != plans.StatusDone {
		t.Fatalf("plan status: want done, got %s", final.Status)
	}
}

// TestGolden_ApprovalGate: planner → human_feedback → agent_task.
// Run 1: planner completes, human_feedback pauses (started→paused).
// Run 2 (after approval): human_feedback resumes+completes, builder completes.
//
// Golden run-1: started(planner), completed(planner), started(feedback), paused(feedback).
// Golden run-2: started(feedback), completed(feedback), started(builder), completed(builder).
func TestGolden_ApprovalGate(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "golden-gate-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	planner, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindPlanner, Title: "plan",
		Inputs: handlers.PlannerInputs{Description: "service"},
	})
	if err != nil {
		t.Fatalf("planner: %v", err)
	}
	feedback, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindHumanFeedback, Title: "approve",
	})
	if err != nil {
		t.Fatalf("feedback: %v", err)
	}
	builder, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "build",
		AssigneeSoul: "dev",
		Inputs:       handlers.AgentTaskInputs{AgentID: "dev", Instruction: "build service"},
	})
	if err != nil {
		t.Fatalf("builder: %v", err)
	}
	if _, err := ps.AddEdge(ctx, p.ID, planner.ID, feedback.ID, plans.CondAlways); err != nil {
		t.Fatalf("edge plan→feedback: %v", err)
	}
	if _, err := ps.AddEdge(ctx, p.ID, feedback.ID, builder.ID, plans.CondApproved); err != nil {
		t.Fatalf("edge feedback→builder: %v", err)
	}

	runner := &scriptedAgent{responses: map[string]string{
		"service":       `{"stack":"go","summary":"svc","agents":[]}`,
		"build service": "built",
	}}

	// --- Run 1 ---
	buf1 := &captureBuf{}
	sse1 := ssestream.NewEmitterWriter(buf1, nil)
	emitter1 := apievents.NewEmitter()
	defer emitter1.Attach("golden-gate-1", sse1)()

	svc := orchestrator.NewService(ps, as, runner, emitter1, nil)
	if err := svc.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("run1 ExecutePlan: %v", err)
	}

	goldenRun1 := []seqEntry{
		{evType: apievents.TypeGraphNodeStarted, nodeID: planner.ID},
		{evType: apievents.TypeGraphNodeCompleted, nodeID: planner.ID},
		{evType: apievents.TypeGraphNodeStarted, nodeID: feedback.ID},
		{evType: apievents.TypeGraphNodePaused, nodeID: feedback.ID},
	}
	matchesGolden(t, goldenRun1, collectGraphNodeEvents(buf1))

	// Plan should still be in-progress (not done/failed).
	mid, _ := ps.GetPlan(ctx, p.ID)
	if mid.Status == plans.StatusDone || mid.Status == plans.StatusFailed {
		t.Fatalf("plan should still be running after pause, got %s", mid.Status)
	}

	// Resolve the approval.
	list, _ := as.ListByPlan(ctx, p.ID)
	if len(list) == 0 {
		t.Fatalf("no approval requested after run1")
	}
	if _, err := as.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: list[0].ID,
		Next:       approvals.StateApproved,
		ResolvedBy: "reviewer",
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// --- Run 2 ---
	buf2 := &captureBuf{}
	sse2 := ssestream.NewEmitterWriter(buf2, nil)
	emitter2 := apievents.NewEmitter()
	defer emitter2.Attach("golden-gate-2", sse2)()

	svc2 := orchestrator.NewService(ps, as, runner, emitter2, nil)
	if err := svc2.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("run2 ExecutePlan: %v", err)
	}

	goldenRun2 := []seqEntry{
		{evType: apievents.TypeGraphNodeStarted, nodeID: feedback.ID},
		{evType: apievents.TypeGraphNodeCompleted, nodeID: feedback.ID},
		{evType: apievents.TypeGraphNodeStarted, nodeID: builder.ID},
		{evType: apievents.TypeGraphNodeCompleted, nodeID: builder.ID},
	}
	matchesGolden(t, goldenRun2, collectGraphNodeEvents(buf2))

	final, _ := ps.GetPlan(ctx, p.ID)
	if final.Status != plans.StatusDone {
		t.Fatalf("final plan status: want done, got %s", final.Status)
	}
}

// TestGolden_NodeFailure: a plan whose only node fails.
// Golden: started → failed. Plan status = failed.
func TestGolden_NodeFailure(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "golden-fail-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	node, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "doomed",
		AssigneeSoul: "dev",
		Inputs:       handlers.AgentTaskInputs{AgentID: "dev", Instruction: "crash"},
	})
	if err != nil {
		t.Fatalf("AppendNode: %v", err)
	}

	buf := &captureBuf{}
	sse := ssestream.NewEmitterWriter(buf, nil)
	emitter := apievents.NewEmitter()
	defer emitter.Attach("golden-fail", sse)()

	runner := &failingAgent{err: "service unavailable"}
	svc := orchestrator.NewService(ps, as, runner, emitter, nil)

	_ = svc.ExecutePlan(ctx, p.ID)

	golden := []seqEntry{
		{evType: apievents.TypeGraphNodeStarted, nodeID: node.ID},
		{evType: apievents.TypeGraphNodeFailed, nodeID: node.ID},
	}
	matchesGolden(t, golden, collectGraphNodeEvents(buf))

	// Verify error text propagated into the failed event.
	observed := collectGraphNodeEvents(buf)
	for _, env := range buf.parse() {
		if env.Type != apievents.TypeGraphNodeFailed {
			continue
		}
		var fp apievents.GraphNodeFailedProps
		if err := env.Decode(&fp); err == nil {
			if fp.Error == "" {
				t.Errorf("graph.node_failed missing error text")
			}
		}
	}
	_ = observed

	final, _ := ps.GetPlan(ctx, p.ID)
	if final.Status != plans.StatusFailed {
		t.Fatalf("plan status: want failed, got %s", final.Status)
	}
}
