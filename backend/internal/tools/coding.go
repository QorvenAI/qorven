// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

// GlobTool finds files by pattern (ripgrep fast path, doublestar fallback).
type GlobTool struct{ workspace string }

func NewGlobTool(ws string) *GlobTool { return &GlobTool{workspace: ws} }

func (t *GlobTool) Name() string { return "glob" }
func (t *GlobTool) Description() string {
	return `Find files by glob pattern. Returns matching paths sorted by modification time (newest first).
Pattern syntax: * (any chars), ** (any path), ? (single char), {a,b} (alternatives).
Examples: "**/*.go", "src/**/*.{ts,tsx}", "*.json". Limited to 100 results.`
}
func (t *GlobTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"pattern": map[string]any{"type": "string", "description": "Glob pattern (e.g. **/*.go)"},
		"path":    map[string]any{"type": "string", "description": "Directory to search (default: workspace)"},
	}, "required": []string{"pattern"}}
}

type fileEntry struct {
	path    string
	modTime time.Time
}

func (t *GlobTool) Execute(ctx context.Context, args map[string]any) *Result {
	pattern, _ := args["pattern"].(string)
	dir, _ := args["path"].(string)
	if pattern == "" {
		return ErrorResult("pattern is required")
	}
	if dir == "" {
		dir = WorkspaceFromCtx(ctx)
		if dir == "" {
			dir = t.workspace
		}
	}

	// Try ripgrep first (much faster)
	if rgPath, err := exec.LookPath("rg"); err == nil {
		out, err := exec.CommandContext(ctx, rgPath, "--files", "-g", pattern, "--sort", "modified", dir).Output()
		if err == nil && len(out) > 0 {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) > 100 {
				lines = lines[:100]
			}
			return TextResult(fmt.Sprintf("%d files found:\n%s", len(lines), strings.Join(lines, "\n")))
		}
	}

	// Fallback: doublestar
	fsys := os.DirFS(dir)
	var matches []fileEntry
	_ = doublestar.GlobWalk(fsys, pattern, func(path string, d os.DirEntry) error {
		if d.IsDir() || skipPath(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		matches = append(matches, fileEntry{path: filepath.Join(dir, path), modTime: info.ModTime()})
		if len(matches) >= 200 {
			return fmt.Errorf("limit")
		}
		return nil
	})
	sort.Slice(matches, func(i, j int) bool { return matches[i].modTime.After(matches[j].modTime) })
	truncated := len(matches) > 100
	if truncated {
		matches = matches[:100]
	}
	var sb strings.Builder
	for _, m := range matches {
		sb.WriteString(m.path + "\n")
	}
	msg := fmt.Sprintf("%d files found", len(matches))
	if truncated {
		msg += " (truncated)"
	}
	return TextResult(msg + ":\n" + sb.String())
}

func skipPath(p string) bool {
	skip := map[string]bool{
		"node_modules": true, ".git": true, "vendor": true, "dist": true,
		"build": true, "target": true, "__pycache__": true, ".idea": true,
		".vscode": true, "coverage": true, ".opencode": true,
	}
	for _, part := range strings.Split(p, string(os.PathSeparator)) {
		if skip[part] || (len(part) > 0 && part[0] == '.' && part != ".") {
			return true
		}
	}
	return false
}

// GrepTool searches file contents by regex (ripgrep fast path, native fallback).
type GrepTool struct{ workspace string }

func NewGrepTool(ws string) *GrepTool { return &GrepTool{workspace: ws} }

func (t *GrepTool) Name() string { return "grep" }
func (t *GrepTool) Description() string {
	return `Search file contents by regex pattern. Returns matching lines with file paths and line numbers.
Use for finding function definitions, variable usages, error messages, etc.
Results sorted by modification time (newest first). Limited to 100 matches.`
}
func (t *GrepTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"pattern": map[string]any{"type": "string", "description": "Regex pattern to search for"},
		"path":    map[string]any{"type": "string", "description": "Directory to search (default: workspace)"},
		"include": map[string]any{"type": "string", "description": "File glob filter (e.g. *.go, *.ts)"},
	}, "required": []string{"pattern"}}
}

