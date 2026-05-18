//go:build unit

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package apps

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/tools"
)

// --- helpers -----------------------------------------------------------------

// toolRegistrar is the test-local interface (matches the interface we'll add to manager.go).
// fakeToolReg captures registered tools.
type fakeToolReg struct {
	tools map[string]tools.Tool
}

func (f *fakeToolReg) Register(t tools.Tool) {
	if f.tools == nil {
		f.tools = make(map[string]tools.Tool)
	}
	f.tools[t.Name()] = t
}

func (f *fakeToolReg) Unregister(name string) { delete(f.tools, name) }

// newTestManager returns a minimal manager with no pool.
func newTestManager(reg *fakeToolReg) *AppManager {
	return &AppManager{
		toolReg: reg,
		loaded:  make(map[string]*loadedApp),
	}
}

func testManifest(toolCmd string, timeout int) Manifest {
	return Manifest{
		Slug:        "test-app",
		Permissions: []string{"tool_register"},
		Tools: []ToolDef{{
			Name:    "my_tool",
			Command: toolCmd,
			Timeout: timeout,
		}},
	}
}

func testApp() App {
	return App{ID: "app-1", TenantID: "tenant-1", Slug: "test-app", InstallPath: "/tmp"}
}

// --- tests -------------------------------------------------------------------

func TestRegisterTools_StructuredOutput_Text(t *testing.T) {
	reg := &fakeToolReg{}
	m := newTestManager(reg)
	m.registerTools(testApp(), testManifest("echo hello", 0))

	tool := reg.tools["my_tool"]
	if tool == nil {
		t.Fatal("tool not registered")
	}
	result := tool.Execute(context.Background(), nil)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "hello") {
		t.Errorf("expected 'hello' in ForLLM, got: %q", result.ForLLM)
	}
}

func TestRegisterTools_StructuredOutput_JSON(t *testing.T) {
	scriptPath := t.TempDir() + "/tool.sh"
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '#!qorven:json\n{"text":"llm says hi","user":"user sees hi"}'
`), 0755)

	reg := &fakeToolReg{}
	m := newTestManager(reg)
	m.registerTools(testApp(), testManifest(scriptPath, 0))

	tool := reg.tools["my_tool"]
	result := tool.Execute(context.Background(), nil)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ForLLM)
	}
	if result.ForLLM != "llm says hi" {
		t.Errorf("ForLLM=%q, want 'llm says hi'", result.ForLLM)
	}
	if result.ForUser != "user sees hi" {
		t.Errorf("ForUser=%q, want 'user sees hi'", result.ForUser)
	}
}

func TestRegisterTools_StructuredOutput_JSON_Widget(t *testing.T) {
	scriptPath := t.TempDir() + "/tool.sh"
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '#!qorven:json\n{"text":"result","widget":{"type":"table","data":{"rows":[]}}}'
`), 0755)

	reg := &fakeToolReg{}
	m := newTestManager(reg)
	m.registerTools(testApp(), testManifest(scriptPath, 0))

	tool := reg.tools["my_tool"]
	result := tool.Execute(context.Background(), nil)
	if result.Widget == nil {
		t.Fatal("expected Widget to be set")
	}
	if result.Widget.Type != "table" {
		t.Errorf("Widget.Type=%q, want 'table'", result.Widget.Type)
	}
}

