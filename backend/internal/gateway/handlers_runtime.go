// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/agent"
)

// POST /v1/agents/{id}/runtime/pause
func (gw *Gateway) handleRuntimePause(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if gw.runtimeMgr == nil || !gw.runtimeMgr.Suspend(id) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no persistent runtime for agent"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "suspended", "agent_id": id})
}

// POST /v1/agents/{id}/runtime/resume
func (gw *Gateway) handleRuntimeResume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if gw.runtimeMgr == nil || !gw.runtimeMgr.Resume(id) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no persistent runtime for agent"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "resumed", "agent_id": id})
}

// POST /v1/agents/{id}/runtime/wakeup
func (gw *Gateway) handleRuntimeWakeup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Message string `json:"message"`
		TaskID  string `json:"task_id"`
	}
	json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck

	source := agent.WakeupManual
	if body.TaskID != "" {
		source = agent.WakeupAssignment
	}
	sig := agent.WakeupSignal{
		Source:  source,
		Message: body.Message,
		TaskID:  body.TaskID,
	}
	if gw.runtimeMgr == nil || !gw.runtimeMgr.WakeAgent(id, sig) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no persistent runtime for agent"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "wakeup_sent", "agent_id": id})
}

// POST /v1/agents/{id}/runtime/override
func (gw *Gateway) handleRuntimeOverride(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message required"})
		return
	}
	if gw.runtimeMgr == nil || !gw.runtimeMgr.Override(id, body.Message) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no persistent runtime for agent"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "override_queued", "agent_id": id})
}

// GET /v1/runtime/states
func (gw *Gateway) handleRuntimeStates(w http.ResponseWriter, r *http.Request) {
	if gw.runtimeMgr == nil {
		writeJSON(w, http.StatusOK, map[string]any{"states": map[string]string{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"states": gw.runtimeMgr.States()})
}

// GET /v1/tasks/{id}/events
func (gw *Gateway) handleListTaskEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if gw.taskStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task store unavailable"})
		return
	}
	events, err := gw.taskStore.ListEvents(r.Context(), id, 200)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
