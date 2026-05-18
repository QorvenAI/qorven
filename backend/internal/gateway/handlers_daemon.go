// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/daemon"
	"github.com/qorvenai/qorven/internal/daemon/providers"
)

// ─── Agent registry endpoints ────────────────────────────────────────────────

// GET /v1/daemon/agents
func (gw *Gateway) handleDaemonListAgents(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 200, map[string]any{"agents": []any{}})
		return
	}
	writeJSON(w, 200, map[string]any{"agents": gw.daemonReg.ListAgents()})
}

// POST /v1/daemon/agents/register
func (gw *Gateway) handleDaemonRegisterAgent(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 503, map[string]string{"error": "daemon not initialized"})
		return
	}
	var req struct {
		Name         string   `json:"name"`
		Provider     string   `json:"provider"`
		Model        string   `json:"model"`
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Provider == "" {
		writeJSON(w, 400, map[string]string{"error": "name and provider required"})
		return
	}

	var impl daemon.AgentProvider
	switch req.Provider {
	case "kiro_cli":
		impl = providers.NewKiroProvider("", req.Name)
	case "claude_code":
		impl = providers.NewClaudeCodeProvider("", req.Name)
	case "qorven_internal":
		if gw.agentLoop != nil && gw.daemonReg != nil {
			// agentID placeholder — QorvenProvider gets the real instance ID via
			// Registry.Register return value, but progress events use req.Name as
			// the display handle until the first heartbeat updates it.
			impl = providers.NewQorvenProvider(gw.agentLoop, gw.daemonReg, req.Name, defaultTenant)
		}
	// "custom" and unknown providers get nil impl — SSE-only delivery
	}

	caps := req.Capabilities
	if len(caps) == 0 && impl != nil {
		caps = impl.Capabilities()
	}

	inst := gw.daemonReg.Register(req.Name, req.Provider, req.Model, caps, impl)
	writeJSON(w, 201, inst)
}

// DELETE /v1/daemon/agents/{id}
func (gw *Gateway) handleDaemonUnregisterAgent(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		w.WriteHeader(204)
		return
	}
	gw.daemonReg.Unregister(chi.URLParam(r, "id"), "manual")
	w.WriteHeader(204)
}

// POST /v1/daemon/agents/{id}/heartbeat
func (gw *Gateway) handleDaemonAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 503, map[string]string{"error": "daemon not initialized"})
		return
	}
	var req struct {
		Status string `json:"status"` // "idle" | "working" | "error"
	}
	json.NewDecoder(r.Body).Decode(&req)
	status := daemon.AgentStatus(req.Status)
	if status == "" {
		status = daemon.StatusIdle
	}
	if !gw.daemonReg.Heartbeat(chi.URLParam(r, "id"), status) {
		writeJSON(w, 404, map[string]string{"error": "agent not found"})
		return
	}
	writeJSON(w, 200, map[string]string{"ok": "1"})
}

// GET /v1/daemon/stream  — SSE push stream for all daemon events.
// Agents connect here to receive task_assigned events in real-time.
// On reconnect, agent should first call GET /v1/daemon/tasks?agent=<id>&status=queued
// to catch up, then re-subscribe here.
func (gw *Gateway) handleDaemonStream(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 503, map[string]string{"error": "daemon not initialized"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, map[string]string{"error": "streaming not supported"})
		return
	}

	sseSetup(w)
	sendSSE := makeSender(w, flusher)

	// Send current agent roster so the subscriber has initial state.
	sendSSE("agent_snapshot", gw.daemonReg.ListAgents())

	subID, ch := gw.daemonReg.Subscribe()
	defer gw.daemonReg.Unsubscribe(subID)

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, open := <-ch:
			if !open {
				return
			}
			sendSSE(string(evt.Type), evt.Data)
		}
	}
}

// ─── Task endpoints ───────────────────────────────────────────────────────────

