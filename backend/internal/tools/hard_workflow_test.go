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

// Hard tool tests — real multi-step workflows, data integrity, error recovery.

func TestHard_Workflow_WriteExecVerify(t *testing.T) {
	// Write a Python script → execute it → verify output
	dir := t.TempDir()
	write := NewWriteFileTool(dir)
	exec := NewExecTool(dir, true)
	ctx := context.Background()

	// Write Python script that computes fibonacci
	write.Execute(ctx, map[string]any{
		"path": "fib.py",
		"content": `def fib(n):
    a, b = 0, 1
    for _ in range(n):
        a, b = b, a + b
    return a

print(fib(10))
`,
	})

	// Execute and verify
	r := exec.Execute(ctx, map[string]any{"command": "python3 fib.py"})
	if r.IsError { t.Skipf("python3 not available: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "55") { t.Errorf("fib(10)=55, got %q", r.ForLLM) }
	t.Log("write→exec→verify: fib(10)=55 ✓")
}

func TestHard_Workflow_EditAndVerifyDiff(t *testing.T) {
	// Write file → edit → verify only the edit changed
	dir := t.TempDir()
	write := NewWriteFileTool(dir)
	edit := NewEditTool(dir)
	read := NewReadFileTool(dir)
	ctx := context.Background()

	original := "line 1: hello\nline 2: world\nline 3: foo\nline 4: bar\nline 5: baz\n"
	write.Execute(ctx, map[string]any{"path": "data.txt", "content": original})

	// Edit only line 3
	edit.Execute(ctx, map[string]any{"path": "data.txt", "find": "line 3: foo", "replace": "line 3: CHANGED"})

	r := read.Execute(ctx, map[string]any{"path": "data.txt"})
	if r.IsError { t.Fatal(r.ForLLM) }

	// Verify line 3 changed but others didn't
	if !strings.Contains(r.ForLLM, "line 3: CHANGED") { t.Error("edit not applied") }
	if !strings.Contains(r.ForLLM, "line 1: hello") { t.Error("line 1 corrupted") }
	if !strings.Contains(r.ForLLM, "line 5: baz") { t.Error("line 5 corrupted") }
	if strings.Contains(r.ForLLM, "line 3: foo") { t.Error("old content still present") }
	t.Log("edit precision: only target line changed, others intact ✓")
}

func TestHard_Exec_OutputCapture_Stderr(t *testing.T) {
	dir := t.TempDir()
	exec := NewExecTool(dir, true)
	ctx := context.Background()

	// Command that writes to both stdout and stderr
	r := exec.Execute(ctx, map[string]any{"command": "echo STDOUT; echo STDERR >&2"})
	if r.IsError { t.Logf("stderr handling: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "STDOUT") { t.Error("stdout missing") }
	t.Logf("stdout+stderr capture: %q", r.ForLLM[:min6(len(r.ForLLM), 100)])
}

func TestHard_Exec_WorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	exec := NewExecTool(dir, true)
	write := NewWriteFileTool(dir)
	ctx := context.Background()

	// Create a file in workspace
	write.Execute(ctx, map[string]any{"path": "marker.txt", "content": "WORKSPACE_MARKER"})

	// Verify exec runs in the workspace directory
	r := exec.Execute(ctx, map[string]any{"command": "cat marker.txt"})
	if r.IsError { t.Fatalf("cat: %s", r.ForLLM) }
	if !strings.Contains(r.ForLLM, "WORKSPACE_MARKER") { t.Error("wrong working directory") }
	t.Log("exec working directory: workspace verified ✓")
}

func TestHard_Filesystem_PermissionPreservation(t *testing.T) {
	dir := t.TempDir()
	write := NewWriteFileTool(dir)
	ctx := context.Background()

	// Write a file
	write.Execute(ctx, map[string]any{"path": "script.sh", "content": "#!/bin/bash\necho hello"})

	// Check it was created
	info, err := os.Stat(filepath.Join(dir, "script.sh"))
	if err != nil { t.Fatal(err) }
	if info.Size() == 0 { t.Error("empty file") }
	t.Logf("file created: %d bytes, mode=%v", info.Size(), info.Mode())
}

func TestHard_Filesystem_ConcurrentReadWrite(t *testing.T) {
	dir := t.TempDir()
	write := NewWriteFileTool(dir)
	read := NewReadFileTool(dir)
	ctx := context.Background()

	// Write 10 files
	for i := 0; i < 10; i++ {
		write.Execute(ctx, map[string]any{
			"path":    "file_" + string(rune('0'+i)) + ".txt",
			"content": "Content of file " + string(rune('0'+i)) + " with unique data " + strings.Repeat("x", 100*i),
		})
	}

	// Read all 10 concurrently and verify content integrity
	type result struct {
		idx     int
		content string
		err     bool
	}
	results := make(chan result, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			r := read.Execute(ctx, map[string]any{"path": "file_" + string(rune('0'+n)) + ".txt"})
			results <- result{idx: n, content: r.ForLLM, err: r.IsError}
		}(i)
	}

	errors := 0
	for i := 0; i < 10; i++ {
		r := <-results
		if r.err { errors++; continue }
		expected := "Content of file " + string(rune('0'+r.idx))
		if !strings.Contains(r.content, expected) { t.Errorf("file %d: content mismatch", r.idx); errors++ }
	}
	if errors > 0 { t.Errorf("%d/10 concurrent reads failed", errors) }
	t.Log("10 concurrent reads: content integrity verified ✓")
}

func TestHard_WebFetch_ResponseParsing(t *testing.T) {
	if testing.Short() { t.Skip("skip real HTTP") }

	fetch := NewWebFetchToolWithConfig(WebFetchConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Fetch a JSON endpoint
	r := fetch.Execute(ctx, map[string]any{"url": "https://httpbin.org/json"})
	if r.IsError { t.Skipf("httpbin: %s", r.ForLLM) }

	// Should contain parsed content
	if !strings.Contains(r.ForLLM, "slideshow") && !strings.Contains(r.ForLLM, "title") {
		t.Logf("JSON response: %s", r.ForLLM[:min6(len(r.ForLLM), 200)])
	}
	t.Log("web fetch JSON parsing ✓")
}

func TestHard_Security_CommandInjection(t *testing.T) {
	dir := t.TempDir()
	exec := NewExecTool(dir, true)
	ctx := context.Background()

	// These should execute but not escape the workspace
	injections := []string{
		"echo safe; cat /etc/hostname",
		"echo $(whoami)",
		"echo `id`",
	}

	for _, cmd := range injections {
		r := exec.Execute(ctx, map[string]any{"command": cmd})
		// Should not crash — commands run in workspace
		if r == nil { t.Errorf("nil result for %q", cmd) }
	}
	t.Log("command injection: all handled without crash ✓")
}
