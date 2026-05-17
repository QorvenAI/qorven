// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/tools"
)

// computer_use — single-tool ergonomics for the LLM's operator loop.
//
// The browser_* primitives are perfectly usable alone, but for most
// "complete this task" flows the LLM wants to: (1) see the page, (2)
// do one action, (3) see the page again. That's three tool calls and
// three round-trips. computer_use rolls them into one call so the
// typical "click + screenshot to verify" pattern is one turn.
//
// The action vocabulary mirrors the primitives:
//   click       — (x, y) + optional button/clicks
//   type        — text
//   press       — key name
//   scroll      — (x, y) + dx/dy
//   screenshot  — no action, just observe
//   goto        — (url) — useful when you don't want to chain goto+screenshot
//   back / forward / reload
//
// Every invocation returns:
//   - The action's result text ("clicked at (440, 312)")
//   - A fresh viewport screenshot as data:image/png;base64,…
//   - Current URL + viewport dimensions so the LLM recalibrates its
//     coordinate model if the page navigated
//
// When to use vs. primitives:
//   - computer_use: iterative UI tasks ("fill this form", "navigate to
//     the checkout", "keep scrolling until you see Pricing")
//   - browser_* primitives: one-shot actions where a screenshot isn't
//     needed after ("close this tab", "evaluate JS to read a value")

type ComputerUseTool struct{ browserMgrTool }

func NewComputerUseTool(m *Manager) *ComputerUseTool {
	return &ComputerUseTool{browserMgrTool{mgr: m}}
}

func (t *ComputerUseTool) Name() string { return "computer_use" }

func (t *ComputerUseTool) Description() string {
	return "High-level operator-style browser action. Performs one action (click / type / press / scroll / screenshot / goto / back / forward / reload) and returns a fresh screenshot plus the current URL and viewport size. Use this for iterative UI work where you want 'do thing → see result' in one tool call. For one-off actions that don't need verification, prefer the browser_* primitives."
}

func (t *ComputerUseTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "One of: click, type, press, scroll, screenshot, goto, back, forward, reload.",
				"enum":        []string{"click", "type", "press", "scroll", "screenshot", "goto", "back", "forward", "reload"},
			},
			"x":      map[string]any{"type": "number", "description": "click / scroll: viewport X."},
			"y":      map[string]any{"type": "number", "description": "click / scroll: viewport Y."},
			"button": map[string]any{"type": "string", "description": "click: left (default), right, middle."},
			"clicks": map[string]any{"type": "integer", "description": "click: count (default 1, use 2 for double-click)."},
			"text":   map[string]any{"type": "string", "description": "type: the literal text to enter."},
			"key":    map[string]any{"type": "string", "description": "press: Enter, Tab, Escape, ArrowUp, etc."},
			"dx":     map[string]any{"type": "number", "description": "scroll: horizontal delta."},
			"dy":     map[string]any{"type": "number", "description": "scroll: vertical delta (positive = down). Default 300."},
			"url":    map[string]any{"type": "string", "description": "goto: destination URL."},
			"wait_ms": map[string]any{
				"type":        "integer",
				"description": "How long to wait after the action before screenshotting (default 400ms). Raise for pages with animations or network-driven renders.",
			},
		},
		"required": []string{"action"},
	}
}

