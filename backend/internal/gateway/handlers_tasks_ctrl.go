// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/tasks"
)

// POST /v1/tasks/{id}/pause
func (gw *Gateway) handleTaskPause(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if gw.taskStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task store unavailable"})
		return
	}
	if err := gw.taskStore.Transition(r.Context(), id, tasks.StatusPaused); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused", "task_id": id})
}

// POST /v1/tasks/{id}/resume
func (gw *Gateway) handleTaskResume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if gw.taskStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task store unavailable"})
		return
	}
	if err := gw.taskStore.Transition(r.Context(), id, tasks.StatusInProgress); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "in_progress", "task_id": id})
}

// POST /v1/tasks/{id}/message
// Appends a user-injected message to the task scratchpad so the agent
// reads it at the start of the next iteration.
func (gw *Gateway) handleTaskMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if gw.taskStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task store unavailable"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10) // 64 KB ceiling on task messages
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message required"})
		return
	}
	t, err := gw.taskStore.Get(r.Context(), id)
	if err != nil || t == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	updated := t.Scratchpad + "\n[USER]: " + body.Message
	if err := gw.taskStore.UpdateScratchpad(r.Context(), id, updated); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "message_appended", "task_id": id})
}
