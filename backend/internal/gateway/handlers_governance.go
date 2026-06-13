// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/governance"
)

// ─── Designations ────────────────────────────────────────────────────────────

func (gw *Gateway) handleListDesignations(w http.ResponseWriter, r *http.Request) {
	if gw.designationStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	list, err := gw.designationStore.List(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"designations": list, "count": len(list)})
}

func (gw *Gateway) handleGetDesignation(w http.ResponseWriter, r *http.Request) {
	if gw.designationStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	d, err := gw.designationStore.Get(r.Context(), defaultTenant, chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "designation not found"})
		return
	}
	writeJSON(w, 200, d)
}

func (gw *Gateway) handleUpsertDesignation(w http.ResponseWriter, r *http.Request) {
	if gw.designationStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	var d governance.Designation
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	d.TenantID = defaultTenant
	if err := gw.designationStore.Upsert(r.Context(), d); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (gw *Gateway) handleDeleteDesignation(w http.ResponseWriter, r *http.Request) {
	if gw.designationStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	gw.designationStore.Delete(r.Context(), defaultTenant, chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleListSkillFamilies(w http.ResponseWriter, r *http.Request) {
	if gw.designationStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	list, err := gw.designationStore.ListSkillFamilies(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"skill_families": list})
}

// ─── Approval Matrix ─────────────────────────────────────────────────────────

func (gw *Gateway) handleListApprovalRules(w http.ResponseWriter, r *http.Request) {
	if gw.approvalStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	rules, err := gw.approvalStore.ListRules(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"rules": rules})
}

func (gw *Gateway) handleListApprovalRequests(w http.ResponseWriter, r *http.Request) {
	if gw.approvalStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	pending, err := gw.approvalStore.ListPending(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"requests": pending})
}

func (gw *Gateway) handleDecideMatrixApproval(w http.ResponseWriter, r *http.Request) {
	if gw.approvalStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	var body struct {
		Status string `json:"status"` // approved or denied
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	user := userFromContext(r.Context())
	decider := ""
	if user != nil {
		decider = user.ID
	}
	err := gw.approvalStore.Decide(r.Context(), defaultTenant, chi.URLParam(r, "id"), decider, body.Status, body.Reason)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ─── Policy Engine — definitions CRUD ────────────────────────────────────────

func (gw *Gateway) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	if gw.policyEngine == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	ps, err := gw.policyEngine.ListPolicies(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"policies": ps})
}

func (gw *Gateway) handleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.policyEngine == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	var p governance.Policy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	p.TenantID = defaultTenant
	if !governance.ValidTriggerEvent(p.TriggerEvent) {
		writeJSON(w, 400, map[string]string{"error": "invalid trigger_event; must be one of: tool_call, model_switch, output_deliver, memory_write, agent_spawn, budget_spend, external_action, build_approve"})
		return
	}
	if !governance.ValidPolicyAction(p.Action) {
		writeJSON(w, 400, map[string]string{"error": "invalid action; must be one of: allow, deny, warn, require_approval, throttle, log, escalate"})
		return
	}
	id, err := gw.policyEngine.CreatePolicy(r.Context(), p)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	_ = gw.policyEngine.LoadPolicies(r.Context(), defaultTenant)
	writeJSON(w, 201, map[string]string{"id": id})
}

func (gw *Gateway) handleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.policyEngine == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	var p governance.Policy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	p.ID = chi.URLParam(r, "id")
	p.TenantID = defaultTenant
	if !governance.ValidTriggerEvent(p.TriggerEvent) {
		writeJSON(w, 400, map[string]string{"error": "invalid trigger_event; must be one of: tool_call, model_switch, output_deliver, memory_write, agent_spawn, budget_spend, external_action, build_approve"})
		return
	}
	if !governance.ValidPolicyAction(p.Action) {
		writeJSON(w, 400, map[string]string{"error": "invalid action; must be one of: allow, deny, warn, require_approval, throttle, log, escalate"})
		return
	}
	if err := gw.policyEngine.UpdatePolicy(r.Context(), p); err != nil {
		if err.Error() == "policy not found" {
			writeJSON(w, 404, map[string]string{"error": "policy not found"})
			return
		}
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	_ = gw.policyEngine.LoadPolicies(r.Context(), defaultTenant)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (gw *Gateway) handleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if user.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}
	if gw.policyEngine == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	if err := gw.policyEngine.DeletePolicy(r.Context(), defaultTenant, chi.URLParam(r, "id")); err != nil {
		if err.Error() == "policy not found" {
			writeJSON(w, 404, map[string]string{"error": "policy not found"})
			return
		}
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	_ = gw.policyEngine.LoadPolicies(r.Context(), defaultTenant)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ─── Policy Engine — event log ────────────────────────────────────────────────

func (gw *Gateway) handleListPolicyEvents(w http.ResponseWriter, r *http.Request) {
	if gw.policyEngine == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	events, err := gw.policyEngine.ListEvents(r.Context(), defaultTenant, 100)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"events": events})
}