// GET /v1/daemon/tasks
func (gw *Gateway) handleDaemonListTasks(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 200, map[string]any{"tasks": []any{}})
		return
	}
	q := r.URL.Query()
	agentID := q.Get("agent")
	status := q.Get("status")
	limit, _ := strconv.Atoi(q.Get("limit"))
	writeJSON(w, 200, map[string]any{"tasks": gw.daemonReg.ListTasks(agentID, status, limit)})
}

// POST /v1/daemon/tasks
func (gw *Gateway) handleDaemonCreateTask(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 503, map[string]string{"error": "daemon not initialized"})
		return
	}
	var req struct {
		Title       string         `json:"title"`
		Description string         `json:"description"`
		Owner       string         `json:"owner"`
		Priority    string         `json:"priority"`
		DependsOn   []string       `json:"depends_on"`
		Context     map[string]any `json:"context"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Title == "" {
		writeJSON(w, 400, map[string]string{"error": "title required"})
		return
	}
	t := gw.daemonReg.CreateTask(req.Title, req.Description, req.Owner, req.Priority, "human", req.DependsOn, req.Context)
	writeJSON(w, 201, t)
}

// GET /v1/daemon/tasks/{id}
func (gw *Gateway) handleDaemonGetTask(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	t := gw.daemonReg.GetTask(chi.URLParam(r, "id"))
	if t == nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, 200, t)
}

// POST /v1/daemon/tasks/{id}/assign
func (gw *Gateway) handleDaemonAssignTask(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 503, map[string]string{"error": "daemon not initialized"})
		return
	}
	var req struct{ AgentID string `json:"agent_id"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AgentID == "" {
		writeJSON(w, 400, map[string]string{"error": "agent_id required"})
		return
	}
	if !gw.daemonReg.AssignTask(chi.URLParam(r, "id"), req.AgentID) {
		writeJSON(w, 409, map[string]string{"error": "task not found or not queued"})
		return
	}
	writeJSON(w, 200, gw.daemonReg.GetTask(chi.URLParam(r, "id")))
}

// POST /v1/daemon/tasks/{id}/progress
func (gw *Gateway) handleDaemonTaskProgress(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 503, map[string]string{"error": "daemon not initialized"})
		return
	}
	var req struct {
		AgentID  string `json:"agent_id"`
		Message  string `json:"message"`
		Percent  int    `json:"percent"`
		FilePath string `json:"file_path"`
		Action   string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	gw.daemonReg.Progress(daemon.TaskProgress{
		TaskID:   chi.URLParam(r, "id"),
		AgentID:  req.AgentID,
		Message:  req.Message,
		Percent:  req.Percent,
		FilePath: req.FilePath,
		Action:   req.Action,
	})
	w.WriteHeader(204)
}

// POST /v1/daemon/tasks/{id}/complete
func (gw *Gateway) handleDaemonTaskComplete(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 503, map[string]string{"error": "daemon not initialized"})
		return
	}
	var req struct {
		Summary      string   `json:"summary"`
		FilesChanged []string `json:"files_changed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if !gw.daemonReg.Complete(chi.URLParam(r, "id"), req.Summary, req.FilesChanged) {
		writeJSON(w, 404, map[string]string{"error": "task not found"})
		return
	}
	w.WriteHeader(204)
}

// POST /v1/daemon/tasks/{id}/fail
func (gw *Gateway) handleDaemonTaskFail(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 503, map[string]string{"error": "daemon not initialized"})
		return
	}
	var req struct {
		Error     string `json:"error"`
		Retryable bool   `json:"retryable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if !gw.daemonReg.Fail(chi.URLParam(r, "id"), req.Error, req.Retryable) {
		writeJSON(w, 404, map[string]string{"error": "task not found"})
		return
	}
	w.WriteHeader(204)
}

// ─── Plan endpoints ───────────────────────────────────────────────────────────

// GET /v1/daemon/plans
func (gw *Gateway) handleDaemonListPlans(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 200, map[string]any{"plans": []any{}})
		return
	}
	writeJSON(w, 200, map[string]any{"plans": gw.daemonReg.ListPlans()})
}

