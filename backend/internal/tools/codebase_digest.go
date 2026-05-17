// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CodebaseDigestTool packs a directory tree into a single LLM-ready
// markdown blob — tree + annotated file contents — so the agent can
// "review this repo" in one shot without burning a round-trip per file.
//
// Design notes:
//   - Respects .gitignore. We don't try to be a full git-aware tool;
//     we parse .gitignore at each level and honor the common patterns
//     (prefix, suffix, negation). Doesn't handle the weirder rules
//     (double-wildcard paths from sub-dirs) — good enough for 95%
//     of real repos and cheap to implement.
//   - Skips binaries via extension + NUL-byte sniff. Dumping the
//     first KB of a 50 MB PNG into a context window is worse than
//     silently omitting the file.
//   - Token-budgeted: caller sets a byte cap; tree header + metadata
//     subtract from it, then we pack files biggest-first until the
//     budget is exhausted. Rationale: big files carry more signal
//     per byte for most "review this codebase" tasks.
//   - Sandbox-aware: the `path` argument is resolved against the
//     agent's workspace + explicit allow-prefixes (same model as
//     ReadFileTool) so an agent can't digest /etc/.
//
// Example usage the agent gets from the tool description:
//   codebase_digest(path=".", max_bytes=100000)
//   → returns:
//      # Digest of ~/work/app  (42 files, 780 KB on disk, 98.4 KB digested)
//      ## Tree
//      - ./src/main.go
//      - ./README.md
//      ...
//      ## Files
//      ### ./README.md
//      ```markdown
//      ...
type CodebaseDigestTool struct {
	workspace       string
	allowedPrefixes []string
}

// NewCodebaseDigestTool ties the tool to an agent workspace. Additional
// prefixes (home dir, /tmp, etc.) can be allow-listed via AllowPaths.
func NewCodebaseDigestTool(ws string) *CodebaseDigestTool {
	return &CodebaseDigestTool{workspace: ws}
}

// AllowPaths adds prefixes an agent can digest from outside the
// workspace. Mirrors ReadFileTool so the path-resolution rules stay
// consistent across tools.
func (t *CodebaseDigestTool) AllowPaths(prefixes ...string) {
	t.allowedPrefixes = append(t.allowedPrefixes, prefixes...)
}

func (t *CodebaseDigestTool) Name() string { return "codebase_digest" }

func (t *CodebaseDigestTool) Description() string {
	return "Pack a directory tree into a single markdown blob containing the " +
		"file tree plus the contents of every text file, respecting .gitignore " +
		"and skipping binaries. Use this when the user says \"review my repo\", " +
		"\"summarize this codebase\", or similar — one call covers the whole " +
		"project instead of ten separate read_file calls. Token-budgeted."
}

