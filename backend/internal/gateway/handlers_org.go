// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/agent"
	cronpkg "github.com/qorvenai/qorven/internal/cron"
)

// handleGetOrgRoster returns all agents with org-level metadata for the roster table.
// GET /v1/org/roster
func (gw *Gateway) handleGetOrgRoster(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT a.id, a.agent_key, a.display_name, a.role, a.title,
		        COALESCE(a.org_level,'l3'), COALESCE(a.org_role,''), COALESCE(a.avatar,''),
		        COALESCE(a.customer_facing,false), a.status,
		        COALESCE(a.monthly_budget_usd,0), a.hired_at, a.terminated_at,
		        COALESCE(r.total_spend_usd,0), COALESCE(r.total_tokens_in,0), COALESCE(r.total_tokens_out,0)
		 FROM agents a
		 LEFT JOIN org_roster r ON r.agent_id = a.id AND r.status != 'terminated'
		 WHERE a.tenant_id = $1 AND a.deleted_at IS NULL
		 ORDER BY
		   CASE COALESCE(a.org_level,'l3') WHEN 'l1' THEN 0 WHEN 'l2' THEN 1 ELSE 2 END,
		   a.display_name`,
		defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	type RosterEntry struct {
		ID             string   `json:"id"`
		AgentKey       string   `json:"agent_key"`
		DisplayName    string   `json:"display_name"`
		Role           *string  `json:"role"`
		Title          *string  `json:"title"`
		OrgLevel       string   `json:"org_level"`
		OrgRole        string   `json:"org_role"`
		Avatar         string   `json:"avatar"`
		CustomerFacing bool     `json:"customer_facing"`
		Status         string   `json:"status"`
		MonthlyBudget  float64  `json:"monthly_budget_usd"`
		HiredAt        *string  `json:"hired_at"`
		TerminatedAt   *string  `json:"terminated_at"`
		TotalSpendUSD  float64  `json:"total_spend_usd"`
		TotalTokensIn  int64    `json:"total_tokens_in"`
		TotalTokensOut int64    `json:"total_tokens_out"`
	}

	var entries []RosterEntry
	for rows.Next() {
		var e RosterEntry
		var hiredAt, terminatedAt *time.Time
		if err := rows.Scan(&e.ID, &e.AgentKey, &e.DisplayName, &e.Role, &e.Title,
			&e.OrgLevel, &e.OrgRole, &e.Avatar, &e.CustomerFacing, &e.Status,
			&e.MonthlyBudget, &hiredAt, &terminatedAt,
			&e.TotalSpendUSD, &e.TotalTokensIn, &e.TotalTokensOut); err != nil {
			continue
		}
		if hiredAt != nil {
			s := hiredAt.Format(time.RFC3339)
			e.HiredAt = &s
		}
		if terminatedAt != nil {
			s := terminatedAt.Format(time.RFC3339)
			e.TerminatedAt = &s
		}
		entries = append(entries, e)
	}
	if entries == nil {
		entries = []RosterEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"roster": entries})
}

// handleGetOrgFinanceSummary returns total spend this month grouped by agent.
// GET /v1/org/finance/summary
func (gw *Gateway) handleGetOrgFinanceSummary(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT d.agent_id, COALESCE(a.display_name,''), COALESCE(a.org_role,''), COALESCE(a.org_level,'l3'),
		        SUM(d.cost_usd) as month_cost, SUM(d.tokens_in) as tokens_in, SUM(d.tokens_out) as tokens_out
		 FROM org_daily_spend d
		 LEFT JOIN agents a ON a.id = d.agent_id
		 WHERE d.tenant_id = $1 AND d.date >= date_trunc('month', CURRENT_DATE)
		 GROUP BY d.agent_id, a.display_name, a.org_role, a.org_level
		 ORDER BY month_cost DESC`, defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	type AgentSpend struct {
		AgentID     string  `json:"agent_id"`
		DisplayName string  `json:"display_name"`
		OrgRole     string  `json:"org_role"`
		OrgLevel    string  `json:"org_level"`
		MonthCostUSD float64 `json:"month_cost_usd"`
		TokensIn    int64   `json:"tokens_in"`
		TokensOut   int64   `json:"tokens_out"`
	}

	var agents []AgentSpend
	var totalCost float64
	for rows.Next() {
		var a AgentSpend
		rows.Scan(&a.AgentID, &a.DisplayName, &a.OrgRole, &a.OrgLevel, &a.MonthCostUSD, &a.TokensIn, &a.TokensOut)
		totalCost += a.MonthCostUSD
		agents = append(agents, a)
	}
	if agents == nil {
		agents = []AgentSpend{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents, "total_month_usd": totalCost})
}

