// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cdp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// actions.go — Low-level CDP actions for precise browser control.
// These bypass chromedp's high-level API for cases needing exact mouse/keyboard control.

// ActionMethod represents a browser interaction type.
type ActionMethod string

const (
	ActionClick    ActionMethod = "click"
	ActionHover    ActionMethod = "hover"
	ActionType     ActionMethod = "type"
	ActionFill     ActionMethod = "fill"
	ActionPress    ActionMethod = "press"
	ActionSelect   ActionMethod = "select"
	ActionCheck    ActionMethod = "check"
	ActionScroll   ActionMethod = "scroll"
	ActionFocus    ActionMethod = "focus"
)

// Element holds coordinates and IDs for a resolved DOM element.
type Element struct {
	ObjectID  string  `json:"object_id"`
	NodeID    int64   `json:"node_id"`
	Selector  string  `json:"selector,omitempty"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Width     float64 `json:"width"`
	Height    float64 `json:"height"`
}

// DispatchAction routes a low-level CDP action to the appropriate handler.
func DispatchAction(ctx context.Context, method ActionMethod, el Element, value string) error {
	slog.Debug("cdp.action", "method", method, "selector", el.Selector)
	switch method {
	case ActionClick:  return clickAt(ctx, el)
	case ActionHover:  return hoverAt(ctx, el)
	case ActionType:   return typeChars(ctx, value)
	case ActionFill:   return fillElement(ctx, el, value)
	case ActionPress:  return pressKey(ctx, value)
	case ActionScroll: return scrollAt(ctx, el, 0, 300)
	case ActionFocus:  return focusNode(ctx, el)
	case ActionCheck:  return setChecked(ctx, el, true)
	default:           return fmt.Errorf("cdp: unknown action %s", method)
	}
}

// clickAt performs a precise mouse click at element center via CDP Input domain.
func clickAt(ctx context.Context, el Element) error {
	x := el.X + el.Width/2
	y := el.Y + el.Height/2
	return chromedp.Run(ctx,
		input.DispatchMouseEvent(input.MousePressed, x, y).WithButton(input.Left).WithClickCount(1),
		input.DispatchMouseEvent(input.MouseReleased, x, y).WithButton(input.Left).WithClickCount(1),
	)
}

// hoverAt moves the mouse to element center.
func hoverAt(ctx context.Context, el Element) error {
	x := el.X + el.Width/2
	y := el.Y + el.Height/2
	return chromedp.Run(ctx, input.DispatchMouseEvent(input.MouseMoved, x, y))
}

// typeChars types text character by character via CDP Input domain.
func typeChars(ctx context.Context, text string) error {
	for _, ch := range text {
		s := string(ch)
		if err := chromedp.Run(ctx,
			input.DispatchKeyEvent(input.KeyDown).WithText(s),
			input.DispatchKeyEvent(input.KeyUp).WithText(s),
		); err != nil {
			return fmt.Errorf("cdp.type: %w", err)
		}
	}
	return nil
}

// fillElement clears a field and types new text.
func fillElement(ctx context.Context, el Element, value string) error {
	focusNode(ctx, el)
	// Select all + delete to clear
	chromedp.Run(ctx,
		input.DispatchKeyEvent(input.KeyDown).WithKey("a").WithModifiers(2), // Ctrl+A
		input.DispatchKeyEvent(input.KeyUp).WithKey("a").WithModifiers(2),
		input.DispatchKeyEvent(input.KeyDown).WithKey("Backspace"),
		input.DispatchKeyEvent(input.KeyUp).WithKey("Backspace"),
	)
	return typeChars(ctx, value)
}

// pressKey sends a single key press.
func pressKey(ctx context.Context, key string) error {
	return chromedp.Run(ctx,
		input.DispatchKeyEvent(input.KeyDown).WithKey(key),
		input.DispatchKeyEvent(input.KeyUp).WithKey(key),
	)
}

// scrollAt dispatches a mouse wheel event at element position.
func scrollAt(ctx context.Context, el Element, deltaX, deltaY float64) error {
	x := el.X + el.Width/2
	y := el.Y + el.Height/2
	return chromedp.Run(ctx,
		input.DispatchMouseEvent(input.MouseWheel, x, y).WithDeltaX(deltaX).WithDeltaY(deltaY),
	)
}

// focusNode focuses a DOM node by ID.
func focusNode(ctx context.Context, el Element) error {
	if el.Selector != "" {
		return chromedp.Run(ctx, chromedp.Focus(el.Selector))
	}
	return nil
}

// setChecked sets a checkbox/radio state via JS.
func setChecked(ctx context.Context, el Element, checked bool) error {
	if el.Selector == "" { return fmt.Errorf("cdp.check: selector required") }
	js := fmt.Sprintf(`document.querySelector('%s').checked = %v; document.querySelector('%s').dispatchEvent(new Event('change', {bubbles:true}))`, el.Selector, checked, el.Selector)
	return chromedp.Run(ctx, chromedp.Evaluate(js, nil))
}