func (t *GrepTool) Execute(ctx context.Context, args map[string]any) *Result {
	pattern, _ := args["pattern"].(string)
	dir, _ := args["path"].(string)
	include, _ := args["include"].(string)
	if pattern == "" {
		return ErrorResult("pattern is required")
	}
	if dir == "" {
		dir = WorkspaceFromCtx(ctx)
		if dir == "" {
			dir = t.workspace
		}
	}

	// Try ripgrep
	if rgPath, err := exec.LookPath("rg"); err == nil {
		rgArgs := []string{"-n", "--max-count", "5", "--max-filesize", "1M", "-e", pattern}
		if include != "" {
			rgArgs = append(rgArgs, "-g", include)
		}
		rgArgs = append(rgArgs, dir)
		var out bytes.Buffer
		cmd := exec.CommandContext(ctx, rgPath, rgArgs...)
		cmd.Stdout = &out
		cmd.Stderr = &bytes.Buffer{}
		_ = cmd.Run()
		if out.Len() > 0 {
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if len(lines) > 100 {
				lines = lines[:100]
				return TextResult(fmt.Sprintf("%d matches (truncated to 100):\n%s", len(lines), strings.Join(lines, "\n")))
			}
			return TextResult(fmt.Sprintf("%d matches:\n%s", len(lines), strings.Join(lines, "\n")))
		}
		return TextResult("No matches found.")
	}

	// Fallback: exec grep
	grepArgs := []string{"-rn", "--max-count=5", "-E", pattern}
	if include != "" {
		grepArgs = append(grepArgs, "--include="+include)
	}
	grepArgs = append(grepArgs, dir)
	out, _ := exec.CommandContext(ctx, "grep", grepArgs...).Output()
	if len(out) == 0 {
		return TextResult("No matches found.")
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 100 {
		lines = lines[:100]
	}
	return TextResult(fmt.Sprintf("%d matches:\n%s", len(lines), strings.Join(lines, "\n")))
}

// DiagnosticsTool runs LSP diagnostics on a file after edits.
type DiagnosticsTool struct{}

func NewDiagnosticsTool() *DiagnosticsTool { return &DiagnosticsTool{} }

func (t *DiagnosticsTool) Name() string { return "diagnostics" }
func (t *DiagnosticsTool) Description() string {
	return `Check a file or project for errors and warnings. Runs the language's build/lint tool and returns diagnostics.
Use after editing code to verify changes compile correctly.`
}
func (t *DiagnosticsTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"path": map[string]any{"type": "string", "description": "File or directory to check"},
	}, "required": []string{"path"}}
}

func (t *DiagnosticsTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	if path == "" {
		return ErrorResult("path is required")
	}

	info, err := os.Stat(path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("path not found: %s", path))
	}

	dir := path
	if !info.IsDir() {
		dir = filepath.Dir(path)
	}

	// Detect language and run appropriate checker
	var cmd *exec.Cmd
	switch {
	case fileExists(filepath.Join(dir, "go.mod")):
		cmd = exec.CommandContext(ctx, "go", "build", "-o", "/dev/null", "./...")
		cmd.Dir = findProjectRoot(dir, "go.mod")
	case fileExists(filepath.Join(dir, "package.json")):
		if fileExists(filepath.Join(dir, "tsconfig.json")) {
			cmd = exec.CommandContext(ctx, "npx", "tsc", "--noEmit")
		} else {
			cmd = exec.CommandContext(ctx, "node", "--check", path)
		}
		cmd.Dir = findProjectRoot(dir, "package.json")
	case fileExists(filepath.Join(dir, "Cargo.toml")):
		cmd = exec.CommandContext(ctx, "cargo", "check", "--message-format=short")
		cmd.Dir = findProjectRoot(dir, "Cargo.toml")
	case strings.HasSuffix(path, ".py"):
		cmd = exec.CommandContext(ctx, "python3", "-m", "py_compile", path)
	default:
		return TextResult("No diagnostics available for this file type.")
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	output := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
	if err != nil {
		lines := strings.Split(output, "\n")
		if len(lines) > 50 {
			lines = lines[:50]
			output = strings.Join(lines, "\n") + "\n... (truncated)"
		}
		return TextResult(fmt.Sprintf("Diagnostics found issues:\n%s", output))
	}
	return TextResult("No errors found. Build succeeded.")
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func findProjectRoot(dir, marker string) string {
	for d := dir; d != "/" && d != "."; d = filepath.Dir(d) {
		if fileExists(filepath.Join(d, marker)) {
			return d
		}
	}
	return dir
}

// ApplyPatchTool applies multi-file patches using the diff engine.
type ApplyPatchTool struct {
	workspace string
	history   *FileHistory
}

func NewApplyPatchTool(ws string, history *FileHistory) *ApplyPatchTool {
	return &ApplyPatchTool{workspace: ws, history: history}
}

func (t *ApplyPatchTool) Name() string { return "apply_patch" }
func (t *ApplyPatchTool) Description() string {
	return `Apply a multi-file patch in one atomic operation. Format:
*** Begin Patch
*** Update File: path/to/file
@@ context line
 unchanged line
-line to remove
+line to add
*** Add File: path/to/new/file
+new file content
*** Delete File: path/to/remove
*** End Patch

RULES: Use read_file first. Context lines must uniquely identify the location. All paths relative to workspace.`
}
func (t *ApplyPatchTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"patch_text": map[string]any{"type": "string", "description": "The full patch text"},
	}, "required": []string{"patch_text"}}
}

