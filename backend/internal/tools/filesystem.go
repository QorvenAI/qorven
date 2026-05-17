// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/bootstrap"
	"github.com/qorvenai/qorven/internal/sandbox"
)

// virtualSystemFiles are files dynamically injected into the system prompt.
// They don't exist on disk — if the model tries to read them, return a hint.
var virtualSystemFiles = map[string]string{
	bootstrap.TeamFile:         "TEAM.md is already loaded in your system prompt. Refer to the TEAM.md section in your context above.",
	bootstrap.AvailabilityFile: "AVAILABILITY.md is already loaded in your system prompt. Refer to the AVAILABILITY.md section in your context above.",
}

// binaryFileExts are file extensions that should not be read as text.
var binaryFileExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".bmp": true, ".ico": true, ".tiff": true, ".tif": true,
	".mp3": true, ".wav": true, ".ogg": true, ".flac": true, ".aac": true, ".m4a": true,
	".mp4": true, ".avi": true, ".mov": true, ".mkv": true, ".webm": true,
	".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".7z": true, ".rar": true,
	".pdf": true, ".docx": true, ".xlsx": true, ".pptx": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true,
}

// --- read_file ---

type ReadFileTool struct {
	workspace       string
	restrict        bool
	allowedPrefixes []string
	deniedPrefixes  []string
	sandboxMgr      sandbox.Manager
}

func NewReadFileTool(ws string) *ReadFileTool { return &ReadFileTool{workspace: ws, restrict: true} }

func NewSandboxedReadFileTool(ws string, restrict bool, mgr sandbox.Manager) *ReadFileTool {
	return &ReadFileTool{workspace: ws, restrict: restrict, sandboxMgr: mgr}
}

func (t *ReadFileTool) AllowPaths(prefixes ...string) { t.allowedPrefixes = append(t.allowedPrefixes, prefixes...) }
func (t *ReadFileTool) DenyPaths(prefixes ...string)  { t.deniedPrefixes = append(t.deniedPrefixes, prefixes...) }
func (t *ReadFileTool) Name() string                  { return "read_file" }
func (t *ReadFileTool) Description() string           { return "Read the contents of a file. Supports optional line range." }
func (t *ReadFileTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"path":       map[string]any{"type": "string", "description": "File path (relative to workspace or absolute)"},
		"start_line": map[string]any{"type": "integer", "description": "Start line (1-based, optional)"},
		"end_line":   map[string]any{"type": "integer", "description": "End line (inclusive, optional)"},
	}, "required": []string{"path"}}
}

func (t *ReadFileTool) Execute(ctx context.Context, args map[string]any) *Result {
	ws := WorkspaceFromCtx(ctx)
	if ws == "" { ws = t.workspace }
	rawPath, _ := args["path"].(string)
	if rawPath == "" { return ErrorResult("path is required") }

	// Virtual system files hint
	if hint, ok := virtualSystemFiles[filepath.Base(rawPath)]; ok {
		return SilentResult(hint)
	}

	// Block binary files
	if isBinaryFileExt(rawPath) {
		ext := strings.ToLower(filepath.Ext(rawPath))
		return ErrorResult(fmt.Sprintf("cannot read binary file (%s). Use appropriate tool: read_image, read_document, etc.", ext))
	}

	// Sandbox routing
	sandboxKey := SandboxKeyFromCtx(ctx)
	if t.sandboxMgr != nil && sandboxKey != "" {
		return t.executeInSandbox(ctx, rawPath, sandboxKey, args)
	}

	safe, err := t.resolvePath(rawPath, ws)
	if err != nil { return ErrorResult(err.Error()) }

	data, err := os.ReadFile(safe)
	if err != nil { return ErrorResult(fmt.Sprintf("cannot read file: %v", err)) }

	// Detect binary content by checking for null bytes in first 8KB
	checkLen := len(data)
	if checkLen > 8192 { checkLen = 8192 }
	for _, b := range data[:checkLen] {
		if b == 0 {
			return ErrorResult(fmt.Sprintf("binary file detected (%d bytes). Use appropriate tool for binary files.", len(data)))
		}
	}

	RecordFileRead(safe)
	return t.paginateOutput(string(data), args)
}

