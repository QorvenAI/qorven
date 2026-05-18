// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// WorkspaceManager creates and destroys per-task git worktrees so concurrent
// agents never touch each other's working files.
//
// Layout (repoRoot must be a git repo):
//
//	<repoRoot>/../.qorven-workspaces/<taskID>/   ← isolated worktree
//
// Each worktree is on a fresh branch: task/<taskID>.
// Cleanup removes the worktree directory AND the branch.
type WorkspaceManager struct {
	repoRoot string
	baseDir  string // parent directory for all worktrees

	mu         sync.Mutex
	worktrees  map[string]string // taskID → abs path
}

// NewWorkspaceManager creates a manager rooted at repoRoot.
// The workspaces directory is created on first use.
func NewWorkspaceManager(repoRoot string) (*WorkspaceManager, error) {
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("workspace: resolve repoRoot: %w", err)
	}
	baseDir := filepath.Join(filepath.Dir(abs), ".qorven-workspaces")
	return &WorkspaceManager{
		repoRoot:  abs,
		baseDir:   baseDir,
		worktrees: make(map[string]string),
	}, nil
}

// Create sets up an isolated git worktree for taskID on a new branch.
// Returns the absolute path to the worktree root.
func (w *WorkspaceManager) Create(taskID string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if path, ok := w.worktrees[taskID]; ok {
		return path, nil // idempotent
	}

	if err := os.MkdirAll(w.baseDir, 0o755); err != nil {
		return "", fmt.Errorf("workspace: mkdir %s: %w", w.baseDir, err)
	}

	worktreePath := filepath.Join(w.baseDir, taskID)
	branch := "task/" + taskID

	// Remove a leftover directory if it somehow exists but git doesn't know about it.
	if _, err := os.Stat(worktreePath); err == nil {
		_ = os.RemoveAll(worktreePath)
	}

	// git worktree add -b <branch> <path> HEAD
	out, err := runGit(w.repoRoot, "worktree", "add", "-b", branch, worktreePath, "HEAD")
	if err != nil {
		return "", fmt.Errorf("workspace: git worktree add: %w\n%s", err, out)
	}

	w.worktrees[taskID] = worktreePath
	slog.Info("daemon.workspace.created", "task", taskID, "path", worktreePath, "branch", branch)
	return worktreePath, nil
}

// Path returns the worktree path for taskID, or "" if it was never created.
func (w *WorkspaceManager) Path(taskID string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.worktrees[taskID]
}

// Cleanup removes the worktree and its branch. Safe to call if never created.
func (w *WorkspaceManager) Cleanup(taskID string) error {
	w.mu.Lock()
	path, ok := w.worktrees[taskID]
	if ok {
		delete(w.worktrees, taskID)
	}
	w.mu.Unlock()

	if !ok {
		return nil
	}

	branch := "task/" + taskID

	// git worktree remove --force <path>
	if out, err := runGit(w.repoRoot, "worktree", "remove", "--force", path); err != nil {
		slog.Warn("daemon.workspace.remove_failed", "task", taskID, "err", err, "out", string(out))
		// Fall back to manual rm — the branch delete still matters.
		_ = os.RemoveAll(path)
	}

	// git branch -D task/<taskID>
	if out, err := runGit(w.repoRoot, "branch", "-D", branch); err != nil {
		slog.Warn("daemon.workspace.branch_delete_failed", "task", taskID, "branch", branch, "err", err, "out", string(out))
	}

	slog.Info("daemon.workspace.cleaned", "task", taskID, "branch", branch)
	return nil
}

// CleanupAll removes all tracked worktrees. Called on shutdown.
func (w *WorkspaceManager) CleanupAll() {
	w.mu.Lock()
	ids := make([]string, 0, len(w.worktrees))
	for id := range w.worktrees {
		ids = append(ids, id)
	}
	w.mu.Unlock()

	for _, id := range ids {
		if err := w.Cleanup(id); err != nil {
			slog.Warn("daemon.workspace.cleanup_all_err", "task", id, "err", err)
		}
	}
}

// List returns all active worktree paths keyed by taskID.
func (w *WorkspaceManager) List() map[string]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	cp := make(map[string]string, len(w.worktrees))
	for k, v := range w.worktrees {
		cp[k] = v
	}
	return cp
}

// runGit runs a git command in dir and returns combined output.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return s, nil
}
