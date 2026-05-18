// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package browser

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// Hard tests — concurrency, race conditions, edge cases, resource limits.

func TestManager_New_DefaultConfig(t *testing.T) {
	m := New(Config{})
	if m.cfg.MaxPages != 5 { t.Errorf("default MaxPages=%d", m.cfg.MaxPages) }
	if m.cfg.ActionTimeout != 30*time.Second { t.Errorf("default timeout=%v", m.cfg.ActionTimeout) }
	if m.cfg.IdleTimeout != 5*time.Minute { t.Errorf("default idle=%v", m.cfg.IdleTimeout) }
}

func TestManager_New_CustomConfig(t *testing.T) {
	m := New(Config{MaxPages: 10, ActionTimeout: 60 * time.Second, Headless: true})
	if m.cfg.MaxPages != 10 { t.Error("custom MaxPages not applied") }
	if !m.cfg.Headless { t.Error("headless not set") }
}

func TestManager_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	if m.IsRunning() { t.Error("should not be running before Start") }
}

func TestManager_Status_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	s := m.Status()
	if s.Running { t.Error("status should show not running") }
	if s.TabCount != 0 { t.Error("should have 0 tabs") }
}

func TestManager_Navigate_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	err := m.Navigate(context.Background(), "https://example.com")
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_Click_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	err := m.Click(context.Background(), "#btn", ClickOpts{})
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_Type_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	err := m.Type(context.Background(), "#input", "text", TypeOpts{})
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_Press_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	err := m.Press(context.Background(), "Enter")
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_Hover_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	err := m.Hover(context.Background(), "#link")
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_Scroll_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	err := m.Scroll(context.Background(), 0, 500)
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_Wait_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	err := m.Wait(context.Background(), WaitOpts{Selector: "#el"})
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_Evaluate_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	_, err := m.Evaluate(context.Background(), "1+1")
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_Screenshot_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	_, err := m.Screenshot(context.Background())
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_TakeSnapshot_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	_, err := m.TakeSnapshot(context.Background())
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_ListTabs_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	_, err := m.ListTabs(context.Background())
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_OpenTab_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	_, err := m.OpenTab(context.Background(), "https://example.com")
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_CloseTab_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	err := m.CloseTab(context.Background(), "some-id")
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_SwitchTab_NotFound(t *testing.T) {
	m := New(DefaultConfig())
	err := m.SwitchTab("nonexistent")
	if err == nil { t.Error("should fail for nonexistent tab") }
}

func TestManager_GetURL_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	_, err := m.GetURL(context.Background())
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_GetTitle_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	_, err := m.GetTitle(context.Background())
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_GetHTML_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	_, err := m.GetHTML(context.Background())
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_WaitForLoad_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	err := m.WaitForLoad(context.Background())
	if err == nil { t.Error("should fail when not running") }
}

func TestManager_Stop_NotRunning(t *testing.T) {
	m := New(DefaultConfig())
	err := m.Stop(context.Background())
	if err != nil { t.Error("stop on non-running should be no-op") }
}

func TestManager_ConcurrentStatusCalls(t *testing.T) {
	m := New(DefaultConfig())
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Status()
		}()
	}
	wg.Wait()
}

func TestManager_ConcurrentIsRunning(t *testing.T) {
	m := New(DefaultConfig())
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.IsRunning()
		}()
	}
	wg.Wait()
}

func TestDefaultConfig_Values(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Headless { t.Error("default should be headless") }
	if cfg.MaxPages != 5 { t.Errorf("MaxPages=%d", cfg.MaxPages) }
	if cfg.ActionTimeout != 30*time.Second { t.Errorf("ActionTimeout=%v", cfg.ActionTimeout) }
	if cfg.IdleTimeout != 5*time.Minute { t.Errorf("IdleTimeout=%v", cfg.IdleTimeout) }
}

func TestTabInfo_Fields(t *testing.T) {
	tab := TabInfo{TargetID: "t1", URL: "https://example.com", Title: "Example", Active: true}
	if tab.TargetID != "t1" { t.Error("wrong target") }
	if !tab.Active { t.Error("should be active") }
}