func (t *ReadFileTool) executeInSandbox(ctx context.Context, path, sandboxKey string, args map[string]any) *Result {
	sb, err := t.sandboxMgr.Get(ctx, sandboxKey, t.workspace, nil)
	if err != nil { return ErrorResult(fmt.Sprintf("sandbox error: %v", err)) }
	bridge := sandbox.NewFsBridge(sb.ID(), sandbox.DefaultContainerWorkdir)
	data, err := bridge.ReadFile(ctx, path)
	if err != nil { return ErrorResult(fmt.Sprintf("failed to read file: %v", err)) }
	return t.paginateOutput(data, args)
}

func (t *ReadFileTool) resolvePath(path, workspace string) (string, error) {
	resolved, err := SafePathWithRestrict(workspace, path, t.restrict, t.allowedPrefixes)
	if err != nil { return "", err }
	if err := checkDeniedPath(resolved, workspace, t.deniedPrefixes); err != nil { return "", err }
	return resolved, nil
}

const readFileMaxChars = 50000

func (t *ReadFileTool) paginateOutput(content string, args map[string]any) *Result {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	startLine, _ := toInt(args["start_line"])
	endLine, _ := toInt(args["end_line"])

	// Line range support
	if startLine > 0 || endLine > 0 {
		if startLine < 1 { startLine = 1 }
		if endLine < 1 || endLine > totalLines { endLine = totalLines }
		if startLine > totalLines { return ErrorResult("start_line beyond file length") }
		content = strings.Join(lines[startLine-1:endLine], "\n")
		return SilentResult(content)
	}

	// Auto-truncate large files
	if len(content) > readFileMaxChars {
		charCount := 0
		truncIdx := len(lines)
		for i, line := range lines {
			charCount += len(line) + 1
			if charCount > readFileMaxChars { truncIdx = i; break }
		}
		content = strings.Join(lines[:truncIdx], "\n")
		content += fmt.Sprintf("\n\n[Output capped. File has %d lines, showed %d. Use start_line/end_line to continue.]", totalLines, truncIdx)
	}
	return SilentResult(content)
}

// --- write_file ---

type WriteFileTool struct {
	workspace       string
	restrict        bool
	sandboxMgr      sandbox.Manager
	allowedPrefixes []string
}

func NewWriteFileTool(ws string) *WriteFileTool { return &WriteFileTool{workspace: ws, restrict: true} }

func (t *WriteFileTool) AllowPaths(prefixes ...string) { t.allowedPrefixes = append(t.allowedPrefixes, prefixes...) }

func NewSandboxedWriteFileTool(ws string, restrict bool, mgr sandbox.Manager) *WriteFileTool {
	return &WriteFileTool{workspace: ws, restrict: restrict, sandboxMgr: mgr}
}

func (t *WriteFileTool) Name() string        { return "write_file" }
func (t *WriteFileTool) Description() string { return "Create or overwrite a file. Creates parent directories automatically." }
func (t *WriteFileTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"path":    map[string]any{"type": "string", "description": "File path"},
		"content": map[string]any{"type": "string", "description": "File content"},
	}, "required": []string{"path", "content"}}
}

