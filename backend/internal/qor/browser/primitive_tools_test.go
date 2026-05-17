// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package browser

import (
	"context"
	"strings"
	"testing"
)

// These tests cover the argument-parsing + error paths of every
// primitive tool without needing a real Chromium process. When a tool
// is called with malformed args we should get a clear error BEFORE
// the browser starts — that's the contract these tests enforce.
//
// For end-to-end (real Chromium) coverage we'd spin up headless
// Chrome, which is out of scope for unit tests. The `browser_test.go`
// file in this package already exercises that path.

// unstartedManager returns a Manager that has never called Start().
// Every tool guards with ensureStarted which will try to spawn Chrome;
// we test pre-start validation by passing malformed args so the
// "required arg missing" check fires first.
func unstartedManager() *Manager {
	return New(Config{})
}

// TestBrowserGotoTool_MissingURL: missing url must error BEFORE the
// ensureStarted call tries to launch Chrome. Otherwise a typo in the
// LLM's args would cost a 2-second Chrome boot.
func TestBrowserGotoTool_MissingURL(t *testing.T) {
	tool := NewBrowserGotoTool(unstartedManager())
	r := tool.Execute(context.Background(), map[string]any{})
	if !r.IsError {
		t.Fatal("missing url should error")
	}
	if !strings.Contains(r.ForLLM, "url is required") {
		t.Errorf("got %q", r.ForLLM)
	}
}

// TestBrowserClickTool_RequiresCoords: x and y both required.
func TestBrowserClickTool_RequiresCoords(t *testing.T) {
	tool := NewBrowserClickTool(unstartedManager())
	cases := []map[string]any{
		{},
		{"x": 10.0},
		{"y": 20.0},
	}
	for _, args := range cases {
		r := tool.Execute(context.Background(), args)
		if !r.IsError {
			t.Errorf("args %+v should error", args)
		}
	}
}

// TestBrowserTypeTool_RequiresText: empty text rejected up-front.
func TestBrowserTypeTool_RequiresText(t *testing.T) {
	tool := NewBrowserTypeTool(unstartedManager())
	r := tool.Execute(context.Background(), map[string]any{})
	if !r.IsError {
		t.Fatal("missing text should error")
	}
}

// TestBrowserPressTool_RequiresKey: empty key rejected.
func TestBrowserPressTool_RequiresKey(t *testing.T) {
	tool := NewBrowserPressTool(unstartedManager())
	r := tool.Execute(context.Background(), map[string]any{"key": ""})
	if !r.IsError {
		t.Fatal("empty key should error")
	}
}

// TestBrowserJSTool_RequiresExpression: whitespace-only expression
// rejected. Guards against " " being treated as valid JS.
func TestBrowserJSTool_RequiresExpression(t *testing.T) {
	tool := NewBrowserJSTool(unstartedManager())
	cases := []map[string]any{
		{},
		{"expression": ""},
		{"expression": "   "},
	}
	for _, args := range cases {
		r := tool.Execute(context.Background(), args)
		if !r.IsError {
			t.Errorf("args %+v should error", args)
		}
	}
}

// TestBrowserToolNames_AreStable: the LLM references tools by name
// via the registry. If a rename slips through, every existing agent
// config breaks silently.
func TestBrowserToolNames_AreStable(t *testing.T) {
	m := unstartedManager()
	want := map[string]string{
		"browser_goto":       NewBrowserGotoTool(m).Name(),
		"browser_info":       NewBrowserInfoTool(m).Name(),
		"browser_screenshot": NewBrowserScreenshotTool(m).Name(),
		"browser_click":      NewBrowserClickTool(m).Name(),
		"browser_type":       NewBrowserTypeTool(m).Name(),
		"browser_press":      NewBrowserPressTool(m).Name(),
		"browser_scroll":     NewBrowserScrollTool(m).Name(),
		"browser_js":         NewBrowserJSTool(m).Name(),
		"computer_use":       NewComputerUseTool(m).Name(),
	}
	for expected, got := range want {
		if got != expected {
			t.Errorf("tool name drift: got %q, want %q", got, expected)
		}
	}
}