// ─── Exceptions ──────────────────────────────────────────────────────────────

func (gw *Gateway) handleListExceptions(w http.ResponseWriter, r *http.Request) {
	if gw.exceptionStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	list, err := gw.exceptionStore.ListUnresolved(r.Context(), defaultTenant, 100)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	stats, _ := gw.exceptionStore.Stats(r.Context(), defaultTenant)
	writeJSON(w, 200, map[string]any{"exceptions": list, "stats": stats})
}

func (gw *Gateway) handleResolveException(w http.ResponseWriter, r *http.Request) {
	if gw.exceptionStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	var body struct {
		Resolution string `json:"resolution"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	user := userFromContext(r.Context())
	resolvedBy := ""
	if user != nil {
		resolvedBy = user.ID
	}
	gw.exceptionStore.Resolve(r.Context(), defaultTenant, chi.URLParam(r, "id"), resolvedBy, body.Resolution)
	writeJSON(w, 200, map[string]string{"status": "resolved"})
}

// ─── Capacity Forecasts ─────────────────────────────────────────────────────

func (gw *Gateway) handleListForecasts(w http.ResponseWriter, r *http.Request) {
	if gw.forecastStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	list, err := gw.forecastStore.List(r.Context(), defaultTenant, 50)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"forecasts": list})
}

// ─── Segregation of Duties ───────────────────────────────────────────────────

func (gw *Gateway) handleListSoDRules(w http.ResponseWriter, r *http.Request) {
	if gw.sodStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	rules, err := gw.sodStore.ListRules(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"rules": rules})
}

func (gw *Gateway) handleCheckSoD(w http.ResponseWriter, r *http.Request) {
	if gw.sodStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	var body struct {
		AgentID string `json:"agent_id"`
		Action  string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	violated, rule := gw.sodStore.CheckViolation(r.Context(), defaultTenant, body.AgentID, body.Action)
	writeJSON(w, 200, map[string]any{"violated": violated, "rule": rule})
}

// ─── SLA Tracking ───────────────────────────────────────────────────────────

func (gw *Gateway) handleListSLAs(w http.ResponseWriter, r *http.Request) {
	if gw.slaStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	defs, err := gw.slaStore.ListDefinitions(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	stats, _ := gw.slaStore.Stats(r.Context(), defaultTenant)
	writeJSON(w, 200, map[string]any{"definitions": defs, "stats": stats})
}

func (gw *Gateway) handleListSLAMeasurements(w http.ResponseWriter, r *http.Request) {
	if gw.slaStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	measurements, err := gw.slaStore.RecentMeasurements(r.Context(), defaultTenant, 100)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"measurements": measurements})
}

// ─── Asset Library ──────────────────────────────────────────────────────────

func (gw *Gateway) handleListAssets(w http.ResponseWriter, r *http.Request) {
	if gw.assetStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	list, err := gw.assetStore.List(r.Context(), defaultTenant, 200)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	stats, _ := gw.assetStore.Stats(r.Context(), defaultTenant)
	writeJSON(w, 200, map[string]any{"assets": list, "stats": stats})
}

func (gw *Gateway) handleGetAsset(w http.ResponseWriter, r *http.Request) {
	if gw.assetStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	a, err := gw.assetStore.Get(r.Context(), defaultTenant, chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "asset not found"})
		return
	}
	writeJSON(w, 200, a)
}

func (gw *Gateway) handleUpsertAsset(w http.ResponseWriter, r *http.Request) {
	if gw.assetStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	var a governance.Asset
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	a.TenantID = defaultTenant
	if err := gw.assetStore.Upsert(r.Context(), a); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (gw *Gateway) handleDeleteAsset(w http.ResponseWriter, r *http.Request) {
	if gw.assetStore == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	gw.assetStore.Delete(r.Context(), defaultTenant, chi.URLParam(r, "id"))
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// ─── Task State Machine ──────────────────────────────────────────────────────

func (gw *Gateway) handleTaskTransition(w http.ResponseWriter, r *http.Request) {
	if gw.taskStateMachine == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	var body struct {
		FromState string `json:"from_state"`
		ToState   string `json:"to_state"`
		Reason    string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	user := userFromContext(r.Context())
	changedBy := ""
	if user != nil {
		changedBy = user.ID
	}
	taskID := chi.URLParam(r, "taskId")
	if err := gw.taskStateMachine.Transition(r.Context(), defaultTenant, taskID, body.FromState, body.ToState, changedBy, body.Reason); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "transitioned", "new_state": body.ToState})
}

func (gw *Gateway) handleTaskHistory(w http.ResponseWriter, r *http.Request) {
	if gw.taskStateMachine == nil {
		writeJSON(w, 503, map[string]string{"error": "governance not available"})
		return
	}
	history, err := gw.taskStateMachine.History(r.Context(), defaultTenant, chi.URLParam(r, "taskId"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"transitions": history})
}