func (t *WriteFileTool) Execute(ctx context.Context, args map[string]any) *Result {
	ws := WorkspaceFromCtx(ctx)
	if ws == "" { ws = t.workspace }
	rawPath, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if rawPath == "" { return ErrorResult("path is required") }

	// Quota check
	if err := CheckQuota(ws, int64(len(content))); err != nil {
		return ErrorResult(err.Error())
	}

	// Sandbox routing
	sandboxKey := SandboxKeyFromCtx(ctx)
	if t.sandboxMgr != nil && sandboxKey != "" {
		return t.executeInSandbox(ctx, rawPath, content, sandboxKey)
	}

	safe, err := SafePathWithRestrict(ws, rawPath, t.restrict, t.allowedPrefixes)
	if err != nil { return ErrorResult(err.Error()) }

	// Stale file check
	if staleWarn := CheckStaleFile(safe); staleWarn != "" {
		return ErrorResult(staleWarn)
	}

	// Read old content for diff
	oldContent, _ := os.ReadFile(safe)

	if err := os.MkdirAll(filepath.Dir(safe), 0755); err != nil {
		return ErrorResult(fmt.Sprintf("cannot create directory: %v", err))
	}
	if err := os.WriteFile(safe, []byte(content), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("cannot write file: %v", err))
	}

	msg := fmt.Sprintf("wrote %d bytes to %s", len(content), rawPath)

	// Show diff
	if diff := generateDiff(rawPath, string(oldContent), content); diff != "" {
		msg += "\n\n" + diff
	}

	// Auto-verify
	if verr := PostEditVerify(ctx, safe, ws); verr != "" {
		msg += "\n\n" + verr
	}
	// Register in drive for GUI visibility
	if OnFileWritten != nil {
		agentID := AgentIDFromCtx(ctx)
		OnFileWritten(ctx, agentID, rawPath, safe, int64(len(content)))
	}
	// Attach document files as media (for email attachments)
	ext := strings.ToLower(filepath.Ext(safe))
	if ext == ".pdf" || ext == ".docx" || ext == ".xlsx" || ext == ".csv" || ext == ".md" {
		return &Result{ForLLM: msg, ForUser: msg, Media: []MediaFile{{Path: safe, MimeType: MimeFromExt(ext)}}}
	}
	return TextResult(msg)
}

// OnFileWritten is called after write_file creates a file. Set by gateway to sync to drive.
var OnFileWritten func(ctx context.Context, agentID, name, path string, size int64)

func (t *WriteFileTool) executeInSandbox(ctx context.Context, path, content, sandboxKey string) *Result {
	sb, err := t.sandboxMgr.Get(ctx, sandboxKey, t.workspace, nil)
	if err != nil { return ErrorResult(fmt.Sprintf("sandbox error: %v", err)) }
	bridge := sandbox.NewFsBridge(sb.ID(), sandbox.DefaultContainerWorkdir)
	if err := bridge.WriteFile(ctx, path, content); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write file: %v", err))
	}
	return TextResult(fmt.Sprintf("wrote %d bytes to %s (sandbox)", len(content), path))
}

// --- list_files ---

type ListFilesTool struct {
	workspace       string
	restrict        bool
	sandboxMgr      sandbox.Manager
	allowedPrefixes []string
}

func NewListFilesTool(ws string) *ListFilesTool { return &ListFilesTool{workspace: ws, restrict: true} }

func (t *ListFilesTool) AllowPaths(prefixes ...string) { t.allowedPrefixes = append(t.allowedPrefixes, prefixes...) }

func NewSandboxedListFilesTool(ws string, restrict bool, mgr sandbox.Manager) *ListFilesTool {
	return &ListFilesTool{workspace: ws, restrict: restrict, sandboxMgr: mgr}
}

func (t *ListFilesTool) Name() string        { return "list_files" }
func (t *ListFilesTool) Description() string { return "List files and directories in a path." }
func (t *ListFilesTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"path": map[string]any{"type": "string", "description": "Directory path (default: workspace root)"},
	}}
}