// TestBrowserToolDescriptions_NonEmpty: empty descriptions make the
// LLM unable to choose the right tool. Regression guard.
func TestBrowserToolDescriptions_NonEmpty(t *testing.T) {
	m := unstartedManager()
	descs := map[string]string{
		"browser_goto":       NewBrowserGotoTool(m).Description(),
		"browser_info":       NewBrowserInfoTool(m).Description(),
		"browser_screenshot": NewBrowserScreenshotTool(m).Description(),
		"browser_click":      NewBrowserClickTool(m).Description(),
		"browser_type":       NewBrowserTypeTool(m).Description(),
		"browser_press":      NewBrowserPressTool(m).Description(),
		"browser_scroll":     NewBrowserScrollTool(m).Description(),
		"browser_js":         NewBrowserJSTool(m).Description(),
		"computer_use":       NewComputerUseTool(m).Description(),
	}
	for tool, d := range descs {
		if len(d) < 30 {
			t.Errorf("%s: description too short (%d chars) — agents rely on these", tool, len(d))
		}
	}
}

// TestBrowserToolParameters_WellFormed: every tool's Parameters()
// must be a valid JSON Schema object with a "properties" field.
// The registry passes this payload through to the LLM's function-
// calling protocol, so malformed schema = non-callable tool.
func TestBrowserToolParameters_WellFormed(t *testing.T) {
	m := unstartedManager()
	tools := []interface {
		Name() string
		Parameters() map[string]any
	}{
		NewBrowserGotoTool(m),
		NewBrowserInfoTool(m),
		NewBrowserScreenshotTool(m),
		NewBrowserClickTool(m),
		NewBrowserTypeTool(m),
		NewBrowserPressTool(m),
		NewBrowserScrollTool(m),
		NewBrowserJSTool(m),
		NewComputerUseTool(m),
	}
	for _, t2 := range tools {
		params := t2.Parameters()
		if params["type"] != "object" {
			t.Errorf("%s: type must be \"object\"", t2.Name())
		}
		if _, ok := params["properties"]; !ok {
			t.Errorf("%s: missing properties field", t2.Name())
		}
	}
}

// TestComputerUseTool_UnknownAction: unknown action names are rejected
// with a clear message citing the bad value.
func TestComputerUseTool_UnknownAction(t *testing.T) {
	tool := NewComputerUseTool(unstartedManager())
	r := tool.Execute(context.Background(), map[string]any{"action": "teleport"})
	if !r.IsError {
		t.Fatal("unknown action should error")
	}
	if !strings.Contains(r.ForLLM, "teleport") {
		t.Errorf("error should echo the bad action name; got %q", r.ForLLM)
	}
}

// TestComputerUseTool_MissingAction: action field is required.
func TestComputerUseTool_MissingAction(t *testing.T) {
	tool := NewComputerUseTool(unstartedManager())
	r := tool.Execute(context.Background(), map[string]any{})
	if !r.IsError {
		t.Fatal("missing action should error")
	}
}

// TestComputerUseTool_ActionRequiresArgs: each action's required
// sub-args are validated. click needs x/y, type needs text, press
// needs key, goto needs url.
func TestComputerUseTool_ActionRequiresArgs(t *testing.T) {
	tool := NewComputerUseTool(unstartedManager())
	cases := []struct {
		args map[string]any
		must string // substring that must appear in the error
	}{
		{map[string]any{"action": "click"}, "x and y"},
		{map[string]any{"action": "type"}, "text"},
		{map[string]any{"action": "press"}, "key"},
		{map[string]any{"action": "goto"}, "url"},
	}
	for _, c := range cases {
		r := tool.Execute(context.Background(), c.args)
		if !r.IsError {
			t.Errorf("action %q without required arg should error", c.args["action"])
			continue
		}
		if !strings.Contains(r.ForLLM, c.must) {
			t.Errorf("action %q error should mention %q; got %q",
				c.args["action"], c.must, r.ForLLM)
		}
	}
}
