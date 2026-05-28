// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/agent"
)

// ─── Org Hierarchy ───────────────────────────────────────────────────────────

func (gw *Gateway) handleGetOrgHierarchy(w http.ResponseWriter, r *http.Request) {
	if gw.orgChartStore == nil {
		writeJSON(w, 503, map[string]string{"error": "org chart not configured"})
		return
	}
	tid, _ := uuid.Parse(defaultTenant)
	nodes, err := gw.orgChartStore.GetTree(r.Context(), tid)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"nodes": nodes})
}

func (gw *Gateway) handleUpdateOrgNode(w http.ResponseWriter, r *http.Request) {
	if gw.orgChartStore == nil {
		writeJSON(w, 503, map[string]string{"error": "org chart not configured"})
		return
	}
	agentID := chi.URLParam(r, "agent_id")
	if agentID == "" {
		writeJSON(w, 400, map[string]string{"error": "agent_id required"})
		return
	}

	var node agent.OrgNode
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}

	aid, err := uuid.Parse(agentID)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid agent_id"})
		return
	}
	tid, _ := uuid.Parse(defaultTenant)
	node.TenantID = tid
	node.AgentID = aid

	if err := gw.orgChartStore.Upsert(r.Context(), node); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

// ─── Routing Rules ───────────────────────────────────────────────────────────

func (gw *Gateway) handleListRoutingRules(w http.ResponseWriter, r *http.Request) {
	if gw.intentRouter == nil {
		writeJSON(w, 503, map[string]string{"error": "intent router not configured"})
		return
	}
	rules, err := gw.intentRouter.ListRules(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"rules": rules})
}

func (gw *Gateway) handleCreateRoutingRule(w http.ResponseWriter, r *http.Request) {
	if gw.intentRouter == nil {
		writeJSON(w, 503, map[string]string{"error": "intent router not configured"})
		return
	}
	var rule agent.RoutingRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	rule.TenantID = defaultTenant
	if err := gw.intentRouter.CreateRule(r.Context(), rule); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 201, map[string]string{"status": "created"})
}