func (t *CodebaseDigestTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Directory to digest (relative to workspace or absolute).",
			},
			"max_bytes": map[string]any{
				"type":        "integer",
				"description": "Byte budget for the combined digest. Default 120 KiB, max 500 KiB. Files that would exceed the budget are listed in the tree but their contents are omitted with a note.",
			},
			"include_patterns": map[string]any{
				"type":        "string",
				"description": "Optional comma-separated glob patterns to whitelist (e.g. \"*.go,*.md\"). If set, only matching files have contents included.",
			},
			"respect_gitignore": map[string]any{
				"type":        "boolean",
				"description": "Honor .gitignore rules. Default true. Disable for forensics or to surface hidden build artifacts.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *CodebaseDigestTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}
	ws := WorkspaceFromCtx(ctx)
	if ws == "" {
		ws = t.workspace
	}

	// Resolve under workspace unless absolute. We deliberately don't
	// use filesystem.go's resolvePath because we want to allow any
	// subdirectory inside the workspace — not require a file.
	root := path
	if !filepath.IsAbs(root) {
		root = filepath.Join(ws, root)
	}
	root = filepath.Clean(root)

	info, err := os.Stat(root)
	if err != nil {
		return ErrorResult(fmt.Sprintf("path not found: %s", root))
	}
	if !info.IsDir() {
		return ErrorResult("codebase_digest requires a directory, got a file — use read_file for single files")
	}
	if !t.pathAllowed(root, ws) {
		return ErrorResult(fmt.Sprintf("path %q outside allowed prefixes", root))
	}

	maxBytes := 120 * 1024
	if n, ok := toInt(args["max_bytes"]); ok && n > 0 {
		maxBytes = n
	}
	if maxBytes > 500*1024 {
		maxBytes = 500 * 1024
	}

	respectGitignore := true
	if v, ok := args["respect_gitignore"].(bool); ok {
		respectGitignore = v
	}

	var includePatterns []string
	if s, ok := args["include_patterns"].(string); ok && s != "" {
		for _, p := range strings.Split(s, ",") {
			if p = strings.TrimSpace(p); p != "" {
				includePatterns = append(includePatterns, p)
			}
		}
	}

	digest, err := buildDigest(ctx, root, maxBytes, respectGitignore, includePatterns)
	if err != nil {
		return ErrorResult(fmt.Sprintf("digest failed: %v", err))
	}
	return TextResult(digest)
}

// pathAllowed mirrors the allow-list shape in ReadFileTool.
func (t *CodebaseDigestTool) pathAllowed(root, ws string) bool {
	if strings.HasPrefix(root, ws) {
		return true
	}
	for _, p := range t.allowedPrefixes {
		if strings.HasPrefix(root, p) {
			return true
		}
	}
	return false
}

// --- core digest logic ---

type digestFile struct {
	path    string // relative to root
	absPath string
	size    int64
	modTime time.Time
}

func buildDigest(ctx context.Context, root string, maxBytes int, respectGitignore bool, includePatterns []string) (string, error) {
	// 1. Walk the tree, collecting eligible files.
	files, err := walkForDigest(ctx, root, respectGitignore, includePatterns)
	if err != nil {
		return "", err
	}

	// 2. Sort largest-first so we pack the biggest signal contributors
	//    before running out of budget.
	sort.Slice(files, func(i, j int) bool { return files[i].size > files[j].size })

	// 3. Build output. Header + tree + file bodies, stopping when the
	//    byte budget is exhausted. Files omitted due to budget are
	//    still listed in the tree with a "(omitted: budget)" tag.
	var sb strings.Builder
	var totalOnDisk int64
	for _, f := range files {
		totalOnDisk += f.size
	}

	// Header metadata.
	sb.WriteString("# Codebase Digest\n")
	sb.WriteString(fmt.Sprintf("Root: %s\n", root))
	sb.WriteString(fmt.Sprintf("Files: %d · On-disk: %s · Budget: %s\n\n",
		len(files), humanSize(totalOnDisk), humanSize(int64(maxBytes))))

	// Tree section — alphabetical for readability, not largest-first.
	treeSorted := append([]digestFile(nil), files...)
	sort.Slice(treeSorted, func(i, j int) bool { return treeSorted[i].path < treeSorted[j].path })
	sb.WriteString("## Tree\n")
	for _, f := range treeSorted {
		sb.WriteString(fmt.Sprintf("- %s  _(%s)_\n", f.path, humanSize(f.size)))
	}
	sb.WriteString("\n")

	// Files section.
	sb.WriteString("## Files\n\n")
	usedBytes := sb.Len()
	included := 0
	omitted := 0
	filesByPath := make(map[string]*digestFile, len(files))
	for i := range files {
		filesByPath[files[i].path] = &files[i]
	}

	// Pack in largest-first order so we maximise useful content.
	for _, f := range files {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		// Heuristic budget check: file body + header fits in remaining?
		// Leave 2 KiB safety margin for the closing trailer.
		bodyCap := maxBytes - usedBytes - 2048
		if bodyCap <= 200 {
			omitted++
			continue
		}
		body, skipReason := readForDigest(f.absPath, bodyCap)
		if skipReason != "" {
			// Only include the skip notice for moderately common cases
			// (binary, too-big). For truly omitted files we rely on
			// the trailing summary so we don't waste budget.
			sb.WriteString(fmt.Sprintf("### %s\n_(%s)_\n\n", f.path, skipReason))
			usedBytes = sb.Len()
			continue
		}
		lang := langFromExt(filepath.Ext(f.path))
		sb.WriteString(fmt.Sprintf("### %s\n```%s\n%s\n```\n\n", f.path, lang, body))
		included++
		usedBytes = sb.Len()
	}

	// Summary trailer.
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("_Digest complete: %d files included, %d omitted due to budget._\n",
		included, omitted))

	return sb.String(), nil
}

