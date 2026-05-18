// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cdp

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// input.go — Keyboard and mouse input helpers for precise CDP control.

// KeyCombo sends a key combination (e.g., Ctrl+C, Ctrl+Shift+P).
func KeyCombo(ctx context.Context, modifiers int, key string) error {
	return chromedp.Run(ctx,
		input.DispatchKeyEvent(input.KeyDown).WithKey(key).WithModifiers(input.Modifier(modifiers)),
		input.DispatchKeyEvent(input.KeyUp).WithKey(key).WithModifiers(input.Modifier(modifiers)),
	)
}

// Modifier constants for KeyCombo.
const (
	ModNone  = 0
	ModAlt   = 1
	ModCtrl  = 2
	ModMeta  = 4
	ModShift = 8
)

// DragDrop performs a drag from (x1,y1) to (x2,y2).
func DragDrop(ctx context.Context, x1, y1, x2, y2 float64) error {
	return chromedp.Run(ctx,
		input.DispatchMouseEvent(input.MousePressed, x1, y1).WithButton(input.Left),
		input.DispatchMouseEvent(input.MouseMoved, x2, y2),
		input.DispatchMouseEvent(input.MouseReleased, x2, y2).WithButton(input.Left),
	)
}

// DoubleClick performs a double-click at coordinates.
func DoubleClick(ctx context.Context, x, y float64) error {
	return chromedp.Run(ctx,
		input.DispatchMouseEvent(input.MousePressed, x, y).WithButton(input.Left).WithClickCount(2),
		input.DispatchMouseEvent(input.MouseReleased, x, y).WithButton(input.Left).WithClickCount(2),
	)
}

// RightClick performs a right-click at coordinates.
func RightClick(ctx context.Context, x, y float64) error {
	return chromedp.Run(ctx,
		input.DispatchMouseEvent(input.MousePressed, x, y).WithButton(input.Right).WithClickCount(1),
		input.DispatchMouseEvent(input.MouseReleased, x, y).WithButton(input.Right).WithClickCount(1),
	)
}

// TypeWithDelay types text with a delay between each character (ms).
func TypeWithDelay(ctx context.Context, text string, delayMs int) error {
	for _, ch := range text {
		s := string(ch)
		if err := chromedp.Run(ctx,
			input.DispatchKeyEvent(input.KeyDown).WithText(s),
			input.DispatchKeyEvent(input.KeyUp).WithText(s),
		); err != nil {
			return fmt.Errorf("cdp.typeDelay: %w", err)
		}
		if delayMs > 0 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}
	}
	return nil
}

// SelectAll sends Ctrl+A to select all text in the focused element.
func SelectAll(ctx context.Context) error {
	return KeyCombo(ctx, ModCtrl, "a")
}

// Copy sends Ctrl+C.
func Copy(ctx context.Context) error { return KeyCombo(ctx, ModCtrl, "c") }

// Paste sends Ctrl+V.
func Paste(ctx context.Context) error { return KeyCombo(ctx, ModCtrl, "v") }

// Undo sends Ctrl+Z.
func Undo(ctx context.Context) error { return KeyCombo(ctx, ModCtrl, "z") }
