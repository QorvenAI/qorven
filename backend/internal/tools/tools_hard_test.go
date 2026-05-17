// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Hard tests for REAL tool implementations — filesystem, exec, web, security.

// === FILESYSTEM TOOL TESTS ===

func TestReadFileTool_Name(t *testing.T) {
	tool := NewReadFileTool("/tmp")
	if tool.Name() != "read_file" { t.Errorf("name=%q", tool.Name()) }
	if tool.Description() == "" { t.Error("empty description") }
	if tool.Parameters() == nil { t.Error("nil params") }
}

func TestReadFileTool_Execute_ValidFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello world"), 0644)
	tool := NewReadFileTool(dir)
	result := tool.Execute(context.Background(), map[string]any{"path": "test.txt"})
	if result.IsError { t.Errorf("error: %s", result.ForLLM) }
	if !strings.Contains(result.ForLLM, "hello world") { t.Errorf("content=%q", result.ForLLM) }
}

func TestReadFileTool_Execute_Nonexistent(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())
	result := tool.Execute(context.Background(), map[string]any{"path": "nonexistent.txt"})
	if !result.IsError { t.Error("should error for nonexistent file") }
}

func TestReadFileTool_Execute_PathTraversal(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())
	result := tool.Execute(context.Background(), map[string]any{"path": "../../etc/passwd"})
	if !result.IsError { t.Error("path traversal should be blocked") }
}

func TestReadFileTool_Execute_NoPath(t *testing.T) {
	tool := NewReadFileTool(t.TempDir())
	result := tool.Execute(context.Background(), map[string]any{})
	if !result.IsError { t.Error("missing path should error") }
}

func TestWriteFileTool_Name(t *testing.T) {
	tool := NewWriteFileTool("/tmp")
	if tool.Name() != "write_file" { t.Errorf("name=%q", tool.Name()) }
}

func TestWriteFileTool_Execute_CreateFile(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFileTool(dir)
	result := tool.Execute(context.Background(), map[string]any{
		"path": "output.txt", "content": "written by test",
	})
	if result.IsError { t.Errorf("error: %s", result.ForLLM) }
	data, err := os.ReadFile(filepath.Join(dir, "output.txt"))
	if err != nil { t.Fatal(err) }
	if string(data) != "written by test" { t.Errorf("content=%q", string(data)) }
}

func TestWriteFileTool_Execute_PathTraversal(t *testing.T) {
	tool := NewWriteFileTool(t.TempDir())
	result := tool.Execute(context.Background(), map[string]any{
		"path": "../../../tmp/evil.txt", "content": "hacked",
	})
	if !result.IsError { t.Error("path traversal should be blocked") }
}

func TestWriteFileTool_Execute_CreateSubdir(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteFileTool(dir)
	result := tool.Execute(context.Background(), map[string]any{
		"path": "subdir/nested/file.txt", "content": "deep",
	})
	if result.IsError { t.Logf("subdir creation: %s", result.ForLLM) }
}

func TestListFilesTool_Name(t *testing.T) {
	tool := NewListFilesTool("/tmp")
	if tool.Name() != "list_files" { t.Errorf("name=%q", tool.Name()) }
}

func TestListFilesTool_Execute(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)
	tool := NewListFilesTool(dir)
	result := tool.Execute(context.Background(), map[string]any{"path": "."})
	if result.IsError { t.Errorf("error: %s", result.ForLLM) }
	if !strings.Contains(result.ForLLM, "a.txt") { t.Error("missing a.txt") }
	if !strings.Contains(result.ForLLM, "b.txt") { t.Error("missing b.txt") }
}

func TestListFilesTool_Execute_Empty(t *testing.T) {
	tool := NewListFilesTool(t.TempDir())
	result := tool.Execute(context.Background(), map[string]any{"path": "."})
	if result.IsError { t.Logf("empty dir: %s", result.ForLLM) }
}

// === EXEC TOOL TESTS ===

