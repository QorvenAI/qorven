// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MultiEditTool applies multiple file writes in a single atomic-ish operation.
// All files are validated first; if any path is unsafe, nothing is written.
// On partial OS failure the successful writes are reported and the failures
// are listed — the agent can re-run just the failed subset.
type MultiEditTool struct {
	workspace string
	restrict  bool
}

func NewMultiEditTool(workspace string) *MultiEditTool {
	return &MultiEditTool{workspace: workspace, restrict: true}
}

func (t *MultiEditTool) Name() string { return "multi_edit" }
func (t *MultiEditTool) Description() string {
	return `Write multiple files in one operation. All paths are validated before any file is touched.
Use this instead of calling write_file repeatedly when you need to create or update several files at once.

Each entry in the edits array must have:
- path: file path relative to the workspace (or absolute if allowed)
- content: new full content for the file`
}
func (t *MultiEditTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"edits": map[string]any{
				"type":        "array",
				"description": "List of files to write",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "File path (relative to workspace or absolute)",
						},
						"content": map[string]any{
							"type":        "string",
							"description": "Full new content for the file",
						},
					},
					"required": []string{"path", "content"},
				},
				"minItems": 2,
				"maxItems": 20,
			},
		},
		"required": []string{"edits"},
	}
}

func (t *MultiEditTool) Execute(ctx context.Context, args map[string]any) *Result {
	rawEdits, ok := args["edits"].([]any)
	if !ok || len(rawEdits) == 0 {
		return ErrorResult("edits array is required")
	}

	ws := WorkspaceFromCtx(ctx)
	if ws == "" {
		ws = t.workspace
	}

	type editEntry struct {
		rawPath string
		safePath string
		content string
	}

	// Phase 1: validate all paths before touching the filesystem.
	entries := make([]editEntry, 0, len(rawEdits))
	for i, r := range rawEdits {
		m, ok := r.(map[string]any)
		if !ok {
			return ErrorResult(fmt.Sprintf("edits[%d]: must be an object with path and content", i))
		}
		rawPath, _ := m["path"].(string)
		content, _ := m["content"].(string)
		if rawPath == "" {
			return ErrorResult(fmt.Sprintf("edits[%d]: path is required", i))
		}
		safe, err := SafePathWithRestrict(ws, rawPath, t.restrict, nil)
		if err != nil {
			return ErrorResult(fmt.Sprintf("edits[%d] path %q: %v", i, rawPath, err))
		}
		entries = append(entries, editEntry{rawPath: rawPath, safePath: safe, content: content})
	}

	// Phase 2: write all files, collect results.
	var succeeded []string
	var failed []string

	for _, e := range entries {
		if err := os.MkdirAll(filepath.Dir(e.safePath), 0755); err != nil {
			failed = append(failed, fmt.Sprintf("%s (mkdir: %v)", e.rawPath, err))
			continue
		}
		if err := os.WriteFile(e.safePath, []byte(e.content), 0644); err != nil {
			failed = append(failed, fmt.Sprintf("%s (write: %v)", e.rawPath, err))
			continue
		}
		succeeded = append(succeeded, fmt.Sprintf("  wrote %d bytes → %s", len(e.content), e.rawPath))
		if OnFileWritten != nil {
			agentID := AgentIDFromCtx(ctx)
			OnFileWritten(ctx, agentID, e.rawPath, e.safePath, int64(len(e.content)))
		}
	}

	var sb strings.Builder
	if len(succeeded) > 0 {
		sb.WriteString(fmt.Sprintf("%d file(s) written:\n", len(succeeded)))
		for _, s := range succeeded {
			sb.WriteString(s + "\n")
		}
	}
	if len(failed) > 0 {
		sb.WriteString(fmt.Sprintf("\n%d file(s) failed:\n", len(failed)))
		for _, f := range failed {
			sb.WriteString("  FAILED: " + f + "\n")
		}
	}

	result := strings.TrimSpace(sb.String())
	if len(failed) > 0 {
		return &Result{ForLLM: result, ForUser: result, IsError: len(succeeded) == 0}
	}
	return TextResult(result)
}
