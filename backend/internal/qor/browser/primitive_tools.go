// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/tools"
)

// Harness-style browser primitives — a tight action vocabulary that
// mirrors how browser-use/browser-harness exposes the DOM: coordinate-
// based input, screenshots as the primary state representation, and
// optional direct JS evaluation for structured reads. No selector
// engines, no fancy element finders — the LLM does the seeing.
//
// Rationale for coordinate-first:
//   1. Iframes and shadow DOM: Input.dispatchMouseEvent at (x,y)
//      works across every boundary. CSS selectors do not.
//   2. Vision-capable LLMs read screenshots more reliably than they
//      serialize accessibility trees. Half the token budget, better
//      grounding on canvas-heavy apps.
//   3. Smaller, stable action vocabulary = fewer hallucinated verbs.
//
// Tools registered (8):
//   browser_goto       — navigate to URL
//   browser_info       — viewport + scroll + page size + title
//   browser_screenshot — PNG of viewport or full page (base64)
//   browser_click      — click at (x, y)
//   browser_type       — type text into the currently focused element
//   browser_press      — press a single key (Enter, Tab, Escape, …)
//   browser_scroll     — wheel at (x, y) with dx/dy delta
//   browser_js         — evaluate JS expression, return JSON result
//
// Every tool re-uses the shared *Manager. The manager handles
// start/stop + tab lifecycle. If the manager isn't running, we
// auto-start on first use so a single browser_click gets the agent
// going without a setup round-trip.

// --- shared manager plumbing ---

// browserMgrTool is the base for every primitive — embeds the
// manager so child tools reuse the same browser lifecycle.
type browserMgrTool struct {
	mgr *Manager
}

// ensureStarted brings the browser up if it's not already. Idempotent;
// safe to call from multiple goroutines (Manager.Start is locked).
func (t *browserMgrTool) ensureStarted(ctx context.Context) error {
	if t.mgr.IsRunning() {
		return nil
	}
	return t.mgr.Start(ctx)
}

// --- browser_goto ---

type BrowserGotoTool struct{ browserMgrTool }

func NewBrowserGotoTool(m *Manager) *BrowserGotoTool {
	return &BrowserGotoTool{browserMgrTool{mgr: m}}
}

func (t *BrowserGotoTool) Name() string { return "browser_goto" }
func (t *BrowserGotoTool) Description() string {
	return "Navigate the browser to a URL. Starts the browser automatically if not already running. " +
		"Waits for the page to reach readyState=complete before returning."
}
func (t *BrowserGotoTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{"type": "string", "description": "URL to open — https:// or http://"},
			"wait_timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Max seconds to wait for page ready. Default 15.",
			},
		},
		"required": []string{"url"},
	}
}
func (t *BrowserGotoTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	url, _ := args["url"].(string)
	if url == "" {
		return tools.ErrorResult("url is required")
	}
	if err := t.ensureStarted(ctx); err != nil {
		return tools.ErrorResult(fmt.Sprintf("start browser: %v", err))
	}
	if err := t.mgr.Navigate(ctx, url); err != nil {
		return tools.ErrorResult(fmt.Sprintf("navigate: %v", err))
	}
	timeout := 15 * time.Second
	if s, ok := tools.ToInt(args["wait_timeout_seconds"]); ok && s > 0 {
		timeout = time.Duration(s) * time.Second
	}
	if err := t.mgr.WaitForReadyState(ctx, timeout); err != nil {
		// Don't hard-error — the page may be usable even if something's
		// still loading. Surface the condition as a warning string so
		// the agent can decide to retry or proceed.
		return tools.TextResult(fmt.Sprintf("navigated to %s — warning: %v", url, err))
	}
	return tools.TextResult("navigated to " + url)
}

// --- browser_info ---

type BrowserInfoTool struct{ browserMgrTool }

func NewBrowserInfoTool(m *Manager) *BrowserInfoTool {
	return &BrowserInfoTool{browserMgrTool{mgr: m}}
}

func (t *BrowserInfoTool) Name() string { return "browser_info" }
func (t *BrowserInfoTool) Description() string {
	return "Return viewport dimensions, scroll position, full page size, current URL, and title. " +
		"Use this to plan scroll operations without taking an extra screenshot."
}
func (t *BrowserInfoTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *BrowserInfoTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	if err := t.ensureStarted(ctx); err != nil {
		return tools.ErrorResult(fmt.Sprintf("start browser: %v", err))
	}
	info, err := t.mgr.PageInfo(ctx)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("page_info: %v", err))
	}
	buf, _ := json.MarshalIndent(info, "", "  ")
	return tools.TextResult(string(buf))
}

// --- browser_screenshot ---

type BrowserScreenshotTool struct{ browserMgrTool }

func NewBrowserScreenshotTool(m *Manager) *BrowserScreenshotTool {
	return &BrowserScreenshotTool{browserMgrTool{mgr: m}}
}

