// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/budgets"
	"github.com/qorvenai/qorven/internal/realtime"
)

// handleProjectBurn returns live µUSD lifetime spend vs. the project cap.
//
//	GET /v1/projects/{id}/burn
//
// Response shape:
//
//	{
//	  "project_id": "...",
//	  "used_uusd":  12345678,
//	  "cap_uusd":   100000000,
//	  "used_usd":   12.345678,
//	  "cap_usd":    100.0,
//	  "pct":        12,
//	  "warn_pct":   80
//	}
//
// If no budget row exists for the project, cap_uusd will be 0 (uncapped).
func (gw *Gateway) handleProjectBurn(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	projectID := chi.URLParam(r, "id")
	used, cap, warnPct, err := projectUsedAndCap(r.Context(), gw.db.Pool, projectID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	const uusdPerUSD = 1_000_000
	pct := 0
	if cap > 0 {
		pct = int(used * 100 / cap)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id": projectID,
		"used_uusd":  used,
		"cap_uusd":   cap,
		"used_usd":   float64(used) / float64(uusdPerUSD),
		"cap_usd":    float64(cap) / float64(uusdPerUSD),
		"pct":        pct,
		"warn_pct":   warnPct,
	})
}

// projectUsedAndCap reads the monthly cap from gateway_budgets and the
// month-to-date spend from gateway_spend_raw for the given project.
// capUUSD == 0 means uncapped. warnPct defaults to 80 when not set.
func projectUsedAndCap(ctx context.Context, db *pgxpool.Pool, projectID string) (usedUUSD, capUUSD int64, warnPct int, err error) {
	warnPct = 80

	// 1. Read the monthly cap from gateway_budgets.
	var monthlyUSD *float64
	var wp *int
	row := db.QueryRow(ctx, `
		SELECT monthly_usd, warn_percent
		FROM gateway_budgets
		WHERE tenant_id = $1 AND scope = 'project' AND project_id = $2
		LIMIT 1
	`, defaultTenant, projectID)
	if scanErr := row.Scan(&monthlyUSD, &wp); scanErr == nil {
		if monthlyUSD != nil {
			capUUSD = int64(*monthlyUSD * 1_000_000)
		}
		if wp != nil {
			warnPct = *wp
		}
	}
	// No budget row is not an error — just uncapped.

	// 2. Sum lifetime spend from the raw ledger (gateway_spend_raw). The project
	// cap is a one-off lifetime allocation from the resource plan, not a monthly
	// allowance, so spend must accumulate across month boundaries — a /code
	// project runs to completion and may span more than one calendar month.
	var spent *int64
	_ = db.QueryRow(ctx, `
		SELECT SUM(cost_total_uusd)
		FROM gateway_spend_raw
		WHERE tenant_id = $1
		  AND project_id = $2
	`, defaultTenant, projectID).Scan(&spent)
	if spent != nil {
		usedUUSD = *spent
	}
	return
}

// broadcastBudgetWarning pushes a budget_warning WebSocket event to all
// connected clients. It is wired as the enforcer's OnWarn hook in gateway.go.
func (gw *Gateway) broadcastBudgetWarning(scope, scopeID string, pct int) {
	if gw.rtHub == nil {
		return
	}
	gw.rtHub.Broadcast(realtime.Event{
		Type: realtime.EventBudgetWarning,
		Data: map[string]any{
			"scope":    scope,
			"scope_id": scopeID,
			"pct":      pct,
		},
	})
}