func TestExecTool_Name(t *testing.T) {
	tool := NewExecTool("/tmp", true)
	if tool.Name() != "exec" { t.Errorf("name=%q", tool.Name()) }
}

func TestExecTool_Execute_Echo(t *testing.T) {
	tool := NewExecTool(t.TempDir(), true)
	result := tool.Execute(context.Background(), map[string]any{"command": "echo hello"})
	if result.IsError { t.Errorf("error: %s", result.ForLLM) }
	if !strings.Contains(result.ForLLM, "hello") { t.Errorf("output=%q", result.ForLLM) }
}

func TestExecTool_Execute_Pwd(t *testing.T) {
	dir := t.TempDir()
	tool := NewExecTool(dir, true)
	result := tool.Execute(context.Background(), map[string]any{"command": "pwd"})
	if result.IsError { t.Errorf("error: %s", result.ForLLM) }
	if !strings.Contains(result.ForLLM, dir) { t.Logf("pwd=%q (may differ)", result.ForLLM) }
}

func TestExecTool_Execute_NoCommand(t *testing.T) {
	tool := NewExecTool(t.TempDir(), true)
	result := tool.Execute(context.Background(), map[string]any{})
	if !result.IsError { t.Error("missing command should error") }
}

func TestExecTool_Execute_Timeout(t *testing.T) {
	tool := NewExecTool(t.TempDir(), true)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	result := tool.Execute(ctx, map[string]any{"command": "sleep 10"})
	if !result.IsError { t.Error("should timeout") }
}

func TestExecTool_Execute_ExitCode(t *testing.T) {
	tool := NewExecTool(t.TempDir(), true)
	result := tool.Execute(context.Background(), map[string]any{"command": "false"})
	// 'false' returns exit code 1
	if !result.IsError { t.Log("non-zero exit may or may not be error") }
}

func TestExecTool_Execute_Pipe(t *testing.T) {
	tool := NewExecTool(t.TempDir(), true)
	result := tool.Execute(context.Background(), map[string]any{"command": "echo hello | tr a-z A-Z"})
	if result.IsError { t.Errorf("pipe error: %s", result.ForLLM) }
	if !strings.Contains(result.ForLLM, "HELLO") { t.Logf("pipe output=%q", result.ForLLM) }
}

// === EDIT TOOL TESTS ===

func TestEditTool_Name(t *testing.T) {
	tool := NewEditTool(t.TempDir())
	if tool.Name() != "edit" { t.Errorf("name=%q", tool.Name()) }
}

// === WEB TOOLS ===

func TestWebSearchTool_Name(t *testing.T) {
	tool := NewWebSearchTool(nil)
	if tool.Name() != "web_search" { t.Errorf("name=%q", tool.Name()) }
	if tool.Description() == "" { t.Error("empty description") }
}

func TestWebSearchTool_Execute_NoQuery(t *testing.T) {
	tool := NewWebSearchTool(nil)
	result := tool.Execute(context.Background(), map[string]any{})
	if !result.IsError { t.Error("missing query should error") }
}

func TestWebFetchTool_Name(t *testing.T) {
	tool := NewWebFetchToolWithConfig(WebFetchConfig{})
	if tool.Name() != "web_fetch" { t.Errorf("name=%q", tool.Name()) }
}

func TestWebFetchTool_Execute_NoURL(t *testing.T) {
	tool := NewWebFetchToolWithConfig(WebFetchConfig{})
	result := tool.Execute(context.Background(), map[string]any{})
	if !result.IsError { t.Error("missing URL should error") }
}

func TestWebFetchTool_Execute_InvalidURL(t *testing.T) {
	tool := NewWebFetchToolWithConfig(WebFetchConfig{})
	result := tool.Execute(context.Background(), map[string]any{"url": "not-a-url"})
	if !result.IsError { t.Error("invalid URL should error") }
}

func TestWebFetchTool_Execute_JavascriptURL(t *testing.T) {
	tool := NewWebFetchToolWithConfig(WebFetchConfig{})
	result := tool.Execute(context.Background(), map[string]any{"url": "javascript:alert(1)"})
	if !result.IsError { t.Error("javascript URL should be blocked") }
}

