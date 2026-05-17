// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package browser

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// page.go — Page navigation and management.

// Navigate opens a URL in the active tab.
func (m *Manager) Navigate(ctx context.Context, url string) error {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	return chromedp.Run(bctx, chromedp.Navigate(url))
}

// WaitForLoad waits for the page to finish loading.
func (m *Manager) WaitForLoad(ctx context.Context) error {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	return chromedp.Run(bctx, chromedp.WaitReady("body"))
}

// GetURL returns the current page URL.
func (m *Manager) GetURL(ctx context.Context) (string, error) {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return "", fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	var url string
	if err := chromedp.Run(bctx, chromedp.Location(&url)); err != nil { return "", err }
	return url, nil
}

// GetTitle returns the current page title.
func (m *Manager) GetTitle(ctx context.Context) (string, error) {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return "", fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	var title string
	if err := chromedp.Run(bctx, chromedp.Title(&title)); err != nil { return "", err }
	return title, nil
}

// GetHTML returns the page's outer HTML.
func (m *Manager) GetHTML(ctx context.Context) (string, error) {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return "", fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	var html string
	if err := chromedp.Run(bctx, chromedp.OuterHTML("html", &html)); err != nil { return "", err }
	return html, nil
}
