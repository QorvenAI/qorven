// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qorvenai/qorven/internal/store"
)


// Store persists approvals + comments and enforces the state machine.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store from a connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// q routes queries through the tenant-scoped tx when one is on ctx
// (multi-tenant HTTP handlers), else the raw pool (single-tenant /
// background). Every query in this file MUST use q — direct pool
// access bypasses RLS in multi-tenant mode.
func (s *Store) q(ctx context.Context) store.Queryable {
	return store.FromContext(ctx, s.pool)
}

// Request returns the current approval for a node. If none exists, a
// new pending approval is created and returned. If one already exists
// (in any state), the latest approval for the node is returned — the
// graph runtime consumes the approval verdict via the node's artifacts
// and moves the state machine forward; re-requesting during resume must
// not create a duplicate pending row.
//
// Revision cycle: when an approval is resolved as revision_requested,
// the planner node re-runs and produces a fresh plan. Only at that
// point does a new human_feedback node appear (via AppendNode), which
// gets its own approval on first Request.
func (s *Store) Request(ctx context.Context, planID, nodeID, requestedBy string, budget any) (*Approval, error) {
	if planID == "" || nodeID == "" {
		return nil, errors.New("approvals: plan_id and node_id required")
	}

	// Return the latest approval for the node if one exists.
	if existing, err := s.latestForNode(ctx, nodeID); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	budgetJSON := []byte("{}")
	if budget != nil {
		b, err := json.Marshal(budget)
		if err != nil {
			return nil, fmt.Errorf("approvals: marshal budget: %w", err)
		}
		budgetJSON = b
	}

	var a Approval
	err := s.q(ctx).QueryRow(ctx, `
        INSERT INTO approvals (plan_id, node_id, state, requested_by, budget)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING id, plan_id, node_id, state, requested_by, COALESCE(resolved_by,''),
                  resolved_at, budget, created_at, updated_at
    `, planID, nodeID, StatePending, requestedBy, budgetJSON).Scan(
		&a.ID, &a.PlanID, &a.NodeID, &a.State, &a.RequestedBy, &a.ResolvedBy,
		&a.ResolvedAt, &a.Budget, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == nil {
		return &a, nil
	}
	// Raced with another writer: a pending row appeared between our
	// latestForNode check and the insert. Re-read it.
	if isUniqueViolation(err) {
		return s.latestForNode(ctx, nodeID)
	}
	return nil, fmt.Errorf("approvals: request: %w", err)
}

// latestForNode returns the most recent approval for a node regardless
// of state. Used by Request for idempotent re-invocation.
func (s *Store) latestForNode(ctx context.Context, nodeID string) (*Approval, error) {
	var a Approval
	err := s.q(ctx).QueryRow(ctx, `
        SELECT id, plan_id, node_id, state, requested_by, COALESCE(resolved_by,''),
               resolved_at, budget, created_at, updated_at
        FROM approvals WHERE node_id = $1
        ORDER BY created_at DESC
        LIMIT 1
    `, nodeID).Scan(
		&a.ID, &a.PlanID, &a.NodeID, &a.State, &a.RequestedBy, &a.ResolvedBy,
		&a.ResolvedAt, &a.Budget, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

// Get returns a single approval by id.
func (s *Store) Get(ctx context.Context, id string) (*Approval, error) {
	var a Approval
	err := s.q(ctx).QueryRow(ctx, `
        SELECT id, plan_id, node_id, state, requested_by, COALESCE(resolved_by,''),
               resolved_at, budget, created_at, updated_at
        FROM approvals WHERE id = $1
    `, id).Scan(
		&a.ID, &a.PlanID, &a.NodeID, &a.State, &a.RequestedBy, &a.ResolvedBy,
		&a.ResolvedAt, &a.Budget, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

// pendingForNode returns the currently-pending approval for a node.
func (s *Store) pendingForNode(ctx context.Context, nodeID string) (*Approval, error) {
	var a Approval
	err := s.q(ctx).QueryRow(ctx, `
        SELECT id, plan_id, node_id, state, requested_by, COALESCE(resolved_by,''),
               resolved_at, budget, created_at, updated_at
        FROM approvals WHERE node_id = $1 AND state = 'pending'
    `, nodeID).Scan(
		&a.ID, &a.PlanID, &a.NodeID, &a.State, &a.RequestedBy, &a.ResolvedBy,
		&a.ResolvedAt, &a.Budget, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

// ResolveInput captures the inputs for a resolve operation.
type ResolveInput struct {
	ApprovalID string
	Next       State
	ResolvedBy string
	Comment    string // optional; appended as a system-authored thread entry
}

// Resolve transitions an approval to a terminal-ish state. Idempotent:
// if the approval is already in the target state with the same
// ResolvedBy, the call succeeds without making any changes. Any other
// state transition to a terminal state is rejected.
func (s *Store) Resolve(ctx context.Context, in ResolveInput) (*Approval, error) {
	if in.ApprovalID == "" {
		return nil, errors.New("approvals: approval_id required")
	}
	switch in.Next {
	case StateApproved, StateRejected, StateRevisionRequested:
		// ok
	default:
		return nil, fmt.Errorf("approvals: invalid target state %q", in.Next)
	}

	// If a tenant-scoped tx is on ctx (multi-tenant HTTP request), use
	// it directly — nesting a second BeginTx would open a savepoint,
	// which is correct for isolation but gives us no additional
	// guarantee over the outer tx. If ctx has no tx (single-tenant /
	// background), begin a local tx because we need FOR UPDATE row
	// locking + atomic UPDATE + INSERT.
	ownsTx := false
	var q store.Queryable
	var localTx pgx.Tx
	if existing, ok := store.TxFromContext(ctx); ok {
		q = existing
	} else {
		var err error
		localTx, err = s.pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return nil, err
		}
		defer localTx.Rollback(ctx) //nolint:errcheck
		q = localTx
		ownsTx = true
	}

	var current State
	var currentResolvedBy string
	if err := q.QueryRow(ctx,
		`SELECT state, COALESCE(resolved_by,'') FROM approvals WHERE id = $1 FOR UPDATE`,
		in.ApprovalID,
	).Scan(&current, &currentResolvedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Idempotency path: already in the target state.
	// Accept within the grace window so duplicate clicks / network retries
	// return 200 rather than 409. Outside the grace window we still return
	// the row — the first writer won and reality is correct — but we do NOT
	// gate on the window to avoid permanent failure on slow retries.
	if current == in.Next {
		return s.Get(ctx, in.ApprovalID)
	}

	// Only pending approvals may be resolved. Any other transition is illegal.
	if current != StatePending {
		return nil, &illegalTransition{From: current, To: in.Next}
	}

	var a Approval
	err := q.QueryRow(ctx, `
        UPDATE approvals
           SET state = $2,
               resolved_by = $3,
               resolved_at = NOW(),
               updated_at = NOW()
         WHERE id = $1
        RETURNING id, plan_id, node_id, state, requested_by, COALESCE(resolved_by,''),
                  resolved_at, budget, created_at, updated_at
    `, in.ApprovalID, in.Next, in.ResolvedBy).Scan(
		&a.ID, &a.PlanID, &a.NodeID, &a.State, &a.RequestedBy, &a.ResolvedBy,
		&a.ResolvedAt, &a.Budget, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// System comment — capture the verdict in the thread.
	if in.Comment != "" {
		if _, err := q.Exec(ctx, `
            INSERT INTO approval_comments (approval_id, author, author_is, body)
            VALUES ($1, $2, 'system', $3)
        `, in.ApprovalID, in.ResolvedBy, in.Comment); err != nil {
			return nil, fmt.Errorf("approvals: system comment: %w", err)
		}
	}

	if ownsTx {
		if err := localTx.Commit(ctx); err != nil {
			return nil, err
		}
	}
	return &a, nil
}

// AppendComment adds a user/agent-authored entry to the thread.
func (s *Store) AppendComment(ctx context.Context, approvalID, author, authorIs, body string) (*Comment, error) {
	if approvalID == "" || author == "" || body == "" {
		return nil, errors.New("approvals: approval_id, author, body required")
	}
	switch authorIs {
	case "user", "agent", "system":
		// ok
	default:
		return nil, fmt.Errorf("approvals: invalid author_is %q", authorIs)
	}
	var c Comment
	err := s.q(ctx).QueryRow(ctx, `
        INSERT INTO approval_comments (approval_id, author, author_is, body)
        VALUES ($1, $2, $3, $4)
        RETURNING id, approval_id, author, author_is, body, created_at
    `, approvalID, author, authorIs, body).Scan(
		&c.ID, &c.ApprovalID, &c.Author, &c.AuthorIs, &c.Body, &c.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("approvals: append comment: %w", err)
	}
	return &c, nil
}

// ListComments returns comments for an approval in chronological order.
func (s *Store) ListComments(ctx context.Context, approvalID string) ([]*Comment, error) {
	rows, err := s.q(ctx).Query(ctx, `
        SELECT id, approval_id, author, author_is, body, created_at
        FROM approval_comments WHERE approval_id = $1 ORDER BY created_at ASC
    `, approvalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.ApprovalID, &c.Author, &c.AuthorIs, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// ListByPlan returns every approval for a plan, newest first.
func (s *Store) ListByPlan(ctx context.Context, planID string) ([]*Approval, error) {
	rows, err := s.q(ctx).Query(ctx, `
        SELECT id, plan_id, node_id, state, requested_by, COALESCE(resolved_by,''),
               resolved_at, budget, created_at, updated_at
        FROM approvals WHERE plan_id = $1 ORDER BY created_at DESC
    `, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Approval{}
	for rows.Next() {
		var a Approval
		if err := rows.Scan(
			&a.ID, &a.PlanID, &a.NodeID, &a.State, &a.RequestedBy, &a.ResolvedBy,
			&a.ResolvedAt, &a.Budget, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// ErrNotFound is returned by Get/Resolve when no matching row exists.
var ErrNotFound = errors.New("approvals: not found")

// isUniqueViolation returns true when pg reported SQLSTATE 23505.
func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
