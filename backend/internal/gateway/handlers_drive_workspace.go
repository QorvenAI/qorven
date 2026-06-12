// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/bootstrap"
	"github.com/qorvenai/qorven/internal/tools"
)

// workspaceEditableFiles is the allow-list of bootstrap files a user may view
// and edit through Drive. These are the files the agent loads from the DB into
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

// workspaceWriteAllowed gates the workspace endpoints. Editing an agent's
// SOUL.md / persona files changes what the agent loads into its prompt next run,
// so it is a privileged, behavior-altering operation. Today (single-tenant) we
// require an admin operator; when multi-user lands this should additionally gate
// on agent ownership.
func (gw *Gateway) workspaceWriteAllowed(r *http.Request) bool {
	u := userFromContext(r.Context())
	return u != nil && u.Role == "admin"
}

func (gw *Gateway) handleListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	if !gw.workspaceWriteAllowed(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
		return
	}
	agentID := chi.URLParam(r, "agent_id")
	if agentID == "" || gw.agents == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id required"})
		return
	}
	existing, _ := gw.agents.GetAgentContextFiles(r.Context(), agentID)
	out := []map[string]any{}
	for name := range workspaceEditableFiles {
		content, present := existing[name]
		out = append(out, map[string]any{"name": name, "missing": !present, "editable": true, "size": len(content)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": out})
}

func (gw *Gateway) handleGetWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	if !gw.workspaceWriteAllowed(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
		return
	}
	agentID := chi.URLParam(r, "agent_id")
	name := chi.URLParam(r, "name")
	if !workspaceFileEditable(name) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "file not editable"})
		return
	}
	existing, _ := gw.agents.GetAgentContextFiles(r.Context(), agentID)
	content, present := existing[name]
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "content": content, "missing": !present})
}

func (gw *Gateway) handlePutWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	if !gw.workspaceWriteAllowed(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
		return
	}
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
	if err := gw.agents.SetAgentContextFile(r.Context(), agentID, name, body.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListWorkspaceVersions lists prior saved versions of a context file.
func (gw *Gateway) handleListWorkspaceVersions(w http.ResponseWriter, r *http.Request) {
	if !gw.workspaceWriteAllowed(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
		return
	}
	agentID := chi.URLParam(r, "agent_id")
	name := chi.URLParam(r, "name")
	if !workspaceFileEditable(name) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "file not editable"})
		return
	}
	versions, err := gw.agents.ListContextFileVersions(r.Context(), agentID, name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

// handleRestoreWorkspaceVersion restores a prior version's content as the current file.
func (gw *Gateway) handleRestoreWorkspaceVersion(w http.ResponseWriter, r *http.Request) {
	if !gw.workspaceWriteAllowed(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin role required"})
		return
	}
	agentID := chi.URLParam(r, "agent_id")
	versionID := chi.URLParam(r, "version_id")
	v, err := gw.agents.GetContextFileVersion(r.Context(), agentID, versionID)
	if err != nil || v == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "version not found"})
		return
	}
	if err := gw.agents.SetAgentContextFile(r.Context(), agentID, v.FileName, v.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
