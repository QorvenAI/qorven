// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// Vision-driven browsing. The LLM looks at a screenshot and emits a
// coordinate-based action. Same loop shape as BrowseAndAct (the
// selector-based variant) but with three important differences:
//
//   1. No accessibility tree serialisation. The screenshot is the
//      state. Vision models ground coordinates better than they
//      read serialised a11y trees.
//   2. Coordinate actions. Clicks go to (x, y); works through iframes,
//      shadow DOM, and canvas-rendered UIs (Figma, Google Docs,
//      Canva) that the selector-based variant can't touch.
//   3. Post-action screenshot returned on the next turn. The LLM sees
//      its own action's result, closing the perception loop.
//
// Falls back to BrowseAndAct automatically when the selected model
// doesn't support vision — callers get a usable result either way.

// VisionAction is the structured JSON the LLM emits. Narrow set of
// verbs so the model doesn't hallucinate new ones.
type VisionAction struct {
	Thoughts   string  `json:"thoughts"`
	Memory     string  `json:"memory"`
	ActionType string  `json:"action_type"` // click | type | press | scroll | navigate | complete
	X          float64 `json:"x,omitempty"`
	Y          float64 `json:"y,omitempty"`
	Text       string  `json:"text,omitempty"` // type / complete payload
	Key        string  `json:"key,omitempty"`  // press
	URL        string  `json:"url,omitempty"`  // navigate
	DY         float64 `json:"dy,omitempty"`   // scroll delta; positive = down
	Clicks     int     `json:"clicks,omitempty"`
}

const visionSystemPrompt = `You drive a web browser by looking at screenshots.

EVERY turn you receive:
- Goal (what to achieve)
- Viewport size (width x height in pixels, (0,0) is top-left)
- Current URL and title
- Screenshot of the page
- Scratchpad memory from prior steps

You respond with EXACTLY one JSON object describing the next action. No prose, no code fences — JSON only.

Schema:
{
  "thoughts":   "<one sentence on what you see and why you're acting>",
  "memory":     "<anything future steps will need to remember>",
  "action_type": "click | type | press | scroll | navigate | complete",
  "x":          <number, viewport pixel X for click/scroll>,
  "y":          <number, viewport pixel Y for click/scroll>,
  "text":       "<text to type, or final answer when action_type=complete>",
  "key":        "<named key for press: Enter, Tab, Escape, ArrowDown, ...>",
  "url":        "<destination URL for navigate>",
  "dy":         <scroll delta in pixels, positive=down>,
  "clicks":     <1 for click, 2 for double-click>
}

Rules:
- Coordinate (0,0) is TOP-LEFT of the visible viewport.
- Read the screenshot carefully. Click text and buttons at their visible center.
- To type into a field: FIRST click the field, THEN on the NEXT turn emit action_type=type.
  (One action per turn — don't compound.)
- Use action_type=press for Enter/Tab/Escape, not literal text.
- Use action_type=scroll with a positive dy (default 300) to scroll down; negative to scroll up.
- Use action_type=complete when the goal is achieved. Put the final answer in "text".
- Do NOT make up coordinates. If you can't see the element, scroll first.
- Stay on-goal. Don't get distracted by cookie banners / newsletters unless they block the flow.`

// BrowseAndActVision runs the vision-driven loop. Requires a provider
// + model that supports vision; caller is responsible for that check.
func (ba *BrowseAgent) BrowseAndActVision(ctx context.Context, goal, startURL string) (string, []AgentStep, error) {
	if err := ba.browser.Start(ctx); err != nil {
		return "", nil, fmt.Errorf("browser start: %w", err)
	}
	if err := ba.browser.Navigate(ctx, startURL); err != nil {
		return "", nil, fmt.Errorf("navigate: %w", err)
	}
	_ = ba.browser.WaitForReadyState(ctx, 10*time.Second)

	var steps []AgentStep
	var memory string

	for step := 0; step < ba.maxSteps; step++ {
		stepStart := time.Now()

		// 1. Screenshot + page info for this turn.
		png, err := ba.browser.Screenshot(ctx)
		if err != nil {
			return "", steps, fmt.Errorf("step %d screenshot: %w", step, err)
		}
		info, _ := ba.browser.PageInfo(ctx)
		currentURL, _ := info["url"].(string)
		currentTitle, _ := info["title"].(string)
		vpW, vpH := 0.0, 0.0
		if vp, ok := info["viewport"].(map[string]any); ok {
			vpW, _ = vp["width"].(float64)
			vpH, _ = vp["height"].(float64)
		}

		userText := fmt.Sprintf(`=== Goal ===
%s

=== Viewport ===
%.0f x %.0f pixels (0,0 = top-left)

=== Current URL ===
%s

=== Title ===
%s

=== Memory ===
%s

=== Step %d of %d ===

Look at the screenshot, decide the next single action, emit one JSON object.`,
			goal, vpW, vpH, currentURL, currentTitle, memory, step+1, ba.maxSteps)

		// 2. Ask the vision LLM. Image goes in the user turn.
		resp, err := ba.provider.Chat(ctx, providers.ChatRequest{
			Model: ba.model,
			Messages: []providers.Message{
				{Role: "system", Content: visionSystemPrompt},
				{Role: "user", Content: userText, Images: []providers.ImageContent{
					{MimeType: "image/png", Data: base64.StdEncoding.EncodeToString(png)},
				}},
			},
			Options: map[string]any{"temperature": 0.1, "max_tokens": 400},
		})
		if err != nil {
			return "", steps, fmt.Errorf("step %d llm: %w", step, err)
		}

		// 3. Parse the JSON action.
		action, parseErr := parseVisionAction(resp.Content)
		if parseErr != nil {
			slog.Warn("browse_vision.parse_failed", "step", step,
				"raw_head", truncStr(resp.Content, 120))
			// Degrade gracefully — a scroll is the safest recovery.
			action = &VisionAction{ActionType: "scroll", DY: 300,
				Thoughts: "parse failed, scrolling to re-orient"}
		}

		if action.Memory != "" {
			memory = action.Memory
		}
		slog.Info("browse_vision.step", "step", step,
			"action", action.ActionType,
			"x", action.X, "y", action.Y)

		// 4. Execute. Fallback to selector-based equivalents isn't
		//    needed — everything here is coord-based.
		result := ba.executeVisionAction(ctx, action)

		// Record with a compat shim to AgentAction for the summary.
		steps = append(steps, AgentStep{
			Step: step, URL: currentURL, Action: compatAction(action),
			Result: result, Duration: time.Since(stepStart).Milliseconds(),
		})

		// 5. Done conditions.
		if action.ActionType == "complete" {
			return action.Text, steps, nil
		}
	}

	return "Reached max steps without completing goal", steps, nil
}

