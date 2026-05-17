// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package approvals_test

import (
	"testing"

	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/testutil"
)

func newHumanFeedbackNode(t *testing.T) (*plans.Store, *plans.Plan, *plans.Node) {
	t.Helper()
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	ps := plans.NewStore(pool)

	p, err := ps.CreatePlan(ctx, plans.CreatePlanInput{
		TenantID: testutil.TestTenantID,
		Title:    "approval-test-" + testutil.TempID("p"),
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	n, err := ps.AppendNode(ctx, plans.AppendNodeInput{
		PlanID: p.ID, Kind: plans.KindHumanFeedback, Title: "approve",
	})
	if err != nil {
		t.Fatalf("AppendNode: %v", err)
	}
	return ps, p, n
}

func TestApprovals_RequestIdempotent(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	_, p, n := newHumanFeedbackNode(t)
	s := approvals.NewStore(pool)

	a1, err := s.Request(ctx, p.ID, n.ID, "user-1", nil)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	a2, err := s.Request(ctx, p.ID, n.ID, "user-2", nil)
	if err != nil {
		t.Fatalf("Request idempotent: %v", err)
	}
	if a1.ID != a2.ID {
		t.Fatalf("second Request should return same pending row: %s vs %s", a1.ID, a2.ID)
	}
	if a2.State != approvals.StatePending {
		t.Fatalf("state: %s", a2.State)
	}
}

func TestApprovals_Resolve_Approved(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	_, p, n := newHumanFeedbackNode(t)
	s := approvals.NewStore(pool)

	a, err := s.Request(ctx, p.ID, n.ID, "user-1", nil)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	got, err := s.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: a.ID, Next: approvals.StateApproved, ResolvedBy: "user-2", Comment: "lgtm",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.State != approvals.StateApproved {
		t.Fatalf("state: %s", got.State)
	}
	if got.ResolvedBy != "user-2" {
		t.Fatalf("resolved_by: %s", got.ResolvedBy)
	}
	comments, _ := s.ListComments(ctx, a.ID)
	if len(comments) != 1 || comments[0].Body != "lgtm" {
		t.Fatalf("comments: %+v", comments)
	}
}

func TestApprovals_Resolve_Idempotent(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	_, p, n := newHumanFeedbackNode(t)
	s := approvals.NewStore(pool)

	a, _ := s.Request(ctx, p.ID, n.ID, "user-1", nil)
	if _, err := s.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: a.ID, Next: approvals.StateApproved, ResolvedBy: "user-1",
	}); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	// Second approve must succeed as a no-op.
	got, err := s.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: a.ID, Next: approvals.StateApproved, ResolvedBy: "user-1",
	})
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if got.State != approvals.StateApproved {
		t.Fatalf("state: %s", got.State)
	}
}

func TestApprovals_Resolve_IllegalTransition(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	_, p, n := newHumanFeedbackNode(t)
	s := approvals.NewStore(pool)

	a, _ := s.Request(ctx, p.ID, n.ID, "user-1", nil)
	// approve, then try to reject — rejected-after-approved is illegal.
	if _, err := s.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: a.ID, Next: approvals.StateApproved, ResolvedBy: "user-1",
	}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := s.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: a.ID, Next: approvals.StateRejected, ResolvedBy: "user-2",
	}); err == nil {
		t.Fatalf("rejected-after-approved must be illegal")
	} else if !approvals.IsIllegalTransition(err) {
		t.Fatalf("expected IllegalTransition, got %T: %v", err, err)
	}
}

func TestApprovals_RevisionRequested(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	_, p, n := newHumanFeedbackNode(t)
	s := approvals.NewStore(pool)

	a, _ := s.Request(ctx, p.ID, n.ID, "user-1", nil)
	got, err := s.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: a.ID, Next: approvals.StateRevisionRequested, ResolvedBy: "reviewer",
		Comment: "use Next.js not Remix",
	})
	if err != nil {
		t.Fatalf("revision: %v", err)
	}
	if got.State != approvals.StateRevisionRequested {
		t.Fatalf("state: %s", got.State)
	}
}

func TestApprovals_CommentsThread(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	_, p, n := newHumanFeedbackNode(t)
	s := approvals.NewStore(pool)

	a, _ := s.Request(ctx, p.ID, n.ID, "user-1", nil)
	if _, err := s.AppendComment(ctx, a.ID, "user-1", "user", "first look"); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := s.AppendComment(ctx, a.ID, "prime", "agent", "here is why"); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := s.AppendComment(ctx, a.ID, "x", "alien", "x"); err == nil {
		t.Fatalf("invalid author_is must fail")
	}
	out, err := s.ListComments(ctx, a.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(out))
	}
	if out[0].AuthorIs != "user" || out[1].AuthorIs != "agent" {
		t.Fatalf("authors: %+v", out)
	}
}

func TestApprovals_InvalidTargetState(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	_, p, n := newHumanFeedbackNode(t)
	s := approvals.NewStore(pool)

	a, _ := s.Request(ctx, p.ID, n.ID, "user-1", nil)
	if _, err := s.Resolve(ctx, approvals.ResolveInput{
		ApprovalID: a.ID, Next: "garbled", ResolvedBy: "u",
	}); err == nil {
		t.Fatalf("invalid target state must fail")
	}
}

func TestApprovals_NotFound(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := testutil.Ctx(t)
	s := approvals.NewStore(pool)
	if _, err := s.Get(ctx, "00000000-0000-0000-0000-000000000000"); err != approvals.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
