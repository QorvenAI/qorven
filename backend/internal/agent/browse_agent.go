// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	browser "github.com/qorvenai/qorven/internal/qor/browser"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/tools"
)

// BrowseAgent implements the autonomous browse-and-act loop.
// The LLM outputs structured JSON actions, not free text.
//
// Loop: navigate → snapshot → LLM decides (JSON) → execute → repeat
// Until: LLM outputs "complete" action or max steps reached.

type BrowseAgent struct {
	browser  *browser.Manager
	provider providers.Provider
	model    string
	maxSteps int
}

func NewBrowseAgent(mgr *browser.Manager, provider providers.Provider, model string) *BrowseAgent {
	return &BrowseAgent{browser: mgr, provider: provider, model: model, maxSteps: 15}
}

// AgentStep records one step in the browse loop.
type AgentStep struct {
	Step     int         `json:"step"`
	URL      string      `json:"url"`
	Action   AgentAction `json:"action"`
	Result   string      `json:"result"`
	Duration int64       `json:"duration_ms"`
}

// AgentAction is the structured output from the LLM.
type AgentAction struct {
	Thoughts   string `json:"thoughts"`
	Memory     string `json:"memory"`
	ActionType string `json:"action_type"` // click, type, scroll, navigate, extract, wait, complete
	Selector   string `json:"selector,omitempty"`
	Value      string `json:"value,omitempty"`
	URL        string `json:"url,omitempty"`
}

const browseSystemPrompt = `You are a web browsing agent. You navigate websites to achieve goals.

You receive the page's accessibility tree and must decide the next action.

ALWAYS respond with valid JSON in this exact format:
{
  "thoughts": "what I observe and my reasoning",
  "memory": "key facts to remember for later steps",
  "action_type": "click|type|scroll|navigate|extract|wait|complete",
  "selector": "CSS selector or element ref (for click/type)",
  "value": "text to type (for type) or extracted data (for extract/complete)",
  "url": "URL (for navigate)"
}

Action types:
- click: click an element. Provide selector.
- type: type text into a field. Provide selector and value.
- scroll: scroll down the page. No selector needed.
- navigate: go to a URL. Provide url.
- extract: extract information from current page. Put data in value.
- wait: wait for page to load. No params needed.
- complete: goal achieved. Put final result in value.

Rules:
- ONLY output JSON. No other text.
- Use the accessibility tree to find elements.
- If you can't find what you need, try scrolling or navigating.
- Always complete within 15 steps.`

// BrowseAndAct autonomously browses the web to achieve a goal.
func (ba *BrowseAgent) BrowseAndAct(ctx context.Context, goal, startURL string) (string, []AgentStep, error) {
	if err := ba.browser.Start(ctx); err != nil {
		return "", nil, fmt.Errorf("browser start: %w", err)
	}
	if err := ba.browser.Navigate(ctx, startURL); err != nil {
		return "", nil, fmt.Errorf("navigate: %w", err)
	}
	ba.browser.WaitIdle(ctx, 2e9)

	var steps []AgentStep
	var memory string

	for step := 0; step < ba.maxSteps; step++ {
		stepStart := time.Now()

		// 1. Snapshot the page
		snap, err := ba.browser.TakeSnapshot(ctx)
		if err != nil {
			return "", steps, fmt.Errorf("step %d snapshot: %w", step, err)
		}

		tree := snap.Tree
		if len(tree) > 4000 { tree = tree[:4000] + "\n... (truncated)" }

		// 2. Build prompt with context
		userPrompt := fmt.Sprintf("=== Goal ===\n%s\n\n=== Current URL ===\n%s\n\n=== Page Title ===\n%s\n\n=== Memory ===\n%s\n\n=== Step %d of %d ===\n\n=== Accessibility Tree ===\n%s",
			goal, snap.URL, snap.Title, memory, step+1, ba.maxSteps, tree)

		// 3. Ask LLM for structured action
		resp, err := ba.provider.Chat(ctx, providers.ChatRequest{
			Model: ba.model,
			Messages: []providers.Message{
				{Role: "system", Content: browseSystemPrompt},
				{Role: "user", Content: userPrompt},
			},
			Options: map[string]any{"temperature": 0.1, "max_tokens": 300},
		})
		if err != nil {
			return "", steps, fmt.Errorf("step %d llm: %w", step, err)
		}

		// 4. Parse JSON action
		action, parseErr := parseAction(resp.Content)
		if parseErr != nil {
			slog.Warn("browse.parse_failed", "step", step, "raw", resp.Content[:min(len(resp.Content), 100)])
			// Try to extract JSON from response
			action = &AgentAction{ActionType: "scroll", Thoughts: "parse failed, scrolling"}
		}

		// Update memory
		if action.Memory != "" { memory = action.Memory }

		slog.Info("browse.step", "step", step, "action", action.ActionType, "thoughts", action.Thoughts[:min(len(action.Thoughts), 60)])

		// 5. Execute action
		result := ba.executeAction(ctx, action)

		steps = append(steps, AgentStep{
			Step: step, URL: snap.URL, Action: *action,
			Result: result, Duration: time.Since(stepStart).Milliseconds(),
		})

		// 6. Check if done
		if action.ActionType == "complete" {
			return action.Value, steps, nil
		}
		if action.ActionType == "extract" {
			return action.Value, steps, nil
		}
	}

	return "Reached max steps without completing goal", steps, nil
}