func (ba *BrowseAgent) executeVisionAction(ctx context.Context, a *VisionAction) string {
	switch a.ActionType {
	case "click":
		clicks := a.Clicks
		if clicks <= 0 {
			clicks = 1
		}
		if err := ba.browser.MouseClick(ctx, a.X, a.Y, "left", clicks); err != nil {
			return fmt.Sprintf("click (%.0f, %.0f) failed: %v", a.X, a.Y, err)
		}
		// Brief settle so the next screenshot catches the result.
		_ = ba.browser.WaitForReadyState(ctx, 500*time.Millisecond)
		return fmt.Sprintf("clicked (%.0f, %.0f)", a.X, a.Y)

	case "type":
		if a.Text == "" {
			return "error: type requires text"
		}
		if err := ba.browser.TypeText(ctx, a.Text); err != nil {
			return fmt.Sprintf("type failed: %v", err)
		}
		return fmt.Sprintf("typed %q", truncStr(a.Text, 40))

	case "press":
		if a.Key == "" {
			return "error: press requires key"
		}
		if err := ba.browser.PressKey(ctx, a.Key); err != nil {
			return fmt.Sprintf("press %s failed: %v", a.Key, err)
		}
		_ = ba.browser.WaitForReadyState(ctx, 500*time.Millisecond)
		return fmt.Sprintf("pressed %s", a.Key)

	case "scroll":
		dy := a.DY
		if dy == 0 {
			dy = 300
		}
		// Default to viewport center when x/y missing — same logic
		// as the browser_scroll tool.
		x, y := a.X, a.Y
		if x == 0 && y == 0 {
			if info, err := ba.browser.PageInfo(ctx); err == nil {
				if vp, ok := info["viewport"].(map[string]any); ok {
					if vw, ok := vp["width"].(float64); ok {
						x = vw / 2
					}
					if vh, ok := vp["height"].(float64); ok {
						y = vh / 2
					}
				}
			}
		}
		if err := ba.browser.ScrollAt(ctx, x, y, dy, 0); err != nil {
			return fmt.Sprintf("scroll failed: %v", err)
		}
		return fmt.Sprintf("scrolled by %.0f", dy)

	case "navigate":
		if a.URL == "" {
			return "error: navigate requires url"
		}
		if err := ba.browser.Navigate(ctx, a.URL); err != nil {
			return fmt.Sprintf("navigate failed: %v", err)
		}
		_ = ba.browser.WaitForReadyState(ctx, 10*time.Second)
		return "navigated to " + a.URL

	case "complete":
		return "completed: " + truncStr(a.Text, 80)

	default:
		return "unknown action: " + a.ActionType
	}
}

func parseVisionAction(raw string) (*VisionAction, error) {
	raw = strings.TrimSpace(raw)
	// Tolerate fenced output — some models ignore instructions to
	// skip the fence. Strip and re-parse.
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	// If the model wrote prose before the JSON, trim to the first { ... last }.
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}

	var a VisionAction
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		return nil, fmt.Errorf("parse vision action: %w", err)
	}
	if a.ActionType == "" {
		return nil, fmt.Errorf("empty action_type")
	}
	return &a, nil
}

// compatAction converts a VisionAction to the legacy AgentAction shape
// so steps render uniformly in the tool's summary output.
func compatAction(a *VisionAction) AgentAction {
	return AgentAction{
		Thoughts:   a.Thoughts,
		Memory:     a.Memory,
		ActionType: a.ActionType,
		Value:      a.Text,
		URL:        a.URL,
		// selector intentionally left empty — we're coord-based
	}
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
