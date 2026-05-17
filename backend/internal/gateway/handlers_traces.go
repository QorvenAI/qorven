// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// traceRow is the wire shape returned by GET /v1/traces.
type traceRow struct {
	ID           string     `json:"id"`
	AgentID      *string    `json:"agent_id,omitempty"`
	SessionKey   *string    `json:"session_key,omitempty"`
	StartTime    time.Time  `json:"start_time"`
	EndTime      *time.Time `json:"end_time,omitempty"`
	DurationMS   *int       `json:"duration_ms,omitempty"`
	InputTokens  int        `json:"input_tokens"`
	OutputTokens int        `json:"output_tokens"`
	CostCents    int        `json:"cost_cents"`
	Status       string     `json:"status"`
	Error        *string    `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// spanRow is the wire shape returned by GET /v1/traces/{id}/spans.
type spanRow struct {
	ID           string     `json:"id"`
	TraceID      string     `json:"trace_id"`
	SpanType     string     `json:"span_type"`
	Name         *string    `json:"name,omitempty"`
	Model        *string    `json:"model,omitempty"`
	Provider     *string    `json:"provider,omitempty"`
	InputTokens  *int       `json:"input_tokens,omitempty"`
	OutputTokens *int       `json:"output_tokens,omitempty"`
	CostCents    int        `json:"cost_cents"`
	StartTime    *time.Time `json:"start_time,omitempty"`
	EndTime      *time.Time `json:"end_time,omitempty"`
	DurationMS   *int       `json:"duration_ms,omitempty"`
	Status       string     `json:"status"`
	Error        *string    `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// handleListTraces handles GET /v1/traces
// Query params: agent_id, limit (default 50), offset (default 0)
func (gw *Gateway) handleListTraces(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var rows []traceRow
	var err error
	if agentID != "" {
		err = gw.queryTraces(r, &rows,
			`SELECT id, agent_id, session_key, start_time, end_time, duration_ms,
			        COALESCE(total_input_tokens,0), COALESCE(total_output_tokens,0),
			        COALESCE(total_cost_cents,0), status, error, created_at
			 FROM traces
			 WHERE tenant_id = $1 AND agent_id = $2
			 ORDER BY created_at DESC LIMIT $3 OFFSET $4`,
			defaultTenant, agentID, limit, offset)
	} else {
		err = gw.queryTraces(r, &rows,
			`SELECT id, agent_id, session_key, start_time, end_time, duration_ms,
			        COALESCE(total_input_tokens,0), COALESCE(total_output_tokens,0),
			        COALESCE(total_cost_cents,0), status, error, created_at
			 FROM traces
			 WHERE tenant_id = $1
			 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			defaultTenant, limit, offset)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rows == nil {
		rows = []traceRow{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rows)
}

func (gw *Gateway) queryTraces(r *http.Request, dest *[]traceRow, query string, args ...any) error {
	if gw.db == nil {
		return nil
	}
	dbRows, err := gw.db.Pool.Query(r.Context(), query, args...)
	if err != nil {
		return err
	}
	defer dbRows.Close()
	for dbRows.Next() {
		var t traceRow
		if err := dbRows.Scan(
			&t.ID, &t.AgentID, &t.SessionKey,
			&t.StartTime, &t.EndTime, &t.DurationMS,
			&t.InputTokens, &t.OutputTokens, &t.CostCents,
			&t.Status, &t.Error, &t.CreatedAt,
		); err != nil {
			return err
		}
		*dest = append(*dest, t)
	}
	return dbRows.Err()
}

// handleGetTrace handles GET /v1/traces/{id}
func (gw *Gateway) handleGetTrace(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		http.Error(w, "trace not found", http.StatusNotFound)
		return
	}
	traceID := chi.URLParam(r, "id")
	var t traceRow
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT id, agent_id, session_key, start_time, end_time, duration_ms,
		        COALESCE(total_input_tokens,0), COALESCE(total_output_tokens,0),
		        COALESCE(total_cost_cents,0), status, error, created_at
		 FROM traces WHERE id = $1 AND tenant_id = $2`,
		traceID, defaultTenant,
	).Scan(&t.ID, &t.AgentID, &t.SessionKey, &t.StartTime, &t.EndTime, &t.DurationMS,
		&t.InputTokens, &t.OutputTokens, &t.CostCents, &t.Status, &t.Error, &t.CreatedAt)
	if err != nil {
		http.Error(w, "trace not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}

// handleListTraceSpans handles GET /v1/traces/{id}/spans
func (gw *Gateway) handleListTraceSpans(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]spanRow{})
		return
	}
	traceID := chi.URLParam(r, "id")
	dbRows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, trace_id, span_type, name, model, provider,
		        input_tokens, output_tokens, COALESCE(cost_cents,0),
		        start_time, end_time, duration_ms, status, error, created_at
		 FROM spans WHERE trace_id = $1 AND tenant_id = $2
		 ORDER BY created_at ASC`,
		traceID, defaultTenant,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer dbRows.Close()

	spans := []spanRow{}
	for dbRows.Next() {
		var s spanRow
		if err := dbRows.Scan(
			&s.ID, &s.TraceID, &s.SpanType, &s.Name, &s.Model, &s.Provider,
			&s.InputTokens, &s.OutputTokens, &s.CostCents,
			&s.StartTime, &s.EndTime, &s.DurationMS,
			&s.Status, &s.Error, &s.CreatedAt,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		spans = append(spans, s)
	}
	if err := dbRows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(spans)
}

// handleGetTraceSummary handles GET /v1/traces/summary
// Returns aggregated token + cost stats for the current month, grouped by agent.
func (gw *Gateway) handleGetTraceSummary(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"summaries": []any{}, "total_cost_cents": 0})
		return
	}
	type agentSummary struct {
		AgentID      string  `json:"agent_id"`
		Traces       int     `json:"traces"`
		InputTokens  int64   `json:"input_tokens"`
		OutputTokens int64   `json:"output_tokens"`
		CostCents    int64   `json:"cost_cents"`
	}

	dbRows, err := gw.db.Pool.Query(r.Context(),
		`SELECT COALESCE(agent_id::text,'unknown'),
		        COUNT(*),
		        COALESCE(SUM(total_input_tokens),0),
		        COALESCE(SUM(total_output_tokens),0),
		        COALESCE(SUM(total_cost_cents),0)
		 FROM traces
		 WHERE tenant_id = $1
		   AND created_at >= date_trunc('month', NOW())
		 GROUP BY agent_id
		 ORDER BY SUM(total_cost_cents) DESC NULLS LAST`,
		defaultTenant,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer dbRows.Close()

	summaries := []agentSummary{}
	for dbRows.Next() {
		var s agentSummary
		if err := dbRows.Scan(&s.AgentID, &s.Traces, &s.InputTokens, &s.OutputTokens, &s.CostCents); err != nil {
			continue
		}
		summaries = append(summaries, s)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summaries)
}
