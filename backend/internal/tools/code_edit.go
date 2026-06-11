// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/agent/editapply"
	"github.com/qorvenai/qorven/internal/diff"
)

// CodeEditTool applies a single search/replace block to a file inside the
// task workspace. It uses the editapply 4-strategy cascade so minor
// whitespace differences don't break the match. An mtime guard detects
// out-of-band file changes so the agent re-reads before editing.
//
// An optional verify function is injected at construction; when set it is
// run after every successful write. Failure output is returned as the tool
// result so the agent can repair forward — the write is NOT rolled back.
//
// An optional onEdit callback is called after every successful write with the
// absolute path and a unified diff string (old→new). Used by the gateway to
// emit file.edited realtime events without coupling the tool to the event bus.
type CodeEditTool struct {
	baseDir string
	verify  func(ctx context.Context, dir string) (string, bool) // nil = skip
	onEdit  func(path, diffText string)                          // nil = skip

	mu     sync.Mutex
	mtimes map[string]time.Time // tracked last-seen mtime per absolute path
}

// NewCodeEditTool constructs a CodeEditTool scoped to baseDir.
// verify may be nil (post-edit verification is skipped).
// onEdit may be nil (file.edited event emission is skipped).
func NewCodeEditTool(baseDir string, verify func(ctx context.Context, dir string) (string, bool), onEdit func(path, diffText string)) *CodeEditTool {
	return &CodeEditTool{
		baseDir: baseDir,
		verify:  verify,
		onEdit:  onEdit,
		mtimes:  make(map[string]time.Time),
	}
}

func (t *CodeEditTool) Name() string { return "code_edit" }

func (t *CodeEditTool) Description() string {
	return `Apply a single SEARCH/REPLACE block to a file.

The SEARCH text must match the existing file content exactly (modulo leading-whitespace flexibility).
The matched region is replaced with REPLACE. One call = one block; compose multiple calls for multiple edits.

Rules:
- Read the file first so your SEARCH text is accurate.
- If the file has changed since you last read it, the tool will tell you — re-read and retry.
- If SEARCH matches more than one location, the tool will tell you — add more surrounding context to make it unique.
- Paths are relative to the task workspace root.`
}

func (t *CodeEditTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File path relative to the workspace root.",
			},
			"search": map[string]any{
				"type":        "string",
				"description": "The exact text to find in the file (must match existing content).",
			},
			"replace": map[string]any{
				"type":        "string",
				"description": "The text to substitute in place of the matched SEARCH block.",
			},
		},
		"required": []string{"path", "search", "replace"},
	}
}

func (t *CodeEditTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	search, _ := args["search"].(string)
	replace, _ := args["replace"].(string)

	if path == "" {
		return ErrorResult("code_edit: path is required")
	}
	if search == "" {
		return ErrorResult("code_edit: search is required")
	}

	// ── 1. Resolve + sandbox path ─────────────────────────────────────────────
	// resolvePath is defined in exec.go (same package). mustExist=true so we
	// catch missing files before reading.
	absPath, err := resolvePath(path, t.baseDir, true)
	if err != nil {
		return ErrorResult(fmt.Sprintf("code_edit: %v", err))
	}

	// ── 2. mtime-assert ───────────────────────────────────────────────────────
	info, err := os.Stat(absPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("code_edit: cannot stat %s: %v", path, err))
	}
	currentMtime := info.ModTime()

	t.mu.Lock()
	storedMtime, seen := t.mtimes[absPath]
	t.mu.Unlock()

	if seen && !storedMtime.Equal(currentMtime) {
		// File changed out-of-band since we last touched it. Record the new
		// mtime so the retry (after the agent re-reads) will succeed.
		t.mu.Lock()
		t.mtimes[absPath] = currentMtime
		t.mu.Unlock()
		return ErrorResult(fmt.Sprintf(
			"code_edit: %s was modified externally (mtime changed); re-read the file before editing",
			path,
		))
	}

	// ── 3. Read content ───────────────────────────────────────────────────────
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return ErrorResult(fmt.Sprintf("code_edit: read %s: %v", path, err))
	}
	content := string(raw)

	// ── 4. Apply search/replace ───────────────────────────────────────────────
	out, applyErr := editapply.Apply(content, search, replace)
	if applyErr != nil {
		// Return the error as text — it's a recoverable repair prompt for the
		// agent (not an infrastructure failure).
		return TextResult(fmt.Sprintf("code_edit failed: %v", applyErr))
	}

	// ── 5. Write back (preserve file mode) ───────────────────────────────────
	mode := info.Mode()
	if err := os.WriteFile(absPath, []byte(out), mode); err != nil {
		return ErrorResult(fmt.Sprintf("code_edit: write %s: %v", path, err))
	}

	// Update stored mtime from the just-written file.
	if newInfo, err := os.Stat(absPath); err == nil {
		t.mu.Lock()
		t.mtimes[absPath] = newInfo.ModTime()
		t.mu.Unlock()
	} else {
		// Can't stat — clear the entry so the next call re-baselines cleanly.
		t.mu.Lock()
		delete(t.mtimes, absPath)
		t.mu.Unlock()
	}

	// ── 6. Notify caller of the edit (file.edited event hook) ────────────────
	if t.onEdit != nil {
		editDiff, _, _ := diff.GenerateDiff(content, out, path)
		t.onEdit(absPath, editDiff)
	}

	// ── 7. Post-edit verify ───────────────────────────────────────────────────
	if t.verify != nil {
		verifyOut, ok := t.verify(ctx, t.baseDir)
		if !ok {
			// Write stays; agent repairs forward using the compiler/linter output.
			return TextResult(fmt.Sprintf(
				"code_edit: wrote %s — verify FAILED:\n%s",
				path, verifyOut,
			))
		}
		return SuccessResult(fmt.Sprintf("code_edit: edited %s; verify ok", path))
	}

	return SuccessResult(fmt.Sprintf("code_edit: edited %s", path))
}
