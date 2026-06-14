// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	gatewayllm "github.com/qorvenai/qorven/internal/gateway/llm"
)

// handleGatewayStats returns live pipeline status and cache stats.
//
// GET /v1/gateway/stats
func (gw *Gateway) handleGatewayStats(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{
		"uptime_seconds": int(time.Since(gw.startTime).Seconds()),
		"pipeline":       gw.llmPipeline != nil,
		"metrics":        gatewayllm.Metrics.Snapshot(),
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGatewayStatsStream sends gateway metrics as SSE events every 2 seconds.
// The client opens this as an EventSource; each event is a JSON MetricsSnapshot.
//
// GET /v1/gateway/stats/stream
func (gw *Gateway) handleGatewayStatsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	send := func() {
		snap := gatewayllm.Metrics.Snapshot()
		data, _ := json.Marshal(map[string]any{
			"uptime_seconds": int(time.Since(gw.startTime).Seconds()),
			"pipeline":       gw.llmPipeline != nil,
			"metrics":        snap,
		})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	send()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

// handleGatewayCircuit returns the state of all circuit breakers.
//
// GET /v1/gateway/circuit
func (gw *Gateway) handleGatewayCircuit(w http.ResponseWriter, r *http.Request) {
	if gw.llmPipeline == nil {
		writeJSON(w, http.StatusOK, map[string]any{"breakers": []any{}})
		return
	}
	cb := gw.llmPipeline.CircuitBreaker()
	if cb == nil {
		writeJSON(w, http.StatusOK, map[string]any{"breakers": []any{}})
		return
	}
	type entry struct {
		KeyID    string `json:"key_id"`
		State    string `json:"state"`
		Requests uint32 `json:"requests"`
		Failures uint32 `json:"failures"`
	}
	var out []entry
	for keyID, s := range cb.AllStates() {
		st := gatewayllm.CircuitStateName(s)
		stats := cb.Stats(keyID)
		out = append(out, entry{
			KeyID:    keyID,
			State:    st,
			Requests: stats.Requests,
			Failures: stats.TotalFailures,
		})
	}
	if out == nil {
		out = []entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"breakers": out})
}

// handleGatewayQueue returns the current priority queue depths.
//
// GET /v1/gateway/queue
func (gw *Gateway) handleGatewayQueue(w http.ResponseWriter, r *http.Request) {
	if gw.llmPipeline == nil {
		writeJSON(w, http.StatusOK, map[string]any{"interactive": 0, "background": 0, "batch": 0})
		return
	}
	q := gw.llmPipeline.Queue()
	if q == nil {
		writeJSON(w, http.StatusOK, map[string]any{"interactive": 0, "background": 0, "batch": 0})
		return
	}
	depths := q.Depths()
	caps := q.Capacities()
	writeJSON(w, http.StatusOK, map[string]any{
		"interactive": depths["interactive"],
		"background":  depths["background"],
		"batch":       depths["batch"],
		"capacities": map[string]any{
			"interactive": caps["interactive"],
			"background":  caps["background"],
			"batch":       caps["batch"],
		},
	})
}

// handleGatewayAliasesList returns all model aliases for the tenant.
//
// GET /v1/gateway/aliases
func (gw *Gateway) handleGatewayAliasesList(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	type aliasRow struct {
		TenantID string `json:"tenant_id"`
		Alias    string `json:"alias"`
		ModelID  string `json:"model_id"`
		Priority int    `json:"priority"`
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT tenant_id::text, alias, model_id, priority FROM model_aliases WHERE tenant_id = $1 ORDER BY alias, priority`,
		defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusOK, []aliasRow{})
		return
	}
	defer rows.Close()
	var out []aliasRow
	for rows.Next() {
		var a aliasRow
		if err := rows.Scan(&a.TenantID, &a.Alias, &a.ModelID, &a.Priority); err == nil {
			out = append(out, a)
		}
	}
	if out == nil {
		out = []aliasRow{}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGatewayAliasesUpsert upserts a model alias.
//
// PUT /v1/gateway/aliases/{alias}
func (gw *Gateway) handleGatewayAliasesUpsert(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	var body struct {
		ModelID  string `json:"model_id"`
		Priority int    `json:"priority"`
	}
	if err := decodeGatewayAdminJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	aliasParam := chi.URLParam(r, "alias")
	_, err := gw.db.Pool.Exec(r.Context(), `
		INSERT INTO model_aliases (tenant_id, alias, model_id, priority)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant_id, alias, model_id) DO UPDATE
		  SET priority = $4
	`, defaultTenant, aliasParam, body.ModelID, body.Priority)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleGatewayAliasesDelete removes all aliases for a given alias name.
//
// DELETE /v1/gateway/aliases/{alias}
func (gw *Gateway) handleGatewayAliasesDelete(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	aliasParam := chi.URLParam(r, "alias")
	_, err := gw.db.Pool.Exec(r.Context(),
		`DELETE FROM model_aliases WHERE tenant_id = $1 AND alias = $2`,
		defaultTenant, aliasParam)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleGatewayBudgetsList returns all agent budgets with current spend.
//
// GET /v1/gateway/budgets
func (gw *Gateway) handleGatewayBudgetsList(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	type budgetRow struct {
		ID         string   `json:"id"`
		AgentID    *string  `json:"agent_id"`
		TeamID     *string  `json:"team_id"`
		MonthlyUSD *float64 `json:"monthly_usd"`
		DailyUSD   *float64 `json:"daily_usd"`
		SpentMonth float64  `json:"spent_month_usd"`
		SpentToday float64  `json:"spent_today_usd"`
	}
	dbRows, err := gw.db.Pool.Query(r.Context(), `
		SELECT b.id::text, b.agent_id::text, b.team_id::text, b.monthly_usd, b.daily_usd,
		  COALESCE((SELECT SUM(cost_usd) FROM gateway_spend s
		             WHERE s.tenant_id = b.tenant_id AND s.agent_id::text = b.agent_id::text
		               AND s.period >= date_trunc('month', CURRENT_DATE)), 0),
		  COALESCE((SELECT SUM(cost_usd) FROM gateway_spend s
		             WHERE s.tenant_id = b.tenant_id AND s.agent_id::text = b.agent_id::text
		               AND s.period = CURRENT_DATE), 0)
		FROM gateway_budgets b
		WHERE b.tenant_id = $1
		ORDER BY b.created_at DESC
	`, defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusOK, []budgetRow{})
		return
	}
	defer dbRows.Close()
	var out []budgetRow
	for dbRows.Next() {
		var b budgetRow
		if err := dbRows.Scan(&b.ID, &b.AgentID, &b.TeamID, &b.MonthlyUSD, &b.DailyUSD, &b.SpentMonth, &b.SpentToday); err == nil {
			out = append(out, b)
		}
	}
	if out == nil {
		out = []budgetRow{}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGatewayBudgetsUpsert upserts a monthly/daily budget for an agent.
//
// PUT /v1/gateway/budgets/{agentId}
func (gw *Gateway) handleGatewayBudgetsUpsert(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	agentID := chi.URLParam(r, "agentId")
	var body struct {
		MonthlyUSD *float64 `json:"monthly_usd"`
		DailyUSD   *float64 `json:"daily_usd"`
		AllowZero  bool     `json:"allow_zero"`
	}
	if err := decodeGatewayAdminJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	// A stored 0 cap means "block all spend" (unlimited is a NULL/absent value).
	// Reject a negative cap, and an incidental 0 unless explicitly intended, so an
	// admin can't silently disable an agent by typing 0. Send null to make a cap
	// unlimited. Mirrors the guard in budgets.SetBudget.
	for _, v := range []*float64{body.MonthlyUSD, body.DailyUSD} {
		if v == nil {
			continue
		}
		if *v < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "budget cap cannot be negative"})
			return
		}
		if *v == 0 && !body.AllowZero {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "a zero cap blocks all spend; pass allow_zero to set it deliberately, or send null to make it unlimited", "code": "invalid_budget"})
			return
		}
	}
	// Upsert: try UPDATE first; if no row matched, INSERT.
	tag, err := gw.db.Pool.Exec(r.Context(), `
		UPDATE gateway_budgets
		SET monthly_usd = $3, daily_usd = $4, updated_at = now()
		WHERE tenant_id = $1 AND agent_id = $2::uuid
	`, defaultTenant, agentID, body.MonthlyUSD, body.DailyUSD)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	if tag.RowsAffected() == 0 {
		_, err = gw.db.Pool.Exec(r.Context(), `
			INSERT INTO gateway_budgets (tenant_id, agent_id, monthly_usd, daily_usd)
			VALUES ($1, $2::uuid, $3, $4)
		`, defaultTenant, agentID, body.MonthlyUSD, body.DailyUSD)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleGatewayCacheStats returns the LLM cache size and capacity.
//
// GET /v1/gateway/cache/stats
func (gw *Gateway) handleGatewayCacheStats(w http.ResponseWriter, r *http.Request) {
	if gw.llmPipeline == nil {
		writeJSON(w, http.StatusOK, map[string]any{"size": 0, "capacity": 0})
		return
	}
	cache := gw.llmPipeline.Cache()
	if cache == nil {
		writeJSON(w, http.StatusOK, map[string]any{"size": 0, "capacity": 0})
		return
	}
	writeJSON(w, http.StatusOK, cache.Stats())
}

// handleGatewayCacheFlush evicts all entries from the LLM cache.
//
// DELETE /v1/gateway/cache
func (gw *Gateway) handleGatewayCacheFlush(w http.ResponseWriter, r *http.Request) {
	if gw.llmPipeline != nil {
		cache := gw.llmPipeline.Cache()
		if cache != nil {
			cache.Flush()
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "flushed"})
}

// decodeGatewayAdminJSON reads and JSON-decodes the request body (max 64KB).
func decodeGatewayAdminJSON(r *http.Request, dst any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dst)
}