// POST /v1/daemon/plans
func (gw *Gateway) handleDaemonProposePlan(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 503, map[string]string{"error": "daemon not initialized"})
		return
	}
	var req struct {
		Title       string            `json:"title"`
		Description string            `json:"description"`
		ProposedBy  string            `json:"proposed_by"`
		Tasks       []daemon.PlanTask `json:"tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Title == "" || len(req.Tasks) == 0 {
		writeJSON(w, 400, map[string]string{"error": "title and tasks required"})
		return
	}
	if req.ProposedBy == "" {
		req.ProposedBy = "human"
	}
	p := gw.daemonReg.ProposePlan(req.Title, req.Description, req.ProposedBy, req.Tasks)

	// Mirror multi-task plans into the durable approvals store so the UI
	// surfaces them in the Approvals tab. Single-task plans auto-approve
	// in the daemon registry; they don't need a human gate.
	if gw.approvals != nil && p.Status == daemon.PlanPending {
		nodeID := "daemon:plan:" + p.ID
		gw.approvals.Request(r.Context(), p.ID, nodeID, req.ProposedBy, map[string]any{
			"title":       p.Title,
			"description": p.Description,
			"tasks":       p.Tasks,
		})
	}

	writeJSON(w, 201, p)
}

// GET /v1/daemon/plans/{id}
func (gw *Gateway) handleDaemonGetPlan(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	p := gw.daemonReg.GetPlan(chi.URLParam(r, "id"))
	if p == nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, 200, p)
}

// POST /v1/daemon/plans/{id}/approve
func (gw *Gateway) handleDaemonApprovePlan(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 503, map[string]string{"error": "daemon not initialized"})
		return
	}
	var req struct {
		Modifications string `json:"modifications"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	planID := chi.URLParam(r, "id")
	approvedBy := "human"
	if !gw.daemonReg.ApprovePlan(planID, approvedBy, req.Modifications) {
		writeJSON(w, 409, map[string]string{"error": "plan not found or not pending"})
		return
	}

	// Resolve the mirrored approvals record if present.
	if gw.approvals != nil {
		nodeID := "daemon:plan:" + planID
		if approval, err := gw.approvals.Request(r.Context(), planID, nodeID, approvedBy, nil); err == nil {
			gw.approvals.Resolve(r.Context(), approvals.ResolveInput{
				ApprovalID: approval.ID,
				Next:       approvals.StateApproved,
				ResolvedBy: approvedBy,
				Comment:    req.Modifications,
			})
		}
	}

	writeJSON(w, 200, gw.daemonReg.GetPlan(planID))
}

// POST /v1/daemon/plans/{id}/reject
func (gw *Gateway) handleDaemonRejectPlan(w http.ResponseWriter, r *http.Request) {
	if gw.daemonReg == nil {
		writeJSON(w, 503, map[string]string{"error": "daemon not initialized"})
		return
	}
	var req struct{ Reason string `json:"reason"` }
	json.NewDecoder(r.Body).Decode(&req)

	planID := chi.URLParam(r, "id")
	if !gw.daemonReg.RejectPlan(planID, "human", req.Reason) {
		writeJSON(w, 409, map[string]string{"error": "plan not found or not pending"})
		return
	}

	// Resolve the mirrored approvals record as rejected.
	if gw.approvals != nil {
		nodeID := "daemon:plan:" + planID
		if approval, err := gw.approvals.Request(r.Context(), planID, nodeID, "human", nil); err == nil {
			gw.approvals.Resolve(r.Context(), approvals.ResolveInput{
				ApprovalID: approval.ID,
				Next:       approvals.StateRejected,
				ResolvedBy: "human",
				Comment:    req.Reason,
			})
		}
	}

	w.WriteHeader(204)
}

// ─── Unused import guard ──────────────────────────────────────────────────────

var _ = fmt.Sprintf // ensure fmt is used