func (t *ComputerUseTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	action, _ := args["action"].(string)
	if action == "" {
		return tools.ErrorResult("action is required")
	}
	// Validate required sub-args BEFORE spawning Chrome. A typo in
	// the LLM's args should error instantly, not after a 2s process
	// boot. Also makes this unit-testable without a real browser.
	switch action {
	case "click":
		if _, xOK := tools.ToFloat(args["x"]); !xOK {
			return tools.ErrorResult("click requires x and y")
		}
		if _, yOK := tools.ToFloat(args["y"]); !yOK {
			return tools.ErrorResult("click requires x and y")
		}
	case "type":
		if text, _ := args["text"].(string); text == "" {
			return tools.ErrorResult("type requires text")
		}
	case "press":
		if key, _ := args["key"].(string); key == "" {
			return tools.ErrorResult("press requires key")
		}
	case "goto":
		if url, _ := args["url"].(string); url == "" {
			return tools.ErrorResult("goto requires url")
		}
	case "scroll", "screenshot", "back", "forward", "reload":
		// No required args — defaults fill in during dispatch.
	default:
		return tools.ErrorResult(fmt.Sprintf("unknown action %q", action))
	}
	if err := t.ensureStarted(ctx); err != nil {
		return tools.ErrorResult(fmt.Sprintf("start browser: %v", err))
	}

	// Execute the requested action.
	var actionMsg string
	var actionErr error
	switch action {
	case "click":
		x, _ := tools.ToFloat(args["x"])
		y, _ := tools.ToFloat(args["y"])
		btn, _ := args["button"].(string)
		clicks, _ := tools.ToInt(args["clicks"])
		if clicks <= 0 {
			clicks = 1
		}
		actionErr = t.mgr.MouseClick(ctx, x, y, btn, clicks)
		actionMsg = fmt.Sprintf("clicked at (%.0f, %.0f)", x, y)

	case "type":
		text, _ := args["text"].(string)
		actionErr = t.mgr.TypeText(ctx, text)
		preview := text
		if len(preview) > 40 {
			preview = preview[:40] + "…"
		}
		actionMsg = fmt.Sprintf("typed %q", preview)

	case "press":
		key, _ := args["key"].(string)
		actionErr = t.mgr.PressKey(ctx, key)
		actionMsg = fmt.Sprintf("pressed %s", key)

	case "scroll":
		x, xOK := tools.ToFloat(args["x"])
		y, yOK := tools.ToFloat(args["y"])
		dx, _ := tools.ToFloat(args["dx"])
		dy, _ := tools.ToFloat(args["dy"])
		if dy == 0 && dx == 0 {
			dy = 300
		}
		// Default to viewport center when x/y omitted.
		if !xOK || !yOK {
			info, _ := t.mgr.PageInfo(ctx)
			if vp, ok := info["viewport"].(map[string]any); ok {
				if vw, ok := vp["width"].(float64); ok && !xOK {
					x = vw / 2
				}
				if vh, ok := vp["height"].(float64); ok && !yOK {
					y = vh / 2
				}
			}
		}
		actionErr = t.mgr.ScrollAt(ctx, x, y, dy, dx)
		actionMsg = fmt.Sprintf("scrolled (%.0f, %.0f) by (%.0f, %.0f)", x, y, dx, dy)

	case "screenshot":
		// No action; the follow-up screenshot IS the action.
		actionMsg = "observed"

	case "goto":
		url, _ := args["url"].(string)
		if err := t.mgr.Navigate(ctx, url); err != nil {
			return tools.ErrorResult(fmt.Sprintf("navigate: %v", err))
		}
		_ = t.mgr.WaitForReadyState(ctx, 15*time.Second)
		actionMsg = "navigated to " + url

	case "back":
		actionErr = t.mgr.Evaluate2(ctx, "history.back()")
		actionMsg = "navigated back"

	case "forward":
		actionErr = t.mgr.Evaluate2(ctx, "history.forward()")
		actionMsg = "navigated forward"

	case "reload":
		actionErr = t.mgr.Evaluate2(ctx, "location.reload()")
		actionMsg = "reloaded"
	}
	if actionErr != nil {
		return tools.ErrorResult(fmt.Sprintf("%s: %v", action, actionErr))
	}

	// Wait for UI to settle. 400ms is enough for most click-triggered
	// animations; raise via wait_ms if the caller knows the page
	// takes longer (React app with server-rendered state, etc.).
	waitMs := 400
	if n, ok := tools.ToInt(args["wait_ms"]); ok && n > 0 {
		waitMs = n
	}
	if waitMs > 10_000 {
		waitMs = 10_000
	}
	select {
	case <-ctx.Done():
		return tools.ErrorResult(ctx.Err().Error())
	case <-time.After(time.Duration(waitMs) * time.Millisecond):
	}

	// Take the post-action screenshot + gather page info. If either
	// fails we still report the action's success — a failed screenshot
	// doesn't mean the click didn't happen.
	png, shotErr := t.mgr.Screenshot(ctx)
	info, _ := t.mgr.PageInfo(ctx)

	vpW, vpH := 0.0, 0.0
	var currentURL string
	if vp, ok := info["viewport"].(map[string]any); ok {
		vpW, _ = vp["width"].(float64)
		vpH, _ = vp["height"].(float64)
	}
	if u, ok := info["url"].(string); ok {
		currentURL = u
	}

	var sb strings.Builder
	sb.WriteString(actionMsg)
	if shotErr != nil {
		sb.WriteString(fmt.Sprintf("\n\n[screenshot failed: %v]", shotErr))
	} else {
		sb.WriteString(fmt.Sprintf("\n\nAfter (viewport %.0fx%.0f, url %s):\ndata:image/png;base64,%s",
			vpW, vpH, currentURL, base64.StdEncoding.EncodeToString(png)))
	}

	return &tools.Result{
		ForLLM:  sb.String(),
		ForUser: fmt.Sprintf("%s — %s", action, actionMsg),
	}
}

// Evaluate2 is a thin alias so computer_use can run arbitrary JS
// without exporting Evaluate's error-unwrap semantics. Kept separate
// from Evaluate so the existing callers don't inherit the
// RunWithRecovery wrapping (which could mask bugs in their tests).
func (m *Manager) Evaluate2(ctx context.Context, js string) error {
	_, err := m.Evaluate(ctx, js)
	return err
}
