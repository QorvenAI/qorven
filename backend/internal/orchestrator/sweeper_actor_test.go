// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package orchestrator_test

// FU-023: Actor propagation — sweeper → ExecutePlan → graph.node_* events.
//
// The sweeper calls latestApprovalActor to look up approvals.resolved_by,
// injects it via WithActor(planCtx, actor), and calls ExecutePlan.
// The graph emitter picks it up via actorFromCtx and stamps it on every
// graph.node_* event. This test asserts the full chain end-to-end.

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

// TestSweeper_ActorPropagates proves that the human identity stored in
// approvals.resolved_by flows through the sweeper → ExecutePlan →
// graph.node_* SSE events as the Actor field.
//
// Acceptance criterion (FU-023): "Test asserts actor propagates through
// the sweeper → ExecutePlan → SessionIdle emission."
func TestSweeper_ActorPropagates(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	// Build a plan: planner → human_feedback → agent_task.
	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "actor-prop-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	planner, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindPlanner, Title: "plan",
		Inputs: handlers.PlannerInputs{Description: "actor test"},
	})
	if err != nil {
		t.Fatalf("AppendNode planner: %v", err)
	}
	feedback, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindHumanFeedback, Title: "approve",
	})
	if err != nil {
		t.Fatalf("AppendNode feedback: %v", err)
	}
	builder, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "build",
		AssigneeSoul: "dev",
		Inputs:       handlers.AgentTaskInputs{AgentID: "dev", Instruction: "do the thing"},
	})
	if err != nil {
		t.Fatalf("AppendNode builder: %v", err)
	}
	if _, err := ps.AddEdge(ctx, p.ID, planner.ID, feedback.ID, plans.CondAlways); err != nil {
		t.Fatalf("edge planner→feedback: %v", err)
	}
	if _, err := ps.AddEdge(ctx, p.ID, feedback.ID, builder.ID, plans.CondApproved); err != nil {
		t.Fatalf("edge feedback→builder: %v", err)
	}

	runner := &scriptedAgent{responses: map[string]string{
		"actor test":    `{"stack":"go","summary":"actor","agents":[]}`,
		"do the thing":  "done",
	}}

	// Run 1: drives plan to the approval gate (pauses at human_feedback).
	svc1 := orchestrator.NewService(ps, as, runner, apievents.NewEmitter(), nil)
	if err := svc1.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("run1 ExecutePlan: %v", err)
	}

	// Resolve the approval with a known actor.
	const wantActor = "alice@example.com"
	list, _ := as.ListByPlan(ctx, p.ID)
	if len(list) == 0 {
		t.Fatalf("no approval pending after run1")
	}
	if _, err := as.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: list[0].ID,
		Next:       approvals.StateApproved,
		ResolvedBy: wantActor,
	}); err != nil {
		t.Fatalf("resolve approval: %v", err)
	}

	// Insert the wakeup_requests row exactly as the approval handler does.
	if _, err := pool.Exec(ctx, `
		INSERT INTO wakeup_requests
		    (agent_id, tenant_id, source, actor_type, cause, payload, plan_id)
		VALUES ('orchestrator', $2, 'test', 'user', 'approval_resolved',
		        ('{"approval_id":"' || $3 || '"}')::jsonb, $1)
	`, p.ID, tenantID, list[0].ID); err != nil {
		t.Fatalf("insert wakeup: %v", err)
	}

	// Attach an event capture buffer before the sweeper's run-2 call.
	buf := &captureBuf{}
	sse := ssestream.NewEmitterWriter(buf, nil)
	emitter2 := apievents.NewEmitter()
	defer emitter2.Attach("actor-prop", sse)()

	// Sweeper uses a fresh service wired to the capturing emitter.
	svc2 := orchestrator.NewService(ps, as, runner, emitter2, nil)
	sw := orchestrator.NewSweeper(pool, svc2, nil)
	sw.TenantScope = tenantID

	resumed, err := sw.Run(ctx)
	if err != nil {
		t.Fatalf("sweeper.Run: %v", err)
	}
	if resumed != 1 {
		t.Fatalf("expected 1 plan resumed, got %d", resumed)
	}

	// Collect run-2 graph.node_* events and verify all carry the expected actor.
	events := collectGraphNodeEventsWithActor(buf)
	if len(events) == 0 {
		t.Fatalf("no graph.node_* events captured after sweeper resume")
	}
	for _, ev := range events {
		if ev.actor != wantActor {
			t.Errorf("event %s node %s: actor want %q got %q",
				ev.evType, ev.nodeID, wantActor, ev.actor)
		}
	}

	// Plan must be done after the sweeper completes the builder node.
	final, _ := ps.GetPlan(ctx, p.ID)
	if final.Status != plans.StatusDone {
		t.Fatalf("final plan status: want done, got %s", final.Status)
	}
}

// actorEntry extends seqEntry with the actor field for FU-023 assertions.
type actorEntry struct {
	evType apievents.Type
	nodeID string
	actor  string
}

// collectGraphNodeEventsWithActor is like collectGraphNodeEvents but
// also captures the Actor field from each graph.node_* event.
func collectGraphNodeEventsWithActor(buf *captureBuf) []actorEntry {
	var out []actorEntry
	for _, env := range buf.parse() {
		switch env.Type {
		case apievents.TypeGraphNodeStarted:
			var p apievents.GraphNodeStartedProps
			if err := env.Decode(&p); err == nil {
				out = append(out, actorEntry{evType: env.Type, nodeID: p.NodeID, actor: p.Actor})
			}
		case apievents.TypeGraphNodeCompleted:
			var p apievents.GraphNodeCompletedProps
			if err := env.Decode(&p); err == nil {
				out = append(out, actorEntry{evType: env.Type, nodeID: p.NodeID, actor: p.Actor})
			}
		case apievents.TypeGraphNodePaused:
			var p apievents.GraphNodePausedProps
			if err := env.Decode(&p); err == nil {
				out = append(out, actorEntry{evType: env.Type, nodeID: p.NodeID, actor: p.Actor})
			}
		case apievents.TypeGraphNodeFailed:
			var p apievents.GraphNodeFailedProps
			if err := env.Decode(&p); err == nil {
				out = append(out, actorEntry{evType: env.Type, nodeID: p.NodeID, actor: p.Actor})
			}
		}
	}
	return out
}
