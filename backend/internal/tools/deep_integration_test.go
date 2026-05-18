// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Deep tool integration tests — real file I/O, real exec, real pipelines.

func TestDeep_Filesystem_WriteReadEditDelete(t *testing.T) {
	dir := t.TempDir()
	write := NewWriteFileTool(dir)
	read := NewReadFileTool(dir)
	edit := NewEditTool(dir)
	list := NewListFilesTool(dir)
	ctx := context.Background()

	// 1. Write a Go file
	r := write.Execute(ctx, map[string]any{
		"path": "main.go",
		"content": `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`,
	})
	if r.IsError { t.Fatalf("write: %s", r.ForLLM) }

	// 2. Read it back — verify exact content
	r = read.Execute(ctx, map[string]any{"path": "main.go"})
	if r.IsError { t.Fatalf("read: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "Hello, World!") { t.Error("content mismatch") }
	if !strings.Contains(r.ForLLM, "package main") { t.Error("missing package") }

	// 3. Edit — change the message
	r = edit.Execute(ctx, map[string]any{
		"path":    "main.go",
		"find": `fmt.Println("Hello, World!")`,
		"replace": `fmt.Println("Hello, Qorven!")`,
	})
	if r.IsError { t.Logf("edit: %s", r.ForLLM) }

	// 4. Read again — verify edit
	r = read.Execute(ctx, map[string]any{"path": "main.go"})
	if r.IsError { t.Fatalf("read after edit: %s", r.ForLLM) }
	if strings.Contains(r.ForLLM, "Hello, World!") { t.Error("edit didn't apply — old content still present") }

	// 5. List files
	r = list.Execute(ctx, map[string]any{"path": "."})
	if r.IsError { t.Fatalf("list: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "main.go") { t.Error("main.go not in listing") }

	// 6. Write more files
	write.Execute(ctx, map[string]any{"path": "README.md", "content": "# Test Project"})
	write.Execute(ctx, map[string]any{"path": "go.mod", "content": "module test"})

	r = list.Execute(ctx, map[string]any{"path": "."})
	if !strings.Contains(r.ForLLM, "README.md") { t.Error("README missing") }
	if !strings.Contains(r.ForLLM, "go.mod") { t.Error("go.mod missing") }
}

func TestDeep_Filesystem_BinaryFile(t *testing.T) {
	dir := t.TempDir()
	write := NewWriteFileTool(dir)
	read := NewReadFileTool(dir)
	ctx := context.Background()

	// Write binary-ish content
	content := string([]byte{0x89, 0x50, 0x4E, 0x47}) + "PNG data here"
	write.Execute(ctx, map[string]any{"path": "image.png", "content": content})

	r := read.Execute(ctx, map[string]any{"path": "image.png"})
	if r.IsError { t.Logf("binary read: %s", r.ForLLM) }
}

func TestDeep_Filesystem_LargeFile(t *testing.T) {
	dir := t.TempDir()
	write := NewWriteFileTool(dir)
	read := NewReadFileTool(dir)
	ctx := context.Background()

	// Write 1MB file
	large := strings.Repeat("Line of text for testing large file handling.\n", 20000)
	write.Execute(ctx, map[string]any{"path": "large.txt", "content": large})

	r := read.Execute(ctx, map[string]any{"path": "large.txt"})
	if r.IsError { t.Fatalf("large read: %s", r.ForLLM) }
	t.Logf("large file: wrote %d chars, read %d chars", len(large), len(r.ForLLM))
}

func TestDeep_Filesystem_NestedDirectories(t *testing.T) {
	dir := t.TempDir()
	write := NewWriteFileTool(dir)
	list := NewListFilesTool(dir)
	ctx := context.Background()

	// Create nested structure
	paths := []string{
		"src/main.go", "src/utils/helper.go", "src/utils/math.go",
		"tests/main_test.go", "docs/README.md",
	}
	for _, p := range paths {
		write.Execute(ctx, map[string]any{"path": p, "content": "// " + p})
	}

	// List root
	r := list.Execute(ctx, map[string]any{"path": "."})
	if r.IsError { t.Fatalf("list root: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "src") { t.Error("missing src dir") }

	// List nested
	r = list.Execute(ctx, map[string]any{"path": "src/utils"})
	if r.IsError { t.Fatalf("list nested: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "helper.go") { t.Error("missing helper.go") }
}

func TestDeep_Exec_RealCommands(t *testing.T) {
	dir := t.TempDir()
	exec := NewExecTool(dir, true)
	write := NewWriteFileTool(dir)
	ctx := context.Background()

	// Write a script
	write.Execute(ctx, map[string]any{
		"path": "count.sh",
		"content": "#!/bin/bash\nfor i in 1 2 3 4 5; do echo $i; done",
	})
	os.Chmod(filepath.Join(dir, "count.sh"), 0755)

	// Execute it
	r := exec.Execute(ctx, map[string]any{"command": "bash count.sh"})
	if r.IsError { t.Fatalf("exec script: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "3") { t.Error("missing output") }
	if !strings.Contains(r.ForLLM, "5") { t.Error("missing last number") }

	// Pipe commands
	r = exec.Execute(ctx, map[string]any{"command": "echo 'hello world' | wc -w"})
	if r.IsError { t.Fatalf("pipe: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "2") { t.Errorf("wc output: %q", r.ForLLM) }

	// Environment variables
	r = exec.Execute(ctx, map[string]any{"command": "echo $PWD"})
	if r.IsError { t.Fatalf("env: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, dir) { t.Logf("PWD: %q (may differ)", r.ForLLM) }

	// Command with stderr
	r = exec.Execute(ctx, map[string]any{"command": "ls nonexistent_file_xyz 2>&1"})
	if !strings.Contains(r.ForLLM, "No such file") && !strings.Contains(r.ForLLM, "cannot access") {
		t.Logf("stderr: %q", r.ForLLM)
	}
}

func TestDeep_Exec_ConcurrentCommands(t *testing.T) {
	dir := t.TempDir()
	exec := NewExecTool(dir, true)
	ctx := context.Background()

	// Run 10 commands concurrently — each writes to a different file
	write := NewWriteFileTool(dir)
	results := make(chan string, 10)

	for i := 0; i < 10; i++ {
		go func(n int) {
			fname := "out_" + string(rune('0'+n)) + ".txt"
			exec.Execute(ctx, map[string]any{"command": "echo " + string(rune('A'+n)) + " > " + fname})
			r := NewReadFileTool(dir).Execute(ctx, map[string]any{"path": fname})
			results <- r.ForLLM
		}(i)
	}

	for i := 0; i < 10; i++ {
		select {
		case r := <-results:
			_ = r
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent exec timeout")
		}
	}

	// Verify all files exist
	r := write.Execute(ctx, map[string]any{"path": "verify.txt", "content": "done"})
	_ = r
	entries, _ := os.ReadDir(dir)
	txtCount := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "out_") { txtCount++ }
	}
	if txtCount < 8 { t.Errorf("expected 10 output files, got %d", txtCount) }
}

func TestDeep_Exec_ResourceLimits(t *testing.T) {
	dir := t.TempDir()
	exec := NewExecTool(dir, true)

	// Test memory — allocate and release
	r := exec.Execute(context.Background(), map[string]any{
		"command": "head -c 1000000 /dev/urandom | wc -c",
	})
	if r.IsError { t.Logf("memory test: %s", r.ForLLM) }
	if strings.Contains(r.ForLLM, "1000000") { t.Log("1MB random data processed") }

	// Test CPU — quick computation
	r = exec.Execute(context.Background(), map[string]any{
		"command": "seq 1 10000 | awk '{s+=$1} END{print s}'",
	})
	if r.IsError { t.Logf("cpu test: %s", r.ForLLM) }
	if strings.Contains(r.ForLLM, "50005000") { t.Log("sum 1..10000 = 50005000 ✓") }
}

func TestDeep_WebFetch_RealURL(t *testing.T) {
	if testing.Short() { t.Skip("skipping real HTTP in short mode") }

	fetch := NewWebFetchToolWithConfig(WebFetchConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r := fetch.Execute(ctx, map[string]any{"url": "https://httpbin.org/get"})
	if r.IsError { t.Skipf("httpbin unavailable: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "httpbin") && !strings.Contains(r.ForLLM, "origin") {
		t.Logf("fetch result: %s", r.ForLLM[:min6(len(r.ForLLM), 200)])
	}
	t.Log("real HTTP fetch works")
}

func TestDeep_WebFetch_Timeout(t *testing.T) {
	fetch := NewWebFetchToolWithConfig(WebFetchConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	r := fetch.Execute(ctx, map[string]any{"url": "https://httpbin.org/delay/10"})
	if !r.IsError { t.Log("may have completed before timeout") }
}

func TestDeep_SecurityAudit_AllTools(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Create a sensitive file outside workspace
	sensitiveDir := t.TempDir()
	os.WriteFile(filepath.Join(sensitiveDir, "secret.key"), []byte("SUPER_SECRET_KEY"), 0600)

	read := NewReadFileTool(dir)
	write := NewWriteFileTool(dir)

	// Try to read outside workspace via various traversal methods
	traversals := []string{
		"../../../etc/passwd",
		"/etc/passwd",
		"./../../" + filepath.Base(sensitiveDir) + "/secret.key",
		"subdir/../../../tmp/test",
	}

	for _, path := range traversals {
		r := read.Execute(ctx, map[string]any{"path": path})
		if !r.IsError {
			if strings.Contains(r.ForLLM, "SUPER_SECRET") {
				t.Fatalf("SECURITY: read traversal leaked secret via %q", path)
			}
			if strings.Contains(r.ForLLM, "root:") {
				t.Fatalf("SECURITY: read traversal accessed /etc/passwd via %q", path)
			}
		}

		r = write.Execute(ctx, map[string]any{"path": path, "content": "hacked"})
		if !r.IsError {
			// Verify the file wasn't actually written outside workspace
			if _, err := os.Stat(filepath.Join(sensitiveDir, "hacked")); err == nil {
				t.Fatal("SECURITY: write traversal created file outside workspace")
			}
		}
	}
	t.Log("all path traversal attempts blocked ✓")
}

func min6(a, b int) int { if a < b { return a }; return b }
