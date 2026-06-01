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

// handleInjectMessage injects a user message into a currently-running agent session.
//
// POST /v1/sessions/{id}/inject
// Body: {"message": "string", "hide_input": false}
//
// 200 → message queued
// 400 → missing message body
// 409 → session is not currently running (agent idle)
// 429 → inject queue is full (8 messages in-flight max)
func (gw *Gateway) handleInjectMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	var body struct {
		Message   string `json:"message"`
		HideInput bool   `json:"hide_input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message required"})
		return
	}

	if gw.runRouter == nil || !gw.runRouter.IsSessionBusy(sessionID) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "agent is not currently running",
			"busy":  false,
		})
		return
	}

	ok := gw.runRouter.InjectMessage(sessionID, agent.InjectedMessage{
		Content:   body.Message,
		HideInput: body.HideInput,
	})
	if !ok {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "inject queue full"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "queued": true})
}

// handleSessionStatus reports whether a session has an active agent run.
//
// GET /v1/sessions/{id}/status
// Returns: {"busy": bool, "run_id": string, "phase": string, "tool": string, "iteration": int}
func (gw *Gateway) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	busy := gw.runRouter != nil && gw.runRouter.IsSessionBusy(sessionID)
	resp := map[string]any{"busy": busy}

	if busy {
		if runID, ok := gw.runRouter.SessionRunID(sessionID); ok {
			resp["run_id"] = runID
		}
		if act := gw.runRouter.GetActivity(sessionID); act != nil {
			resp["phase"] = act.Phase
			resp["tool"] = act.Tool
			resp["iteration"] = act.Iteration
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
