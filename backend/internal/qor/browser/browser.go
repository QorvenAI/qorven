// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package browser

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"os"
	"github.com/chromedp/chromedp"
)

// browser.go — Browser session manager using chromedp (Chrome DevTools Protocol).

// Manager manages browser sessions, pages, and lifecycle.
type Manager struct {
	mu          sync.Mutex
	cfg         Config
	ctx         context.Context
	cancel      context.CancelFunc
	allocCancel context.CancelFunc
	tabs        map[string]context.Context // targetID → tab context
	activeTab   string
	running     bool
	startedAt   time.Time
	console     []ConsoleMessage
	logger      *slog.Logger
	// Live-stream state (see live_stream.go). Guarded by liveMu,
	// NOT by mu — we don't want a stream tick to contend with
	// normal browser operations.
	liveMu    sync.Mutex
	liveState *streamState
}

// New creates a browser manager with the given config.
func New(cfg Config) *Manager {
	if cfg.MaxPages <= 0 { cfg.MaxPages = 5 }
	if cfg.ActionTimeout <= 0 { cfg.ActionTimeout = 30 * time.Second }
	if cfg.IdleTimeout <= 0 { cfg.IdleTimeout = 5 * time.Minute }
	return &Manager{
		cfg:    cfg,
		tabs:   make(map[string]context.Context),
		logger: slog.Default(),
	}
}

// Start launches the browser process.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running { return nil }

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", m.cfg.Headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	// Find Chrome binary
	if p := os.Getenv("CHROME_PATH"); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	if m.cfg.UserAgent != "" {
		opts = append(opts, chromedp.UserAgent(m.cfg.UserAgent))
	}

	var allocCtx context.Context
	if m.cfg.RemoteURL != "" {
		allocCtx, m.allocCancel = chromedp.NewRemoteAllocator(ctx, m.cfg.RemoteURL)
	} else {
		allocCtx, m.allocCancel = chromedp.NewExecAllocator(ctx, opts...)
	}

	m.ctx, m.cancel = chromedp.NewContext(allocCtx)
	m.running = true
	m.startedAt = time.Now()

	// Navigate to blank page to ensure browser is alive
	if err := chromedp.Run(m.ctx, chromedp.Navigate("about:blank")); err != nil {
		m.running = false
		return fmt.Errorf("browser.start: %w", err)
	}

	m.logger.Info("browser.started", "headless", m.cfg.Headless, "remote", m.cfg.RemoteURL != "")
	return nil
}

// Stop shuts down the browser.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running { return nil }

	if m.cancel != nil { m.cancel() }
	if m.allocCancel != nil { m.allocCancel() }
	m.running = false
	m.tabs = make(map[string]context.Context)
	m.activeTab = ""
	m.logger.Info("browser.stopped")
	return nil
}

// IsRunning returns whether the browser is active.
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Status returns current browser state.
func (m *Manager) Status() StatusInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := StatusInfo{
		Running:  m.running,
		Headless: m.cfg.Headless,
		TabCount: len(m.tabs),
		ActiveTab: m.activeTab,
	}
	if m.running { s.Uptime = time.Since(m.startedAt).Round(time.Second).String() }
	return s
}

// Context returns the browser's chromedp context (for direct CDP calls).
func (m *Manager) Context() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ctx
}
