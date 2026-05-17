// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package browser

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// Stale-session recovery.
//
// Chrome's CDP returns "Session with given id not found" in two
// common scenarios: (a) the target tab was closed (user action or
// navigation chain that ended at about:blank), (b) the browser was
// restarted and we're still holding the old target ID. A vanilla
// chromedp.Run then errors out and every subsequent call fails.
//
// Pattern lifted from browser-use/go-harnessless: on that specific
// error, reattach to any live non-internal target and retry the
// action once. If we still can't find a live target after reattach,
// give up honestly — something's catastrophically wrong.
//
// Callers wrap their chromedp.Run with RunWithRecovery(m, ctx, ...).

// staleSessionMarker is the substring chromedp/CDP uses for the
// "session gone" error. Matching on substring is brittle but there's
// no typed error — chromedp passes the CDP error through as a plain
// message. We double-check with a generic "session" fallback so a
// future rewording doesn't silently break recovery.
const staleSessionMarker = "Session with given id not found"

// isStaleSessionErr reports whether err is the "target detached"
// shape CDP returns after a tab closes or the browser restarts.
func isStaleSessionErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, staleSessionMarker) {
		return true
	}
	// Fallback: CDP sometimes phrases this as "No target with given id".
	if strings.Contains(msg, "No target with given id") {
		return true
	}
	// chromedp's own "target closed" surface.
	if strings.Contains(msg, "target closed") {
		return true
	}
	return false
}

// RunWithRecovery runs the given chromedp actions under the active
// tab context, and on stale-session failure attempts one automatic
// reattach-and-retry. Returns the original error if recovery also
// fails so callers can still observe the underlying cause.
//
// Call this instead of `chromedp.Run(tabCtx, actions...)` in hot
// paths that touch CDP. Cheap — one string check per invocation.
func (m *Manager) RunWithRecovery(ctx context.Context, actions ...chromedp.Action) error {
	tabCtx, err := m.activeTabContext()
	if err != nil {
		return err
	}
	if runErr := chromedp.Run(tabCtx, actions...); runErr != nil {
		if !isStaleSessionErr(runErr) {
			return runErr
		}
		slog.Warn("browser.recovery.stale_session", "error", runErr)
		// Try to find a live target and swap the active tab over.
		if reErr := m.reattachToFirstLiveTarget(ctx); reErr != nil {
			return fmt.Errorf("stale session and reattach failed: %w (original: %v)",
				reErr, runErr)
		}
		// Retry once on the new active tab.
		freshCtx, err := m.activeTabContext()
		if err != nil {
			return err
		}
		if retryErr := chromedp.Run(freshCtx, actions...); retryErr != nil {
			return fmt.Errorf("after reattach: %w", retryErr)
		}
		slog.Info("browser.recovery.succeeded")
		return nil
	}
	return nil
}

// reattachToFirstLiveTarget scans tabs and picks the first one that
// isn't a chrome:// or devtools:// internal page. The active-tab
// field is updated under lock so subsequent operations use the new
// target.
func (m *Manager) reattachToFirstLiveTarget(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running || m.ctx == nil {
		return fmt.Errorf("browser not running")
	}

	// Enumerate targets via the browser-level context. This list
	// reflects live Chrome state, not our in-memory tabs map (which
	// can drift if tabs closed without notifying us).
	targets, err := listChromeTargets(ctx, m.ctx)
	if err != nil {
		return fmt.Errorf("list targets: %w", err)
	}
	for _, t := range targets {
		if t.Type != "page" {
			continue
		}
		if strings.HasPrefix(t.URL, "chrome://") ||
			strings.HasPrefix(t.URL, "devtools://") ||
			strings.HasPrefix(t.URL, "chrome-extension://") {
			continue
		}
		// Build a new chromedp context pointed at this target and
		// make it our active tab.
		newTabCtx, _ := chromedp.NewContext(m.ctx, chromedp.WithTargetID(target.ID(t.TargetID)))
		m.tabs[t.TargetID] = newTabCtx
		m.activeTab = t.TargetID
		slog.Info("browser.recovery.reattached", "target", t.TargetID, "url", t.URL)
		return nil
	}
	return fmt.Errorf("no live page targets available for reattach")
}

// chromeTarget is our minimal view of a CDP Target.TargetInfo.
type chromeTarget struct {
	TargetID string
	Type     string
	URL      string
}

// listChromeTargets asks Chrome for its current target list. Uses
// chromedp.Targets which wraps Target.getTargets.
func listChromeTargets(ctx context.Context, browserCtx context.Context) ([]chromeTarget, error) {
	infos, err := chromedp.Targets(browserCtx)
	if err != nil {
		return nil, err
	}
	out := make([]chromeTarget, 0, len(infos))
	for _, t := range infos {
		out = append(out, chromeTarget{
			TargetID: string(t.TargetID),
			Type:     t.Type,
			URL:      t.URL,
		})
	}
	return out, nil
}