func (t *BrowserScreenshotTool) Name() string { return "browser_screenshot" }
func (t *BrowserScreenshotTool) Description() string {
	return "Capture a PNG screenshot of the current tab. Returns a base64-encoded image " +
		"the LLM can see if it's vision-capable. Set `full=true` to capture beyond the " +
		"viewport (useful for short pages). Dimensions are also returned so the LLM knows " +
		"the coordinate space for subsequent clicks."
}
func (t *BrowserScreenshotTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"full": map[string]any{
				"type":        "boolean",
				"description": "true = full-page screenshot; false (default) = viewport only.",
			},
		},
	}
}
func (t *BrowserScreenshotTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	if err := t.ensureStarted(ctx); err != nil {
		return tools.ErrorResult(fmt.Sprintf("start browser: %v", err))
	}
	full, _ := args["full"].(bool)
	var png []byte
	var err error
	if full {
		png, err = t.mgr.FullPageScreenshot(ctx)
	} else {
		png, err = t.mgr.Screenshot(ctx)
	}
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("screenshot: %v", err))
	}
	info, _ := t.mgr.PageInfo(ctx)
	vpW, vpH := 0.0, 0.0
	if vp, ok := info["viewport"].(map[string]any); ok {
		vpW, _ = vp["width"].(float64)
		vpH, _ = vp["height"].(float64)
	}
	// Return base64 so the agent loop can forward to a vision LLM
	// as an image attachment. Media path is also populated so the
	// frontend can display the shot inline.
	b64 := base64.StdEncoding.EncodeToString(png)
	scope := "viewport"
	if full {
		scope = "full page"
	}
	return &tools.Result{
		ForLLM: fmt.Sprintf("Screenshot (%s, %.0fx%.0f):\ndata:image/png;base64,%s",
			scope, vpW, vpH, b64),
		ForUser: fmt.Sprintf("Screenshot captured (%s, %.0fx%.0f, %d bytes)",
			scope, vpW, vpH, len(png)),
	}
}

// --- browser_click ---

type BrowserClickTool struct{ browserMgrTool }

func NewBrowserClickTool(m *Manager) *BrowserClickTool {
	return &BrowserClickTool{browserMgrTool{mgr: m}}
}

func (t *BrowserClickTool) Name() string { return "browser_click" }
func (t *BrowserClickTool) Description() string {
	return "Click at exact viewport pixel coordinates. (0, 0) is top-left. Use browser_screenshot " +
		"to see the page, then pass x and y from what you observed. Works through iframes, shadow " +
		"DOM, and cross-origin embeds — no selectors needed. `clicks=2` for double-click."
}
func (t *BrowserClickTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x":      map[string]any{"type": "number", "description": "Viewport X pixel (0 = left)."},
			"y":      map[string]any{"type": "number", "description": "Viewport Y pixel (0 = top)."},
			"button": map[string]any{"type": "string", "description": "\"left\" (default), \"right\", or \"middle\"."},
			"clicks": map[string]any{"type": "integer", "description": "Click count. Default 1; 2 for double-click."},
		},
		"required": []string{"x", "y"},
	}
}
func (t *BrowserClickTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	x, xOK := tools.ToFloat(args["x"])
	y, yOK := tools.ToFloat(args["y"])
	if !xOK || !yOK {
		return tools.ErrorResult("x and y are required (numbers)")
	}
	button, _ := args["button"].(string)
	clicks, _ := tools.ToInt(args["clicks"])
	if clicks <= 0 {
		clicks = 1
	}
	if err := t.ensureStarted(ctx); err != nil {
		return tools.ErrorResult(fmt.Sprintf("start browser: %v", err))
	}
	if err := t.mgr.MouseClick(ctx, x, y, button, clicks); err != nil {
		return tools.ErrorResult(fmt.Sprintf("click (%.0f, %.0f): %v", x, y, err))
	}
	return tools.TextResult(fmt.Sprintf("clicked at (%.0f, %.0f)", x, y))
}

// --- browser_type ---

type BrowserTypeTool struct{ browserMgrTool }

func NewBrowserTypeTool(m *Manager) *BrowserTypeTool {
	return &BrowserTypeTool{browserMgrTool{mgr: m}}
}

func (t *BrowserTypeTool) Name() string { return "browser_type" }
func (t *BrowserTypeTool) Description() string {
	return "Type text into whatever element has focus. Click into the input first, THEN call " +
		"browser_type. Supports Unicode. For special keys (Enter, Tab, Escape) use browser_press."
}
func (t *BrowserTypeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string", "description": "Text to type."},
		},
		"required": []string{"text"},
	}
}
func (t *BrowserTypeTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	text, _ := args["text"].(string)
	if text == "" {
		return tools.ErrorResult("text is required")
	}
	if err := t.ensureStarted(ctx); err != nil {
		return tools.ErrorResult(fmt.Sprintf("start browser: %v", err))
	}
	if err := t.mgr.TypeText(ctx, text); err != nil {
		return tools.ErrorResult(fmt.Sprintf("type: %v", err))
	}
	preview := text
	if len(preview) > 80 {
		preview = preview[:80] + "…"
	}
	return tools.TextResult(fmt.Sprintf("typed %q", preview))
}

