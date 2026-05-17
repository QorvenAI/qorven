// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// Coordinate-based primitives. These bypass CSS selectors entirely,
// which matters for two real-world cases:
//
//   1. Iframes / shadow DOM / cross-origin children. Selector-based
//      clicks can't cross those boundaries; a compositor-level
//      Input.dispatchMouseEvent goes through.
//
//   2. Vision-driven agents. The LLM looks at a screenshot and says
//      "click (523, 188)". Selectors require an accessibility tree
//      snapshot that doubles token cost and can still miss modern
//      canvas-heavy UIs (Figma, Google Docs).
//
// Pair these with Screenshot() (already exists) to build a full
// operator-style loop: screenshot → LLM decides x,y → click.

// MouseClick dispatches a mouse click at exact viewport coordinates.
// `button` is one of "left" (default), "right", "middle".
// `clicks` is the click count (1=single, 2=double, etc.).
//
// Coordinates are CSS pixels relative to the top-left of the viewport,
// matching how the LLM reads a screenshot of the current viewport.
// If the target is below the fold, scroll first — we don't auto-scroll
// because the LLM should be aware of its spatial mental model.
func (m *Manager) MouseClick(ctx context.Context, x, y float64, button string, clicks int) error {
	if clicks < 1 {
		clicks = 1
	}
	btn := input.Left
	switch button {
	case "right":
		btn = input.Right
	case "middle":
		btn = input.Middle
	}
	// RunWithRecovery handles "Session with given id not found" by
	// reattaching to a live target and retrying once. Without it, a
	// single closed-tab error permanently wedged every click.
	return m.RunWithRecovery(ctx, chromedp.MouseClickXY(x, y,
		chromedp.ButtonType(btn),
		chromedp.ClickCount(clicks),
	))
}

// MouseMove moves the cursor to (x, y) without clicking — useful for
// hover-triggered menus and tooltip inspections.
func (m *Manager) MouseMove(ctx context.Context, x, y float64) error {
	tabCtx, err := m.activeTabContext()
	if err != nil {
		return err
	}
	actCtx, cancel := context.WithTimeout(tabCtx, m.cfg.ActionTimeout)
	defer cancel()
	return chromedp.Run(actCtx,
		chromedp.MouseEvent(input.MouseMoved, x, y))
}

// ScrollAt dispatches a mouse-wheel event at a specific point. This
// lets the agent scroll INSIDE a scrollable sub-region (modals, code
// viewers, chat panes) rather than the whole viewport. `dy` is the
// vertical wheel delta in pixels; positive = down, negative = up.
// Harness-style ergonomics.
func (m *Manager) ScrollAt(ctx context.Context, x, y, dy, dx float64) error {
	tabCtx, err := m.activeTabContext()
	if err != nil {
		return err
	}
	actCtx, cancel := context.WithTimeout(tabCtx, m.cfg.ActionTimeout)
	defer cancel()
	// The mousewheel event dispatches at (x, y) with delta values,
	// which is what Chrome uses for trackpad/wheel events.
	return chromedp.Run(actCtx, chromedp.ActionFunc(func(c context.Context) error {
		p := &input.DispatchMouseEventParams{
			Type:        input.MouseWheel,
			X:           x,
			Y:           y,
			DeltaX:      dx,
			DeltaY:      dy,
			PointerType: input.Mouse,
		}
		return p.Do(c)
	}))
}

// TypeText dispatches keyboard events for each rune, independent of
// which element has focus in the agent's mental model. The user is
// expected to click into the input first. Unicode is supported —
// chromedp handles code-point-to-key translation.
func (m *Manager) TypeText(ctx context.Context, text string) error {
	tabCtx, err := m.activeTabContext()
	if err != nil {
		return err
	}
	actCtx, cancel := context.WithTimeout(tabCtx, m.cfg.ActionTimeout)
	defer cancel()
	return chromedp.Run(actCtx, chromedp.KeyEvent(text))
}

// PressKey sends a single named key ("Enter", "Tab", "ArrowDown",
// "Escape", etc.) optionally with modifiers. Use TypeText for
// literal strings.
func (m *Manager) PressKey(ctx context.Context, key string) error {
	tabCtx, err := m.activeTabContext()
	if err != nil {
		return err
	}
	actCtx, cancel := context.WithTimeout(tabCtx, m.cfg.ActionTimeout)
	defer cancel()
	return chromedp.Run(actCtx, chromedp.KeyEvent(key))
}

