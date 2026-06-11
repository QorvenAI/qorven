// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// GET /v1/projects/{id}/hub — returns the project's coordination room id (creates it if needed).
func (gw *Gateway) handleProjectHub(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")
	roomID := gw.ensureProjectHub(r.Context(), id)
	if roomID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"room_id": ""})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"room_id": roomID})
}

// GET /v1/projects/{id}/events?limit=100 — newest-first structured timeline.
func (gw *Gateway) handleProjectEvents(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")
	limit := 100
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id::text, type, title, payload, COALESCE(task_id::text,''), COALESCE(agent_id::text,''), created_at
		 FROM project_events WHERE tenant_id=$1 AND project_brief_id=$2
		 ORDER BY created_at DESC LIMIT $3`, defaultTenant, id, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var eid, typ, title, taskID, agentID string
		var payload json.RawMessage
		var created any
		if rows.Scan(&eid, &typ, &title, &payload, &taskID, &agentID, &created) == nil {
			out = append(out, map[string]any{
				"id":         eid,
				"type":       typ,
				"title":      title,
				"payload":    payload,
				"task_id":    taskID,
				"agent_id":   agentID,
				"created_at": created,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out})
}
