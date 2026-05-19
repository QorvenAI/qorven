// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	cronpkg "github.com/qorvenai/qorven/internal/cron"
)

func (gw *Gateway) handleListCronJobs(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"jobs": []any{}})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT cj.id::text, cj.name, cj.cron_expression, cj.enabled,
		        to_char(cj.last_run_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as last_run_at,
		        to_char(cj.next_run_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as next_run_at,
		        cj.agent_id::text,
		        COALESCE(a.display_name,'') as agent_name, COALESCE(a.role,'') as agent_role
		 FROM cron_jobs cj LEFT JOIN agents a ON cj.agent_id = a.id
		 ORDER BY cj.created_at DESC`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()
	jobs := []map[string]any{}
	for rows.Next() {
		var id, name, expr, agentName, agentRole string
		var enabled bool
		var lastRun, nextRun, agentID *string
		if err := rows.Scan(&id, &name, &expr, &enabled, &lastRun, &nextRun, &agentID, &agentName, &agentRole); err != nil {
			continue
		}
		jobs = append(jobs, map[string]any{
			"id": id, "name": name, "cron_expression": expr, "enabled": enabled,
			"last_run_at": lastRun, "next_run_at": nextRun,
			"agent_id": agentID, "agent_name": agentName, "agent_role": agentRole,
		})
	}
	if jobs == nil {
		jobs = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"jobs": jobs})
}

func (gw *Gateway) handleListGoals(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"goals": []any{}})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT g.id, g.agent_id, g.title, g.description, g.target_value, g.current_value, g.unit, g.status, g.due_at,
		        COALESCE(a.display_name,'') as agent_name
		 FROM goals g LEFT JOIN agents a ON g.agent_id = a.id WHERE g.tenant_id = $1 ORDER BY g.created_at DESC`, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, title, desc, unit, status, agentName string
		var agentID *string
		var target, current *float64
		var dueAt interface{}
		rows.Scan(&id, &agentID, &title, &desc, &target, &current, &unit, &status, &dueAt, &agentName)
		entry := map[string]any{"id": id, "title": title, "description": desc, "unit": unit, "status": status, "due_at": dueAt, "agent_name": agentName}
		if agentID != nil {
			entry["agent_id"] = *agentID
		}
		if target != nil {
			entry["target_value"] = *target
		}
		if current != nil {
			entry["current_value"] = *current
		}
		list = append(list, entry)
	}
	if list == nil {
		list = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"goals": list})
}

func (gw *Gateway) handleCreateGoal(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		AgentID     string  `json:"agent_id"`
		Title       string  `json:"title"`
		Description string  `json:"description"`
		TargetValue float64 `json:"target_value"`
		Unit        string  `json:"unit"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Title == "" {
		writeJSON(w, 400, map[string]string{"error": "title required"})
		return
	}
	var agentID *string
	if body.AgentID != "" {
		agentID = &body.AgentID
	}
	var id string
	gw.db.Pool.QueryRow(r.Context(), `INSERT INTO goals (tenant_id, agent_id, title, description, target_value, unit) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		defaultTenant, agentID, body.Title, body.Description, body.TargetValue, body.Unit).Scan(&id)
	writeJSON(w, 201, map[string]string{"id": id})
}

func (gw *Gateway) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"approvals": []any{}})
		return
	}
	list := []map[string]any{}

	// Plan-centric approvals (from the plans/plan_nodes subsystem).
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT ap.id, ap.plan_id, ap.node_id, ap.state, ap.requested_by,
		        COALESCE(ap.resolved_by, '') AS resolved_by,
		        ap.budget, ap.created_at
		 FROM approvals ap
		 ORDER BY ap.created_at DESC LIMIT 50`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, planID, nodeID, state, requestedBy, resolvedBy string
		var budget json.RawMessage
		var createdAt interface{}
		rows.Scan(&id, &planID, &nodeID, &state, &requestedBy, &resolvedBy, &budget, &createdAt)
		list = append(list, map[string]any{
			"id": id, "plan_id": planID, "node_id": nodeID, "kind": "plan",
			"state": state, "requested_by": requestedBy, "resolved_by": resolvedBy,
			"budget": budget, "created_at": createdAt,
			// Backwards-compat shims for older UI code:
			"status": state, "agent_id": requestedBy, "tool_name": "", "tool_args": budget,
		})
	}

	// Tool-level approvals (shell security gate). Table may not exist
	// yet if no command has ever triggered the AskUser branch — the
	// agent loop creates it lazily on first use. ErrNoRows is fine.
	toolRows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id::text, tenant_id, agent_id, tool_name, tool_args, reason, status, created_at
		 FROM tool_approvals
		 ORDER BY created_at DESC LIMIT 50`)
	if err == nil {
		defer toolRows.Close()
		for toolRows.Next() {
			var id, tenantID, agentID, toolName, reason, status string
			var toolArgs json.RawMessage
			var createdAt interface{}
			toolRows.Scan(&id, &tenantID, &agentID, &toolName, &toolArgs, &reason, &status, &createdAt)
			list = append(list, map[string]any{
				"id": id, "kind": "tool",
				"tenant_id": tenantID, "agent_id": agentID,
				"tool_name": toolName, "tool_args": toolArgs,
				"reason": reason, "status": status, "state": status,
				"requested_by": agentID, "resolved_by": "",
				"created_at": createdAt,
			})
		}
	}
	if list == nil {
		list = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"approvals": list})
}

