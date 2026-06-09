// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package budgets

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
)

// GetDepartmentAutonomy returns a department's autonomy policy and threshold (µUSD).
// A missing department falls back to (PolicyAuto, 25_000_000) with a nil error; a real
// query error returns zero values so a caller can't act on a coincidental default.
func (s *Store) GetDepartmentAutonomy(ctx context.Context, tenantID, departmentID string) (string, int64, error) {
	var policy string
	var threshold int64
	err := s.db.QueryRow(ctx,
		`SELECT autonomy_policy, threshold_uusd FROM departments WHERE tenant_id=$1 AND id=$2::uuid`,
		tenantID, departmentID).Scan(&policy, &threshold)
	if errors.Is(err, pgx.ErrNoRows) {
		return PolicyAuto, 25_000_000, nil
	}
	if err != nil {
		return "", 0, err
	}
	return policy, threshold, nil
}

// SetDepartmentAutonomy updates a department's policy + threshold.
// Returns an error if the department does not exist.
func (s *Store) SetDepartmentAutonomy(ctx context.Context, tenantID, departmentID, policy string, thresholdUUSD int64) error {
	tag, err := s.db.Exec(ctx,
		`UPDATE departments SET autonomy_policy=$3, threshold_uusd=$4, updated_at=now()
		 WHERE tenant_id=$1 AND id=$2::uuid`,
		tenantID, departmentID, policy, thresholdUUSD)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("department not found")
	}
	return nil
}

// ProjectDepartmentFeasibility gathers the four projection inputs from the
// ledger and open work and returns whether the planned spend (µUSD) fits.
func (s *Store) ProjectDepartmentFeasibility(ctx context.Context, tenantID, departmentID string, planUUSD int64) (Feasibility, error) {
	const uusdPerUSD = 1_000_000.0

	// Project against the COMPANY (tenant) budget by design — this is an optimistic
	// company-level feasibility check. A department's own carved cap is enforced later
	// by SetBudget's carved-validation when the plan is applied.
	budgetUSD, err := s.scopeCapUSD(ctx, tenantID, "tenant", "")
	if err != nil {
		return Feasibility{}, err
	}
	budgetUUSD := int64(math.Round(budgetUSD * uusdPerUSD))

	var spentUUSD int64
	_ = s.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(cost_total_uusd) FILTER (WHERE period >= date_trunc('month', CURRENT_DATE)), 0)
		 FROM gateway_spend WHERE tenant_id=$1`, tenantID).Scan(&spentUUSD)

	var committedUSD float64
	_ = s.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(bal.proposed_monthly_usd), 0)
		 FROM budget_allocation_lines bal
		 WHERE bal.proposal_id IN (
		     SELECT budget_plan_id FROM work_items
		     WHERE tenant_id=$1 AND budget_plan_id IS NOT NULL AND status NOT IN ('done','cancelled')
		 ) AND bal.status IN ('pending','approved')`, tenantID).Scan(&committedUSD)
	committedUUSD := int64(math.Round(committedUSD * uusdPerUSD))

	var last7UUSD int64
	_ = s.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(cost_total_uusd), 0) FROM gateway_spend
		 WHERE tenant_id=$1 AND period >= CURRENT_DATE - 7`, tenantID).Scan(&last7UUSD)
	dailyBurnUUSD := last7UUSD / 7 // integer floor; max error ~6µUSD/day, negligible and conservative

	var daysRemaining int
	_ = s.db.QueryRow(ctx,
		`SELECT GREATEST(((date_trunc('month', CURRENT_DATE) + INTERVAL '1 month')::date - CURRENT_DATE), 1)`).Scan(&daysRemaining)

	return ProjectFeasibility(budgetUUSD, spentUUSD, committedUUSD, dailyBurnUUSD, daysRemaining, planUUSD), nil
}
