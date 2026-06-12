// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/bootstrap"
	"github.com/qorvenai/qorven/internal/tools"
)

// workspaceEditableFiles is the allow-list of bootstrap files a user may view
// and edit through Drive. These are the real on-disk files the agent loads into
// its system prompt; editing SOUL.md here changes the agent's persona next run.
var workspaceEditableFiles = map[string]bool{
	bootstrap.SoulFile:     true,
	bootstrap.IdentityFile: true,
	bootstrap.UserFile:     true,
	bootstrap.AgentsFile:   true,
	bootstrap.ToolsFile:    true,
	bootstrap.MemoryFile:   true,
}

// workspaceFileEditable reports whether name is an allow-listed bootstrap file
// (exact match only — no paths, no traversal).
func workspaceFileEditable(name string) bool {
	if name != filepath.Base(name) {
		return false
	}
	return workspaceEditableFiles[name]
}

func (gw *Gateway) handleListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")
	if agentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id required"})
		return
	}
	ws := tools.AgentWorkspace(agentID)
	loaded := bootstrap.LoadWorkspaceFiles(ws)
	out := []map[string]any{}
	for _, f := range loaded {
		if !workspaceEditableFiles[f.Name] {
			continue
		}
		out = append(out, map[string]any{
			"name":     f.Name,
			"missing":  f.Missing,
			"editable": true,
			"size":     len(f.Content),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": out})
}

func (gw *Gateway) handleGetWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")
	name := chi.URLParam(r, "name")
	if !workspaceFileEditable(name) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "file not editable"})
		return
	}
	ws := tools.AgentWorkspace(agentID)
	path := filepath.Join(ws, name)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]any{"name": name, "content": "", "missing": true})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "content": string(content), "missing": false})
}

func (gw *Gateway) handlePutWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agent_id")
	name := chi.URLParam(r, "name")
	if !workspaceFileEditable(name) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "file not editable"})
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if len(body.Content) > tools.MaxFileSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file too large"})
		return
	}
	ws := tools.AgentWorkspace(agentID)
	path := filepath.Join(ws, name)
	if err := os.WriteFile(path, []byte(body.Content), 0644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write failed"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