// PageInfo reports viewport + scroll + page dimensions so the agent
// can plan scroll operations without another round-trip to the LLM.
//
// Returned shape mirrors harness `page_info()`:
//
//	{
//	  url, title,
//	  viewport: { width, height, device_pixel_ratio },
//	  scroll:   { x, y, max_x, max_y },
//	  page:     { width, height }
//	}
func (m *Manager) PageInfo(ctx context.Context) (map[string]any, error) {
	tabCtx, err := m.activeTabContext()
	if err != nil {
		return nil, err
	}
	actCtx, cancel := context.WithTimeout(tabCtx, m.cfg.ActionTimeout)
	defer cancel()

	var info struct {
		URL           string  `json:"url"`
		Title         string  `json:"title"`
		VW            float64 `json:"vw"`
		VH            float64 `json:"vh"`
		DPR           float64 `json:"dpr"`
		ScrollX       float64 `json:"sx"`
		ScrollY       float64 `json:"sy"`
		ScrollMaxX    float64 `json:"smx"`
		ScrollMaxY    float64 `json:"smy"`
		PageW         float64 `json:"pw"`
		PageH         float64 `json:"ph"`
	}
	err = chromedp.Run(actCtx, chromedp.Evaluate(`({
		url: location.href,
		title: document.title,
		vw: window.innerWidth,
		vh: window.innerHeight,
		dpr: window.devicePixelRatio,
		sx: window.scrollX,
		sy: window.scrollY,
		smx: Math.max(0, document.documentElement.scrollWidth - window.innerWidth),
		smy: Math.max(0, document.documentElement.scrollHeight - window.innerHeight),
		pw: document.documentElement.scrollWidth,
		ph: document.documentElement.scrollHeight
	})`, &info))
	if err != nil {
		return nil, fmt.Errorf("page_info eval: %w", err)
	}

	return map[string]any{
		"url":   info.URL,
		"title": info.Title,
		"viewport": map[string]any{
			"width":              info.VW,
			"height":             info.VH,
			"device_pixel_ratio": info.DPR,
		},
		"scroll": map[string]any{
			"x":     info.ScrollX,
			"y":     info.ScrollY,
			"max_x": info.ScrollMaxX,
			"max_y": info.ScrollMaxY,
		},
		"page": map[string]any{
			"width":  info.PageW,
			"height": info.PageH,
		},
	}, nil
}

// FullPageScreenshot captures the entire page, not just the viewport,
// by using Page.captureScreenshot with captureBeyondViewport=true.
// Useful for short pages where the agent wants one-shot context.
// Returns PNG bytes.
func (m *Manager) FullPageScreenshot(ctx context.Context) ([]byte, error) {
	tabCtx, err := m.activeTabContext()
	if err != nil {
		return nil, err
	}
	actCtx, cancel := context.WithTimeout(tabCtx, m.cfg.ActionTimeout)
	defer cancel()

	var data []byte
	err = chromedp.Run(actCtx, chromedp.ActionFunc(func(c context.Context) error {
		b, err := page.CaptureScreenshot().
			WithFormat(page.CaptureScreenshotFormatPng).
			WithCaptureBeyondViewport(true).
			Do(c)
		if err != nil {
			return err
		}
		data = b
		return nil
	}))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// JPEGScreenshot captures the viewport as JPEG at the requested
// quality (1-100). Smaller than PNG so it's the right choice for the
// live-stream broadcaster where we emit many frames per second. PNG
// stays the default for one-off snapshots because it's lossless and
// more useful to vision LLMs.
func (m *Manager) JPEGScreenshot(ctx context.Context, quality int) ([]byte, error) {
	tabCtx, err := m.activeTabContext()
	if err != nil {
		return nil, err
	}
	if quality < 1 {
		quality = 60
	}
	if quality > 100 {
		quality = 100
	}
	actCtx, cancel := context.WithTimeout(tabCtx, m.cfg.ActionTimeout)
	defer cancel()
	var data []byte
	err = chromedp.Run(actCtx, chromedp.ActionFunc(func(c context.Context) error {
		q := int64(quality)
		b, err := page.CaptureScreenshot().
			WithFormat(page.CaptureScreenshotFormatJpeg).
			WithQuality(q).
			Do(c)
		if err != nil {
			return err
		}
		data = b
		return nil
	}))
	return data, err
}

// WaitForLoad polls document.readyState until "complete" or timeout.
// Unlike WaitIdle (which watches network), this is the "page is
// structurally ready to interact with" check that matches harness
// wait_for_load semantics.
func (m *Manager) WaitForReadyState(ctx context.Context, timeout time.Duration) error {
	tabCtx, err := m.activeTabContext()
	if err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		actCtx, cancel := context.WithTimeout(tabCtx, 2*time.Second)
		var state string
		err := chromedp.Run(actCtx,
			chromedp.Evaluate(`document.readyState`, &state))
		cancel()
		if err == nil && state == "complete" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("readyState != complete within %s", timeout)
}

// activeTabContext returns the context for the currently active tab.
// Kept private — callers don't need a raw CDP handle.
func (m *Manager) activeTabContext() (context.Context, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return nil, fmt.Errorf("browser not running; call Start() first")
	}
	if m.activeTab != "" {
		if tc, ok := m.tabs[m.activeTab]; ok {
			return tc, nil
		}
	}
	// Fall back to the top-level context if no tab is explicitly
	// active. chromedp treats the original browser context as the
	// default target.
	if m.ctx != nil {
		return m.ctx, nil
	}
	return nil, fmt.Errorf("no active tab")
}