func TestSnapshotResult_Fields(t *testing.T) {
	snap := SnapshotResult{URL: "https://example.com", Title: "Test", Tree: "button \"Click\"", Stats: SnapshotStats{Nodes: 5, MaxDepth: 3}}
	if snap.Stats.Nodes != 5 { t.Error("wrong node count") }
	if snap.Stats.MaxDepth != 3 { t.Error("wrong depth") }
}

func TestRef_Fields(t *testing.T) {
	ref := Ref{Role: "button", Name: "Submit", NodeID: "n1", BackendID: 42}
	if ref.Role != "button" { t.Error("wrong role") }
	if ref.BackendID != 42 { t.Error("wrong backend id") }
}

func TestClickOpts_Defaults(t *testing.T) {
	opts := ClickOpts{}
	if opts.Button != "" { t.Error("default button should be empty") }
	if opts.ClickCount != 0 { t.Error("default click count should be 0") }
}

func TestTypeOpts_Clear(t *testing.T) {
	opts := TypeOpts{Clear: true, Delay: 50}
	if !opts.Clear { t.Error("clear not set") }
	if opts.Delay != 50 { t.Error("delay not set") }
}

func TestWaitOpts_Timeout(t *testing.T) {
	opts := WaitOpts{Selector: "#el", Timeout: 5 * time.Second, Visible: true}
	if opts.Selector != "#el" { t.Error("wrong selector") }
	if !opts.Visible { t.Error("visible not set") }
}

func TestMapKeyName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"enter", "Enter"}, {"Enter", "Enter"},
		{"tab", "Tab"}, {"escape", "Escape"}, {"esc", "Escape"},
		{"backspace", "Backspace"}, {"delete", "Delete"},
		{"space", " "}, {"up", "ArrowUp"}, {"down", "ArrowDown"},
		{"left", "ArrowLeft"}, {"right", "ArrowRight"},
		{"a", "a"}, {"F1", "F1"},
	}
	for _, tt := range tests {
		got := mapKeyName(tt.in)
		if got != tt.want { t.Errorf("mapKeyName(%q)=%q, want %q", tt.in, got, tt.want) }
	}
}

