// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupMiniRepo creates a realistic little repo on disk for digest tests
// and returns its root. The layout covers the interesting cases:
//
//   ./README.md          — markdown, should include
//   ./main.go            — source, should include
//   ./secret.env         — .gitignored, should be skipped
//   ./build/output.bin   — .gitignored via dir pattern
//   ./src/app.ts         — nested source, should include
//   ./assets/logo.png    — binary extension, should be skipped
//   ./big.log            — oversize (>1MiB), should be skipped
//   ./.gitignore         — defines the rules used above
func setupMiniRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	write := func(rel, content string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(".gitignore", "secret.env\nbuild/\n*.log\n")
	write("README.md", "# My App\n\nHello.\n")
	write("main.go", "package main\n\nfunc main() {}\n")
	write("secret.env", "API_KEY=nope\n")
	write("build/output.bin", "binary here")
	write("src/app.ts", "export const x = 1;\n")
	write("assets/logo.png", "PNG_PLACEHOLDER") // .png = binary extension

	// Oversize file: 1.2 MiB of "a".
	big := make([]byte, 1024*1024+200*1024)
	for i := range big {
		big[i] = 'a'
	}
	if err := os.WriteFile(filepath.Join(root, "big.log"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	return root
}

// TestCodebaseDigest_HappyPath: the default case — digest a small repo
// and verify that tracked text files are included and ignored /
// binary / oversize ones are not.
func TestCodebaseDigest_HappyPath(t *testing.T) {
	root := setupMiniRepo(t)

	tool := NewCodebaseDigestTool(root)
	r := tool.Execute(context.Background(), map[string]any{"path": "."})
	if r.IsError {
		t.Fatalf("unexpected error: %s", r.ForLLM)
	}
	out := r.ForLLM

	// Must include the tracked text files.
	for _, want := range []string{"README.md", "main.go", "src/app.ts"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}

	// .gitignored content must NOT appear anywhere.
	if strings.Contains(out, "API_KEY=nope") {
		t.Error("gitignored file content leaked into digest")
	}
	// build/ is an ignored dir — no mentions at all.
	if strings.Contains(out, "build/output.bin") {
		t.Error("gitignored directory child appeared in digest")
	}
	// big.log is over size cap AND matches *.log in gitignore.
	if strings.Contains(out, "big.log") {
		t.Error("oversize/ignored file appeared in digest")
	}
	// Binary by extension — PNG body is 15 bytes, so size cap isn't
	// the reason it's missing; it's the extension filter.
	if strings.Contains(out, "PNG_PLACEHOLDER") {
		t.Error("binary file content leaked into digest")
	}
}

// TestCodebaseDigest_HeaderAndTree: every digest has a header with
// file count + on-disk size, a Tree section listing every included
// file with its size, and a Files section with fenced blocks. Any
// regression to the output format breaks downstream LLM parsing.
func TestCodebaseDigest_HeaderAndTree(t *testing.T) {
	root := setupMiniRepo(t)
	tool := NewCodebaseDigestTool(root)
	r := tool.Execute(context.Background(), map[string]any{"path": "."})
	if r.IsError {
		t.Fatal(r.ForLLM)
	}

	out := r.ForLLM
	for _, section := range []string{
		"# Codebase Digest",
		"## Tree",
		"## Files",
		"_Digest complete:",
	} {
		if !strings.Contains(out, section) {
			t.Errorf("missing section header %q", section)
		}
	}
	// Fenced block with language hint for go file.
	if !strings.Contains(out, "```go") {
		t.Error("missing go fenced-block language hint")
	}
}

// TestCodebaseDigest_ByteBudget: when the caller sets a small cap, the
// tool packs bigger files first (because they carry more signal) and
// stops when budget is exhausted — rather than silently returning a
// half-cooked output.
func TestCodebaseDigest_ByteBudget(t *testing.T) {
	root := setupMiniRepo(t)
	tool := NewCodebaseDigestTool(root)

	// Tight budget — only header + tree + maybe one file should fit.
	r := tool.Execute(context.Background(), map[string]any{
		"path":      ".",
		"max_bytes": 900,
	})
	if r.IsError {
		t.Fatalf("tight budget should still produce output: %s", r.ForLLM)
	}
	out := r.ForLLM
	// Output shouldn't dramatically exceed budget (small overhead for
	// trailer is fine; we left 2 KiB margin inside the packer).
	if len(out) > 4000 {
		t.Errorf("output %d bytes for 900 budget — too generous", len(out))
	}
	// Must still include the tree regardless of budget pressure.
	if !strings.Contains(out, "## Tree") {
		t.Error("budget-constrained digest missing tree")
	}
}

// TestCodebaseDigest_IncludePatterns: whitelisting extensions filters
// the file list before budgeting — digesting only *.go from a mixed
// repo should not include README.md or ts files.
func TestCodebaseDigest_IncludePatterns(t *testing.T) {
	root := setupMiniRepo(t)
	tool := NewCodebaseDigestTool(root)

	r := tool.Execute(context.Background(), map[string]any{
		"path":             ".",
		"include_patterns": "*.go",
	})
	if r.IsError {
		t.Fatal(r.ForLLM)
	}
	out := r.ForLLM
	if !strings.Contains(out, "main.go") {
		t.Error("include_patterns=*.go should include main.go")
	}
	if strings.Contains(out, "README.md") {
		t.Error("include_patterns=*.go should NOT include README.md")
	}
	if strings.Contains(out, "app.ts") {
		t.Error("include_patterns=*.go should NOT include app.ts")
	}
}

// TestCodebaseDigest_RespectGitignoreDisabled: operators investigating
// a misbuilt artifact can turn off gitignore to see what's actually
// on disk.
func TestCodebaseDigest_RespectGitignoreDisabled(t *testing.T) {
	root := setupMiniRepo(t)
	tool := NewCodebaseDigestTool(root)

	r := tool.Execute(context.Background(), map[string]any{
		"path":              ".",
		"respect_gitignore": false,
	})
	if r.IsError {
		t.Fatal(r.ForLLM)
	}
	out := r.ForLLM
	// secret.env should now appear in Tree. (Content may or may not
	// be included depending on budget; existence in tree is enough.)
	if !strings.Contains(out, "secret.env") {
		t.Error("respect_gitignore=false should expose secret.env")
	}
}

// TestCodebaseDigest_SinglePathNotDir: the tool is dir-only. A file
// path must get a clear "use read_file instead" message.
func TestCodebaseDigest_SinglePathNotDir(t *testing.T) {
	root := setupMiniRepo(t)
	tool := NewCodebaseDigestTool(root)

	r := tool.Execute(context.Background(), map[string]any{"path": "README.md"})
	if !r.IsError {
		t.Fatal("expected error when path is a file")
	}
	if !strings.Contains(r.ForLLM, "read_file") {
		t.Errorf("error should point at read_file; got %q", r.ForLLM)
	}
}

// TestCodebaseDigest_MissingPath: non-existent path produces a clean
// error, not a panic or empty-but-successful response.
func TestCodebaseDigest_MissingPath(t *testing.T) {
	root := setupMiniRepo(t)
	tool := NewCodebaseDigestTool(root)

	r := tool.Execute(context.Background(), map[string]any{"path": "nope/does/not/exist"})
	if !r.IsError {
		t.Fatal("expected error for missing path")
	}
}

// TestCodebaseDigest_PathTraversal: an absolute path outside the
// workspace AND not in the allow-list must be rejected. Guards
// against an agent digesting /etc/ or /root/.
func TestCodebaseDigest_PathTraversal(t *testing.T) {
	root := setupMiniRepo(t)
	tool := NewCodebaseDigestTool(root) // no AllowPaths — strict

	// /tmp exists on every test runner. It's outside the workspace
	// and not in the allow-list, so the tool must refuse.
	r := tool.Execute(context.Background(), map[string]any{"path": "/tmp"})
	if !r.IsError {
		t.Fatal("digesting /tmp should be blocked without explicit AllowPaths")
	}
	if !strings.Contains(r.ForLLM, "outside") {
		t.Errorf("error should mention the path is outside allowed prefixes; got %q", r.ForLLM)
	}
}

// TestCodebaseDigest_AllowPathsExpandsScope: after AllowPaths adds a
// prefix, the tool can digest inside it.
func TestCodebaseDigest_AllowPathsExpandsScope(t *testing.T) {
	root := setupMiniRepo(t)
	otherDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(otherDir, "note.md"), []byte("allowed!\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewCodebaseDigestTool(root)
	tool.AllowPaths(otherDir)

	r := tool.Execute(context.Background(), map[string]any{"path": otherDir})
	if r.IsError {
		t.Fatalf("allow-listed path should succeed: %s", r.ForLLM)
	}
	if !strings.Contains(r.ForLLM, "allowed!") {
		t.Error("allow-listed path should include its note.md content")
	}
}

// TestCodebaseDigest_SkipsNoiseDirs: node_modules, .git, dist, etc. are
// hard-skipped even without a .gitignore rule. This is the most common
// source of waste in real repos.
func TestCodebaseDigest_SkipsNoiseDirs(t *testing.T) {
	root := t.TempDir()
	// Intentionally no .gitignore.
	write := func(rel, content string) {
		full := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte(content), 0o644)
	}
	write("README.md", "hi\n")
	write("node_modules/pkg/index.js", "const a=1;\n")
	write(".git/config", "[core]\n")
	write("dist/bundle.js", "console.log(1);\n")

	tool := NewCodebaseDigestTool(root)
	r := tool.Execute(context.Background(), map[string]any{"path": "."})
	if r.IsError {
		t.Fatal(r.ForLLM)
	}
	for _, noise := range []string{"node_modules", ".git/config", "dist/bundle.js"} {
		if strings.Contains(r.ForLLM, noise) {
			t.Errorf("noise dir %q not skipped", noise)
		}
	}
}

// TestGitignoreStack_Basics: direct test of the mini gitignore impl.
// Covers the patterns that 95% of repos use in practice.
func TestGitignoreStack_Basics(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, ".gitignore"),
		[]byte("secret.env\nbuild/\n*.log\n!keep.log\n/root-only\n"), 0o644)

	g := newGitignoreStack()
	g.loadAt(dir)

	cases := []struct {
		path string
		dir  bool
		want bool
	}{
		{"secret.env", false, true},
		{"subdir/secret.env", false, true},      // non-anchored match
		{"build", true, true},                    // dir-only rule
		{"build/anything.txt", false, true},      // under ignored dir
		{"app.log", false, true},                 // *.log
		{"keep.log", false, false},               // negation wins
		{"root-only", false, true},               // anchored + root
		{"nested/root-only", false, false},       // anchored rule; no match nested
		{"README.md", false, false},              // unrelated
	}
	for _, c := range cases {
		got := g.ignored(c.path, c.dir)
		if got != c.want {
			t.Errorf("ignored(%q, dir=%v) = %v, want %v", c.path, c.dir, got, c.want)
		}
	}
}
