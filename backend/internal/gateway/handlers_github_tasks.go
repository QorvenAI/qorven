// Copyright 2026 Tekky AI Academy LLP. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/agent"
)

// handleListGitHubTasks returns all entries in the in-memory GitHubTaskQueue.
//
// GET /v1/github/tasks
func (gw *Gateway) handleListGitHubTasks(w http.ResponseWriter, r *http.Request) {
	tasks := agent.GlobalGitHubTaskQueue.ListAll()
	if tasks == nil {
		tasks = []*agent.GitHubTask{}
	}
	writeJSON(w, 200, map[string]any{"tasks": tasks})
}

// handleGetGitHubTask returns a single GitHubTask by ID.
//
// GET /v1/github/tasks/:id
func (gw *Gateway) handleGetGitHubTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t := agent.GlobalGitHubTaskQueue.Get(id)
	if t == nil {
		writeJSON(w, 404, map[string]string{"error": "task not found"})
		return
	}
	writeJSON(w, 200, t)
}

// handleAdvanceGitHubTask manually advances a task to the next phase.
// Used by the TUI and web UI for operator overrides.
//
// POST /v1/github/tasks/:id/advance
// Body: { "phase": "coding" }
func (gw *Gateway) handleAdvanceGitHubTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t := agent.GlobalGitHubTaskQueue.Get(id)
	if t == nil {
		writeJSON(w, 404, map[string]string{"error": "task not found"})
		return
	}
	var body struct {
		Phase string `json:"phase"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Phase == "" {
		writeJSON(w, 400, map[string]string{"error": "phase is required"})
		return
	}
	agent.GlobalGitHubTaskQueue.Advance(id, agent.GitHubTaskPhase(body.Phase))
	writeJSON(w, 200, agent.GlobalGitHubTaskQueue.Get(id))
}

// handleBlockGitHubTask marks a task as blocked with a reason.
//
// POST /v1/github/tasks/:id/block
// Body: { "reason": "CI flaky — needs human review" }
func (gw *Gateway) handleBlockGitHubTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t := agent.GlobalGitHubTaskQueue.Get(id)
	if t == nil {
		writeJSON(w, 404, map[string]string{"error": "task not found"})
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	agent.GlobalGitHubTaskQueue.RecordTestFailure(id, body.Reason)
	writeJSON(w, 200, agent.GlobalGitHubTaskQueue.Get(id))
}
