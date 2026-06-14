// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/budgets"
)

func (gw *Gateway) handleListDepartments(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
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
	list, err := gw.budgetStore.ListDepartments(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	if list == nil {
		list = []budgets.Department{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"departments": list})
}

func (gw *Gateway) handleCreateDepartment(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
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
	var body struct {
		Name        string `json:"name"`
		HeadAgentID string `json:"head_agent_id"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	id, err := gw.budgetStore.CreateDepartment(r.Context(), defaultTenant, body.Name, body.HeadAgentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (gw *Gateway) handleListProjectsBudget(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
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
	list, err := gw.budgetStore.ListProjects(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	if list == nil {
		list = []budgets.Project{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": list})
}

func (gw *Gateway) handleCreateProjectBudget(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
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
	var body struct {
		Name         string `json:"name"`
		DepartmentID string `json:"department_id"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}
	id, err := gw.budgetStore.CreateProject(r.Context(), defaultTenant, body.Name, body.DepartmentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (gw *Gateway) handleSetScopeBudget(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
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
	var b budgets.BudgetScope
	if json.NewDecoder(r.Body).Decode(&b) != nil || b.Scope == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope required"})
		return
	}
	if err := gw.budgetStore.SetBudget(r.Context(), defaultTenant, b); err != nil {
		if errors.Is(err, budgets.ErrOverAllocated) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "code": "over_allocated"})
			return
		}
		if errors.Is(err, budgets.ErrInvalidBudget) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "code": "invalid_budget"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "set"})
}

func (gw *Gateway) handleEffectiveBudget(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
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
	res, err := gw.budgetStore.EffectiveAvailable(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (gw *Gateway) handleListProposals(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
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
	list, err := gw.budgetStore.ListPendingProposals(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	if list == nil {
		list = []budgets.Proposal{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"proposals": list})
}

func (gw *Gateway) handleDecideProposal(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
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
	proposalID := chi.URLParam(r, "id")
	var body struct {
		Decisions []budgets.LineDecision `json:"decisions"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := gw.budgetStore.DecideProposal(r.Context(), defaultTenant, proposalID, user.ID, body.Decisions); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	// Stop the escalation ladder now that the user has decided.
	if gw.reach != nil {
		_ = gw.reach.Ack(r.Context(), "budget_proposal", proposalID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "decided"})
}

func (gw *Gateway) handleGetFinanceSettings(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
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
	writeJSON(w, http.StatusOK, gw.budgetStore.GetFinanceSettings(r.Context(), defaultTenant))
}

func (gw *Gateway) handleSetFinanceSettings(w http.ResponseWriter, r *http.Request) {
	if gw.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
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
	var fs budgets.FinanceSettings
	if json.NewDecoder(r.Body).Decode(&fs) != nil || fs.Authority == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cfo_authority required"})
		return
	}
	if err := gw.budgetStore.SetFinanceSettings(r.Context(), defaultTenant, fs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "set"})
}
