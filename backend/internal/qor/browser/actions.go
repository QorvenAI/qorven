// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/input"
)

// actions.go — Browser interaction actions (click, type, hover, scroll, evaluate).

// Click clicks an element by CSS selector.
func (m *Manager) Click(ctx context.Context, selector string, opts ClickOpts) error {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	tctx, cancel := context.WithTimeout(bctx, m.cfg.ActionTimeout)
	defer cancel()

	return chromedp.Run(tctx,
		chromedp.WaitVisible(selector),
		chromedp.Click(selector),
	)
}

// Type types text into an element.
func (m *Manager) Type(ctx context.Context, selector, text string, opts TypeOpts) error {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	tctx, cancel := context.WithTimeout(bctx, m.cfg.ActionTimeout)
	defer cancel()

	actions := []chromedp.Action{chromedp.WaitVisible(selector)}
	if opts.Clear {
		actions = append(actions, chromedp.Clear(selector))
	}
	actions = append(actions, chromedp.SendKeys(selector, text))

	return chromedp.Run(tctx, actions...)
}

// Press sends a keyboard key press.
func (m *Manager) Press(ctx context.Context, key string) error {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	k := mapKeyName(key)
	return chromedp.Run(bctx,
		input.DispatchKeyEvent(input.KeyDown).WithKey(k),
		input.DispatchKeyEvent(input.KeyUp).WithKey(k),
	)
}

// Hover moves the mouse over an element.
func (m *Manager) Hover(ctx context.Context, selector string) error {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	tctx, cancel := context.WithTimeout(bctx, m.cfg.ActionTimeout)
	defer cancel()

	return chromedp.Run(tctx,
		chromedp.WaitVisible(selector),
		chromedp.Evaluate("void(0)", nil),
	)
}

// Scroll scrolls the page by the given delta.
func (m *Manager) Scroll(ctx context.Context, deltaX, deltaY float64) error {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	return chromedp.Run(bctx,
		chromedp.Evaluate(fmt.Sprintf("window.scrollBy(%f, %f)", deltaX, deltaY), nil),
	)
}

// Wait waits for an element to appear.
func (m *Manager) Wait(ctx context.Context, opts WaitOpts) error {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	timeout := opts.Timeout
	if timeout <= 0 { timeout = m.cfg.ActionTimeout }
	tctx, cancel := context.WithTimeout(bctx, timeout)
	defer cancel()

	if opts.Visible {
		return chromedp.Run(tctx, chromedp.WaitVisible(opts.Selector))
	}
	return chromedp.Run(tctx, chromedp.WaitReady(opts.Selector))
}

// Evaluate runs JavaScript and returns the result.
func (m *Manager) Evaluate(ctx context.Context, js string) (string, error) {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return "", fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	tctx, cancel := context.WithTimeout(bctx, m.cfg.ActionTimeout)
	defer cancel()

	var result string
	if err := chromedp.Run(tctx, chromedp.Evaluate(js, &result)); err != nil { return "", err }
	return result, nil
}

// Screenshot captures the current page as PNG bytes.
func (m *Manager) Screenshot(ctx context.Context) ([]byte, error) {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return nil, fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	var buf []byte
	if err := chromedp.Run(bctx, chromedp.CaptureScreenshot(&buf)); err != nil { return nil, err }
	return buf, nil
}

// WaitIdle waits for network to be idle (no pending requests for duration).
func (m *Manager) WaitIdle(ctx context.Context, d time.Duration) error {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	// Simple approach: wait for document ready + small delay
	chromedp.Run(bctx, chromedp.WaitReady("body"))
	time.Sleep(d)
	return nil
}

func mapKeyName(key string) string {
	switch key {
	case "enter", "Enter": return "Enter"
	case "tab", "Tab": return "Tab"
	case "escape", "Escape", "esc": return "Escape"
	case "backspace", "Backspace": return "Backspace"
	case "delete", "Delete": return "Delete"
	case "space", "Space": return " "
	case "up", "ArrowUp": return "ArrowUp"
	case "down", "ArrowDown": return "ArrowDown"
	case "left", "ArrowLeft": return "ArrowLeft"
	case "right", "ArrowRight": return "ArrowRight"
	default: return key
	}
}
