// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/tasks"
)

func (gw *Gateway) handleListTasks(w http.ResponseWriter, r *http.Request) {
	if gw.taskStore == nil {
		writeJSON(w, 200, map[string]any{"tasks": []any{}})
		return
	}

	// Count-only mode: ?count=true returns {"count": N} without fetching full rows.
	if r.URL.Query().Get("count") == "true" {
		tenantID := defaultTenant
		status := r.URL.Query().Get("status")
		var count int
		if status != "" {
			gw.db.Pool.QueryRow(r.Context(),
				`SELECT count(*) FROM tasks WHERE tenant_id = $1 AND status = $2`,
				tenantID, status).Scan(&count)
		} else {
			gw.db.Pool.QueryRow(r.Context(),
				`SELECT count(*) FROM tasks WHERE tenant_id = $1`,
				tenantID).Scan(&count)
		}
		writeJSON(w, 200, map[string]int{"count": count})
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")

	// Full-text search mode — q= overrides agent_id filter.
	if q != "" {
		list, err := gw.taskStore.Search(r.Context(), defaultTenant, q, 20)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if list == nil {
			list = []tasks.Task{}
		}
		writeJSON(w, 200, map[string]any{"tasks": list})
		return
	}

	if agentID != "" {
		list, err := gw.taskStore.ListForAgent(r.Context(), agentID, status, 50)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"tasks": list})
		return
	}
	// No agent_id: return ALL tasks
	list, err := gw.taskStore.ListAll(r.Context(), defaultTenant, status, 100)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []tasks.Task{}
	}
	writeJSON(w, 200, map[string]any{"tasks": list})
}

func (gw *Gateway) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if gw.taskStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var t tasks.Task
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&t); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if t.Title == "" {
		writeJSON(w, 400, map[string]string{"error": "title required"})
		return
	}
	id, err := gw.taskStore.Create(r.Context(), defaultTenant, t)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	slog.Info("task created", "id", id, "title", t.Title)
	// Wake persistent runtime if the task is assigned
	if t.AssignedTo != nil && gw.runtimeMgr != nil {
		gw.runtimeMgr.WakeAgent(*t.AssignedTo, agent.WakeupSignal{
			Source: agent.WakeupAssignment,
			TaskID: id,
		})
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (gw *Gateway) handleGetTask(w http.ResponseWriter, r *http.Request) {
	if gw.taskStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	t, err := gw.taskStore.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, 200, t)
}

func (gw *Gateway) handleUpdateTaskStatus(w http.ResponseWriter, r *http.Request) {
	if gw.taskStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if err := gw.taskStore.Transition(r.Context(), chi.URLParam(r, "id"), body.Status); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (gw *Gateway) handleCancelTask(w http.ResponseWriter, r *http.Request) {
	if gw.taskStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	if err := gw.taskStore.Transition(r.Context(), chi.URLParam(r, "id"), "cancelled"); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}

func (gw *Gateway) handleGetAgentMessages(w http.ResponseWriter, r *http.Request) {
	if gw.msgStore == nil {
		writeJSON(w, 200, map[string]any{"messages": []any{}})
		return
	}
	msgs, err := gw.msgStore.GetUnread(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if msgs == nil {
		msgs = []agent.AgentMessage{}
	}
	writeJSON(w, 200, map[string]any{"messages": msgs})
}

func (gw *Gateway) handleSendAgentMessage(w http.ResponseWriter, r *http.Request) {
	if gw.msgStore == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var msg agent.AgentMessage
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&msg)
	msg.ToAgent = chi.URLParam(r, "id")
	if msg.Content == "" || msg.FromAgent == "" {
		writeJSON(w, 400, map[string]string{"error": "from_agent and content required"})
		return
	}
	id, err := gw.msgStore.Send(r.Context(), defaultTenant, msg)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (gw *Gateway) handleGetBudgets(w http.ResponseWriter, r *http.Request) {
	if gw.agents == nil {
		writeJSON(w, 200, map[string]any{"budgets": []any{}})
		return
	}
	summary, err := gw.agents.GetBudgetSummary(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if summary == nil {
		summary = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"budgets": summary})
}

func (gw *Gateway) handleSetBudget(w http.ResponseWriter, r *http.Request) {
	if gw.agents == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		BudgetCents int64 `json:"budget_cents"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if err := gw.agents.Update(r.Context(), id, map[string]any{"credit_budget_cents": body.BudgetCents}); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "budget_set"})
}

func (gw *Gateway) handleGetOrgChart(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"agents": []any{}})
		return
	}
	chart, err := agent.GetOrgChart(r.Context(), gw.db.Pool, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if chart == nil {
		chart = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"agents": chart})
}
