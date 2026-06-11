// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