func TestRegisterTools_StructuredOutput_JSON_Invalid_FallsBack(t *testing.T) {
	scriptPath := t.TempDir() + "/tool.sh"
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '#!qorven:json\nnot valid json'
`), 0755)

	reg := &fakeToolReg{}
	m := newTestManager(reg)
	m.registerTools(testApp(), testManifest(scriptPath, 0))

	tool := reg.tools["my_tool"]
	result := tool.Execute(context.Background(), nil)
	if result.IsError {
		t.Errorf("expected fallback to TextResult (not error) on malformed JSON")
	}
}

func TestRegisterTools_Timeout_Enforced(t *testing.T) {
	scriptPath := t.TempDir() + "/slow.sh"
	os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 5\necho done"), 0755)

	reg := &fakeToolReg{}
	m := newTestManager(reg)
	m.registerTools(testApp(), testManifest(scriptPath, 1)) // 1 second timeout

	start := time.Now()
	tool := reg.tools["my_tool"]
	result := tool.Execute(context.Background(), nil)
	elapsed := time.Since(start)

	if elapsed >= 4*time.Second {
		t.Errorf("timeout not enforced: elapsed %v", elapsed)
	}
	if !result.IsError {
		t.Error("expected IsError=true on timeout")
	}
}

func TestRegisterTools_DSN_Injected(t *testing.T) {
	scriptPath := t.TempDir() + "/env.sh"
	os.WriteFile(scriptPath, []byte("#!/bin/sh\necho \"DSN=$QORVEN_DB_DSN\""), 0755)

	reg := &fakeToolReg{}
	m := &AppManager{
		toolReg: reg,
		loaded:  make(map[string]*loadedApp),
		dsn:     "postgres://user:pass@localhost/db",
	}
	m.registerTools(testApp(), testManifest(scriptPath, 0))

	tool := reg.tools["my_tool"]
	result := tool.Execute(context.Background(), nil)
	if !strings.Contains(result.ForLLM, "postgres://user:pass@localhost/db") {
		t.Errorf("DSN not injected, got: %q", result.ForLLM)
	}
}

// --- RunTool unit tests ---

func TestRunTool_NotFound_App(t *testing.T) {
	reg := &fakeToolReg{}
	m := newTestManager(reg)
	// No apps loaded — any slug should return ErrAppNotLoaded
	_, err := m.RunTool(context.Background(), "nonexistent-app", "any_tool", nil)
	if err == nil {
		t.Fatal("expected error for unknown app slug")
	}
	if !errors.Is(err, ErrAppNotLoaded) {
		t.Errorf("expected ErrAppNotLoaded, got: %v", err)
	}
}

func TestRunTool_NotFound_Tool(t *testing.T) {
	reg := &fakeToolReg{}
	m := &AppManager{
		toolReg: reg,
		loaded:  make(map[string]*loadedApp),
	}
	// Manually insert a loaded app with no tools
	m.loaded["my-app"] = &loadedApp{
		app: App{ID: "a1", TenantID: "t1", Slug: "my-app", Enabled: true, InstallPath: "/tmp"},
		manifest: Manifest{
			Slug:        "my-app",
			Permissions: []string{"tool_register"},
			Tools:       []ToolDef{},
		},
	}
	_, err := m.RunTool(context.Background(), "my-app", "missing_tool", nil)
	if err == nil {
		t.Fatal("expected error for missing tool")
	}
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got: %v", err)
	}
}

func TestRunTool_StructuredOutput(t *testing.T) {
	scriptPath := t.TempDir() + "/tool.sh"
	os.WriteFile(scriptPath, []byte(`#!/bin/sh
printf '#!qorven:json\n{"text":"from RunTool","user":"user text"}'
`), 0755)

	reg := &fakeToolReg{}
	m := &AppManager{
		toolReg: reg,
		loaded:  make(map[string]*loadedApp),
	}
	m.loaded["my-app"] = &loadedApp{
		app: App{ID: "a1", TenantID: "t1", Slug: "my-app", Enabled: true, InstallPath: "/tmp"},
		manifest: Manifest{
			Slug:        "my-app",
			Permissions: []string{"tool_register"},
			Tools: []ToolDef{{
				Name:    "my_tool",
				Command: scriptPath,
			}},
		},
	}

	result, err := m.RunTool(context.Background(), "my-app", "my_tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ForLLM != "from RunTool" {
		t.Errorf("ForLLM=%q, want 'from RunTool'", result.ForLLM)
	}
	if result.ForUser != "user text" {
		t.Errorf("ForUser=%q, want 'user text'", result.ForUser)
	}
}

func TestRunTool_Timeout(t *testing.T) {
	scriptPath := t.TempDir() + "/slow.sh"
	os.WriteFile(scriptPath, []byte("#!/bin/sh\nsleep 5"), 0755)

	reg := &fakeToolReg{}
	m := &AppManager{
		toolReg: reg,
		loaded:  make(map[string]*loadedApp),
	}
	m.loaded["my-app"] = &loadedApp{
		app: App{ID: "a1", TenantID: "t1", Slug: "my-app", Enabled: true, InstallPath: "/tmp"},
		manifest: Manifest{
			Slug:        "my-app",
			Permissions: []string{"tool_register"},
			Tools: []ToolDef{{
				Name:    "slow_tool",
				Command: scriptPath,
				Timeout: 1, // 1 second
			}},
		},
	}

	start := time.Now()
	result, err := m.RunTool(context.Background(), "my-app", "slow_tool", nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if elapsed >= 4*time.Second {
		t.Errorf("timeout not enforced: elapsed %v", elapsed)
	}
	if !result.IsError {
		t.Error("expected IsError=true on timeout")
	}
}
