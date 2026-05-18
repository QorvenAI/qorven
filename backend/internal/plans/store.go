// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package plans

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qorvenai/qorven/internal/store"
)

// Store wraps pgxpool with typed plan/node/edge operations.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store from a connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// q picks the tenant-scoped tx from ctx when one is present (multi-
// tenant HTTP handler after TenantScopeMiddleware), or falls back to
// the raw pool (single-tenant / background jobs / tests). Every query
// method in this file MUST go through q — direct s.pool.Query calls
// bypass RLS in multi-tenant mode.
func (s *Store) q(ctx context.Context) store.Queryable {
	return store.FromContext(ctx, s.pool)
}

// ─────────────────────────── Plan operations ────────────────────────────

// CreatePlanInput is the minimal payload to start a new plan.
type CreatePlanInput struct {
	TenantID   string
	ProjectID  string
	SessionID  string
	Title      string
	Summary    string
	CreatedBy  string
	Spec       any // marshaled to JSON; pass nil for {}
}

// CreatePlan inserts a draft plan and returns its id.
func (s *Store) CreatePlan(ctx context.Context, in CreatePlanInput) (*Plan, error) {
	if in.TenantID == "" {
		return nil, errors.New("plans: tenant_id required")
	}
	specJSON := []byte("{}")
	if in.Spec != nil {
		b, err := json.Marshal(in.Spec)
		if err != nil {
			return nil, fmt.Errorf("plans: marshal spec: %w", err)
		}
		specJSON = b
	}

	var p Plan
	err := s.q(ctx).QueryRow(ctx, `
        INSERT INTO plans (tenant_id, project_id, session_id, title, status, spec, summary, created_by)
        VALUES ($1, NULLIF($2,'')::uuid, NULLIF($3,'')::uuid, $4, $5, $6, $7, $8)
        RETURNING id, tenant_id, COALESCE(project_id::text,''), COALESCE(session_id::text,''),
                  title, status, spec, COALESCE(summary,''), created_by, created_at, updated_at, archived_at
    `,
		in.TenantID, in.ProjectID, in.SessionID, in.Title,
		StatusDraft, specJSON, in.Summary, in.CreatedBy,
	).Scan(
		&p.ID, &p.TenantID, &p.ProjectID, &p.SessionID,
		&p.Title, &p.Status, &p.Spec, &p.Summary, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("plans: insert: %w", err)
	}
	return &p, nil
}

