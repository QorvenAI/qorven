// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"errors"
	"net/http"

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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "set"})
}
