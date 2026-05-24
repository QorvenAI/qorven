// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// handleProjectCheckpoint creates a git checkpoint commit in the project workspace.
// POST /v1/projects/{id}/checkpoint
func (gw *Gateway) handleProjectCheckpoint(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.projectReg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not available"})
		return
	}

	id := chi.URLParam(r, "id")
	project := gw.projectReg.Get(id)
	if project == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	workspace := resolveWorkspace(project)
	label := fmt.Sprintf("qorven:checkpoint:auto:%d", time.Now().UnixMilli())

	// git add -A && git commit
	gitAdd := exec.CommandContext(r.Context(), "git", "-C", workspace, "add", "-A")
	if out, err := gitAdd.CombinedOutput(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "git add failed: " + strings.TrimSpace(string(out))})
		return
	}

	gitCommit := exec.CommandContext(r.Context(), "git", "-C", workspace, "commit", "-m", label, "--allow-empty")
	out, err := gitCommit.CombinedOutput()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "git commit failed: " + strings.TrimSpace(string(out))})
		return
	}

	// Get the commit hash
	hashOut, _ := exec.CommandContext(r.Context(), "git", "-C", workspace, "rev-parse", "--short", "HEAD").Output()
	hash := strings.TrimSpace(string(hashOut))

	writeJSON(w, http.StatusOK, map[string]any{
		"checkpoint": label,
		"hash":       hash,
		"workspace":  workspace,
	})
}

// handleProjectUndo reverts the last commit in the project workspace.
// POST /v1/projects/{id}/undo
func (gw *Gateway) handleProjectUndo(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.projectReg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not available"})
		return
	}

	id := chi.URLParam(r, "id")
	project := gw.projectReg.Get(id)
	if project == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	workspace := resolveWorkspace(project)

	// git revert HEAD --no-edit (creates a new revert commit, safe for history)
	cmd := exec.CommandContext(r.Context(), "git", "-C", workspace, "revert", "HEAD", "--no-edit")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// If nothing to revert, it's not a fatal error
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "undo failed: " + strings.TrimSpace(string(out))})
		return
	}

	hashOut, _ := exec.CommandContext(r.Context(), "git", "-C", workspace, "rev-parse", "--short", "HEAD").Output()
	hash := strings.TrimSpace(string(hashOut))

	writeJSON(w, http.StatusOK, map[string]any{
		"reverted": true,
		"hash":     hash,
	})
}

// handleProjectCheckpoints lists recent checkpoint commits.
// GET /v1/projects/{id}/checkpoints
func (gw *Gateway) handleProjectCheckpoints(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.projectReg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not available"})
		return
	}

	id := chi.URLParam(r, "id")
	project := gw.projectReg.Get(id)
	if project == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	workspace := resolveWorkspace(project)

	// git log for checkpoint commits
	cmd := exec.CommandContext(r.Context(), "git", "-C", workspace, "log",
		"--oneline", "--grep=qorven:checkpoint", "-20",
		"--format=%H|%h|%ai|%s")
	out, err := cmd.Output()
	if err != nil {
		// Not a git repo yet — return empty list
		writeJSON(w, http.StatusOK, map[string]any{"checkpoints": []any{}})
		return
	}

	type Checkpoint struct {
		Hash      string `json:"hash"`
		ShortHash string `json:"short_hash"`
		Date      string `json:"date"`
		Message   string `json:"message"`
	}

	var checkpoints []Checkpoint
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		checkpoints = append(checkpoints, Checkpoint{
			Hash: parts[0], ShortHash: parts[1], Date: parts[2], Message: parts[3],
		})
	}

	if checkpoints == nil {
		checkpoints = []Checkpoint{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"checkpoints": checkpoints})
}

// handleProjectRestore restores the project to a specific checkpoint commit.
// POST /v1/projects/{id}/restore
func (gw *Gateway) handleProjectRestore(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.projectReg == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "projects not available"})
		return
	}

	id := chi.URLParam(r, "id")
	project := gw.projectReg.Get(id)
	if project == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}

	commit := r.URL.Query().Get("commit")
	if commit == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "commit parameter required"})
		return
	}
	// Sanitize: only allow hex chars (git hash)
	for _, c := range commit {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid commit hash"})
			return
		}
	}

	workspace := resolveWorkspace(project)

	cmd := exec.CommandContext(r.Context(), "git", "-C", workspace, "checkout", commit, "--", ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "restore failed: " + strings.TrimSpace(string(out))})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"restored": true, "commit": commit})
}