func parseAction(raw string) (*AgentAction, error) {
	raw = strings.TrimSpace(raw)
	// Strip markdown code blocks
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	// Find JSON object
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}

	var action AgentAction
	if err := json.Unmarshal([]byte(raw), &action); err != nil {
		return nil, fmt.Errorf("parse action: %w", err)
	}
	if action.ActionType == "" {
		return nil, fmt.Errorf("empty action_type")
	}
	return &action, nil
}

func (ba *BrowseAgent) executeAction(ctx context.Context, action *AgentAction) string {
	switch action.ActionType {
	case "click":
		if action.Selector == "" { return "error: no selector" }
		if err := ba.browser.Click(ctx, action.Selector, browser.ClickOpts{}); err != nil {
			return fmt.Sprintf("click failed: %v", err)
		}
		ba.browser.WaitIdle(ctx, 2e9)
		return "clicked " + action.Selector

	case "type":
		if action.Selector == "" || action.Value == "" { return "error: selector and value required" }
		if err := ba.browser.Type(ctx, action.Selector, action.Value, browser.TypeOpts{Clear: true}); err != nil {
			return fmt.Sprintf("type failed: %v", err)
		}
		return "typed into " + action.Selector

	case "scroll":
		if err := ba.browser.Scroll(ctx, 0, 500); err != nil {
			return fmt.Sprintf("scroll failed: %v", err)
		}
		ba.browser.WaitIdle(ctx, 1e9)
		return "scrolled down"

	case "navigate":
		if action.URL == "" { return "error: no url" }
		if err := ba.browser.Navigate(ctx, action.URL); err != nil {
			return fmt.Sprintf("navigate failed: %v", err)
		}
		ba.browser.WaitIdle(ctx, 3e9)
		return "navigated to " + action.URL

	case "wait":
		ba.browser.WaitIdle(ctx, 3e9)
		return "waited"

	case "extract":
		return "extracted: " + action.Value

	case "complete":
		return "completed: " + action.Value

	default:
		return "unknown action: " + action.ActionType
	}
}

// --- Tool wrapper ---

type BrowseTool struct {
	agent *BrowseAgent
}

func NewBrowseTool(agent *BrowseAgent) *BrowseTool {
	return &BrowseTool{agent: agent}
}

