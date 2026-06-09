// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/approvalsx"
	"github.com/qorvenai/qorven/internal/budgets"
)

// PlannedWorkResult is the outcome of evaluating a department's planned spend.
type PlannedWorkResult struct {
	Decision    string              `json:"decision"` // "apply" | "propose"
	Feasibility budgets.Feasibility `json:"feasibility"`
	ApprovalID  string              `json:"approval_id,omitempty"` // set when proposed
}

// EvaluatePlannedWork runs the CFO projection for a department plan, applies the
// policy decision, and either applies the budget (validated) or opens a unified
// approval that reaches the user. requesterAgentID/summary describe the asker.
func (gw *Gateway) EvaluatePlannedWork(ctx context.Context, departmentID string, planUUSD int64, requesterAgentID, summary string) (PlannedWorkResult, error) {
	if gw.budgetStore == nil {
		return PlannedWorkResult{}, fmt.Errorf("budget store not available")
	}
	policy, thresholdUUSD, err := gw.budgetStore.GetDepartmentAutonomy(ctx, defaultTenant, departmentID)
	if err != nil {
		return PlannedWorkResult{}, err
	}
	f, err := gw.budgetStore.ProjectDepartmentFeasibility(ctx, defaultTenant, departmentID, planUUSD)
	if err != nil {
		return PlannedWorkResult{}, err
	}
	decision := budgets.DepartmentDecision(policy, thresholdUUSD, planUUSD, f.Fits)
	res := PlannedWorkResult{Decision: decision, Feasibility: f}

	if decision == "apply" {
		planUSD := float64(planUUSD) / 1_000_000.0
		if err := gw.budgetStore.SetBudget(ctx, defaultTenant, budgets.BudgetScope{
			Scope: "department", ScopeID: departmentID, MonthlyUSD: planUSD,
			AllocationMode: "carved", ParentScope: "tenant", ParentScopeID: "",
		}); err != nil {
			return PlannedWorkResult{}, err
		}
		return res, nil
	}

	amt := planUUSD
	id, err := gw.OpenApproval(ctx, "user", approvalsx.Approval{
		TenantID: defaultTenant, Kind: "budget_allocation", RequesterAgentID: requesterAgentID,
		Summary: summary, AmountUUSD: &amt, Risk: "normal",
		Context: map[string]any{
			"department_id":       departmentID,
			"available_uusd":      f.AvailableUUSD,
			"projected_burn_uusd": f.ProjectedBurnUUSD,
			"committed_uusd":      f.CommittedUUSD,
			"fits":                f.Fits,
		},
	})
	if err != nil {
		return PlannedWorkResult{}, err
	}
	res.ApprovalID = id
	return res, nil
}

// handleGetDepartmentAutonomy returns a department's policy + threshold. Admin.
func (gw *Gateway) handleGetDepartmentAutonomy(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "budget store not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	id := chi.URLParam(r, "id")
	policy, threshold, err := gw.budgetStore.GetDepartmentAutonomy(r.Context(), defaultTenant, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"autonomy_policy": policy, "threshold_uusd": threshold})
}

// handleSetDepartmentAutonomy sets a department's policy + threshold. Admin.
func (gw *Gateway) handleSetDepartmentAutonomy(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "budget store not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		AutonomyPolicy string `json:"autonomy_policy"`
		ThresholdUUSD  int64  `json:"threshold_uusd"`
	}
	if msg := decodeJSONBody(r, &body, 1<<20); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	switch body.AutonomyPolicy {
	case budgets.PolicyAuto, budgets.PolicyUserApproval, budgets.PolicyBoth:
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid autonomy_policy: must be auto_within_budget, user_approval, or both"})
		return
	}
	if err := gw.budgetStore.SetDepartmentAutonomy(r.Context(), defaultTenant, id, body.AutonomyPolicy, body.ThresholdUUSD); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleEvaluatePlannedWork projects a department plan and applies/proposes it. Admin.
func (gw *Gateway) handleEvaluatePlannedWork(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "budget store not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		PlanUUSD int64  `json:"plan_uusd"`
		Summary  string `json:"summary"`
	}
	if msg := decodeJSONBody(r, &body, 1<<20); msg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
		return
	}
	if body.PlanUUSD <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_uusd must be > 0"})
		return
	}
	res, err := gw.EvaluatePlannedWork(r.Context(), id, body.PlanUUSD, user.ID, body.Summary)
	if err != nil {
		if errors.Is(err, budgets.ErrOverAllocated) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": sanitizeError(err)})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, res)
}
