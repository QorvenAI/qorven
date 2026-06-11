// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GET /v1/projects/{id}/analytics — the glass-box rollup: cost burn trend,
// agent workload, task flow, and PR/CI status for a project brief.
func (gw *Gateway) handleProjectAnalytics(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// 1. Cost burn trend — daily µUSD spend (lifetime; project caps are one-off).
	burnTrend := []map[string]any{}
	if rows, err := gw.db.Pool.Query(ctx,
		`SELECT date_trunc('day', created_at) AS day, COALESCE(SUM(cost_total_uusd),0)
		 FROM gateway_spend_raw WHERE tenant_id=$1 AND project_id=$2
		 GROUP BY day ORDER BY day`, defaultTenant, id); err == nil {
		defer rows.Close()
		for rows.Next() {
			var day any
			var uusd int64
			if rows.Scan(&day, &uusd) == nil {
				burnTrend = append(burnTrend, map[string]any{"day": day, "uusd": uusd})
			}
		}
	}

	// 2. Agent workload — project agents joined to supervisor health.
	health := map[string]any{}
	if gw.supervisor != nil {
		for _, h := range gw.supervisor.AgentHealthList() {
			health[h.AgentID] = h
		}
	}
	agents := []map[string]any{}
	if rows, err := gw.db.Pool.Query(ctx,
		`SELECT id::text, COALESCE(display_name,''), COALESCE(role,''), COALESCE(status,'')
		 FROM agents WHERE tenant_id=$1 AND project_brief_id=$2 AND deleted_at IS NULL`, defaultTenant, id); err == nil {
		defer rows.Close()
		for rows.Next() {
			var aid, name, role, status string
			if rows.Scan(&aid, &name, &role, &status) == nil {
				agents = append(agents, map[string]any{
					"agent_id": aid, "name": name, "role": role, "status": status, "health": health[aid],
				})
			}
		}
	}

	// 3. Task flow — status counts for the project's tasks.
	taskCounts := map[string]int{}
	if rows, err := gw.db.Pool.Query(ctx,
		`SELECT status, COUNT(*) FROM tasks WHERE project_brief_id=$1 GROUP BY status`, id); err == nil {
		defer rows.Close()
		for rows.Next() {
			var st string
			var n int
			if rows.Scan(&st, &n) == nil {
				taskCounts[st] = n
			}
		}
	}

	// 4. PR/CI — connected repo (graceful empty if none). FE pulls live PR/CI
	// from the existing /github proxy; here we only surface owner/repo.
	pr := map[string]any{"connected": false}
	var owner, repo string
	_ = gw.db.Pool.QueryRow(ctx, `SELECT github_owner, github_repo FROM project_briefs WHERE id=$1`, id).Scan(&owner, &repo)
	if owner != "" && repo != "" {
		pr["connected"] = true
		pr["owner"] = owner
		pr["repo"] = repo
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"burn_trend":  burnTrend,
		"agents":      agents,
		"task_counts": taskCounts,
		"pr":          pr,
	})
}
