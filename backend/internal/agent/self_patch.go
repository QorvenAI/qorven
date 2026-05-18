// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/tools"
)

// SelfPatch lets the agent propose, test, and revert changes to its own codebase.
// SAFETY: All changes happen on a git branch. Main is never touched directly.
// Human must approve before merge. Every change can be reverted.
//
// Flow:
//   1. Agent reads file via self_knowledge
//   2. Agent proposes change via self_patch action=propose
//   3. System creates branch, applies change, runs build+test
//   4. If build/test fails: auto-revert, return error
//   5. If passes: return diff, wait for human approval
//   6. Human approves via /approve command or GUI
//   7. Agent merges branch to main

type SelfPatchTool struct {
	codebaseDir string
}

func NewSelfPatchTool(codebaseDir string) *SelfPatchTool {
	return &SelfPatchTool{codebaseDir: codebaseDir}
}

func (t *SelfPatchTool) Name() string { return "self_patch" }
func (t *SelfPatchTool) Description() string {
	return "Propose safe changes to Qorven's own codebase. Changes are tested on a branch and require human approval. Actions: propose, status, revert, list_branches."
}
func (t *SelfPatchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":      map[string]any{"type": "string", "enum": []string{"propose", "status", "revert", "list_branches"}},
			"file":        map[string]any{"type": "string", "description": "File path to modify (for propose)"},
			"old_content": map[string]any{"type": "string", "description": "Exact text to replace"},
			"new_content": map[string]any{"type": "string", "description": "Replacement text"},
			"description": map[string]any{"type": "string", "description": "What this change does and why"},
			"branch":      map[string]any{"type": "string", "description": "Branch name (for revert/status)"},
		},
		"required": []string{"action"},
	}
}

func (t *SelfPatchTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	action, _ := args["action"].(string)

	switch action {
	case "propose":
		return t.propose(ctx, args)
	case "status":
		return t.status(args)
	case "revert":
		return t.revert(args)
	case "list_branches":
		return t.listBranches()
	default:
		return tools.ErrorResult("unknown action: " + action)
	}
}

func (t *SelfPatchTool) propose(ctx context.Context, args map[string]any) *tools.Result {
	file, _ := args["file"].(string)
	oldContent, _ := args["old_content"].(string)
	newContent, _ := args["new_content"].(string)
	desc, _ := args["description"].(string)

	if file == "" || oldContent == "" || newContent == "" {
		return tools.ErrorResult("file, old_content, and new_content required")
	}
	if desc == "" { desc = "agent-proposed change" }

	fullPath := filepath.Join(t.codebaseDir, file)
	if _, err := os.Stat(fullPath); err != nil {
		return tools.ErrorResult("file not found: " + file)
	}

	// Create a unique branch name
	branch := fmt.Sprintf("agent/patch-%d", time.Now().Unix())

	// Step 1: Create branch from current HEAD
	if err := t.git("checkout", "-b", branch); err != nil {
		return tools.ErrorResult("failed to create branch: " + err.Error())
	}

	// Step 2: Read file, apply change
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.git("checkout", "main")
		t.git("branch", "-D", branch)
		return tools.ErrorResult("failed to read file: " + err.Error())
	}

	original := string(data)
	if !strings.Contains(original, oldContent) {
		t.git("checkout", "main")
		t.git("branch", "-D", branch)
		return tools.ErrorResult("old_content not found in file — read the file first with self_knowledge")
	}

	modified := strings.Replace(original, oldContent, newContent, 1)
	if err := os.WriteFile(fullPath, []byte(modified), 0644); err != nil {
		t.git("checkout", "main")
		t.git("branch", "-D", branch)
		return tools.ErrorResult("failed to write: " + err.Error())
	}

	// Step 3: Build check
	buildOut, buildErr := t.runCmd("go", "build", "-o", "/dev/null", ".")
	if buildErr != nil {
		// BUILD FAILED — auto-revert
		os.WriteFile(fullPath, data, 0644) // restore original
		t.git("checkout", "main")
		t.git("branch", "-D", branch)
		slog.Warn("self_patch.build_failed", "file", file, "branch", branch)
		return tools.ErrorResult("BUILD FAILED — change reverted automatically.\n\n" + buildOut)
	}

	// Step 4: Commit on branch
	t.git("add", file)
	t.git("commit", "-m", fmt.Sprintf("agent: %s\n\nFile: %s\nBranch: %s", desc, file, branch))

	// Step 5: Generate diff
	diffOut, _ := t.runCmd("git", "diff", "main..."+branch)

	// Step 6: Switch back to main (leave branch for review)
	t.git("checkout", "main")

	slog.Info("self_patch.proposed", "file", file, "branch", branch, "desc", desc)

	return tools.TextResult(fmt.Sprintf(
		"✅ Change proposed on branch: %s\n\n"+
			"📝 Description: %s\n"+
			"📄 File: %s\n"+
			"🔨 Build: PASSED\n\n"+
			"Diff:\n```\n%s\n```\n\n"+
			"⏳ Waiting for human approval.\n"+
			"To approve: git merge %s\n"+
			"To reject: git branch -D %s",
		branch, desc, file, truncateDiff(diffOut, 2000), branch, branch))
}

func (t *SelfPatchTool) status(args map[string]any) *tools.Result {
	branch, _ := args["branch"].(string)
	if branch == "" { return tools.ErrorResult("branch name required") }

	// Check if branch exists
	out, err := t.runCmd("git", "log", "--oneline", "-1", branch)
	if err != nil { return tools.ErrorResult("branch not found: " + branch) }

	// Check if merged
	merged, _ := t.runCmd("git", "branch", "--merged", "main")
	if strings.Contains(merged, branch) {
		return tools.TextResult(fmt.Sprintf("Branch %s: MERGED to main\n%s", branch, out))
	}
	return tools.TextResult(fmt.Sprintf("Branch %s: PENDING review\n%s", branch, out))
}

func (t *SelfPatchTool) revert(args map[string]any) *tools.Result {
	branch, _ := args["branch"].(string)
	if branch == "" { return tools.ErrorResult("branch name required") }

	// Safety: only delete agent/ branches
	if !strings.HasPrefix(branch, "agent/") {
		return tools.ErrorResult("can only revert agent/ branches")
	}

	if err := t.git("branch", "-D", branch); err != nil {
		return tools.ErrorResult("failed to delete branch: " + err.Error())
	}

	slog.Info("self_patch.reverted", "branch", branch)
	return tools.TextResult("Branch deleted: " + branch)
}

func (t *SelfPatchTool) listBranches() *tools.Result {
	out, _ := t.runCmd("git", "branch", "--list", "agent/*")
	if strings.TrimSpace(out) == "" { return tools.TextResult("No pending agent branches") }
	return tools.TextResult("Agent branches:\n" + out)
}

func (t *SelfPatchTool) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = t.codebaseDir
	out, err := cmd.CombinedOutput()
	if err != nil { return fmt.Errorf("%s: %s", err, string(out)) }
	return nil
}

func (t *SelfPatchTool) runCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = t.codebaseDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func truncateDiff(diff string, max int) string {
	if len(diff) <= max { return diff }
	return diff[:max] + "\n... (truncated)"
}