// handleGetOrgFinanceDaily returns day-by-day spend for the past N days (default 30).
// GET /v1/org/finance/daily?days=30
func (gw *Gateway) handleGetOrgFinanceDaily(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 && v <= 365 {
			days = v
		}
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT date, SUM(cost_usd), SUM(tokens_in), SUM(tokens_out)
		 FROM org_daily_spend
		 WHERE tenant_id = $1 AND date >= CURRENT_DATE - ($2::int - 1)
		 GROUP BY date ORDER BY date`, defaultTenant, days)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	type DaySpend struct {
		Date      string  `json:"date"`
		CostUSD   float64 `json:"cost_usd"`
		TokensIn  int64   `json:"tokens_in"`
		TokensOut int64   `json:"tokens_out"`
	}

	var daily []DaySpend
	for rows.Next() {
		var d DaySpend
		var date time.Time
		rows.Scan(&date, &d.CostUSD, &d.TokensIn, &d.TokensOut)
		d.Date = date.Format("2006-01-02")
		daily = append(daily, d)
	}
	if daily == nil {
		daily = []DaySpend{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"daily": daily})
}

// handleOrgHireAgent inserts or updates an agent's org_level/org_role and creates an org_roster entry.
// POST /v1/org/roster/hire
func (gw *Gateway) handleOrgHireAgent(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	var body struct {
		AgentID  string `json:"agent_id"`
		OrgLevel string `json:"org_level"`
		OrgRole  string `json:"org_role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id required"})
		return
	}
	if body.OrgLevel == "" {
		body.OrgLevel = "l3"
	}

	u := userFromContext(r.Context())
	hiredBy := ""
	if u != nil {
		hiredBy = u.ID
	}

	_, err := gw.db.Pool.Exec(r.Context(),
		`UPDATE agents SET org_level=$1, org_role=$2, hired_at=now(), terminated_at=NULL
		 WHERE id=$3 AND tenant_id=$4`,
		body.OrgLevel, body.OrgRole, body.AgentID, defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	// If this hire/update demotes the agent to a non-executive level (L3),
	// disable any channels it owns — workers never face the outside world.
	if !levelAllowsChannel(body.OrgLevel) {
		gw.disableAgentChannels(r.Context(), body.AgentID)
	}

	var displayName, existingPrompt string
	_ = gw.db.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(display_name,''), COALESCE(system_prompt,'') FROM agents WHERE id=$1`, body.AgentID).
		Scan(&displayName, &existingPrompt)

	_, _ = gw.db.Pool.Exec(r.Context(),
		`INSERT INTO org_roster (tenant_id, agent_id, org_level, org_role, display_name, status, hired_by)
		 VALUES ($1,$2,$3,$4,$5,'active',$6)
		 ON CONFLICT DO NOTHING`,
		defaultTenant, body.AgentID, body.OrgLevel, body.OrgRole, displayName, hiredBy)

	// Seed soul bundle from archetype if agent has no system prompt yet
	if gw.bundleStore != nil && body.OrgRole != "" {
		var soulContent string
		if seed, ok := agent.AgentSeeds[body.OrgRole]; ok && seed.Soul != "" {
			soulContent = seed.Soul
		} else if existingPrompt == "" {
			soulContent = fmt.Sprintf("You are %s, an AI %s at this organisation.", displayName, body.OrgRole)
		}
		if soulContent != "" {
			gw.bundleStore.Upsert(r.Context(), agent.Bundle{
				AgentID:    body.AgentID,
				BundleType: "soul",
				Name:       "soul",
				Content:    soulContent,
				Priority:   200,
				Enabled:    true,
			})
		}
		// Seed default tool bundles for the org role's archetype
		archetype := ""
		switch body.OrgRole {
		case "cto":
			archetype = "code"
		case "cmo":
			archetype = "marketer"
		case "cso":
			archetype = "sales"
		case "cco":
			archetype = "support"
		case "ciso", "cko":
			archetype = "researcher"
		case "cfo":
			archetype = "analyst"
		default:
			archetype = "general"
		}
		gw.bundleStore.SeedDefaults(r.Context(), body.AgentID, archetype)
	}

	// Seed default cron schedules for org roles (proactive activation)
	if gw.db != nil && body.OrgRole != "" {
		type cronSeed struct {
			expr string
			name string
			task string
		}
		var schedules []cronSeed
		switch body.OrgRole {
		case "caio":
			schedules = []cronSeed{
				{"*/15 * * * *", "Fleet health check", "Check fleet health: look for stuck sessions, agents with errors, or unusually high token burn. Report anomalies."},
				{"0 9 * * *", "Daily org digest", "Generate daily org digest: agent status, total spend yesterday, open delegations, any escalations."},
			}
		case "cfo":
			schedules = []cronSeed{
				{"0 9 * * 1", "Weekly finance report", "Generate weekly finance report: month-to-date spend vs previous, top 5 agents by cost, agents over 80% of budget cap, model cost breakdown, optimization recommendations."},
				{"0 18 * * *", "Daily spend check", "Daily spend check: flag any agent that spent more than 50% of their daily budget, report total org spend today."},
			}
		case "coo":
			schedules = []cronSeed{
				{"0 8 * * *", "Morning ops summary", "Morning operations summary: what tasks are in progress, what completed yesterday, what's blocked, who's idle."},
			}
		case "chro":
			schedules = []cronSeed{
				{"0 9 * * 1", "Weekly org health", "Weekly org health check: total headcount, any agents idle >48h, budget utilisation per tier, recommend hiring or termination."},
			}
		}
		for _, s := range schedules {
			payload := fmt.Sprintf(`{"instruction":%q}`, s.task)
			nextRun := cronpkg.NextRunFromExpr(s.expr)
			gw.db.Pool.Exec(r.Context(),
				`INSERT INTO cron_jobs (tenant_id, agent_id, name, cron_expression, payload, next_run_at, enabled)
				 VALUES ($1, $2::uuid, $3, $4, $5::jsonb, $6, true)
				 ON CONFLICT DO NOTHING`,
				defaultTenant, body.AgentID, s.name, s.expr, payload, nextRun)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "hired", "agent_id": body.AgentID})
}

// handleOrgTerminateAgent marks an agent as terminated in org_roster and sets terminated_at.
// Before terminating, it reparents the agent's direct reports to the agent's own manager
// (reparent-to-grandparent) so no subordinates are left pointing at a terminated agent.
// POST /v1/org/roster/{id}/terminate
func (gw *Gateway) handleOrgTerminateAgent(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "id")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	u := userFromContext(r.Context())
	terminatedBy := ""
	if u != nil {
		terminatedBy = u.ID
	}

	// Reparent direct reports to the terminated agent's manager (reparent-to-grandparent),
	// then mark the agent terminated — all in one transaction so a reparent failure
	// aborts the termination and leaves no subordinates pointing at a terminated agent.
	// If the terminated agent is top-level (manager_id IS NULL), reports become
	// top-level too, which is the correct outcome.
	tx, err := gw.db.Pool.Begin(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer tx.Rollback(r.Context())

	var grandparentID *uuid.UUID
	if err := tx.QueryRow(r.Context(),
		`SELECT manager_id FROM agents WHERE id=$1 AND tenant_id=$2`,
		agentID, defaultTenant).Scan(&grandparentID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}

	if _, err := tx.Exec(r.Context(),
		`UPDATE agents SET manager_id=$1 WHERE manager_id=$2 AND tenant_id=$3`,
		grandparentID, agentID, defaultTenant); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	// Sync the overlay (org_hierarchy.reports_to) for all reparented agents in bulk.
	if _, err := tx.Exec(r.Context(),
		`UPDATE org_hierarchy SET reports_to=$1 WHERE reports_to=$2 AND tenant_id=$3`,
		grandparentID, agentID, defaultTenant); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	if _, err := tx.Exec(r.Context(),
		`UPDATE agents SET terminated_at=now() WHERE id=$1 AND tenant_id=$2`,
		agentID, defaultTenant); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	// Snapshot final spend from org_daily_spend into org_roster
	_, _ = gw.db.Pool.Exec(r.Context(),
		`UPDATE org_roster SET
		    status='terminated',
		    terminated_at=now(),
		    terminated_by=$1,
		    termination_reason=$2,
		    total_spend_usd=(SELECT COALESCE(SUM(cost_usd),0) FROM org_daily_spend WHERE agent_id=$3),
		    total_tokens_in=(SELECT COALESCE(SUM(tokens_in),0) FROM org_daily_spend WHERE agent_id=$3),
		    total_tokens_out=(SELECT COALESCE(SUM(tokens_out),0) FROM org_daily_spend WHERE agent_id=$3)
		 WHERE agent_id=$3 AND status='active'`,
		terminatedBy, body.Reason, agentID)

	// A terminated agent must not keep channels polling.
	gw.disableAgentChannels(r.Context(), agentID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "terminated", "agent_id": agentID})
}

// handleOrgReassignManager reassigns an agent's direct manager.
// Body: {"manager_id": "<uuid>"} or {"manager_id": null} to make the agent top-level.
// Rejects the change if it would create a cycle (proposed manager is a subordinate of the agent).
// PATCH /v1/org/roster/{id}/manager
func (gw *Gateway) handleOrgReassignManager(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	// Defense-in-depth: verify admin even though the route is in the RequireAdmin group.
	u := userFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return
	}

	rawID := chi.URLParam(r, "id")
	agentID, err := uuid.Parse(rawID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	var body struct {
		ManagerID *string `json:"manager_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Verify the target agent exists in this tenant.
	var existingOrgLevel, existingOrgRole string
	var existingBudget float64
	err = gw.db.Pool.QueryRow(r.Context(),
		`SELECT COALESCE(org_level,''), COALESCE(org_role,''), COALESCE(monthly_budget_usd,0)
		 FROM agents WHERE id=$1 AND tenant_id=$2`,
		agentID, defaultTenant).Scan(&existingOrgLevel, &existingOrgRole, &existingBudget)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}

	var newManagerID *uuid.UUID
	if body.ManagerID != nil {
		mid, err := uuid.Parse(*body.ManagerID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid manager_id"})
			return
		}

		// Reject self-assignment.
		if mid == agentID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "an agent cannot be its own manager"})
			return
		}

		// Verify the proposed manager exists in this tenant.
		var managerExists bool
		_ = gw.db.Pool.QueryRow(r.Context(),
			`SELECT true FROM agents WHERE id=$1 AND tenant_id=$2`,
			mid, defaultTenant).Scan(&managerExists)
		if !managerExists {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "manager agent not found"})
			return
		}

		// Cycle detection: reject if the proposed manager is a subordinate of this agent.
		if gw.orgChartStore != nil {
			tenantUID, _ := uuid.Parse(defaultTenant)
			isSub, err := gw.orgChartStore.IsSubordinate(r.Context(), tenantUID, agentID, mid)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
				return
			}
			if isSub {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reassignment would create a reporting cycle"})
				return
			}
		}

		newManagerID = &mid
	}

	// Apply the manager update.
	_, err = gw.db.Pool.Exec(r.Context(),
		`UPDATE agents SET manager_id=$1 WHERE id=$2 AND tenant_id=$3`,
		newManagerID, agentID, defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	// Sync the org_hierarchy overlay so reports_to matches the new manager.
	if gw.orgChartStore != nil {
		tenantUID, _ := uuid.Parse(defaultTenant)
		_ = gw.orgChartStore.SyncFromAgent(r.Context(), tenantUID, agentID, newManagerID, existingOrgLevel, existingOrgRole, existingBudget)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
