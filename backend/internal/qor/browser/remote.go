// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package browser

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/chromedp/chromedp"
)

// remote.go — Connect to a remote browser (Chrome DevTools, Browserless, etc.)

// ConnectRemote connects to a remote Chrome instance via DevTools WebSocket URL.
func ConnectRemote(ctx context.Context, wsURL string) (*Manager, error) {
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, wsURL)
	bctx, cancel := chromedp.NewContext(allocCtx)

	// Test connection
	if err := chromedp.Run(bctx, chromedp.Navigate("about:blank")); err != nil {
		cancel()
		allocCancel()
		return nil, fmt.Errorf("browser.remote: connect failed: %w", err)
	}

	m := &Manager{
		cfg:         DefaultConfig(),
		ctx:         bctx,
		cancel:      cancel,
		allocCancel: allocCancel,
		tabs:        make(map[string]context.Context),
		running:     true,
		startedAt:   time.Now(),
		logger:      slog.Default(),
	}

	slog.Info("browser.remote: connected", "url", wsURL)
	return m, nil
}

// HealthCheck verifies a remote browser endpoint is reachable.
func HealthCheck(ctx context.Context, wsURL string) error {
	httpURL := wsURL
	// Convert ws:// to http:// for health check
	if len(httpURL) > 5 && httpURL[:5] == "ws://" {
		httpURL = "http://" + httpURL[5:]
	} else if len(httpURL) > 6 && httpURL[:6] == "wss://" {
		httpURL = "https://" + httpURL[6:]
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", httpURL+"/json/version", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil { return fmt.Errorf("browser.health: %w", err) }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("browser.health: HTTP %d", resp.StatusCode)
	}
	return nil
}
