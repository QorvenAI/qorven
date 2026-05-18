// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cdp

import (
	"context"
	"log/slog"
	"sync"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// inject.go — Script injection with deduplication per execution context.
// Ensures helper scripts are registered once and evaluated per context.

// ScriptInjector manages JavaScript injection into CDP sessions.
type ScriptInjector struct {
	mu         sync.Mutex
	registered map[string]bool            // key → registered for new documents
	evaluated  map[string]map[string]bool // key → set of context tokens
}

func NewScriptInjector() *ScriptInjector {
	return &ScriptInjector{
		registered: make(map[string]bool),
		evaluated:  make(map[string]map[string]bool),
	}
}

// Inject ensures a script is registered for new documents and evaluated in the current context.
// key is a unique identifier for the script (e.g., "highlight-helper").
// source is the JavaScript code to inject.
func (si *ScriptInjector) Inject(ctx context.Context, key, source string) error {
	si.mu.Lock()
	defer si.mu.Unlock()

	// Register for future navigations (once per key)
	if !si.registered[key] {
		if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(source).Do(ctx)
			return err
		})); err != nil {
			slog.Warn("cdp.inject: register failed", "key", key, "err", err)
		} else {
			si.registered[key] = true
		}
	}

	// Check if already evaluated in this context
	ctxToken := "default"
	ctxSet, ok := si.evaluated[key]
	if !ok {
		ctxSet = make(map[string]bool)
		si.evaluated[key] = ctxSet
	}
	if ctxSet[ctxToken] { return nil }

	// Evaluate in current context
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, _, err := runtime.Evaluate(source).Do(ctx)
		return err
	})); err != nil {
		slog.Warn("cdp.inject: evaluate failed", "key", key, "err", err)
		return err
	}

	ctxSet[ctxToken] = true
	return nil
}

// InjectInContext evaluates a script in a specific execution context (for iframes).
func (si *ScriptInjector) InjectInContext(ctx context.Context, key, source string, executionContextID int64) error {
	si.mu.Lock()
	defer si.mu.Unlock()

	token := contextToken(executionContextID)
	ctxSet, ok := si.evaluated[key]
	if !ok {
		ctxSet = make(map[string]bool)
		si.evaluated[key] = ctxSet
	}
	if ctxSet[token] { return nil }

	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		p := runtime.Evaluate(source)
		if executionContextID > 0 {
			p = p.WithContextID(runtime.ExecutionContextID(executionContextID))
		}
		_, _, err := p.Do(ctx)
		return err
	})); err != nil {
		return err
	}

	ctxSet[token] = true
	return nil
}

// Reset clears all injection state (call on navigation or session disconnect).
func (si *ScriptInjector) Reset() {
	si.mu.Lock()
	defer si.mu.Unlock()
	si.registered = make(map[string]bool)
	si.evaluated = make(map[string]map[string]bool)
}

func contextToken(id int64) string {
	if id == 0 { return "default" }
	return string(rune('0' + id))
}
