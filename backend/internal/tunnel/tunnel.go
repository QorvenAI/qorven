// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package tunnel provides a provider abstraction for exposing a chosen local
// URL to the internet, plus a thread-safe manager that supervises a single
// active provider. The first supported provider is a Cloudflare quick tunnel
// (no account required, random *.trycloudflare.com URL).
package tunnel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// State is the lifecycle state of the active tunnel.
type State string

const (
	// StateOff means no tunnel is active.
	StateOff State = "off"
	// StateStarting means a tunnel is being established.
	StateStarting State = "starting"
	// StateConnected means a tunnel is up and the public URL is known.
	StateConnected State = "connected"
	// StateError means the tunnel failed to start or terminated unexpectedly.
	StateError State = "error"
)

// Status is a snapshot of the manager's current tunnel state.
type Status struct {
	Provider  string `json:"provider"`   // "cloudflare" | "tailscale" | ""
	State     State  `json:"state"`
	PublicURL string `json:"public_url"`     // the external URL once connected
	Error     string `json:"error,omitempty"`
	Since     string `json:"since,omitempty"` // RFC3339; set when state changes
}

// Provider starts/stops a tunnel to a local URL and reports the public URL.
type Provider interface {
	Name() string
	// Start launches the tunnel pointing at localURL (e.g. "http://localhost:8487").
	// It blocks until the public URL is known or ctx is cancelled / it errors.
	// Returns the public URL.
	Start(ctx context.Context, localURL string) (publicURL string, err error)
	Stop() error
}

// Manager supervises a single active provider. Thread-safe.
type Manager struct {
	mu     sync.Mutex
	status Status
	active Provider
	cancel context.CancelFunc
	nowFn  func() time.Time
}

// NewManager returns a Manager in the StateOff state.
func NewManager() *Manager {
	return &Manager{
		status: Status{State: StateOff},
		nowFn:  time.Now,
	}
}

// Status returns a copy of the current status. Safe for concurrent use.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// setStatus mutates status while holding the lock. Callers must hold m.mu.
func (m *Manager) setStatus(provider string, state State, publicURL, errMsg string) {
	m.status = Status{
		Provider:  provider,
		State:     state,
		PublicURL: publicURL,
		Error:     errMsg,
		Since:     m.nowFn().UTC().Format(time.RFC3339),
	}
}

// Enable starts the given provider pointing at localURL. If a provider is
// already active it is stopped first. The provider's Start call runs in a
// goroutine; the state moves StateStarting → StateConnected on success or
// StateError on failure.
func (m *Manager) Enable(p Provider, localURL string) error {
	if p == nil {
		return fmt.Errorf("tunnel: nil provider")
	}

	m.mu.Lock()
	// If already active, stop the old one first.
	if m.active != nil {
		m.mu.Unlock()
		if err := m.Disable(); err != nil {
			slog.Warn("tunnel.disable_before_enable_failed", "error", err)
		}
		m.mu.Lock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.active = p
	m.cancel = cancel
	m.setStatus(p.Name(), StateStarting, "", "")
	m.mu.Unlock()

	slog.Info("tunnel.starting", "provider", p.Name(), "local_url", localURL)

	go func() {
		publicURL, err := p.Start(ctx, localURL)

		m.mu.Lock()
		defer m.mu.Unlock()
		// If we were disabled/replaced while starting, ignore the result.
		if m.active != p {
			return
		}
		if err != nil {
			m.setStatus(p.Name(), StateError, "", err.Error())
			slog.Error("tunnel.error", "provider", p.Name(), "error", err)
			return
		}
		m.setStatus(p.Name(), StateConnected, publicURL, "")
		slog.Info("tunnel.started", "provider", p.Name(), "url", publicURL)
	}()

	return nil
}

// Disable cancels the active provider's context, calls Stop, and returns to
// StateOff. It is safe to call when nothing is active.
func (m *Manager) Disable() error {
	m.mu.Lock()
	active := m.active
	cancel := m.cancel
	m.active = nil
	m.cancel = nil
	m.setStatus("", StateOff, "", "")
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if active == nil {
		return nil
	}

	slog.Info("tunnel.stopping", "provider", active.Name())
	if err := active.Stop(); err != nil {
		slog.Warn("tunnel.stop_failed", "provider", active.Name(), "error", err)
		return err
	}
	slog.Info("tunnel.stopped", "provider", active.Name())
	return nil
}