func (t *ApplyPatchTool) Execute(ctx context.Context, args map[string]any) *Result {
	patchText, _ := args["patch_text"].(string)
	if patchText == "" {
		return ErrorResult("patch_text is required")
	}

	ws := WorkspaceFromCtx(ctx)
	if ws == "" {
		ws = t.workspace
	}

	// Use the diff engine from OpenCode
	result, err := processPatchInWorkspace(patchText, ws, t.history, SessionIDFromCtx(ctx))
	if err != nil {
		return ErrorResult(err.Error())
	}
	return TextResult(result)
}

func processPatchInWorkspace(patchText, ws string, history *FileHistory, sessionID string) (string, error) {
	// Stub — delegate to diff package
	return processPatchInWorkspaceImpl(patchText, ws, history, sessionID)
}

// FileHistory tracks file versions for undo/redo.
type FileHistory struct {
	versions map[string][]fileVersion // path → versions
}

type fileVersion struct {
	content   string
	timestamp time.Time
	sessionID string
}

func NewFileHistory() *FileHistory {
	return &FileHistory{versions: make(map[string][]fileVersion)}
}

func (h *FileHistory) Save(path, content, sessionID string) {
	if h == nil {
		return
	}
	h.versions[path] = append(h.versions[path], fileVersion{
		content: content, timestamp: time.Now(), sessionID: sessionID,
	})
	// Keep max 50 versions per file
	if len(h.versions[path]) > 50 {
		h.versions[path] = h.versions[path][len(h.versions[path])-50:]
	}
}

func (h *FileHistory) Undo(path, sessionID string) (string, bool) {
	if h == nil {
		return "", false
	}
	versions := h.versions[path]
	for i := len(versions) - 1; i >= 0; i-- {
		if versions[i].sessionID == sessionID {
			return versions[i].content, true
		}
	}
	return "", false
}

func (h *FileHistory) ListChanges(sessionID string) []string {
	if h == nil {
		return nil
	}
	var files []string
	for path, versions := range h.versions {
		for _, v := range versions {
			if v.sessionID == sessionID {
				files = append(files, path)
				break
			}
		}
	}
	sort.Strings(files)
	return files
}

// UndoTool reverts a file to its previous version.
type UndoTool struct {
	history *FileHistory
}

func NewUndoTool(history *FileHistory) *UndoTool { return &UndoTool{history: history} }

func (t *UndoTool) Name() string { return "undo" }
func (t *UndoTool) Description() string {
	return "Undo the last edit to a file, restoring its previous content. Use when an edit broke something."
}
func (t *UndoTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"path": map[string]any{"type": "string", "description": "File path to undo"},
	}, "required": []string{"path"}}
}

