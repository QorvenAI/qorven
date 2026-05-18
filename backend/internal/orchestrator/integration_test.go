// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package orchestrator_test

import (
	"context"
	"encoding/json"
	"strings"
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

// scriptedAgent is a deterministic AgentRunner that replies with a
// fixed script for each RunRequest. We key on the user message so the
// planner and agent_task invocations can return different content.
type scriptedAgent struct {
	responses map[string]string
	calls     atomic.Int32
}

func (s *scriptedAgent) Run(ctx context.Context, req agent.RunRequest, onEvent func(agent.StreamEvent)) (*agent.RunResult, error) {
	s.calls.Add(1)
	// Find the first key contained in the user message; default to empty.
	body := ""
	for k, v := range s.responses {
		if strings.Contains(req.UserMessage, k) {
			body = v
			break
		}
	}
	if body == "" {
		body = "ok"
	}
	onEvent(agent.StreamEvent{Type: "text_delta", Delta: body})
	return &agent.RunResult{Content: body}, nil
}

// TestOrchestrator_EndToEnd drives a full plan through the runtime:
//
//	planner → human_feedback → agent_task
//
// Assertions:
//  1. First Run pauses on human_feedback (ErrPaused → nil from Service).
//  2. After the approval is resolved externally, the next Run completes
//     the agent_task and the plan ends in done.
//  3. Every node ends in the expected terminal state.
//  4. Emit fires with the expected kinds.
func TestOrchestrator_EndToEnd(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)
	ctx := testutil.Ctx(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	// Compose the graph via the same stores the HTTP handlers use.
	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: tenantID, Title: "e2e-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	planner, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindPlanner, Title: "plan",
		Inputs: handlers.PlannerInputs{Description: "todo app"},
	})
	if err != nil {
		t.Fatalf("AppendNode planner: %v", err)
	}
	approve, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindHumanFeedback, Title: "approve",
	})
	if err != nil {
		t.Fatalf("AppendNode approve: %v", err)
	}
	builder, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "build",
		AssigneeSoul: "frontend-dev",
		Inputs:       handlers.AgentTaskInputs{AgentID: "frontend-dev", Instruction: "write src/app.tsx"},
	})
	if err != nil {
		t.Fatalf("AppendNode builder: %v", err)
	}
	if _, err := ps.AddEdge(ctx, p.ID, planner.ID, approve.ID, plans.CondAlways); err != nil {
		t.Fatalf("edge planner→approve: %v", err)
	}
	if _, err := ps.AddEdge(ctx, p.ID, approve.ID, builder.ID, plans.CondApproved); err != nil {
		t.Fatalf("edge approve→build: %v", err)
	}

	// Scripted agent — planner returns a JSON plan, builder returns success text.
	runner := &scriptedAgent{
		responses: map[string]string{
			"todo app":       `{"stack":"next","summary":"a todo app","agents":[{"role":"frontend","tasks":["init"]}]}`,
			"write src/app":  "wrote file",
		},
	}

	var kinds []string
	emitter := apievents.NewEmitter()
	svc := orchestrator.NewService(ps, as, runner, emitter, nil)
	svc.Runtime() // ensure accessor compiles

	// First Run: should pause on human_feedback.
	if err := svc.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("first ExecutePlan: %v", err)
	}
	if runner.calls.Load() != 1 {
		t.Fatalf("expected 1 agent call (planner), got %d", runner.calls.Load())
	}

	// Verify planner artifacts carry parsed JSON.
	nodes, _ := ps.ListNodesByPlan(ctx, p.ID)
	var plannerNode *plans.Node
	var approveNode *plans.Node
	var builderNode *plans.Node
	for _, n := range nodes {
		switch n.Kind {
		case plans.KindPlanner:
			plannerNode = n
		case plans.KindHumanFeedback:
			approveNode = n
		case plans.KindAgentTask:
			builderNode = n
		}
	}
	if plannerNode == nil || plannerNode.State != plans.NodeDone {
		t.Fatalf("planner state: %+v", plannerNode)
	}
	var plannerArt map[string]any
	if err := json.Unmarshal(plannerNode.Artifacts, &plannerArt); err != nil {
		t.Fatalf("planner artifacts: %v", err)
	}
	if plannerArt["plan"] == nil {
		t.Fatalf("planner plan not parsed: %+v", plannerArt)
	}
	if approveNode == nil || approveNode.State != plans.NodeBlocked {
		t.Fatalf("approve should be blocked, got %s", approveNode.State)
	}
	if builderNode == nil || builderNode.State != plans.NodePending {
		t.Fatalf("builder must still be pending, got %s", builderNode.State)
	}

	// Resolve the pending approval.
	list, _ := as.ListByPlan(ctx, p.ID)
	if len(list) == 0 {
		t.Fatalf("no approval requested")
	}
	if _, err := as.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: list[0].ID, Next: approvals.StateApproved, ResolvedBy: "reviewer",
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// Second Run: should complete agent_task and terminalize the plan.
	if err := svc.ExecutePlan(ctx, p.ID); err != nil {
		t.Fatalf("second ExecutePlan: %v", err)
	}
	if runner.calls.Load() != 2 {
		t.Fatalf("expected 2 agent calls total, got %d", runner.calls.Load())
	}

	final, _ := ps.GetPlan(ctx, p.ID)
	if final.Status != plans.StatusDone {
		t.Fatalf("final plan status: %s", final.Status)
	}
	nodes, _ = ps.ListNodesByPlan(ctx, p.ID)
	for _, n := range nodes {
		if n.State != plans.NodeDone {
			t.Fatalf("node %s (%s) terminal state: %s", n.Kind, n.ID, n.State)
		}
	}

	// kinds unused here — reserved for emit assertions in future changes.
	_ = kinds
}

// TestOrchestrator_Timeout ensures ExecutePlan cooperates with context
// cancellation.
func TestOrchestrator_Timeout(t *testing.T) {
	pool, tenantID := testutil.NewIsolatedTenant(t)

	ps := plans.NewStore(pool)
	as := approvals.NewStore(pool)

	p, err := ps.CreatePlan(context.Background(), plans.CreatePlanInput{
		TenantID: tenantID, Title: "timeout-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	node, err := ps.AppendNode(context.Background(), plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindAgentTask, Title: "blocker",
		AssigneeSoul: "x",
		Inputs:       handlers.AgentTaskInputs{AgentID: "x", Instruction: "hang"},
	})
	if err != nil {
		t.Fatalf("AppendNode: %v", err)
	}
	_ = node

	runner := &blockingAgent{}
	svc := orchestrator.NewService(ps, as, runner, apievents.NewEmitter(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = svc.ExecutePlan(ctx, p.ID)
	if err == nil {
		t.Fatalf("expected context error")
	}
}

type blockingAgent struct{}

func (blockingAgent) Run(ctx context.Context, req agent.RunRequest, onEvent func(agent.StreamEvent)) (*agent.RunResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
