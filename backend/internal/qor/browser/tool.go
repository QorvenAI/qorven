// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/qorvenai/qorven/internal/tools"
)

// tool.go — Browser tool for agent use. Exposes browser actions as a Qorven tool.

type BrowserTool struct {
	mgr *Manager
}

func NewBrowserTool(mgr *Manager) *BrowserTool { return &BrowserTool{mgr: mgr} }

func (t *BrowserTool) Name() string { return "browser" }
func (t *BrowserTool) Description() string {
	return "Control a headless browser: navigate, click, type, take snapshots, screenshots. Use snapshot to see the page as an accessibility tree."
}

func (t *BrowserTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{"status", "start", "stop", "navigate", "snapshot", "screenshot", "click", "type", "press", "scroll", "evaluate", "tabs", "open", "close", "wait"},
			},
			"url":      map[string]any{"type": "string", "description": "URL for navigate/open"},
			"selector": map[string]any{"type": "string", "description": "CSS selector for click/type/wait"},
			"ref":      map[string]any{"type": "string", "description": "Element ref from snapshot (e.g. e1, e5)"},
			"text":     map[string]any{"type": "string", "description": "Text for type action"},
			"key":      map[string]any{"type": "string", "description": "Key for press action (Enter, Tab, etc.)"},
			"js":       map[string]any{"type": "string", "description": "JavaScript for evaluate action"},
			"target_id": map[string]any{"type": "string", "description": "Tab target ID for close/switch"},
		},
		"required": []string{"action"},
	}
}

func (t *BrowserTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	action, _ := args["action"].(string)
	switch action {
	case "status":
		return jsonResult(t.mgr.Status())
	case "start":
		if err := t.mgr.Start(ctx); err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult("Browser started")
	case "stop":
		if err := t.mgr.Stop(ctx); err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult("Browser stopped")
	case "navigate":
		url, _ := args["url"].(string)
		if url == "" { return tools.ErrorResult("url required") }
		if err := t.mgr.Navigate(ctx, url); err != nil { return tools.ErrorResult(err.Error()) }
		t.mgr.WaitIdle(ctx, 1e9) // 1 second
		title, _ := t.mgr.GetTitle(ctx)
		return tools.SuccessResult(fmt.Sprintf("Navigated to %s — %s", url, title))
	case "snapshot":
		snap, err := t.mgr.TakeSnapshot(ctx)
		if err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult(fmt.Sprintf("Page: %s\nTitle: %s\n%d nodes\n\n%s", snap.URL, snap.Title, snap.Stats.Nodes, snap.Tree))
	case "screenshot":
		buf, err := t.mgr.Screenshot(ctx)
		if err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult(fmt.Sprintf("Screenshot captured (%d bytes, base64): %s", len(buf), base64.StdEncoding.EncodeToString(buf[:min(len(buf), 100)])))
	case "click":
		sel, _ := args["selector"].(string)
		if sel == "" { sel, _ = args["ref"].(string) }
		if sel == "" { return tools.ErrorResult("selector or ref required") }
		if err := t.mgr.Click(ctx, sel, ClickOpts{}); err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult("Clicked " + sel)
	case "type":
		sel, _ := args["selector"].(string)
		text, _ := args["text"].(string)
		if sel == "" || text == "" { return tools.ErrorResult("selector and text required") }
		if err := t.mgr.Type(ctx, sel, text, TypeOpts{Clear: true}); err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult("Typed into " + sel)
	case "press":
		key, _ := args["key"].(string)
		if key == "" { return tools.ErrorResult("key required") }
		if err := t.mgr.Press(ctx, key); err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult("Pressed " + key)
	case "scroll":
		if err := t.mgr.Scroll(ctx, 0, 500); err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult("Scrolled down")
	case "evaluate":
		js, _ := args["js"].(string)
		if js == "" { return tools.ErrorResult("js required") }
		result, err := t.mgr.Evaluate(ctx, js)
		if err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult(result)
	case "tabs":
		tabs, err := t.mgr.ListTabs(ctx)
		if err != nil { return tools.ErrorResult(err.Error()) }
		return jsonResult(tabs)
	case "open":
		url, _ := args["url"].(string)
		if url == "" { return tools.ErrorResult("url required") }
		tid, err := t.mgr.OpenTab(ctx, url)
		if err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult("Opened tab " + tid)
	case "close":
		tid, _ := args["target_id"].(string)
		if tid == "" { return tools.ErrorResult("target_id required") }
		if err := t.mgr.CloseTab(ctx, tid); err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult("Closed tab " + tid)
	case "wait":
		sel, _ := args["selector"].(string)
		if sel == "" { return tools.ErrorResult("selector required") }
		if err := t.mgr.Wait(ctx, WaitOpts{Selector: sel, Visible: true}); err != nil { return tools.ErrorResult(err.Error()) }
		return tools.SuccessResult("Element visible: " + sel)
	default:
		return tools.ErrorResult("unknown action: " + action)
	}
}

func jsonResult(v any) *tools.Result {
	data, _ := json.MarshalIndent(v, "", "  ")
	return tools.SuccessResult(string(data))
}

func min(a, b int) int { if a < b { return a }; return b }