func (t *ListFilesTool) Execute(ctx context.Context, args map[string]any) *Result {
	ws := WorkspaceFromCtx(ctx)
	if ws == "" { ws = t.workspace }
	rawPath, _ := args["path"].(string)
	if rawPath == "" { rawPath = "." }

	// Sandbox routing
	sandboxKey := SandboxKeyFromCtx(ctx)
	if t.sandboxMgr != nil && sandboxKey != "" {
		return t.executeInSandbox(ctx, rawPath, sandboxKey)
	}

	safe, err := SafePathWithRestrict(ws, rawPath, t.restrict, t.allowedPrefixes)
	if err != nil { return ErrorResult(err.Error()) }

	entries, err := os.ReadDir(safe)
	if err != nil { return ErrorResult(fmt.Sprintf("cannot list: %v", err)) }

	var b strings.Builder
	for _, e := range entries {
		info, _ := e.Info()
		suffix := ""
		if e.IsDir() { suffix = "/" }
		size := ""
		if info != nil && !e.IsDir() { size = fmt.Sprintf(" (%d bytes)", info.Size()) }
		fmt.Fprintf(&b, "%s%s%s\n", e.Name(), suffix, size)
	}
	if b.Len() == 0 { return TextResult("(empty directory)") }
	return TextResult(b.String())
}

func (t *ListFilesTool) executeInSandbox(ctx context.Context, path, sandboxKey string) *Result {
	sb, err := t.sandboxMgr.Get(ctx, sandboxKey, t.workspace, nil)
	if err != nil { return ErrorResult(fmt.Sprintf("sandbox error: %v", err)) }
	bridge := sandbox.NewFsBridge(sb.ID(), sandbox.DefaultContainerWorkdir)
	output, err := bridge.ListDir(ctx, path)
	if err != nil { return ErrorResult(fmt.Sprintf("failed to list: %v", err)) }
	return TextResult(output)
}

// --- edit (search and replace) ---

type EditTool struct {
	workspace       string
	restrict        bool
	sandboxMgr      sandbox.Manager
	allowedPrefixes []string
}

func NewEditTool(ws string) *EditTool { return &EditTool{workspace: ws, restrict: true} }

func (t *EditTool) AllowPaths(prefixes ...string) { t.allowedPrefixes = append(t.allowedPrefixes, prefixes...) }

func NewSandboxedEditTool(ws string, restrict bool, mgr sandbox.Manager) *EditTool {
	return &EditTool{workspace: ws, restrict: restrict, sandboxMgr: mgr}
}

func (t *EditTool) Name() string { return "edit" }
func (t *EditTool) Description() string {
	return "Apply targeted edits to a file by replacing exact text matches. More efficient than rewriting entire files."
}
func (t *EditTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"path":    map[string]any{"type": "string", "description": "File path"},
		"find":    map[string]any{"type": "string", "description": "Exact text to find"},
		"replace": map[string]any{"type": "string", "description": "Replacement text"},
	}, "required": []string{"path", "find", "replace"}}
}

func (t *EditTool) Execute(ctx context.Context, args map[string]any) *Result {
	ws := WorkspaceFromCtx(ctx)
	if ws == "" { ws = t.workspace }
	rawPath, _ := args["path"].(string)
	find, _ := args["find"].(string)
	replace, _ := args["replace"].(string)
	if rawPath == "" || find == "" { return ErrorResult("path and find are required") }

	// Sandbox routing
	sandboxKey := SandboxKeyFromCtx(ctx)
	if t.sandboxMgr != nil && sandboxKey != "" {
		return t.executeInSandbox(ctx, rawPath, find, replace, sandboxKey)
	}

	safe, err := SafePathWithRestrict(ws, rawPath, t.restrict, t.allowedPrefixes)
	if err != nil { return ErrorResult(err.Error()) }

	// Stale file check
	if staleWarn := CheckStaleFile(safe); staleWarn != "" {
		return ErrorResult(staleWarn)
	}

	data, err := os.ReadFile(safe)
	if err != nil { return ErrorResult(fmt.Sprintf("cannot read: %v", err)) }

	content := string(data)
	count := strings.Count(content, find)
	if count == 0 { return ErrorResult("find text not found in file") }
	if count > 1 { return ErrorResult(fmt.Sprintf("find text matches %d locations — be more specific", count)) }

	newContent := strings.Replace(content, find, replace, 1)
	if err := os.WriteFile(safe, []byte(newContent), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("cannot write: %v", err))
	}

	msg := fmt.Sprintf("edited %s — replaced 1 occurrence", rawPath)
	if diff := generateDiff(rawPath, content, newContent); diff != "" {
		msg += "\n\n" + diff
	}
	if verr := PostEditVerify(ctx, safe, ws); verr != "" {
		msg += "\n\n" + verr
	}
	return TextResult(msg)
}

