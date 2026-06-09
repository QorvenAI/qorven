// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package approvalsx is the unified approval object for the Operations Fabric.
// One object every new module uses; opening one reaches the user via the
// reach-the-user engine, deciding one stops the climb. (Named approvalsx to
// avoid colliding with the legacy plan-approvals package.)
package approvalsx

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Approval is one unified approval request.
type Approval struct {
	ID               string
	TenantID         string
	Kind             string
	RequesterAgentID string
	WorkItemID       string // "" when none
	Summary          string
	AmountUUSD       *int64 // nil when not money
	Risk             string // low|normal|urgent
	Context          map[string]any
	Status           string // pending|approved|rejected|expired
	DecidedBy        string
	DecisionNote     string
}

// Store persists unified approvals.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func nullableID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Open inserts a pending approval and returns its id. The caller (gateway)
// is responsible for reaching the user via the escalation ladder.
func (s *Store) Open(ctx context.Context, a Approval) (string, error) {
	if a.Risk == "" {
		a.Risk = "normal"
	}
	ctxJSON := []byte("{}")
	if a.Context != nil {
		if b, err := json.Marshal(a.Context); err == nil {
			ctxJSON = b
		}
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO approvals_unified (tenant_id, kind, requester_agent_id, work_item_id, summary, amount_uusd, risk, context, status)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'pending') RETURNING id`,
		a.TenantID, a.Kind, a.RequesterAgentID, nullableID(a.WorkItemID), a.Summary, a.AmountUUSD, a.Risk, ctxJSON,
	).Scan(&id)
	return id, err
}

// Get returns one approval by id.
func (s *Store) Get(ctx context.Context, id string) (*Approval, error) {
	var a Approval
	var amt *int64
	var wi *string
	var decidedBy, note *string
	var ctxJSON []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, kind, requester_agent_id, work_item_id, summary, amount_uusd, risk, context, status, decided_by, decision_note
		 FROM approvals_unified WHERE id=$1`, id,
	).Scan(&a.ID, &a.TenantID, &a.Kind, &a.RequesterAgentID, &wi, &a.Summary, &amt, &a.Risk, &ctxJSON, &a.Status, &decidedBy, &note)
	if err != nil {
		return nil, err
	}
	a.AmountUUSD = amt
	if wi != nil {
		a.WorkItemID = *wi
	}
	if decidedBy != nil {
		a.DecidedBy = *decidedBy
	}
	if note != nil {
		a.DecisionNote = *note
	}
	if len(ctxJSON) > 0 {
		_ = json.Unmarshal(ctxJSON, &a.Context)
	}
	return &a, nil
}

// ListPending returns pending approvals for a tenant, newest first.
func (s *Store) ListPending(ctx context.Context, tenantID string) ([]Approval, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, kind, requester_agent_id, work_item_id, summary, amount_uusd, risk, status
		 FROM approvals_unified WHERE tenant_id=$1 AND status='pending' ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Approval{}
	for rows.Next() {
		var a Approval
		var amt *int64
		var wi *string
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Kind, &a.RequesterAgentID, &wi, &a.Summary, &amt, &a.Risk, &a.Status); err != nil {
			slog.Warn("approvalsx.list.scan_failed", "err", err)
			continue
		}
		a.AmountUUSD = amt
		if wi != nil {
			a.WorkItemID = *wi
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Decide sets a pending approval to approved/rejected. No-op if not pending
// (so a double-decide can't flip an already-decided approval).
func (s *Store) Decide(ctx context.Context, id string, approved bool, decidedBy, note string) error {
	status := "rejected"
	if approved {
		status = "approved"
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE approvals_unified SET status=$2, decided_by=$3, decision_note=$4, decided_at=now()
		 WHERE id=$1 AND status='pending'`, id, status, decidedBy, note)
	return err
}
