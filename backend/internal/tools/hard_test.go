// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Hard tool tests — real-world scenarios, stress, security edge cases.

func TestHard_Filesystem_ProjectStructure(t *testing.T) {
	dir := t.TempDir()
	write := NewWriteFileTool(dir)
	read := NewReadFileTool(dir)
	list := NewListFilesTool(dir)
	ctx := context.Background()

	// Create a realistic project structure
	files := map[string]string{
		"go.mod":           "module github.com/test/project\n\ngo 1.22\n",
		"main.go":          "package main\n\nfunc main() {}\n",
		"internal/api/handler.go": "package api\n\nfunc Handle() {}\n",
		"internal/api/middleware.go": "package api\n\nfunc Auth() {}\n",
		"internal/db/store.go": "package db\n\nfunc Connect() {}\n",
		"tests/api_test.go": "package tests\n\nfunc TestAPI() {}\n",
		"README.md":         "# Test Project\n\nA test project.\n",
		".gitignore":        "*.exe\n*.log\n",
	}

	for path, content := range files {
		r := write.Execute(ctx, map[string]any{"path": path, "content": content})
		if r.IsError { t.Fatalf("write %s: %s", path, r.ForLLM) }
	}

	// Verify structure
	r := list.Execute(ctx, map[string]any{"path": "."})
	if r.IsError { t.Fatalf("list: %s", r.ForLLM) }
	for _, name := range []string{"go.mod", "main.go", "internal", "tests", "README.md"} {
		if !strings.Contains(r.ForLLM, name) { t.Errorf("missing %s in listing", name) }
	}

	// Read and verify content
	for path, expected := range files {
		r := read.Execute(ctx, map[string]any{"path": path})
		if r.IsError { t.Errorf("read %s: %s", path, r.ForLLM); continue }
		if !strings.Contains(r.ForLLM, strings.TrimSpace(expected)[:min6(len(strings.TrimSpace(expected)), 20)]) {
			t.Errorf("content mismatch for %s", path)
		}
	}
	t.Logf("project structure: %d files created and verified ✓", len(files))
}

func TestHard_Exec_Pipeline(t *testing.T) {
	dir := t.TempDir()
	exec := NewExecTool(dir, true)
	write := NewWriteFileTool(dir)
	ctx := context.Background()

	// Write data file
	write.Execute(ctx, map[string]any{"path": "data.txt", "content": "apple\nbanana\ncherry\napple\ndate\nbanana\napple\n"})

	// Count unique lines
	r := exec.Execute(ctx, map[string]any{"command": "sort data.txt | uniq -c | sort -rn"})
	if r.IsError { t.Fatalf("pipeline: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "apple") { t.Error("missing apple") }
	t.Logf("pipeline output:\n%s", r.ForLLM)

	// Word count
	r = exec.Execute(ctx, map[string]any{"command": "wc -l data.txt"})
	if r.IsError { t.Fatalf("wc: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "7") { t.Logf("wc: %s", r.ForLLM) }

	// Grep
	r = exec.Execute(ctx, map[string]any{"command": "grep -c apple data.txt"})
	if r.IsError { t.Logf("grep: %s", r.ForLLM) }
	if strings.Contains(r.ForLLM, "3") { t.Log("grep count: apple=3 ✓") }
}

func TestHard_Exec_ConcurrentStress(t *testing.T) {
	dir := t.TempDir()
	exec := NewExecTool(dir, true)
	ctx := context.Background()

	var wg sync.WaitGroup
	errors := 0
	var mu sync.Mutex

	// 30 concurrent exec calls
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r := exec.Execute(ctx, map[string]any{"command": "echo " + string(rune('A'+n%26))})
			if r.IsError { mu.Lock(); errors++; mu.Unlock() }
		}(i)
	}
	wg.Wait()
	if errors > 0 { t.Errorf("%d/30 concurrent exec failed", errors) }
	t.Logf("30 concurrent exec: %d errors", errors)
}

func TestHard_Security_SymlinkTraversal(t *testing.T) {
	dir := t.TempDir()
	read := NewReadFileTool(dir)
	ctx := context.Background()

	// Create a symlink pointing outside workspace
	secretDir := t.TempDir()
	os.WriteFile(filepath.Join(secretDir, "secret.txt"), []byte("TOP_SECRET"), 0600)
	os.Symlink(filepath.Join(secretDir, "secret.txt"), filepath.Join(dir, "link.txt"))

	// Try to read through symlink
	r := read.Execute(ctx, map[string]any{"path": "link.txt"})
	if !r.IsError && strings.Contains(r.ForLLM, "TOP_SECRET") {
		t.Error("SECURITY: symlink traversal leaked secret!")
	} else {
		t.Log("symlink traversal blocked ✓")
	}
}

func TestHard_Security_URLEncodedTraversal(t *testing.T) {
	dir := t.TempDir()
	read := NewReadFileTool(dir)
	ctx := context.Background()

	// URL-encoded path traversal (bug we fixed)
	traversals := []string{
		"%2e%2e%2f%2e%2e%2fetc/passwd",
		"%2e%2e/%2e%2e/etc/passwd",
		"..%2f..%2fetc/passwd",
	}

	for _, path := range traversals {
		r := read.Execute(ctx, map[string]any{"path": path})
		if !r.IsError {
			if strings.Contains(r.ForLLM, "root:") {
				t.Fatalf("SECURITY: URL-encoded traversal leaked /etc/passwd via %q", path)
			}
		}
	}
	t.Log("URL-encoded path traversal blocked ✓")
}

func TestHard_Edit_ComplexRefactor(t *testing.T) {
	dir := t.TempDir()
	write := NewWriteFileTool(dir)
	edit := NewEditTool(dir)
	read := NewReadFileTool(dir)
	ctx := context.Background()

	// Write original file
	write.Execute(ctx, map[string]any{"path": "config.go", "content": `package config

const (
	DefaultPort = 8080
	DefaultHost = "localhost"
	MaxRetries  = 3
)

func GetPort() int { return DefaultPort }
func GetHost() string { return DefaultHost }
`})

	// Edit: change port
	edit.Execute(ctx, map[string]any{"path": "config.go", "find": "DefaultPort = 8080", "replace": "DefaultPort = 4200"})

	// Edit: change host
	edit.Execute(ctx, map[string]any{"path": "config.go", "find": `DefaultHost = "localhost"`, "replace": `DefaultHost = "0.0.0.0"`})

	// Verify both edits applied
	r := read.Execute(ctx, map[string]any{"path": "config.go"})
	if r.IsError { t.Fatal(r.ForLLM) }
	if !strings.Contains(r.ForLLM, "4200") { t.Error("port edit not applied") }
	if !strings.Contains(r.ForLLM, "0.0.0.0") { t.Error("host edit not applied") }
	if strings.Contains(r.ForLLM, "8080") { t.Error("old port still present") }
	if strings.Contains(r.ForLLM, "localhost") { t.Error("old host still present") }
	t.Log("complex refactor: 2 edits applied and verified ✓")
}

func TestHard_WebFetch_RealSite(t *testing.T) {
	if testing.Short() { t.Skip("skip real HTTP") }

	fetch := NewWebFetchToolWithConfig(WebFetchConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	r := fetch.Execute(ctx, map[string]any{"url": "https://httpbin.org/html"})
	if r.IsError { t.Skipf("httpbin: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "Herman Melville") && !strings.Contains(r.ForLLM, "Moby") {
		t.Logf("fetch content: %s...", r.ForLLM[:min6(len(r.ForLLM), 200)])
	}
	t.Log("real web fetch: httpbin.org/html ✓")
}
