// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	cronpkg "github.com/qorvenai/qorven/internal/cron"
)

// handleCalendarSchedule creates a real, executable scheduled agent task.
// mode="once" → a one-shot cron_jobs row that fires at `when` then disables itself.
// mode="repeat" → a recurring row driven by `cron_expression`.
func (gw *Gateway) handleCalendarSchedule(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	var req struct {
		AgentID        string `json:"agent_id"`
		Instruction    string `json:"instruction"`
		Mode           string `json:"mode"`
		When           string `json:"when"`
		CronExpression string `json:"cron_expression"`
		Title          string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Instruction == "" || req.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id and instruction required"})
		return
	}

	title := req.Title
	if title == "" {
		title = req.Instruction
	}
	payloadJSON, _ := json.Marshal(map[string]string{"instruction": req.Instruction})

	var nextRun time.Time
	oneShot := false
	expr := ""
	switch req.Mode {
	case "repeat":
		if req.CronExpression == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cron_expression required for repeat"})
			return
		}
		expr = req.CronExpression
		nextRun = cronpkg.NextRunFromExpr(expr)
	default: // "once"
		oneShot = true
		t, err := time.Parse(time.RFC3339, req.When)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "when must be RFC3339 for a one-shot"})
			return
		}
		nextRun = t
	}

	var id string
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO cron_jobs (tenant_id, agent_id, name, cron_expression, payload, next_run_at, enabled, one_shot)
		 VALUES ($1, NULLIF($2,'')::uuid, $3, $4, $5, $6, true, $7) RETURNING id`,
		defaultTenant, req.AgentID, title, expr, payloadJSON, nextRun, oneShot,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "one_shot": oneShot, "next_run_at": nextRun})
}

// handleCalendarTimeline returns the unified scheduled-work timeline for a window,
// optionally filtered by agent. Past + future across cron, runs, tasks and events.
func (gw *Gateway) handleCalendarTimeline(w http.ResponseWriter, r *http.Request) {
	if gw.calendarStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "calendar not available"})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	var aid *string
	if agentID != "" {
		aid = &agentID
	}
	start, _ := time.Parse(time.RFC3339, r.URL.Query().Get("start"))
	end, _ := time.Parse(time.RFC3339, r.URL.Query().Get("end"))
	if start.IsZero() {
		start = time.Now().AddDate(0, -1, 0)
	}
	if end.IsZero() {
		end = time.Now().AddDate(0, 1, 0)
	}
	items, err := gw.calendarStore.Timeline(r.Context(), defaultTenant, aid, start, end)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleCalendarRun returns a single recorded run's detail.
func (gw *Gateway) handleCalendarRun(w http.ResponseWriter, r *http.Request) {
	if gw.calendarStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "calendar not available"})
		return
	}
	run, err := gw.calendarStore.GetRun(r.Context(), defaultTenant, chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	writeJSON(w, http.StatusOK, run)
}