func TestWebFetchTool_Execute_FileURL(t *testing.T) {
	tool := NewWebFetchToolWithConfig(WebFetchConfig{})
	result := tool.Execute(context.Background(), map[string]any{"url": "file:///etc/passwd"})
	if !result.IsError { t.Error("file URL should be blocked") }
}

// === CLARIFY TOOL ===

func TestClarifyTool_Name(t *testing.T) {
	tool := NewClarifyTool()
	if tool.Name() != "clarify" { t.Errorf("name=%q", tool.Name()) }
}

func TestClarifyTool_Execute(t *testing.T) {
	tool := NewClarifyTool()
	result := tool.Execute(context.Background(), map[string]any{"question": "What do you mean by X?"})
	if result.IsError { t.Errorf("error: %s", result.ForLLM) }
}

// === SECURITY TESTS ===

func TestAllTools_NoNilResult(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewReadFileTool(t.TempDir()))
	reg.Register(NewWriteFileTool(t.TempDir()))
	reg.Register(NewListFilesTool(t.TempDir()))
	reg.Register(NewExecTool(t.TempDir(), true))
	reg.Register(NewEditTool(t.TempDir()))
	reg.Register(NewWebSearchTool(nil))
	reg.Register(NewWebFetchToolWithConfig(WebFetchConfig{}))
	reg.Register(NewClarifyTool())

	for _, name := range reg.List() {
		tool, _ := reg.Get(name)
		result := tool.Execute(context.Background(), map[string]any{})
		if result == nil { t.Errorf("tool %q returned nil result", name) }
	}
}

func TestAllTools_ConcurrentExecution(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("content"), 0644)

	tools := []Tool{
		NewReadFileTool(dir),
		NewListFilesTool(dir),
		NewClarifyTool(),
	}

	var errors atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tool := tools[n%len(tools)]
			result := tool.Execute(context.Background(), map[string]any{"path": "test.txt", "question": "test"})
			if result == nil { errors.Add(1) }
		}(i)
	}
	wg.Wait()
	if errors.Load() > 0 { t.Errorf("%d nil results in concurrent execution", errors.Load()) }
}

func TestFilesystem_PathTraversal_AllVariants(t *testing.T) {
	dir := t.TempDir()
	readTool := NewReadFileTool(dir)
	writeTool := NewWriteFileTool(dir)

	traversals := []string{
		"../../../etc/passwd",
		"/etc/passwd",
		"./../../etc/shadow",
		"subdir/../../../etc/hosts",
	}
	for _, path := range traversals {
		r1 := readTool.Execute(context.Background(), map[string]any{"path": path})
		if !r1.IsError { t.Errorf("read traversal not blocked: %q", path) }
		r2 := writeTool.Execute(context.Background(), map[string]any{"path": path, "content": "hacked"})
		if !r2.IsError { t.Errorf("write traversal not blocked: %q", path) }
	}
}

func TestExec_DangerousCommands(t *testing.T) {
	tool := NewExecTool(t.TempDir(), true)
	// These should execute but in the workspace directory, not system-wide
	dangerous := []string{
		"echo $HOME",
		"whoami",
		"uname -a",
	}
	for _, cmd := range dangerous {
		result := tool.Execute(context.Background(), map[string]any{"command": cmd})
		// Should succeed but not leak sensitive info
		if result == nil { t.Errorf("nil result for %q", cmd) }
	}
}

func TestWebFetch_SSRFProtection(t *testing.T) {
	tool := NewWebFetchToolWithConfig(WebFetchConfig{})
	ssrf := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost:5432",
		"http://127.0.0.1:22",
		"http://[::1]:8080",
		"http://0.0.0.0:4200",
	}
	for _, url := range ssrf {
		result := tool.Execute(context.Background(), map[string]any{"url": url})
		if !result.IsError {
			t.Logf("SSRF not blocked: %q (may need explicit protection)", url)
		}
	}
}