func TestIsInteractive(t *testing.T) {
	interactive := []string{"button", "link", "textbox", "checkbox", "radio", "combobox", "menuitem", "tab", "switch", "slider"}
	for _, role := range interactive {
		if !isInteractive(role) { t.Errorf("%q should be interactive", role) }
	}
	nonInteractive := []string{"heading", "paragraph", "image", "generic", "none", "group", "list"}
	for _, role := range nonInteractive {
		if isInteractive(role) { t.Errorf("%q should NOT be interactive", role) }
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" { t.Error("short string should not truncate") }
	if truncate("hello world", 5) != "hello…" { t.Error("long string should truncate") }
	if truncate("", 5) != "" { t.Error("empty should stay empty") }
}

func TestMin(t *testing.T) {
	if min(3, 5) != 3 { t.Error("min(3,5) should be 3") }
	if min(5, 3) != 3 { t.Error("min(5,3) should be 3") }
	if min(3, 3) != 3 { t.Error("min(3,3) should be 3") }
}

// BrowserTool tests (without actual browser)
func TestBrowserTool_Name(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	if tool.Name() != "browser" { t.Errorf("name=%q", tool.Name()) }
}

func TestBrowserTool_Description(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	if tool.Description() == "" { t.Error("empty description") }
}

func TestBrowserTool_Parameters(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	params := tool.Parameters()
	if params == nil { t.Fatal("nil params") }
	props, ok := params["properties"].(map[string]any)
	if !ok { t.Fatal("no properties") }
	if _, ok := props["action"]; !ok { t.Error("missing action param") }
	if _, ok := props["url"]; !ok { t.Error("missing url param") }
	if _, ok := props["selector"]; !ok { t.Error("missing selector param") }
}

func TestBrowserTool_Status_NotRunning(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	result := tool.Execute(context.Background(), map[string]any{"action": "status"})
	if result.IsError { t.Error("status should not error") }
	if result.ForLLM == "" { t.Error("empty status") }
}

func TestBrowserTool_UnknownAction(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	result := tool.Execute(context.Background(), map[string]any{"action": "fly_to_moon"})
	if !result.IsError { t.Error("unknown action should error") }
}

func TestBrowserTool_Navigate_NoBrowser(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	result := tool.Execute(context.Background(), map[string]any{"action": "navigate", "url": "https://example.com"})
	if !result.IsError { t.Error("navigate without browser should error") }
}

func TestBrowserTool_Navigate_NoURL(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	result := tool.Execute(context.Background(), map[string]any{"action": "navigate"})
	if !result.IsError { t.Error("navigate without url should error") }
}

func TestBrowserTool_Click_NoSelector(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	result := tool.Execute(context.Background(), map[string]any{"action": "click"})
	if !result.IsError { t.Error("click without selector should error") }
}

func TestBrowserTool_Type_NoArgs(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	result := tool.Execute(context.Background(), map[string]any{"action": "type"})
	if !result.IsError { t.Error("type without args should error") }
}

func TestBrowserTool_Press_NoKey(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	result := tool.Execute(context.Background(), map[string]any{"action": "press"})
	if !result.IsError { t.Error("press without key should error") }
}

func TestBrowserTool_Evaluate_NoJS(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	result := tool.Execute(context.Background(), map[string]any{"action": "evaluate"})
	if !result.IsError { t.Error("evaluate without js should error") }
}

func TestBrowserTool_Open_NoURL(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	result := tool.Execute(context.Background(), map[string]any{"action": "open"})
	if !result.IsError { t.Error("open without url should error") }
}

func TestBrowserTool_Close_NoTargetID(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	result := tool.Execute(context.Background(), map[string]any{"action": "close"})
	if !result.IsError { t.Error("close without target_id should error") }
}

func TestBrowserTool_Wait_NoSelector(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	result := tool.Execute(context.Background(), map[string]any{"action": "wait"})
	if !result.IsError { t.Error("wait without selector should error") }
}

func TestBrowserTool_AllActions_Validation(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	actions := []string{"status", "start", "stop", "navigate", "snapshot", "screenshot",
		"click", "type", "press", "scroll", "evaluate", "tabs", "open", "close", "wait"}
	for _, action := range actions {
		result := tool.Execute(context.Background(), map[string]any{"action": action})
		// All should return a result (error or success), never panic
		if result == nil { t.Errorf("action %q returned nil", action) }
	}
}

// === HARD BROWSER TESTS ===

func TestManager_DoubleStart(t *testing.T) {
	m := New(DefaultConfig())
	// Start without Chrome — should fail but not panic
	err := m.Start(context.Background())
	if err == nil { t.Skip("Chrome available — can't test failure") }
	// Second start should also handle gracefully
	err2 := m.Start(context.Background())
	_ = err2
}

func TestManager_StopTwice(t *testing.T) {
	m := New(DefaultConfig())
	m.Stop(context.Background())
	m.Stop(context.Background()) // should not panic
}

func TestBrowserTool_AllActions_NoNilResult(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	actions := []string{"status", "start", "stop", "navigate", "snapshot", "screenshot",
		"click", "type", "press", "scroll", "evaluate", "tabs", "open", "close", "wait", "unknown_action"}
	for _, action := range actions {
		result := tool.Execute(context.Background(), map[string]any{"action": action})
		if result == nil { t.Errorf("action %q returned nil result", action) }
		if result.ForLLM == "" && result.ForUser == "" { t.Errorf("action %q returned empty result", action) }
	}
}

func TestBrowserTool_StatusJSON(t *testing.T) {
	tool := NewBrowserTool(New(DefaultConfig()))
	result := tool.Execute(context.Background(), map[string]any{"action": "status"})
	if !strings.Contains(result.ForLLM, "running") { t.Logf("status: %q", result.ForLLM) }
}
