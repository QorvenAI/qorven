// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package budgets

import (
	"context"
	"errors"
)

// ProposalLine is one allocation within a CFO proposal. Mirrors BudgetScope
// plus a percent (resolved to $ before storage) and a per-line status.
type ProposalLine struct {
	ID                  string  `json:"id"`
	Scope               string  `json:"scope"`
	ScopeID             string  `json:"scope_id"`
	ProposedMonthlyUSD  float64 `json:"proposed_monthly_usd"`
	ProposedLifetimeUSD float64 `json:"proposed_lifetime_usd"`
	ProposedPct         float64 `json:"proposed_pct"`
	AllocationMode      string  `json:"allocation_mode"`
	ParentScope         string  `json:"parent_scope"`
	ParentScopeID       string  `json:"parent_scope_id"`
	FundingMode         string  `json:"funding_mode"`
	Status              string  `json:"status"`
	DecisionNote        string  `json:"decision_note"`
}

// Proposal is a CFO budget-allocation proposal with its lines.
type Proposal struct {
	ID         string         `json:"id"`
	TenantID   string         `json:"tenant_id"`
	ProposedBy string         `json:"proposed_by"`
	Reason     string         `json:"reason"`
	Status     string         `json:"status"`
	Lines      []ProposalLine `json:"lines"`
}

// LineDecision is a per-line approve/reject from the user.
type LineDecision struct {
	LineID  string `json:"line_id"`
	Approve bool   `json:"approve"`
}

// ToBudgetScope maps a proposal line to the validated SetBudget input.
func (l ProposalLine) ToBudgetScope() BudgetScope {
	return BudgetScope{
		Scope:          l.Scope,
		ScopeID:        l.ScopeID,
		MonthlyUSD:     l.ProposedMonthlyUSD,
		AllocationMode: l.AllocationMode,
		ParentScope:    l.ParentScope,
		ParentScopeID:  l.ParentScopeID,
		FundingMode:    l.FundingMode,
		LifetimeUSD:    l.ProposedLifetimeUSD,
	}
}

// CreateProposal inserts a proposal + its lines and returns the proposal id.
func (s *Store) CreateProposal(ctx context.Context, tenantID, proposedBy, reason string, lines []ProposalLine) (string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) // no-op after commit
	var id string
	if err := tx.QueryRow(ctx,
		`INSERT INTO budget_allocation_proposals (tenant_id, proposed_by, reason)
		 VALUES ($1, NULLIF($2,'')::uuid, $3) RETURNING id::text`,
		tenantID, proposedBy, reason).Scan(&id); err != nil {
		return "", err
	}
	for _, l := range lines {
		if _, e := tx.Exec(ctx, `
			INSERT INTO budget_allocation_lines
			  (proposal_id, scope, scope_id, proposed_monthly_usd, proposed_lifetime_usd, proposed_pct,
			   allocation_mode, parent_scope, parent_scope_id, funding_mode)
			VALUES ($1::uuid, $2, NULLIF($3,'')::uuid, $4, $5, NULLIF($6,0),
			        $7, NULLIF($8,''), NULLIF($9,'')::uuid, NULLIF($10,''))
		`, id, l.Scope, l.ScopeID, l.ProposedMonthlyUSD, l.ProposedLifetimeUSD, l.ProposedPct,
			l.AllocationMode, l.ParentScope, l.ParentScopeID, l.FundingMode); e != nil {
			return "", e
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// ListPendingProposals returns pending proposals (with lines) for a tenant.
func (s *Store) ListPendingProposals(ctx context.Context, tenantID string) ([]Proposal, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id::text, COALESCE(proposed_by::text,''), reason, status
		 FROM budget_allocation_proposals
		 WHERE tenant_id = $1 AND status = 'pending' ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	var out []Proposal
	for rows.Next() {
		var p Proposal
		p.TenantID = tenantID
		if err := rows.Scan(&p.ID, &p.ProposedBy, &p.Reason, &p.Status); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		lines, lerr := s.proposalLines(ctx, out[i].ID)
		if lerr != nil {
			return nil, lerr
		}
		out[i].Lines = lines
	}
	return out, nil
}

func (s *Store) proposalLines(ctx context.Context, proposalID string) ([]ProposalLine, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, scope, COALESCE(scope_id::text,''), proposed_monthly_usd, proposed_lifetime_usd,
		       COALESCE(proposed_pct,0), allocation_mode, COALESCE(parent_scope,''),
		       COALESCE(parent_scope_id::text,''), COALESCE(funding_mode,''), status, COALESCE(decision_note,'')
		FROM budget_allocation_lines WHERE proposal_id = $1::uuid ORDER BY created_at`, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProposalLine
	for rows.Next() {
		var l ProposalLine
		if err := rows.Scan(&l.ID, &l.Scope, &l.ScopeID, &l.ProposedMonthlyUSD, &l.ProposedLifetimeUSD,
			&l.ProposedPct, &l.AllocationMode, &l.ParentScope, &l.ParentScopeID, &l.FundingMode,
			&l.Status, &l.DecisionNote); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DecideProposal applies per-line decisions: approved lines are written through
// the validated SetBudget (carved over-allocation marks that line rejected with
// a note and continues). Sets the proposal's final status.
// A line whose ID is not present in decisions is treated as rejected.
func (s *Store) DecideProposal(ctx context.Context, tenantID, proposalID, decidedBy string, decisions []LineDecision) error {
	lines, err := s.proposalLines(ctx, proposalID)
	if err != nil {
		return err
	}
	decide := map[string]bool{}
	for _, d := range decisions {
		decide[d.LineID] = d.Approve
	}
	var anyApproved, anyRejected bool
	for _, l := range lines {
		approve, present := decide[l.ID]
		if !present || !approve {
			anyRejected = true
			_, _ = s.db.Exec(ctx, `UPDATE budget_allocation_lines SET status='rejected' WHERE id=$1::uuid`, l.ID)
			continue
		}
		if l.ProposedMonthlyUSD <= 0 {
			anyRejected = true
			_, _ = s.db.Exec(ctx,
				`UPDATE budget_allocation_lines SET status='rejected', decision_note=$2 WHERE id=$1::uuid`,
				l.ID,
				"rejected: zero or negative amount not allowed; remove the cap explicitly to make it unlimited")
			continue
		}
		if applyErr := s.SetBudget(ctx, tenantID, l.ToBudgetScope()); applyErr != nil {
			anyRejected = true
			note := "rejected: could not apply allocation"
			if errors.Is(applyErr, ErrOverAllocated) {
				note = "rejected: " + applyErr.Error() // ErrOverAllocated is a safe, user-facing sentinel
			}
			_, _ = s.db.Exec(ctx, `UPDATE budget_allocation_lines SET status='rejected', decision_note=$2 WHERE id=$1::uuid`, l.ID, note)
			continue
		}
		anyApproved = true
		_, _ = s.db.Exec(ctx, `UPDATE budget_allocation_lines SET status='applied' WHERE id=$1::uuid`, l.ID)
	}
	status := "rejected"
	switch {
	case anyApproved && anyRejected:
		status = "partially_approved"
	case anyApproved:
		status = "approved"
	}
	_, err = s.db.Exec(ctx, `
		UPDATE budget_allocation_proposals
		SET status=$2, decided_by=$3, decided_at=now()
		WHERE id=$1::uuid`, proposalID, status, decidedBy)
	return err
}