// checkProjectBreaker pauses the project's agents at their safe checkpoint and
// escalates when lifetime spend reaches the project cap. Idempotent: a project
// already paused is left alone. Safe to call from the OnWarn hook on every warn.
func (gw *Gateway) checkProjectBreaker(ctx context.Context, projectID string) {
	if gw.db == nil {
		return
	}
	used, cap, _, err := projectUsedAndCap(ctx, gw.db.Pool, projectID)
	if err != nil || cap <= 0 || used < cap {
		return
	}
	// Idempotency: skip if already paused.
	var paused bool
	_ = gw.db.Pool.QueryRow(ctx, `SELECT paused FROM project_briefs WHERE id=$1`, projectID).Scan(&paused)
	if paused {
		return
	}
	if _, err := gw.db.Pool.Exec(ctx,
		`UPDATE project_briefs SET paused=true, pause_reason='budget cap reached' WHERE id=$1`,
		projectID); err != nil {
		slog.Warn("project.breaker.pause_failed", "project", projectID, "err", err)
		return
	}
	// Suspend every agent provisioned for this brief at its safe checkpoint.
	rows, qerr := gw.db.Pool.Query(ctx,
		`SELECT id FROM agents WHERE project_brief_id=$1`, projectID)
	if qerr == nil {
		defer rows.Close()
		for rows.Next() {
			var aid string
			if rows.Scan(&aid) == nil && gw.runtimeMgr != nil {
				gw.runtimeMgr.Suspend(aid)
			}
		}
	}
	// Escalation: create a budget-allocation proposal so the user is prompted
	// to approve more budget or stop the project. The proposal surfaces in
	// the CFO Resource Planner UI and via the budget_warning WS event.
	if gw.budgetStore != nil {
		const uusdPerUSD = 1_000_000
		currentCapUSD := float64(cap) / float64(uusdPerUSD)
		suggestedUSD := currentCapUSD * 1.5 // suggest 50% increase as a starting point
		_, _ = gw.budgetStore.CreateProposal(ctx, defaultTenant, "", "Project budget cap reached — approve more budget or stop the project",
			[]budgets.ProposalLine{
				{
					Scope:               "project",
					ScopeID:             projectID,
					ProposedLifetimeUSD: suggestedUSD,
					AllocationMode:      "fresh",
				},
			},
		)
	}
	// Broadcast a WS event so the UI surfaces the pause immediately.
	gw.broadcastBudgetWarning("project", projectID, 100)
	slog.Warn("project.breaker.tripped", "project", projectID, "used_uusd", used, "cap_uusd", cap)
}

// handleProjectPause manually trips the circuit breaker for a project:
// sets paused=true and suspends all agents provisioned for the brief.
//
//	POST /v1/projects/{id}/pause
func (gw *Gateway) handleProjectPause(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	projectID := chi.URLParam(r, "id")

	var paused bool
	_ = gw.db.Pool.QueryRow(r.Context(),
		`SELECT paused FROM project_briefs WHERE id=$1`, projectID).Scan(&paused)
	if paused {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_paused", "project_id": projectID})
		return
	}
	if _, err := gw.db.Pool.Exec(r.Context(),
		`UPDATE project_briefs SET paused=true, pause_reason='manual pause' WHERE id=$1`,
		projectID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	// Suspend all agents provisioned for this brief.
	rows, qerr := gw.db.Pool.Query(r.Context(),
		`SELECT id FROM agents WHERE project_brief_id=$1`, projectID)
	var suspended []string
	if qerr == nil {
		defer rows.Close()
		for rows.Next() {
			var aid string
			if rows.Scan(&aid) == nil && gw.runtimeMgr != nil {
				gw.runtimeMgr.Suspend(aid)
				suspended = append(suspended, aid)
			}
		}
	}
	gw.broadcastBudgetWarning("project", projectID, 100)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "paused",
		"project_id":    projectID,
		"agents_paused": suspended,
	})
}

// handleProjectResume resumes a paused project. An optional new_cap_usd in the
// request body raises the project lifetime budget cap before resuming agents.
//
//	POST /v1/projects/{id}/resume
func (gw *Gateway) handleProjectResume(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	projectID := chi.URLParam(r, "id")

	var body struct {
		NewCapUSD float64 `json:"new_cap_usd"`
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck

	// Raise the project cap if requested.
	if body.NewCapUSD > 0 && gw.budgetStore != nil {
		if err := gw.budgetStore.SetBudget(r.Context(), defaultTenant, budgets.BudgetScope{
			Scope:          "project",
			ScopeID:        projectID,
			MonthlyUSD:     body.NewCapUSD,
			AllocationMode: "fresh",
		}); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": sanitizeError(err)})
			return
		}
	}

	if _, err := gw.db.Pool.Exec(r.Context(),
		`UPDATE project_briefs SET paused=false, pause_reason='' WHERE id=$1`,
		projectID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	// Resume all agents provisioned for this brief.
	rows, qerr := gw.db.Pool.Query(r.Context(),
		`SELECT id FROM agents WHERE project_brief_id=$1`, projectID)
	var resumed []string
	if qerr == nil {
		defer rows.Close()
		for rows.Next() {
			var aid string
			if rows.Scan(&aid) == nil && gw.runtimeMgr != nil {
				gw.runtimeMgr.Resume(aid)
				resumed = append(resumed, aid)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "resumed",
		"project_id":     projectID,
		"agents_resumed": resumed,
	})
}