func (t *EditTool) executeInSandbox(ctx context.Context, path, find, replace, sandboxKey string) *Result {
	sb, err := t.sandboxMgr.Get(ctx, sandboxKey, t.workspace, nil)
	if err != nil { return ErrorResult(fmt.Sprintf("sandbox error: %v", err)) }
	bridge := sandbox.NewFsBridge(sb.ID(), sandbox.DefaultContainerWorkdir)

	content, err := bridge.ReadFile(ctx, path)
	if err != nil { return ErrorResult(fmt.Sprintf("cannot read: %v", err)) }

	count := strings.Count(content, find)
	if count == 0 { return ErrorResult("find text not found in file") }
	if count > 1 { return ErrorResult(fmt.Sprintf("find text matches %d locations — be more specific", count)) }

	newContent := strings.Replace(content, find, replace, 1)
	if err := bridge.WriteFile(ctx, path, newContent); err != nil {
		return ErrorResult(fmt.Sprintf("cannot write: %v", err))
	}
	return TextResult(fmt.Sprintf("edited %s — replaced 1 occurrence (sandbox)", path))
}

// --- helpers ---

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64: return int(n), true
	case int: return n, true
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	}
	return 0, false
}

// generateDiff creates a unified diff between old and new content.
func generateDiff(path, oldContent, newContent string) string {
	oldLines := splitFileLines(oldContent)
	newLines := splitFileLines(newContent)

	var diff []string
	diff = append(diff, fmt.Sprintf("--- a/%s", path))
	diff = append(diff, fmt.Sprintf("+++ b/%s", path))

	// Simple line-by-line diff (not full Myers, but good enough for display)
	maxLines := len(oldLines)
	if len(newLines) > maxLines { maxLines = len(newLines) }

	changes := 0
	for i := 0; i < maxLines && changes < 20; i++ {
		old := ""
		new := ""
		if i < len(oldLines) { old = oldLines[i] }
		if i < len(newLines) { new = newLines[i] }
		if old != new {
			if old != "" { diff = append(diff, fmt.Sprintf("-%s", old)) }
			if new != "" { diff = append(diff, fmt.Sprintf("+%s", new)) }
			changes++
		}
	}
	if changes == 0 { return "" }
	return strings.Join(diff, "\n")
}

func splitFileLines(s string) []string {
	if s == "" { return nil }
	return strings.Split(s, "\n")
}

// fileReadTimestamps tracks when files were last read by the agent.
// Used for stale file detection on write/patch.
var fileReadTimestamps = struct {
	sync.RWMutex
	m map[string]time.Time
}{m: make(map[string]time.Time)}

// RecordFileRead records when a file was read.
func RecordFileRead(path string) {
	fileReadTimestamps.Lock()
	fileReadTimestamps.m[path] = time.Now()
	fileReadTimestamps.Unlock()
}

// CheckStaleFile returns a warning if the file was modified since last read.
func CheckStaleFile(path string) string {
	fileReadTimestamps.RLock()
	readTime, ok := fileReadTimestamps.m[path]
	fileReadTimestamps.RUnlock()
	if !ok { return "" }

	info, err := os.Stat(path)
	if err != nil { return "" }
	if info.ModTime().After(readTime) {
		return fmt.Sprintf("⚠️ STALE: %s was modified externally since you last read it (%s ago). Re-read before writing.",
			filepath.Base(path), time.Since(readTime).Round(time.Second))
	}
	return ""
}

// isBinaryFileExt returns true if the file extension indicates a binary file.
func isBinaryFileExt(path string) bool {
	return binaryFileExts[strings.ToLower(filepath.Ext(path))]
}