// --- browser_press ---

type BrowserPressTool struct{ browserMgrTool }

func NewBrowserPressTool(m *Manager) *BrowserPressTool {
	return &BrowserPressTool{browserMgrTool{mgr: m}}
}

func (t *BrowserPressTool) Name() string { return "browser_press" }
func (t *BrowserPressTool) Description() string {
	return "Press a named key — Enter, Tab, Escape, ArrowUp, ArrowDown, ArrowLeft, ArrowRight, " +
		"Backspace, Delete, PageDown, PageUp, Home, End. Use browser_type for literal text."
}
func (t *BrowserPressTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"key": map[string]any{"type": "string", "description": "Key name, e.g. \"Enter\"."},
		},
		"required": []string{"key"},
	}
}
func (t *BrowserPressTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	key, _ := args["key"].(string)
	if key == "" {
		return tools.ErrorResult("key is required")
	}
	if err := t.ensureStarted(ctx); err != nil {
		return tools.ErrorResult(fmt.Sprintf("start browser: %v", err))
	}
	if err := t.mgr.PressKey(ctx, key); err != nil {
		return tools.ErrorResult(fmt.Sprintf("press %s: %v", key, err))
	}
	return tools.TextResult(fmt.Sprintf("pressed %s", key))
}

// --- browser_scroll ---

type BrowserScrollTool struct{ browserMgrTool }

func NewBrowserScrollTool(m *Manager) *BrowserScrollTool {
	return &BrowserScrollTool{browserMgrTool{mgr: m}}
}

func (t *BrowserScrollTool) Name() string { return "browser_scroll" }
func (t *BrowserScrollTool) Description() string {
	return "Dispatch a mouse-wheel event at viewport coordinates (x, y) with a delta. " +
		"Positive `dy` scrolls down, negative scrolls up. The (x, y) point matters for " +
		"scrolling INSIDE a sub-region like a modal or chat pane — give coords that fall " +
		"inside that region. For whole-page scroll, use the middle of the viewport."
}
func (t *BrowserScrollTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"x":  map[string]any{"type": "number", "description": "Viewport X. Default center of viewport."},
			"y":  map[string]any{"type": "number", "description": "Viewport Y. Default center of viewport."},
			"dy": map[string]any{"type": "number", "description": "Vertical delta px. Positive = down. Default 300."},
			"dx": map[string]any{"type": "number", "description": "Horizontal delta px. Default 0."},
		},
	}
}
func (t *BrowserScrollTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	if err := t.ensureStarted(ctx); err != nil {
		return tools.ErrorResult(fmt.Sprintf("start browser: %v", err))
	}
	x, xOK := tools.ToFloat(args["x"])
	y, yOK := tools.ToFloat(args["y"])
	dy, _ := tools.ToFloat(args["dy"])
	dx, _ := tools.ToFloat(args["dx"])
	if dy == 0 && dx == 0 {
		dy = 300
	}
	// Default to viewport center when x/y are missing — a sensible
	// "scroll the main page" default.
	if !xOK || !yOK {
		info, err := t.mgr.PageInfo(ctx)
		if err == nil {
			if vp, ok := info["viewport"].(map[string]any); ok {
				if vw, ok := vp["width"].(float64); ok && !xOK {
					x = vw / 2
				}
				if vh, ok := vp["height"].(float64); ok && !yOK {
					y = vh / 2
				}
			}
		}
	}
	if err := t.mgr.ScrollAt(ctx, x, y, dy, dx); err != nil {
		return tools.ErrorResult(fmt.Sprintf("scroll: %v", err))
	}
	return tools.TextResult(fmt.Sprintf("scrolled at (%.0f, %.0f) by (%.0f, %.0f)", x, y, dx, dy))
}

// --- browser_js ---

type BrowserJSTool struct{ browserMgrTool }

func NewBrowserJSTool(m *Manager) *BrowserJSTool {
	return &BrowserJSTool{browserMgrTool{mgr: m}}
}

func (t *BrowserJSTool) Name() string { return "browser_js" }
func (t *BrowserJSTool) Description() string {
	return "Evaluate a JavaScript expression in the page and return the result as a string. " +
		"Use this for structured reads the LLM can't easily get from a screenshot — table data, " +
		"form values, computed styles, URL parts, element bounding boxes for click targets. " +
		"The expression must evaluate to a value; use an IIFE for multi-statement logic."
}
func (t *BrowserJSTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{"type": "string", "description": "JS expression to evaluate."},
		},
		"required": []string{"expression"},
	}
}
func (t *BrowserJSTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	expr, _ := args["expression"].(string)
	if strings.TrimSpace(expr) == "" {
		return tools.ErrorResult("expression is required")
	}
	if err := t.ensureStarted(ctx); err != nil {
		return tools.ErrorResult(fmt.Sprintf("start browser: %v", err))
	}
	out, err := t.mgr.Evaluate(ctx, expr)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("js: %v", err))
	}
	if len(out) > 100_000 {
		out = out[:100_000] + "\n…[truncated]…"
	}
	return tools.TextResult(out)
}