func (t *BrowseTool) Name() string { return "browse_and_act" }
func (t *BrowseTool) Description() string {
	return "Autonomously browse the web to achieve a goal. The agent navigates pages, reads content, clicks buttons, fills forms, and extracts information. Use for tasks that require interacting with websites."
}
func (t *BrowseTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"goal":      map[string]any{"type": "string", "description": "What to achieve (e.g., 'find the current IPL score on cricbuzz')"},
			"start_url": map[string]any{"type": "string", "description": "URL to start browsing from"},
			"mode": map[string]any{
				"type":        "string",
				"description": "Override mode. \"vision\" = screenshot + coordinates (best for canvas/iframes). \"selector\" = accessibility tree + CSS (text-model fallback). Omit to auto-pick based on model capability.",
				"enum":        []string{"vision", "selector"},
			},
		},
		"required": []string{"goal", "start_url"},
	}
}

func (t *BrowseTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	goal, _ := args["goal"].(string)
	startURL, _ := args["start_url"].(string)
	if goal == "" || startURL == "" {
		return tools.ErrorResult("goal and start_url required")
	}

	// Pick vision vs. selector mode. Vision is strictly better (works
	// through iframes/shadow DOM, handles canvas-rendered UIs, doesn't
	// need an a11y tree) but only works on vision-capable models. An
	// optional `mode` arg lets callers force one or the other.
	mode, _ := args["mode"].(string)
	useVision := false
	switch mode {
	case "vision":
		useVision = true
	case "selector":
		useVision = false
	default:
		useVision = modelSupportsVision(t.agent.model)
	}

	var result string
	var steps []AgentStep
	var err error
	if useVision {
		result, steps, err = t.agent.BrowseAndActVision(ctx, goal, startURL)
	} else {
		result, steps, err = t.agent.BrowseAndAct(ctx, goal, startURL)
	}
	if err != nil {
		return tools.ErrorResult(err.Error())
	}

	// Format result with step summary
	var sb strings.Builder
	sb.WriteString(result)
	if len(steps) > 0 {
		sb.WriteString(fmt.Sprintf("\n\n(%d steps taken", len(steps)))
		totalMs := int64(0)
		for _, s := range steps { totalMs += s.Duration }
		sb.WriteString(fmt.Sprintf(", %dms total", totalMs))
		if useVision {
			sb.WriteString(", vision mode")
		}
		sb.WriteString(")")
	}

	// Emit a browser_step widget per step so the chat UI renders a
	// step-by-step replay — matches the BrowserStepCard design.
	// Cap at 20 steps in the payload to keep event traffic modest;
	// if more happened, the text summary still captures them.
	widgets := make([]tools.Widget, 0, len(steps))
	maxCards := 20
	for i, s := range steps {
		if i >= maxCards { break }
		widgets = append(widgets, tools.Widget{
			Type: "browser_step",
			Data: map[string]any{
				"step":        s.Step + 1, // 1-indexed for humans
				"url":         s.URL,
				"action_type": s.Action.ActionType,
				"thoughts":    s.Action.Thoughts,
				"result":      s.Result,
				"duration_ms": s.Duration,
			},
		})
	}

	return &tools.Result{
		ForLLM:  sb.String(),
		ForUser: sb.String(),
		Widgets: widgets,
	}
}

// modelSupportsVision is a quick substring check against known vision-
// capable model IDs. We deliberately keep this list narrow — being
// conservative beats silently attempting vision on a text-only model
// and getting "content_parts not allowed" errors back.
func modelSupportsVision(model string) bool {
	m := strings.ToLower(model)
	// Anthropic (Claude 3/4) and OpenAI GPT-4o/o1 all accept images.
	// Google Gemini 1.5+, Amazon Nova Pro, Mistral Pixtral too.
	hints := []string{
		"claude-3", "claude-4", "claude-sonnet-4", "claude-opus-4",
		"gpt-4o", "gpt-4-vision", "gpt-4.1", "gpt-5", "o1-", "o3-", "o4-",
		"gemini-1.5", "gemini-2", "nova-pro", "nova-lite", "nova-premier",
		"pixtral",
	}
	for _, h := range hints {
		if strings.Contains(m, h) {
			return true
		}
	}
	return false
}