// SafePathWithRestrict resolves a path with optional restriction and allowed prefixes.
func SafePathWithRestrict(workspace, path string, restrict bool, allowedPrefixes []string) (string, error) {
	// Expand tilde to actual home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("access denied: cannot resolve home directory")
		}
		path = home + path[1:]
	} else if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("access denied: cannot resolve home directory")
		}
		path = home
	}

	// Block null bytes
	if strings.ContainsRune(path, 0) {
		return "", fmt.Errorf("access denied: null bytes in path")
	}

	// URL-decode to prevent %2e%2e%2f traversal
	if decoded, err := url.PathUnescape(path); err == nil { path = decoded }
	// Double-decode for %252f attacks
	if decoded, err := url.PathUnescape(path); err == nil { path = decoded }

	// Normalize backslashes
	path = strings.ReplaceAll(path, "\\", "/")

	var resolved string
	if filepath.IsAbs(path) {
		resolved = filepath.Clean(path)
	} else {
		resolved = filepath.Clean(filepath.Join(workspace, path))
	}

	if !restrict {
		return resolved, nil
	}

	// Resolve workspace to canonical path
	absWorkspace, _ := filepath.Abs(workspace)
	wsReal, err := filepath.EvalSymlinks(absWorkspace)
	if err != nil { wsReal = absWorkspace }

	// Resolve target path
	absResolved, _ := filepath.Abs(resolved)
	real, err := filepath.EvalSymlinks(absResolved)
	if err != nil {
		if os.IsNotExist(err) {
			// For non-existent files, check parent
			real, err = resolveThroughExistingAncestors(absResolved)
			if err != nil { return "", fmt.Errorf("access denied: cannot resolve path") }
		} else {
			return "", fmt.Errorf("access denied: cannot resolve path")
		}
	}

	// Check if inside workspace
	if isPathInside(real, wsReal) {
		return real, nil
	}

	// Check allowed prefixes
	for _, prefix := range allowedPrefixes {
		absPrefix, _ := filepath.Abs(prefix)
		prefixReal, _ := filepath.EvalSymlinks(absPrefix)
		if prefixReal == "" { prefixReal = absPrefix }
		if isPathInside(real, prefixReal) {
			return real, nil
		}
	}

	slog.Warn("security.path_escape", "path", path, "resolved", real, "workspace", wsReal)
	return "", fmt.Errorf("access denied: path outside workspace")
}

// checkDeniedPath returns an error if the resolved path falls under any denied prefix.
func checkDeniedPath(resolved, workspace string, deniedPrefixes []string) error {
	if len(deniedPrefixes) == 0 { return nil }
	absResolved, _ := filepath.Abs(resolved)
	absWorkspace, _ := filepath.Abs(workspace)
	wsReal, _ := filepath.EvalSymlinks(absWorkspace)
	if wsReal == "" { wsReal = absWorkspace }
	for _, prefix := range deniedPrefixes {
		denied := filepath.Join(wsReal, prefix)
		if isPathInside(absResolved, denied) {
			return fmt.Errorf("access denied: path %s is restricted", prefix)
		}
	}
	return nil
}

// isPathInside checks whether child is inside or equal to parent directory.
func isPathInside(child, parent string) bool {
	if child == parent { return true }
	return strings.HasPrefix(child, parent+string(filepath.Separator))
}

// resolveThroughExistingAncestors resolves a path by finding the deepest existing ancestor.
func resolveThroughExistingAncestors(target string) (string, error) {
	if real, err := filepath.EvalSymlinks(target); err == nil {
		return real, nil
	}
	current := target
	var tail []string
	for {
		parent := filepath.Dir(current)
		if parent == current { break }
		tail = append([]string{filepath.Base(current)}, tail...)
		current = parent
		if realParent, err := filepath.EvalSymlinks(current); err == nil {
			result := realParent
			for _, component := range tail {
				result = filepath.Join(result, component)
			}
			return result, nil
		}
	}
	return filepath.Clean(target), nil
}
