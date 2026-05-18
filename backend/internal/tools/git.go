// Copyright 2026 Tekky AI Academy LLP. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GitStatusTool shows the current working-tree status.
type GitStatusTool struct{}

func NewGitStatusTool() *GitStatusTool { return &GitStatusTool{} }
func (t *GitStatusTool) Name() string  { return "git_status" }
func (t *GitStatusTool) Description() string {
	return "Show the working-tree status (staged, unstaged, untracked files). Run this after making changes to see what changed."
}
func (t *GitStatusTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *GitStatusTool) Execute(ctx context.Context, _ map[string]any) *Result {
	return runGit(ctx, WorkspaceFromCtx(ctx), "status", "--short", "--branch")
}

// GitDiffTool shows the diff for staged or unstaged changes.
type GitDiffTool struct{}

func NewGitDiffTool() *GitDiffTool { return &GitDiffTool{} }
func (t *GitDiffTool) Name() string { return "git_diff" }
func (t *GitDiffTool) Description() string {
	return "Show a unified diff of changes. By default shows unstaged changes; pass staged=true for staged changes, or a file path to limit scope."
}
func (t *GitDiffTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"staged": map[string]any{
				"type":        "boolean",
				"description": "If true, show staged (cached) changes. Default: unstaged.",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Limit diff to this file or directory.",
			},
		},
	}
}
func (t *GitDiffTool) Execute(ctx context.Context, args map[string]any) *Result {
	staged, _ := args["staged"].(bool)
	path, _ := args["path"].(string)
	gitArgs := []string{"diff"}
	if staged {
		gitArgs = append(gitArgs, "--cached")
	}
	gitArgs = append(gitArgs, "--stat", "--patch", "--no-color")
	if path != "" {
		gitArgs = append(gitArgs, "--", path)
	}
	return runGit(ctx, WorkspaceFromCtx(ctx), gitArgs...)
}

// GitLogTool shows recent commit history.
type GitLogTool struct{}

func NewGitLogTool() *GitLogTool { return &GitLogTool{} }
func (t *GitLogTool) Name() string { return "git_log" }
func (t *GitLogTool) Description() string {
	return "Show recent commit history with author, date, and message. Default: last 10 commits."
}
func (t *GitLogTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit": map[string]any{
				"type":        "integer",
				"description": "Number of commits to show (default 10, max 50).",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Limit log to commits touching this file or directory.",
			},
		},
	}
}
func (t *GitLogTool) Execute(ctx context.Context, args map[string]any) *Result {
	limit := 10
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
		if limit > 50 {
			limit = 50
		}
	}
	path, _ := args["path"].(string)
	gitArgs := []string{"log", fmt.Sprintf("--max-count=%d", limit),
		"--pretty=format:%h %ad %an — %s", "--date=short"}
	if path != "" {
		gitArgs = append(gitArgs, "--", path)
	}
	return runGit(ctx, WorkspaceFromCtx(ctx), gitArgs...)
}

// runGit runs a git subcommand in dir, returns a Result.
func runGit(ctx context.Context, dir string, gitArgs ...string) *Result {
	if dir == "" {
		dir = "."
	}
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Dir = dir
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return ErrorResult("git " + gitArgs[0] + ": " + msg)
	}
	result := strings.TrimSpace(out.String())
	if result == "" {
		return TextResult("(no output — working tree is clean)")
	}
	// Cap at 20KB to stay within context budget.
	if len(result) > 20000 {
		result = result[:20000] + "\n[truncated — output exceeded 20KB]"
	}
	return TextResult(result)
}