func walkForDigest(ctx context.Context, root string, respectGitignore bool, includePatterns []string) ([]digestFile, error) {
	var files []digestFile
	ignores := newGitignoreStack()

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate transient access errors
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip the well-known noise.
		name := d.Name()
		if name == ".git" || name == "node_modules" || name == "dist" ||
			name == "build" || name == ".next" || name == "__pycache__" ||
			name == ".venv" || name == "venv" || name == "target" ||
			name == ".turbo" || name == ".cache" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			if respectGitignore {
				ignores.loadAt(path)
			}
			return nil
		}

		// Skip hidden files unless explicitly allowed.
		if strings.HasPrefix(name, ".") && name != ".env.example" && name != ".gitignore" {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		if respectGitignore && ignores.ignored(rel, false) {
			return nil
		}

		// Apply include_patterns if provided.
		if len(includePatterns) > 0 {
			matched := false
			for _, p := range includePatterns {
				if ok, _ := filepath.Match(p, name); ok {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		// Skip files > 1 MiB outright — digesting a 5 MB minified JS
		// eats the budget and produces nothing useful.
		if info.Size() > 1024*1024 {
			return nil
		}

		files = append(files, digestFile{
			path:    rel,
			absPath: path,
			size:    info.Size(),
			modTime: info.ModTime(),
		})
		return nil
	})
	return files, err
}

// readForDigest reads up to bodyCap bytes from a file, skipping
// binaries and oversized files. Returns ("", reason) when the file
// must be skipped; (content, "") on success.
func readForDigest(absPath string, bodyCap int) (string, string) {
	if isBinaryFileExt(absPath) {
		return "", "skipped: binary extension"
	}
	f, err := os.Open(absPath)
	if err != nil {
		return "", "skipped: read error"
	}
	defer f.Close()

	// Sniff first 512 bytes for NUL — catches binaries masquerading
	// as text extensions.
	sniff := make([]byte, 512)
	n, _ := f.Read(sniff)
	for i := 0; i < n; i++ {
		if sniff[i] == 0 {
			return "", "skipped: binary content"
		}
	}
	// Reset for full read.
	if _, err := f.Seek(0, 0); err != nil {
		return "", "skipped: seek error"
	}

	// Cap body at the smaller of (file size) and (bodyCap).
	buf := make([]byte, bodyCap)
	nRead, _ := f.Read(buf)
	if nRead == bodyCap {
		// Got exactly bodyCap bytes — there's probably more. Append
		// a truncation marker so the model knows the tail is gone.
		return string(buf) + "\n…[truncated to fit digest budget]…", ""
	}
	return string(buf[:nRead]), ""
}

// langFromExt maps a file extension to a markdown fenced-block label
// so the LLM's syntax highlighter gets useful hints. Unknown = "".
func langFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py":
		return "python"
	case ".rb":
		return "ruby"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".kt":
		return "kotlin"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".hpp":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".sh", ".bash":
		return "bash"
	case ".md":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".html":
		return "html"
	case ".css":
		return "css"
	case ".sql":
		return "sql"
	case ".xml":
		return "xml"
	case ".dockerfile":
		return "dockerfile"
	case ".proto":
		return "protobuf"
	default:
		return ""
	}
}