func (gw *Gateway) handleUpdateRoutingRule(w http.ResponseWriter, r *http.Request) {
	if gw.intentRouter == nil {
		writeJSON(w, 503, map[string]string{"error": "intent router not configured"})
		return
	}
	ruleID := chi.URLParam(r, "id")
	var rule agent.RoutingRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	rule.ID = ruleID
	rule.TenantID = defaultTenant
	if err := gw.intentRouter.UpdateRule(r.Context(), rule); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (gw *Gateway) handleDeleteRoutingRule(w http.ResponseWriter, r *http.Request) {
	if gw.intentRouter == nil {
		writeJSON(w, 503, map[string]string{"error": "intent router not configured"})
		return
	}
	ruleID := chi.URLParam(r, "id")
	if err := gw.intentRouter.DeleteRule(r.Context(), defaultTenant, ruleID); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (gw *Gateway) handleTestRouting(w http.ResponseWriter, r *http.Request) {
	if gw.intentRouter == nil {
		writeJSON(w, 503, map[string]string{"error": "intent router not configured"})
		return
	}
	var req struct {
		Content  string `json:"content"`
		Channel  string `json:"channel"`
		UserRole string `json:"user_role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}

	decision := gw.intentRouter.Route(r.Context(), agent.RoutingContext{
		Channel:  req.Channel,
		Content:  req.Content,
		UserRole: req.UserRole,
	})
	writeJSON(w, 200, decision)
}

// ─── Output Quality Stats ────────────────────────────────────────────────────

func (gw *Gateway) handleQualityStats(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}

	since := time.Now().AddDate(0, 0, -30)
	if s := r.URL.Query().Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}

	var totalOutputs, passedCount int
	var avgScore float64
	gw.db.Pool.QueryRow(r.Context(), `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE quality_score >= 7.0), COALESCE(AVG(quality_score), 0)
		FROM output_audit WHERE tenant_id = $1 AND delivered_at >= $2
	`, defaultTenant, since).Scan(&totalOutputs, &passedCount, &avgScore)

	type issueCount struct {
		Rule  string `json:"rule"`
		Count int    `json:"count"`
	}
	rows, _ := gw.db.Pool.Query(r.Context(), `
		SELECT rule, COUNT(*) as cnt
		FROM output_audit, jsonb_array_elements(validation_result) AS elem,
		     jsonb_to_record(elem) AS x(rule text)
		WHERE tenant_id = $1 AND delivered_at >= $2
		GROUP BY rule ORDER BY cnt DESC LIMIT 10
	`, defaultTenant, since)
	var topIssues []issueCount
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var ic issueCount
			if rows.Scan(&ic.Rule, &ic.Count) == nil {
				topIssues = append(topIssues, ic)
			}
		}
	}

	writeJSON(w, 200, map[string]any{
		"total_outputs":     totalOutputs,
		"passed_count":      passedCount,
		"avg_quality_score": avgScore,
		"pass_rate":         safePercent(passedCount, totalOutputs),
		"top_issues":        topIssues,
	})
}

// ─── Billing Breakdown (CFO View) ────────────────────────────────────────────

func (gw *Gateway) handleBillingBreakdown(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}

	since := time.Now().AddDate(0, -1, 0)
	if s := r.URL.Query().Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}

	type agentCost struct {
		AgentID  string  `json:"agent_id"`
		AgentKey string  `json:"agent_key"`
		CostUSD  float64 `json:"cost_usd"`
		TokensIn int64   `json:"tokens_in"`
		TokensOut int64  `json:"tokens_out"`
	}
	rows, err := gw.db.Pool.Query(r.Context(), `
		SELECT gs.agent_id, COALESCE(a.key, gs.agent_id::text), SUM(gs.cost_usd), SUM(gs.tokens_in), SUM(gs.tokens_out)
		FROM gateway_spend gs
		LEFT JOIN agents a ON a.id = gs.agent_id
		WHERE gs.tenant_id = $1 AND gs.period >= $2
		GROUP BY gs.agent_id, a.key
		ORDER BY SUM(gs.cost_usd) DESC
	`, defaultTenant, since.Format("2006-01-02"))
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	var agents []agentCost
	var totalCost float64
	for rows.Next() {
		var ac agentCost
		if rows.Scan(&ac.AgentID, &ac.AgentKey, &ac.CostUSD, &ac.TokensIn, &ac.TokensOut) == nil {
			totalCost += ac.CostUSD
			agents = append(agents, ac)
		}
	}

	// Daily trend
	type dailyCost struct {
		Date    string  `json:"date"`
		CostUSD float64 `json:"cost_usd"`
	}
	dayRows, _ := gw.db.Pool.Query(r.Context(), `
		SELECT period::text, SUM(cost_usd)
		FROM gateway_spend
		WHERE tenant_id = $1 AND period >= $2
		GROUP BY period ORDER BY period ASC
	`, defaultTenant, since.Format("2006-01-02"))
	var daily []dailyCost
	if dayRows != nil {
		defer dayRows.Close()
		for dayRows.Next() {
			var dc dailyCost
			if dayRows.Scan(&dc.Date, &dc.CostUSD) == nil {
				daily = append(daily, dc)
			}
		}
	}

	// Forecast (trailing 7d average → project to end of month)
	var avgDaily float64
	gw.db.Pool.QueryRow(r.Context(), `
		SELECT COALESCE(AVG(daily_cost), 0) FROM (
			SELECT SUM(cost_usd) as daily_cost FROM gateway_spend
			WHERE tenant_id = $1 AND period >= CURRENT_DATE - 7
			GROUP BY period
		) sub
	`, defaultTenant).Scan(&avgDaily)

	daysRemaining := daysUntilEndOfMonth()
	projected := totalCost + (avgDaily * float64(daysRemaining))

	writeJSON(w, 200, map[string]any{
		"total_cost_usd":      totalCost,
		"by_agent":            agents,
		"daily_trend":         daily,
		"avg_daily_usd":       avgDaily,
		"days_remaining":      daysRemaining,
		"projected_month_usd": projected,
	})
}

func (gw *Gateway) handleBillingAnomalies(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}

	type anomaly struct {
		AgentID   string  `json:"agent_id"`
		AgentKey  string  `json:"agent_key"`
		TodayCost float64 `json:"today_cost_usd"`
		AvgCost   float64 `json:"avg_daily_cost_usd"`
		Ratio     float64 `json:"ratio"`
	}

	rows, err := gw.db.Pool.Query(r.Context(), `
		WITH today AS (
			SELECT agent_id, SUM(cost_usd) as today_cost
			FROM gateway_spend WHERE tenant_id = $1 AND period = CURRENT_DATE
			GROUP BY agent_id
		),
		avg30 AS (
			SELECT agent_id, AVG(daily_cost) as avg_cost FROM (
				SELECT agent_id, SUM(cost_usd) as daily_cost
				FROM gateway_spend WHERE tenant_id = $1 AND period >= CURRENT_DATE - 30 AND period < CURRENT_DATE
				GROUP BY agent_id, period
			) sub GROUP BY agent_id
		)
		SELECT t.agent_id, COALESCE(a.key, t.agent_id::text), t.today_cost, COALESCE(v.avg_cost, 0)
		FROM today t
		LEFT JOIN avg30 v ON v.agent_id = t.agent_id
		LEFT JOIN agents a ON a.id = t.agent_id
		WHERE v.avg_cost > 0 AND t.today_cost > v.avg_cost * 2
		ORDER BY t.today_cost DESC
	`, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	var anomalies []anomaly
	for rows.Next() {
		var a anomaly
		if rows.Scan(&a.AgentID, &a.AgentKey, &a.TodayCost, &a.AvgCost) == nil {
			if a.AvgCost > 0 {
				a.Ratio = a.TodayCost / a.AvgCost
			}
			anomalies = append(anomalies, a)
		}
	}
	writeJSON(w, 200, map[string]any{"anomalies": anomalies})
}

// ─── Subagent Runs ───────────────────────────────────────────────────────────

func (gw *Gateway) handleListSubagentRuns(w http.ResponseWriter, r *http.Request) {
	if gw.subagentRunStore == nil {
		writeJSON(w, 503, map[string]string{"error": "subagent store not configured"})
		return
	}
	parentID := chi.URLParam(r, "parent_id")
	var runs []agent.SubagentRunRecord
	var err error
	if parentID != "" {
		runs, err = gw.subagentRunStore.ListByParent(r.Context(), defaultTenant, parentID, 50)
	} else {
		runs, err = gw.subagentRunStore.ListByTenant(r.Context(), defaultTenant, 100)
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"runs": runs})
}

func (gw *Gateway) handleGetTraceTree(w http.ResponseWriter, r *http.Request) {
	if gw.subagentRunStore == nil {
		writeJSON(w, 503, map[string]string{"error": "subagent store not configured"})
		return
	}
	rootID := chi.URLParam(r, "root_id")
	tree, err := gw.subagentRunStore.GetTraceTree(r.Context(), defaultTenant, rootID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"tree": tree})
}

// ─── Workflow Runs ───────────────────────────────────────────────────────────

func (gw *Gateway) handleListOrchestrationRuns(w http.ResponseWriter, r *http.Request) {
	if gw.workflowEngine == nil {
		writeJSON(w, 503, map[string]string{"error": "workflow engine not configured"})
		return
	}
	runs, err := gw.workflowEngine.ListRuns(r.Context(), defaultTenant, 50)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]any{"runs": runs})
}

func (gw *Gateway) handleGetOrchestrationRun(w http.ResponseWriter, r *http.Request) {
	if gw.workflowEngine == nil {
		writeJSON(w, 503, map[string]string{"error": "workflow engine not configured"})
		return
	}
	runID := chi.URLParam(r, "run_id")
	run, err := gw.workflowEngine.GetRun(r.Context(), runID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	steps, _ := gw.workflowEngine.GetStepRuns(r.Context(), runID)
	writeJSON(w, 200, map[string]any{"run": run, "steps": steps})
}

func (gw *Gateway) handleCancelOrchestrationRun(w http.ResponseWriter, r *http.Request) {
	if gw.workflowEngine == nil {
		writeJSON(w, 503, map[string]string{"error": "workflow engine not configured"})
		return
	}
	runID := chi.URLParam(r, "run_id")
	gw.workflowEngine.CancelRun(runID)
	writeJSON(w, 200, map[string]string{"status": "cancelled"})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func safePercent(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

func daysUntilEndOfMonth() int {
	now := time.Now()
	eom := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location())
	return eom.Day() - now.Day()
}
