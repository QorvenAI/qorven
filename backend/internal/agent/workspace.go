// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorkspaceMode determines how an agent's workspace is provisioned.
type WorkspaceMode string

const (
	WorkspaceDefault  WorkspaceMode = "default"  // agent's own directory
	WorkspaceShared   WorkspaceMode = "shared"   // shared project directory
	WorkspaceIsolated WorkspaceMode = "isolated" // git worktree per task
)

// Workspace represents a resolved execution environment for an agent run.
type Workspace struct {
	Mode       WorkspaceMode
	Dir        string // absolute path to workspace
	BranchName string // git branch (for isolated mode)
	ProjectDir string // parent project dir (for isolated mode)
	Created    bool   // true if workspace was created by this call
}

// ResolveWorkspace provisions the workspace for an agent run.
func ResolveWorkspace(agentID, taskID string, mode WorkspaceMode, baseDir string) (*Workspace, error) {
	if baseDir == "" {
		baseDir = filepath.Join(os.TempDir(), "qorven-workspaces")
	}

	switch mode {
	case WorkspaceIsolated:
		return resolveIsolatedWorkspace(agentID, taskID, baseDir)
	case WorkspaceShared:
		return resolveSharedWorkspace(baseDir)
	default:
		return resolveDefaultWorkspace(agentID, baseDir)
	}
}

func resolveDefaultWorkspace(agentID, baseDir string) (*Workspace, error) {
	dir := filepath.Join(baseDir, "agents", sanitizePath(agentID))
	created := false
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create workspace: %w", err)
		}
		created = true
	}
	return &Workspace{Mode: WorkspaceDefault, Dir: dir, Created: created}, nil
}

func resolveSharedWorkspace(baseDir string) (*Workspace, error) {
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("shared workspace does not exist: %s", baseDir)
	}
	return &Workspace{Mode: WorkspaceShared, Dir: baseDir}, nil
}

func resolveIsolatedWorkspace(agentID, taskID, baseDir string) (*Workspace, error) {
	projectDir := baseDir
	if _, err := os.Stat(filepath.Join(projectDir, ".git")); os.IsNotExist(err) {
		// Not a git repo — fall back to directory-based isolation
		dir := filepath.Join(projectDir, "worktrees", sanitizePath(taskID))
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create isolated dir: %w", err)
		}
		return &Workspace{Mode: WorkspaceIsolated, Dir: dir, Created: true}, nil
	}

	// Git worktree isolation
	branch := fmt.Sprintf("task/%s", sanitizePath(taskID))
	worktreeDir := filepath.Join(projectDir, ".worktrees", sanitizePath(taskID))

	// Check if worktree already exists
	if _, err := os.Stat(worktreeDir); err == nil {
		return &Workspace{
			Mode: WorkspaceIsolated, Dir: worktreeDir,
			BranchName: branch, ProjectDir: projectDir,
		}, nil
	}

	// Create git worktree
	cmd := exec.Command("git", "worktree", "add", "-b", branch, worktreeDir)
	cmd.Dir = projectDir
	if out, err := cmd.CombinedOutput(); err != nil {
		// Branch might already exist — try without -b
		cmd2 := exec.Command("git", "worktree", "add", worktreeDir, branch)
		cmd2.Dir = projectDir
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			slog.Warn("workspace.worktree_failed", "error", string(out), "error2", string(out2))
			// Fall back to directory isolation
			if mkErr := os.MkdirAll(worktreeDir, 0755); mkErr != nil {
				return nil, fmt.Errorf("create worktree fallback: %w", mkErr)
			}
			return &Workspace{Mode: WorkspaceIsolated, Dir: worktreeDir, Created: true}, nil
		}
	}

	slog.Info("workspace.worktree_created", "dir", worktreeDir, "branch", branch)
	return &Workspace{
		Mode: WorkspaceIsolated, Dir: worktreeDir,
		BranchName: branch, ProjectDir: projectDir, Created: true,
	}, nil
}

// CleanupWorkspace removes an isolated workspace after the task completes.
func CleanupWorkspace(ws *Workspace) error {
	if ws.Mode != WorkspaceIsolated || !ws.Created {
		return nil
	}

	// If git worktree, remove properly
	if ws.ProjectDir != "" && ws.BranchName != "" {
		cmd := exec.Command("git", "worktree", "remove", ws.Dir, "--force")
		cmd.Dir = ws.ProjectDir
		if out, err := cmd.CombinedOutput(); err != nil {
			slog.Warn("workspace.cleanup_worktree_failed", "dir", ws.Dir, "error", string(out))
			// Fall back to rm
			return os.RemoveAll(ws.Dir)
		}
		slog.Info("workspace.worktree_removed", "dir", ws.Dir)
		return nil
	}

	return os.RemoveAll(ws.Dir)
}

func sanitizePath(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, "..", "_")
	s = strings.ReplaceAll(s, " ", "_")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}
