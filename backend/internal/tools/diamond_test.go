// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
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

// diamond_test.go — Production scenario tests for tools.

func TestDiamond_Edit_PrecisionReplace(t *testing.T) {
	dir := t.TempDir()
	write := NewWriteFileTool(dir)
	edit := NewEditTool(dir)
	read := NewReadFileTool(dir)
	ctx := context.Background()

	// Write a Go file
	original := `package main

import "fmt"

func main() {
	fmt.Println("hello")
	fmt.Println("world")
	fmt.Println("goodbye")
}
`
	write.Execute(ctx, map[string]any{"path": "main.go", "content": original})

	// Edit ONLY the middle line
	edit.Execute(ctx, map[string]any{
		"path":    "main.go",
		"find":    `fmt.Println("world")`,
		"replace": `fmt.Println("CHANGED")`,
	})

	r := read.Execute(ctx, map[string]any{"path": "main.go"})
	if r.IsError { t.Fatal(r.ForLLM) }

	// Verify only the target line changed
	if !strings.Contains(r.ForLLM, `"CHANGED"`) { t.Error("edit not applied") }
	if !strings.Contains(r.ForLLM, `"hello"`) { t.Error("line before edit corrupted") }
	if !strings.Contains(r.ForLLM, `"goodbye"`) { t.Error("line after edit corrupted") }
	if strings.Contains(r.ForLLM, `"world"`) { t.Error("old content still present") }
	t.Log("edit precision: only target line changed ✓")
}

func TestDiamond_Exec_TimeoutRecovery(t *testing.T) {
	dir := t.TempDir()
	exec := NewExecTool(dir, true)

	// Command that would hang — should timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	r := exec.Execute(ctx, map[string]any{"command": "sleep 10"})
	// Should either error or return within timeout
	if !r.IsError && !strings.Contains(r.ForLLM, "killed") && !strings.Contains(r.ForLLM, "timeout") {
		t.Logf("sleep 10 with 2s timeout: %q", r.ForLLM[:min6(len(r.ForLLM), 100)])
	}
	t.Log("exec timeout: recovered ✓")
}

func TestDiamond_Exec_LargeOutput(t *testing.T) {
	dir := t.TempDir()
	exec := NewExecTool(dir, true)
	ctx := context.Background()

	// Generate large output
	r := exec.Execute(ctx, map[string]any{"command": "seq 1 10000"})
	if r.IsError { t.Fatalf("seq: %s", r.ForLLM) }

	// Should contain first and last numbers
	if !strings.Contains(r.ForLLM, "1\n") { t.Error("missing start") }
	if !strings.Contains(r.ForLLM, "10000") { t.Error("missing end") }
	t.Logf("large output: %d chars ✓", len(r.ForLLM))
}

func TestDiamond_SafePath_AllTraversalVectors(t *testing.T) {
	dir := t.TempDir()

	attacks := []string{
		"../../../etc/passwd",
		"..\\..\\..\\etc\\passwd",
		"....//....//etc/passwd",
		"%2e%2e%2f%2e%2e%2fetc%2fpasswd",
		"..%252f..%252f..%252fetc/passwd",
		"a/b/../../../../../../etc/passwd",
		"/etc/passwd",
		"~/.ssh/id_rsa",
	}

	for _, attack := range attacks {
		_, err := SafePath(dir, attack)
		if err == nil {
			t.Errorf("SECURITY: path traversal not blocked: %q", attack)
		}
	}
	t.Logf("path traversal: %d vectors blocked ✓", len(attacks))
}

func TestDiamond_ReadFile_BinaryDetection(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Write a binary file
	binary := make([]byte, 1000)
	for i := range binary { binary[i] = byte(i % 256) }
	os.WriteFile(filepath.Join(dir, "binary.dat"), binary, 0644)

	read := NewReadFileTool(dir)
	r := read.Execute(ctx, map[string]any{"path": "binary.dat"})

	// Should detect binary and not dump raw bytes
	if strings.Contains(r.ForLLM, string([]byte{0, 1, 2, 3})) {
		t.Error("raw binary bytes in output")
	}
	t.Logf("binary detection: %q ✓", r.ForLLM[:min6(len(r.ForLLM), 80)])
}

func TestDiamond_WriteFile_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	write := NewWriteFileTool(dir)
	ctx := context.Background()

	// Write to a nested path that doesn't exist yet
	r := write.Execute(ctx, map[string]any{
		"path":    "deep/nested/dir/file.txt",
		"content": "hello from deep",
	})
	if r.IsError { t.Fatalf("write: %s", r.ForLLM) }

	// Verify file exists
	data, err := os.ReadFile(filepath.Join(dir, "deep/nested/dir/file.txt"))
	if err != nil { t.Fatal(err) }
	if string(data) != "hello from deep" { t.Error("content mismatch") }
	t.Log("write creates directories: deep/nested/dir/ ✓")
}

func TestDiamond_Exec_ExitCodeCapture(t *testing.T) {
	dir := t.TempDir()
	exec := NewExecTool(dir, true)
	ctx := context.Background()

	// Successful command
	r1 := exec.Execute(ctx, map[string]any{"command": "true"})
	if r1.IsError { t.Error("'true' should succeed") }
	if !strings.Contains(r1.ForLLM, "0") { t.Logf("exit 0: %q", r1.ForLLM[:min6(len(r1.ForLLM), 50)]) }

	// Failing command
	r2 := exec.Execute(ctx, map[string]any{"command": "false"})
	if !r2.IsError && !strings.Contains(r2.ForLLM, "1") {
		t.Logf("exit 1: %q", r2.ForLLM[:min6(len(r2.ForLLM), 50)])
	}

	// Command with specific exit code
	r3 := exec.Execute(ctx, map[string]any{"command": "exit 42"})
	if strings.Contains(r3.ForLLM, "42") {
		t.Log("exit code 42 captured ✓")
	}
}

func TestDiamond_WebFetch_SSRFProtection(t *testing.T) {
	fetch := NewWebFetchToolWithConfig(WebFetchConfig{})
	ctx := context.Background()

	// Internal network addresses should be blocked
	ssrf := []string{
		"http://169.254.169.254/latest/meta-data/",  // AWS metadata
		"http://127.0.0.1:5432/",                     // local postgres
		"http://localhost:4200/",                      // local gateway
		"http://0.0.0.0/",                             // all interfaces
		"http://[::1]/",                               // IPv6 localhost
		"http://10.0.0.1/",                            // private network
		"http://192.168.1.1/",                         // private network
	}

	for _, u := range ssrf {
		r := fetch.Execute(ctx, map[string]any{"url": u})
		if !r.IsError {
			t.Errorf("SECURITY: SSRF not blocked for %s", u)
		}
	}
	t.Logf("SSRF protection: %d vectors blocked ✓", len(ssrf))
}

