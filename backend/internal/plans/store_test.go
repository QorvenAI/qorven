// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package plans_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/testutil"
)

// These tests hit a real Postgres. They are skipped when a test DB is
// not reachable. The schema at migration 037 or newer must be applied.

func seedTenant(t *testing.T, ctx context.Context, pool interface {
	Exec(ctx context.Context, sql string, args ...any) (pgtag, error)
}) {
	// No-op; the tenant seed lives in migration 001. We assume it is
	// already present in the test DB.
	_ = t
	_ = ctx
	_ = pool
}

// pgtag is a stand-in for pgconn.CommandTag — we do not depend on it.
type pgtag = any

func TestPlans_CreateGet(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := plans.NewStore(pool)

	p, err := s.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID:  testutil.TestTenantID,
		Title:     "test plan " + testutil.TempID("p"),
		CreatedBy: "test-actor",
		Spec:      map[string]string{"stack": "next"},
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	if p.Status != plans.StatusDraft {
		t.Fatalf("status: %s", p.Status)
	}

	got, err := s.GetPlan(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if got.Title != p.Title {
		t.Fatalf("roundtrip title: got %q want %q", got.Title, p.Title)
	}
	var spec map[string]string
	if err := json.Unmarshal(got.Spec, &spec); err != nil {
		t.Fatalf("spec unmarshal: %v", err)
	}
	if spec["stack"] != "next" {
		t.Fatalf("spec roundtrip: %+v", spec)
	}
}

func TestPlans_StatusTransitions(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := plans.NewStore(pool)

	p, err := s.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: testutil.TestTenantID,
		Title:    "status test " + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Legal: draft → pending_approval
	if _, err := s.UpdatePlanStatus(ctx, p.ID, plans.StatusPendingApproval); err != nil {
		t.Fatalf("draft→pending_approval: %v", err)
	}
	// Illegal: pending_approval → done
	if _, err := s.UpdatePlanStatus(ctx, p.ID, plans.StatusDone); err == nil {
		t.Fatalf("pending_approval→done must be illegal")
	} else if !plans.IsIllegalTransition(err) {
		t.Fatalf("expected IllegalTransition, got %T: %v", err, err)
	}
	// Legal: approved → running → done
	if _, err := s.UpdatePlanStatus(ctx, p.ID, plans.StatusApproved); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := s.UpdatePlanStatus(ctx, p.ID, plans.StatusRunning); err != nil {
		t.Fatalf("running: %v", err)
	}
	if _, err := s.UpdatePlanStatus(ctx, p.ID, plans.StatusDone); err != nil {
		t.Fatalf("done: %v", err)
	}
	// Terminal: done → running must fail
	if _, err := s.UpdatePlanStatus(ctx, p.ID, plans.StatusRunning); err == nil {
		t.Fatalf("done is terminal")
	}
}

func TestPlans_Nodes(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := plans.NewStore(pool)

	p, err := s.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: testutil.TestTenantID, Title: "nodes " + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	planner, err := s.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindPlanner, Title: "plan",
	})
	if err != nil {
		t.Fatalf("AppendNode planner: %v", err)
	}
	approve, err := s.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, ParentID: planner.ID, Kind: plans.KindHumanFeedback, Title: "approve",
	})
	if err != nil {
		t.Fatalf("AppendNode approve: %v", err)
	}
	build, err := s.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, ParentID: approve.ID, Kind: plans.KindAgentTask,
		Title: "build", AssigneeSoul: "frontend-dev",
	})
	if err != nil {
		t.Fatalf("AppendNode build: %v", err)
	}

	// Transitions
	if _, err := s.UpdateNodeState(ctx, plans.UpdateNodeStateInput{
		NodeID: build.ID, Next: plans.NodeRunning,
	}); err != nil {
		t.Fatalf("pending→running: %v", err)
	}
	upd, err := s.UpdateNodeState(ctx, plans.UpdateNodeStateInput{
		NodeID: build.ID, Next: plans.NodeDone,
		Artifacts: json.RawMessage(`{"files_written": 3}`),
	})
	if err != nil {
		t.Fatalf("running→done: %v", err)
	}
	if upd.EndedAt == nil {
		t.Fatalf("ended_at not set on done")
	}
	var art map[string]any
	if err := json.Unmarshal(upd.Artifacts, &art); err != nil {
		t.Fatalf("artifacts unmarshal: %v", err)
	}
	if art["files_written"] == nil {
		t.Fatalf("artifacts merge lost data: %+v", art)
	}

	// done → running must fail
	if _, err := s.UpdateNodeState(ctx, plans.UpdateNodeStateInput{
		NodeID: build.ID, Next: plans.NodeRunning,
	}); err == nil {
		t.Fatalf("done is terminal")
	}

	// List
	nodes, err := s.ListNodesByPlan(ctx, p.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}
}

func TestPlans_Edges(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := plans.NewStore(pool)

	p, err := s.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: testutil.TestTenantID, Title: "edges " + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	a, _ := s.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindPlanner, Title: "a"})
	b, _ := s.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindHumanFeedback, Title: "b"})
	c, _ := s.AppendNode(ctx, plans.AppendNodeInput{PlanID: p.ID, Kind: plans.KindAgentTask, Title: "c"})

	if _, err := s.AddEdge(ctx, p.ID, a.ID, b.ID, plans.CondAlways); err != nil {
		t.Fatalf("AddEdge a→b: %v", err)
	}
	if _, err := s.AddEdge(ctx, p.ID, b.ID, c.ID, plans.CondApproved); err != nil {
		t.Fatalf("AddEdge b→c approved: %v", err)
	}
	// Idempotency — same edge twice yields one row.
	if _, err := s.AddEdge(ctx, p.ID, a.ID, b.ID, plans.CondAlways); err != nil {
		t.Fatalf("AddEdge idem: %v", err)
	}

	edges, err := s.ListEdgesByPlan(ctx, p.ID)
	if err != nil {
		t.Fatalf("ListEdges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}

	outgoingFromB, err := s.OutgoingEdges(ctx, p.ID, b.ID)
	if err != nil {
		t.Fatalf("OutgoingEdges: %v", err)
	}
	if len(outgoingFromB) != 1 || outgoingFromB[0].Condition != plans.CondApproved {
		t.Fatalf("outgoing: %+v", outgoingFromB)
	}
}

func TestPlans_NotFound(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := plans.NewStore(pool)
	if _, err := s.GetPlan(ctx, "00000000-0000-0000-0000-00000000ffff"); err != plans.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