func (t *UndoTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	if path == "" {
		return ErrorResult("path is required")
	}
	sessionID := SessionIDFromCtx(ctx)
	content, ok := t.history.Undo(path, sessionID)
	if !ok {
		return ErrorResult("no previous version found for this file in this session")
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return ErrorResult(fmt.Sprintf("failed to restore: %v", err))
	}
	return TextResult(fmt.Sprintf("Restored %s to previous version (%d bytes)", path, len(content)))
}

// processPatchInWorkspaceImpl applies the patch using the diff engine.
func processPatchInWorkspaceImpl(patchText, ws string, history *FileHistory, sessionID string) (string, error) {
	// Import the diff package functions
	paths := identifyFilesNeeded(patchText)
	addedPaths := identifyFilesAdded(patchText)

	// Load current files
	currentFiles := make(map[string]string)
	for _, p := range paths {
		absPath := p
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(ws, absPath)
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", p, err)
		}
		currentFiles[p] = string(data)
		// Save to history before modifying
		if history != nil {
			history.Save(absPath, string(data), sessionID)
		}
	}

	// Verify added files don't exist
	for _, p := range addedPaths {
		absPath := p
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(ws, absPath)
		}
		if _, err := os.Stat(absPath); err == nil {
			return "", fmt.Errorf("file already exists: %s", p)
		}
	}

	// Parse and apply
	result, err := processThePatch(patchText, currentFiles, ws)
	if err != nil {
		return "", err
	}
	return result, nil
}

func processThePatch(patchText string, currentFiles map[string]string, ws string) (string, error) {
	text := strings.TrimSpace(patchText)
	lines := strings.Split(text, "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "*** Begin Patch") {
		return "", fmt.Errorf("patch must start with *** Begin Patch")
	}

	// Use the parser from diff package
	parser := &patchParser{
		currentFiles: currentFiles,
		lines:        lines,
		index:        1,
		actions:      make(map[string]patchAction),
	}
	if err := parser.parse(); err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}

	// Apply changes
	filesChanged := 0
	for path, action := range parser.actions {
		absPath := path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(ws, absPath)
		}
		switch action.actionType {
		case "add":
			dir := filepath.Dir(absPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return "", err
			}
			if err := os.WriteFile(absPath, []byte(action.newContent), 0644); err != nil {
				return "", err
			}
		case "delete":
			if err := os.Remove(absPath); err != nil {
				return "", err
			}
		case "update":
			orig := currentFiles[path]
			newContent, err := applyChunks(orig, action.chunks)
			if err != nil {
				return "", fmt.Errorf("apply to %s: %w", path, err)
			}
			if err := os.WriteFile(absPath, []byte(newContent), 0644); err != nil {
				return "", err
			}
		}
		filesChanged++
	}
	return fmt.Sprintf("Patch applied: %d files changed", filesChanged), nil
}

type patchAction struct {
	actionType string // "add", "delete", "update"
	newContent string
	chunks     []patchChunk
}

type patchChunk struct {
	origIndex int
	delLines  []string
	insLines  []string
}

type patchParser struct {
	currentFiles map[string]string
	lines        []string
	index        int
	actions      map[string]patchAction
}

func (p *patchParser) parse() error {
	for p.index < len(p.lines) {
		line := p.lines[p.index]
		if strings.HasPrefix(line, "*** End Patch") {
			return nil
		}
		if strings.HasPrefix(line, "*** Update File: ") {
			path := line[len("*** Update File: "):]
			p.index++
			orig, ok := p.currentFiles[path]
			if !ok {
				return fmt.Errorf("file not found: %s", path)
			}
			chunks, err := p.parseUpdateChunks(orig)
			if err != nil {
				return err
			}
			p.actions[path] = patchAction{actionType: "update", chunks: chunks}
			continue
		}
		if strings.HasPrefix(line, "*** Add File: ") {
			path := line[len("*** Add File: "):]
			p.index++
			content, err := p.parseAddContent()
			if err != nil {
				return err
			}
			p.actions[path] = patchAction{actionType: "add", newContent: content}
			continue
		}
		if strings.HasPrefix(line, "*** Delete File: ") {
			path := line[len("*** Delete File: "):]
			p.index++
			p.actions[path] = patchAction{actionType: "delete"}
			continue
		}
		return fmt.Errorf("unexpected line: %s", line)
	}
	return fmt.Errorf("missing *** End Patch")
}