// GetPlan returns a single plan by id.
func (s *Store) GetPlan(ctx context.Context, id string) (*Plan, error) {
	var p Plan
	err := s.q(ctx).QueryRow(ctx, `
        SELECT id, tenant_id, COALESCE(project_id::text,''), COALESCE(session_id::text,''),
               title, status, spec, COALESCE(summary,''), created_by, created_at, updated_at, archived_at
        FROM plans WHERE id = $1
    `, id).Scan(
		&p.ID, &p.TenantID, &p.ProjectID, &p.SessionID,
		&p.Title, &p.Status, &p.Spec, &p.Summary, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

// UpdatePlanStatus transitions a plan's status. Rejects illegal moves
// before hitting the DB. Returns the fresh Plan on success.
func (s *Store) UpdatePlanStatus(ctx context.Context, id string, next PlanStatus) (*Plan, error) {
	// Read current status in a transaction so we can gate the transition
	// atomically. The DB CHECK constraint is a backstop but the in-proc
	// validation produces a clearer error.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var current PlanStatus
	if err := tx.QueryRow(ctx, `SELECT status FROM plans WHERE id = $1 FOR UPDATE`, id).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !current.ValidTransition(next) {
		return nil, &illegalTransition{Entity: "plan_status", From: string(current), To: string(next)}
	}
	var p Plan
	err = tx.QueryRow(ctx, `
        UPDATE plans SET status = $2, updated_at = NOW()
        WHERE id = $1
        RETURNING id, tenant_id, COALESCE(project_id::text,''), COALESCE(session_id::text,''),
                  title, status, spec, COALESCE(summary,''), created_by, created_at, updated_at, archived_at
    `, id, next).Scan(
		&p.ID, &p.TenantID, &p.ProjectID, &p.SessionID,
		&p.Title, &p.Status, &p.Spec, &p.Summary, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListByTenant returns all plans for a tenant in reverse-chronological order.
func (s *Store) ListByTenant(ctx context.Context, tenantID string, limit int) ([]*Plan, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q(ctx).Query(ctx, `
        SELECT id, tenant_id, COALESCE(project_id::text,''), COALESCE(session_id::text,''),
               title, status, spec, COALESCE(summary,''), created_by, created_at, updated_at, archived_at
        FROM plans WHERE tenant_id = $1 AND archived_at IS NULL ORDER BY created_at DESC LIMIT $2
    `, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Plan{}
	for rows.Next() {
		var p Plan
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.ProjectID, &p.SessionID,
			&p.Title, &p.Status, &p.Spec, &p.Summary, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// ListPlansBySession returns the plans attached to a given session in
// reverse-chronological order.
func (s *Store) ListPlansBySession(ctx context.Context, sessionID string) ([]*Plan, error) {
	rows, err := s.q(ctx).Query(ctx, `
        SELECT id, tenant_id, COALESCE(project_id::text,''), COALESCE(session_id::text,''),
               title, status, spec, COALESCE(summary,''), created_by, created_at, updated_at, archived_at
        FROM plans WHERE session_id = $1::uuid AND archived_at IS NULL ORDER BY created_at DESC
    `, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Plan{}
	for rows.Next() {
		var p Plan
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.ProjectID, &p.SessionID,
			&p.Title, &p.Status, &p.Spec, &p.Summary, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// ArchivePlan moves a terminal plan to the archived state. Only plans
// with status done, failed, cancelled, or rejected may be archived.
// Returns ErrNotFound when the plan id does not exist.
func (s *Store) ArchivePlan(ctx context.Context, id string) (*Plan, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var current PlanStatus
	if err := tx.QueryRow(ctx, `SELECT status FROM plans WHERE id = $1 FOR UPDATE`, id).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !current.ValidTransition(StatusArchived) {
		return nil, &illegalTransition{Entity: "plan_status", From: string(current), To: string(StatusArchived)}
	}
	var p Plan
	err = tx.QueryRow(ctx, `
        UPDATE plans
           SET status = 'archived', archived_at = NOW(), updated_at = NOW()
         WHERE id = $1
        RETURNING id, tenant_id, COALESCE(project_id::text,''), COALESCE(session_id::text,''),
                  title, status, spec, COALESCE(summary,''), created_by, created_at, updated_at, archived_at
    `, id).Scan(
		&p.ID, &p.TenantID, &p.ProjectID, &p.SessionID,
		&p.Title, &p.Status, &p.Spec, &p.Summary, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &p, nil
}

// ListArchivedByTenant returns archived plans for a tenant in reverse
// archival order, capped at limit rows.
func (s *Store) ListArchivedByTenant(ctx context.Context, tenantID string, limit int) ([]*Plan, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q(ctx).Query(ctx, `
        SELECT id, tenant_id, COALESCE(project_id::text,''), COALESCE(session_id::text,''),
               title, status, spec, COALESCE(summary,''), created_by, created_at, updated_at, archived_at
        FROM plans
        WHERE tenant_id = $1 AND archived_at IS NOT NULL
        ORDER BY archived_at DESC LIMIT $2
    `, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Plan{}
	for rows.Next() {
		var p Plan
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.ProjectID, &p.SessionID,
			&p.Title, &p.Status, &p.Spec, &p.Summary, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// ─────────────────────────── Node operations ────────────────────────────

// AppendNodeInput is the minimal payload to add a plan node.
type AppendNodeInput struct {
	PlanID       string
	ParentID     string
	Kind         NodeKind
	Title        string
	AssigneeSoul string
	Inputs       any
}

// AppendNode inserts a pending node under the plan.
func (s *Store) AppendNode(ctx context.Context, in AppendNodeInput) (*Node, error) {
	if in.PlanID == "" {
		return nil, errors.New("plans: plan_id required")
	}
	if in.Kind == "" {
		return nil, errors.New("plans: kind required")
	}
	inputsJSON := []byte("{}")
	if in.Inputs != nil {
		b, err := json.Marshal(in.Inputs)
		if err != nil {
			return nil, fmt.Errorf("plans: marshal inputs: %w", err)
		}
		inputsJSON = b
	}
	var n Node
	err := s.q(ctx).QueryRow(ctx, `
        INSERT INTO plan_nodes (plan_id, parent_id, kind, title, state, assignee_soul, inputs)
        VALUES ($1, NULLIF($2,'')::uuid, $3, $4, $5, NULLIF($6,''), $7)
        RETURNING id, plan_id, COALESCE(parent_id::text,''), kind, title, state,
                  COALESCE(assignee_soul,''), inputs, artifacts, COALESCE(error,''),
                  started_at, ended_at, created_at, updated_at
    `,
		in.PlanID, in.ParentID, in.Kind, in.Title, NodePending,
		in.AssigneeSoul, inputsJSON,
	).Scan(
		&n.ID, &n.PlanID, &n.ParentID, &n.Kind, &n.Title, &n.State,
		&n.AssigneeSoul, &n.Inputs, &n.Artifacts, &n.Error,
		&n.StartedAt, &n.EndedAt, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("plans: insert node: %w", err)
	}
	return &n, nil
}

// GetNode returns a node by id.
func (s *Store) GetNode(ctx context.Context, id string) (*Node, error) {
	var n Node
	err := s.q(ctx).QueryRow(ctx, `
        SELECT id, plan_id, COALESCE(parent_id::text,''), kind, title, state,
               COALESCE(assignee_soul,''), inputs, artifacts, COALESCE(error,''),
               started_at, ended_at, created_at, updated_at
        FROM plan_nodes WHERE id = $1
    `, id).Scan(
		&n.ID, &n.PlanID, &n.ParentID, &n.Kind, &n.Title, &n.State,
		&n.AssigneeSoul, &n.Inputs, &n.Artifacts, &n.Error,
		&n.StartedAt, &n.EndedAt, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &n, nil
}

// ListNodesByPlan returns every node for a plan in insertion order.
func (s *Store) ListNodesByPlan(ctx context.Context, planID string) ([]*Node, error) {
	rows, err := s.q(ctx).Query(ctx, `
        SELECT id, plan_id, COALESCE(parent_id::text,''), kind, title, state,
               COALESCE(assignee_soul,''), inputs, artifacts, COALESCE(error,''),
               started_at, ended_at, created_at, updated_at
        FROM plan_nodes WHERE plan_id = $1 ORDER BY created_at ASC
    `, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Node{}
	for rows.Next() {
		var n Node
		if err := rows.Scan(
			&n.ID, &n.PlanID, &n.ParentID, &n.Kind, &n.Title, &n.State,
			&n.AssigneeSoul, &n.Inputs, &n.Artifacts, &n.Error,
			&n.StartedAt, &n.EndedAt, &n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &n)
	}
	return out, rows.Err()
}

// UpdateNodeStateInput carries an in-flight update for a node.
type UpdateNodeStateInput struct {
	NodeID    string
	Next      NodeState
	Error     string          // set when Next == NodeFailed
	Artifacts json.RawMessage // optional; merged via ||
}

// UpdateNodeState transitions a node's state with atomic validation,
// merging any provided Artifacts into the existing JSONB column.
func (s *Store) UpdateNodeState(ctx context.Context, in UpdateNodeStateInput) (*Node, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var current NodeState
	if err := tx.QueryRow(ctx,
		`SELECT state FROM plan_nodes WHERE id = $1 FOR UPDATE`, in.NodeID,
	).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !current.ValidNodeTransition(in.Next) {
		return nil, &illegalTransition{Entity: "node_state", From: string(current), To: string(in.Next)}
	}

	// Build the SET clause based on the transition.
	// started_at flips to NOW() when entering Running from Pending/Blocked.
	// ended_at flips to NOW() when entering a terminal state.
	var n Node
	artifacts := in.Artifacts
	if len(artifacts) == 0 {
		artifacts = []byte("{}")
	}
	err = tx.QueryRow(ctx, `
        UPDATE plan_nodes
           SET state = $2,
               error = CASE WHEN $2 = 'failed' THEN $3 ELSE '' END,
               artifacts = artifacts || $4::jsonb,
               started_at = CASE
                    WHEN $2 = 'running' AND started_at IS NULL THEN NOW()
                    ELSE started_at
                END,
               ended_at = CASE
                    WHEN $2 IN ('done','failed','cancelled') THEN NOW()
                    ELSE ended_at
                END,
               updated_at = NOW()
         WHERE id = $1
        RETURNING id, plan_id, COALESCE(parent_id::text,''), kind, title, state,
                  COALESCE(assignee_soul,''), inputs, artifacts, COALESCE(error,''),
                  started_at, ended_at, created_at, updated_at
    `, in.NodeID, in.Next, in.Error, artifacts).Scan(
		&n.ID, &n.PlanID, &n.ParentID, &n.Kind, &n.Title, &n.State,
		&n.AssigneeSoul, &n.Inputs, &n.Artifacts, &n.Error,
		&n.StartedAt, &n.EndedAt, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &n, nil
}

// ─────────────────────────── Edge operations ────────────────────────────

// AddEdge inserts a directed edge. Returns the canonical Edge struct.
func (s *Store) AddEdge(ctx context.Context, planID, from, to string, cond EdgeCondition) (*Edge, error) {
	if cond == "" {
		cond = CondAlways
	}
	_, err := s.q(ctx).Exec(ctx, `
        INSERT INTO plan_edges (plan_id, from_node, to_node, condition)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT DO NOTHING
    `, planID, from, to, cond)
	if err != nil {
		return nil, err
	}
	return &Edge{PlanID: planID, FromNode: from, ToNode: to, Condition: cond}, nil
}

// ListEdgesByPlan returns every edge for a plan.
func (s *Store) ListEdgesByPlan(ctx context.Context, planID string) ([]*Edge, error) {
	rows, err := s.q(ctx).Query(ctx, `
        SELECT plan_id, from_node, to_node, condition
        FROM plan_edges WHERE plan_id = $1
    `, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Edge{}
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.PlanID, &e.FromNode, &e.ToNode, &e.Condition); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// OutgoingEdges returns the edges out of a given node. Used by the
// graph runtime to select the next node after a condition resolves.
func (s *Store) OutgoingEdges(ctx context.Context, planID, fromNode string) ([]*Edge, error) {
	rows, err := s.q(ctx).Query(ctx, `
        SELECT plan_id, from_node, to_node, condition
        FROM plan_edges WHERE plan_id = $1 AND from_node = $2
    `, planID, fromNode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Edge{}
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.PlanID, &e.FromNode, &e.ToNode, &e.Condition); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// ErrNotFound is returned by Get methods when the target row does not exist.
var ErrNotFound = errors.New("plans: not found")
