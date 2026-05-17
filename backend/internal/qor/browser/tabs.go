// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// tabs.go — Tab management (list, open, close, switch).

// ListTabs returns info about all open tabs.
func (m *Manager) ListTabs(ctx context.Context) ([]TabInfo, error) {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return nil, fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	targets, err := chromedp.Targets(bctx)
	if err != nil { return nil, fmt.Errorf("browser.tabs: %w", err) }

	var tabs []TabInfo
	for _, t := range targets {
		if t.Type != "page" { continue }
		tabs = append(tabs, TabInfo{
			TargetID: string(t.TargetID),
			URL:      t.URL,
			Title:    t.Title,
			Active:   string(t.TargetID) == m.activeTab,
		})
	}
	return tabs, nil
}

// OpenTab opens a new tab with the given URL.
func (m *Manager) OpenTab(ctx context.Context, url string) (string, error) {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return "", fmt.Errorf("browser not running") }
	if len(m.tabs) >= m.cfg.MaxPages {
		m.mu.Unlock()
		return "", fmt.Errorf("max tabs (%d) reached", m.cfg.MaxPages)
	}
	bctx := m.ctx
	m.mu.Unlock()

	newCtx, _ := chromedp.NewContext(bctx)
	if err := chromedp.Run(newCtx, chromedp.Navigate(url)); err != nil {
		return "", fmt.Errorf("browser.open: %w", err)
	}

	tid := chromedp.FromContext(newCtx).Target.TargetID
	targetID := string(tid)

	m.mu.Lock()
	m.tabs[targetID] = newCtx
	m.activeTab = targetID
	m.mu.Unlock()

	return targetID, nil
}

// CloseTab closes a tab by target ID.
func (m *Manager) CloseTab(ctx context.Context, targetID string) error {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return fmt.Errorf("browser not running") }
	bctx := m.ctx
	delete(m.tabs, targetID)
	if m.activeTab == targetID { m.activeTab = "" }
	m.mu.Unlock()

	return chromedp.Run(bctx, target.CloseTarget(target.ID(targetID)))
}

// SwitchTab makes a tab the active tab.
func (m *Manager) SwitchTab(targetID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tabs[targetID]; !ok { return fmt.Errorf("tab %s not found", targetID) }
	m.activeTab = targetID
	return nil
}