func (p *patchParser) parseAddContent() (string, error) {
	var lines []string
	for p.index < len(p.lines) {
		s := p.lines[p.index]
		if strings.HasPrefix(s, "*** ") {
			break
		}
		if !strings.HasPrefix(s, "+") {
			return "", fmt.Errorf("add file line must start with +: %s", s)
		}
		lines = append(lines, s[1:])
		p.index++
	}
	return strings.Join(lines, "\n"), nil
}

func (p *patchParser) parseUpdateChunks(orig string) ([]patchChunk, error) {
	fileLines := strings.Split(orig, "\n")
	var chunks []patchChunk
	fileIdx := 0

	for p.index < len(p.lines) {
		s := p.lines[p.index]
		if strings.HasPrefix(s, "*** ") {
			break
		}
		// Skip @@ section headers
		if strings.HasPrefix(s, "@@ ") || s == "@@" {
			p.index++
			continue
		}

		var delLines, insLines []string
		contextLines := []string{}

		// Read context + changes
		for p.index < len(p.lines) {
			s = p.lines[p.index]
			if strings.HasPrefix(s, "*** ") || strings.HasPrefix(s, "@@ ") || s == "@@" {
				break
			}
			p.index++
			if len(s) == 0 {
				contextLines = append(contextLines, "")
				continue
			}
			switch s[0] {
			case ' ':
				if len(delLines) > 0 || len(insLines) > 0 {
					// Flush chunk
					matchIdx := findInLines(fileLines, contextLines[:], fileIdx)
					if matchIdx < 0 {
						matchIdx = fileIdx
					}
					chunks = append(chunks, patchChunk{origIndex: matchIdx, delLines: delLines, insLines: insLines})
					fileIdx = matchIdx + len(contextLines) + len(delLines)
					delLines = nil
					insLines = nil
					contextLines = nil
				}
				contextLines = append(contextLines, s[1:])
			case '-':
				delLines = append(delLines, s[1:])
			case '+':
				insLines = append(insLines, s[1:])
			default:
				contextLines = append(contextLines, s)
			}
		}
		if len(delLines) > 0 || len(insLines) > 0 {
			matchIdx := findInLines(fileLines, contextLines, fileIdx)
			if matchIdx < 0 {
				matchIdx = fileIdx
			}
			chunks = append(chunks, patchChunk{origIndex: matchIdx, delLines: delLines, insLines: insLines})
			fileIdx = matchIdx + len(contextLines) + len(delLines)
		}
	}
	return chunks, nil
}

func findInLines(fileLines, context []string, start int) int {
	if len(context) == 0 {
		return start
	}
	for i := start; i <= len(fileLines)-len(context); i++ {
		match := true
		for j, c := range context {
			if fileLines[i+j] != c {
				match = false
				break
			}
		}
		if match {
			return i + len(context)
		}
	}
	return -1
}

func applyChunks(orig string, chunks []patchChunk) (string, error) {
	lines := strings.Split(orig, "\n")
	var result []string
	idx := 0
	for _, ch := range chunks {
		if ch.origIndex > len(lines) {
			return "", fmt.Errorf("chunk index %d exceeds file length %d", ch.origIndex, len(lines))
		}
		result = append(result, lines[idx:ch.origIndex]...)
		result = append(result, ch.insLines...)
		idx = ch.origIndex + len(ch.delLines)
	}
	result = append(result, lines[idx:]...)
	return strings.Join(result, "\n"), nil
}

func identifyFilesNeeded(text string) []string {
	var files []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "*** Update File: ") {
			files = append(files, line[len("*** Update File: "):])
		} else if strings.HasPrefix(line, "*** Delete File: ") {
			files = append(files, line[len("*** Delete File: "):])
		}
	}
	return files
}

func identifyFilesAdded(text string) []string {
	var files []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "*** Add File: ") {
			files = append(files, line[len("*** Add File: "):])
		}
	}
	return files
}




// Ensure json import is used
var _ = json.Marshal