func (gw *Gateway) handleDecideApproval(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	var body struct {
		Decision string `json:"decision"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	// Map UI decisions to the approvals_state_chk set:
	//   approve → "approved", reject → "rejected".
	state := ""
	switch body.Decision {
	case "approve", "approved":
		state = "approved"
	case "reject", "rejected":
		state = "rejected"
	default:
		writeJSON(w, 400, map[string]string{"error": "decision must be approve or reject"})
		return
	}
	id := chi.URLParam(r, "id")
	// Try the tool-level approvals table first — that's what the shell
	// gate writes to. If no row matches, fall through to the plan-centric
	// approvals table so both surfaces work through one endpoint.
	ct, _ := gw.db.Pool.Exec(r.Context(),
		`UPDATE tool_approvals SET status = $1, decided_at = NOW() WHERE id = $2 AND status = 'pending'`,
		state, id)
	if ct.RowsAffected() == 0 {
		gw.db.Pool.Exec(r.Context(),
			`UPDATE approvals SET state = $1, resolved_at = NOW(), resolved_by = 'user', updated_at = NOW() WHERE id = $2`,
			state, id)
	}
	writeJSON(w, 200, map[string]string{"status": state})
}

func (gw *Gateway) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, map[string]any{"metrics": map[string]any{}})
		return
	}
	agentID := chi.URLParam(r, "id")
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT metric_type, AVG(value) as avg_val, COUNT(*) as count, MAX(recorded_at) as last
		 FROM agent_metrics WHERE agent_id = $1 AND recorded_at > NOW() - INTERVAL '7 days'
		 GROUP BY metric_type`, agentID)
	if err != nil {
		// agent_metrics table is optional — it's populated by a
		// background collector that may not be deployed. Treat "table
		// missing" or any query failure as "no metrics yet" rather
		// than 500ing every poll from the Metrics tab.
		writeJSON(w, 200, map[string]any{"metrics": map[string]any{}})
		return
	}
	defer rows.Close()
	metrics := map[string]any{}
	for rows.Next() {
		var metricType string
		var avgVal float64
		var count int
		var last interface{}
		rows.Scan(&metricType, &avgVal, &count, &last)
		metrics[metricType] = map[string]any{"avg": avgVal, "count": count, "last": last}
	}
	writeJSON(w, 200, map[string]any{"metrics": metrics})
}

func (gw *Gateway) handleCreateCronJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID    string `json:"agent_id"`
		Expression string `json:"expression"`
		Task       string `json:"task"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Expression == "" || req.Task == "" {
		writeJSON(w, 400, map[string]string{"error": "expression and task required"})
		return
	}
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "db not available"})
		return
	}
	payloadJSON, _ := json.Marshal(map[string]string{"instruction": req.Task})
	nextRun := cronpkg.NextRunFromExpr(req.Expression)
	var id string
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO cron_jobs (tenant_id, agent_id, name, cron_expression, payload, next_run_at, enabled)
		 VALUES ($1, NULLIF($2,'')::uuid, $3, $4, $5, $6, true) RETURNING id`,
		defaultTenant, req.AgentID, req.Task, req.Expression, payloadJSON, nextRun,
	).Scan(&id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "status": "created"})
}

func (gw *Gateway) handlePauseCronJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "db not available"})
		return
	}
	_, err := gw.db.Pool.Exec(r.Context(), `UPDATE cron_jobs SET enabled = false WHERE id = $1`, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "paused"})
}

func (gw *Gateway) handleResumeCronJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "db not available"})
		return
	}
	// Re-enable and recompute next_run_at from the stored expression
	var expr string
	gw.db.Pool.QueryRow(r.Context(), `SELECT cron_expression FROM cron_jobs WHERE id = $1`, id).Scan(&expr)
	nextRun := cronpkg.NextRunFromExpr(expr)
	_, err := gw.db.Pool.Exec(r.Context(),
		`UPDATE cron_jobs SET enabled = true, next_run_at = $1 WHERE id = $2`, nextRun, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "resumed"})
}

func (gw *Gateway) handleDeleteCronJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "db not available"})
		return
	}
	// Disable first so the runner won't pick it up in a concurrent tick, then hard delete.
	gw.db.Pool.Exec(r.Context(), `UPDATE cron_jobs SET enabled = false WHERE id = $1`, id)
	_, err := gw.db.Pool.Exec(r.Context(), `DELETE FROM cron_jobs WHERE id = $1`, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(204)
}