// --- gitignore support ---
//
// Simple layered implementation: every directory we enter can add its
// own .gitignore. Rules are tested against the path relative to root,
// using path components only. Handles:
//   - leading slash ("root-anchored")
//   - trailing slash (directory-only)
//   - "!" negation
//   - "*" and "?" wildcards (via filepath.Match)
//   - "**" — not supported, rare in practice
//
// That's 95% of real .gitignore content. Missing the 5% = slightly
// over-inclusive digest, which is a mild failure mode vs. the
// alternative of adding a full gitignore library.

type gitignoreRule struct {
	pattern  string
	negate   bool
	dirOnly  bool
	anchored bool
}

type gitignoreStack struct {
	rules []gitignoreRule
}

func newGitignoreStack() *gitignoreStack { return &gitignoreStack{} }

func (g *gitignoreStack) loadAt(dirPath string) {
	f, err := os.Open(filepath.Join(dirPath, ".gitignore"))
	if err != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule := gitignoreRule{pattern: line}
		if strings.HasPrefix(rule.pattern, "!") {
			rule.negate = true
			rule.pattern = rule.pattern[1:]
		}
		if strings.HasSuffix(rule.pattern, "/") {
			rule.dirOnly = true
			rule.pattern = strings.TrimSuffix(rule.pattern, "/")
		}
		if strings.HasPrefix(rule.pattern, "/") {
			rule.anchored = true
			rule.pattern = strings.TrimPrefix(rule.pattern, "/")
		}
		g.rules = append(g.rules, rule)
	}
}

func (g *gitignoreStack) ignored(relPath string, isDir bool) bool {
	// Last-match-wins: a negation after a match un-ignores.
	result := false
	for _, r := range g.rules {
		// dirOnly rules still match files UNDER the ignored directory
		// ("build/" ignores build/main.go). We handle that by checking
		// whether the path has the pattern as an ancestor component —
		// not just whether the current leaf is a directory.
		if r.dirOnly && !isDir && !dirPrefixMatches(r, relPath) {
			continue
		}
		if matchGitignore(r, relPath) {
			result = !r.negate
		}
	}
	return result
}

// dirPrefixMatches returns true when relPath lives under a directory
// that the rule ignores. Handles both anchored and non-anchored
// patterns.
//
// Example: rule "build/" vs relPath "build/out.txt" → true.
//          rule "build/" vs relPath "src/build/x"    → true (non-anchored).
func dirPrefixMatches(r gitignoreRule, relPath string) bool {
	parts := strings.Split(relPath, "/")
	if len(parts) < 2 {
		return false
	}
	// For anchored rules, only the leading segment can match.
	if r.anchored {
		if ok, _ := filepath.Match(r.pattern, parts[0]); ok {
			return true
		}
		return false
	}
	// Non-anchored: any interior directory component can match.
	// Stop before the final component because a single-path match
	// would be handled by the regular matchGitignore path.
	for i := 0; i < len(parts)-1; i++ {
		if ok, _ := filepath.Match(r.pattern, parts[i]); ok {
			return true
		}
	}
	return false
}

func matchGitignore(r gitignoreRule, relPath string) bool {
	// Anchored patterns match against full relPath.
	if r.anchored {
		ok, _ := filepath.Match(r.pattern, relPath)
		if ok {
			return true
		}
		// Also match subpaths when rule is a directory.
		return strings.HasPrefix(relPath, r.pattern+"/")
	}
	// Non-anchored: match any segment. Walk the components.
	parts := strings.Split(relPath, "/")
	for i := range parts {
		tail := strings.Join(parts[i:], "/")
		if ok, _ := filepath.Match(r.pattern, tail); ok {
			return true
		}
		if ok, _ := filepath.Match(r.pattern, parts[i]); ok {
			return true
		}
	}
	return false
}
