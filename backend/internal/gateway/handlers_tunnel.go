// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/tunnel"
)

// defaultPublicPort is the restricted public listener's port when unset.
const defaultPublicPort = 8487

func (gw *Gateway) publicPort() int {
	if gw.cfg.Tunnel.PublicPort > 0 {
		return gw.cfg.Tunnel.PublicPort
	}
	return defaultPublicPort
}

// buildPublicMux is the ONLY router exposed through the tunnel. The admin API,
// admin UI, and WebSocket endpoints are deliberately absent — "what's public"
// is enforced by which routes exist here, not by request filtering, so the
// admin backend is unreachable through the tunnel by construction.
//
// Item 5 (external-facing apps) registers declared-public app surfaces here.
func (gw *Gateway) buildPublicMux() chi.Router {
	r := chi.NewRouter()
	r.Use(PathTraversalGuard)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","surface":"public"}`))
	})

	// Placeholder landing.
	r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Qorven public surface. No app is published here yet."))
	})

	// External-facing app surfaces (default-deny; only published + public).
	gw.mountPublicApps(r)

	return r
}

// startPublicListener brings up the restricted public listener (idempotent).
// Binds 127.0.0.1 only — reachable from the tunnel process (localhost), never
// directly from the LAN. Binds synchronously so the port is guaranteed ready
// before the tunnel provider tries to connect to it.
func (gw *Gateway) startPublicListener() error {
	gw.publicServerMu.Lock()
	defer gw.publicServerMu.Unlock()
	if gw.publicServer != nil {
		return nil // already running
	}
	addr := fmt.Sprintf("127.0.0.1:%d", gw.publicPort())
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("bind public listener %s: %w", addr, err)
	}
	srv := &http.Server{
		Handler:     gw.buildPublicMux(),
		ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 120 * time.Second,
	}
	gw.publicServer = srv
	go func() {
		slog.Info("tunnel.public_listener.start", "addr", addr)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Warn("tunnel.public_listener.error", "error", err)
		}
	}()
	return nil
}

func (gw *Gateway) stopPublicListener() {
	gw.publicServerMu.Lock()
	srv := gw.publicServer
	gw.publicServer = nil
	gw.publicServerMu.Unlock()
	if srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// enableTunnel starts the public listener and the chosen tunnel provider
// pointing at it. provider is "cloudflare" (quick tunnel) or "tailscale".
func (gw *Gateway) enableTunnel(provider string) error {
	if err := gw.startPublicListener(); err != nil {
		return err
	}
	localURL := fmt.Sprintf("http://127.0.0.1:%d", gw.publicPort())

	var p tunnel.Provider
	switch provider {
	case "tailscale":
		p = tunnel.NewTailscaleProvider(gw.publicPort())
	case "cloudflare", "":
		bin, err := tunnel.EnsureCloudflared(context.Background())
		if err != nil {
			return fmt.Errorf("cloudflared unavailable: %w", err)
		}
		p = tunnel.NewCloudflareProvider(bin)
	default:
		return fmt.Errorf("unknown tunnel provider %q", provider)
	}
	return gw.tunnelMgr.Enable(p, localURL)
}

func (gw *Gateway) disableTunnel() error {
	err := gw.tunnelMgr.Disable()
	gw.stopPublicListener()
	return err
}

// ─── Admin API ──────────────────────────────────────────────────────────────

// tunnelAdminOK enforces that the caller is an authenticated admin. Opening an
// internet-facing tunnel is a privileged infra action — never allow a regular
// user, service account, or agent session to do it.
func (gw *Gateway) tunnelAdminOK(w http.ResponseWriter, r *http.Request) bool {
	u := userFromContext(r.Context())
	if u == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return false
	}
	if u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "admin role required", "code": "admin_only"})
		return false
	}
	return true
}

// GET /v1/tunnel/status
func (gw *Gateway) handleTunnelStatus(w http.ResponseWriter, r *http.Request) {
	if !gw.tunnelAdminOK(w, r) {
		return
	}
	st := gw.tunnelMgr.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      st,
		"public_port": gw.publicPort(),
	})
}

// POST /v1/tunnel/enable  {provider}
func (gw *Gateway) handleTunnelEnable(w http.ResponseWriter, r *http.Request) {
	if !gw.tunnelAdminOK(w, r) {
		return
	}
	var body struct {
		Provider string `json:"provider"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := gw.enableTunnel(body.Provider); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": gw.tunnelMgr.Status()})
}

// POST /v1/tunnel/disable
func (gw *Gateway) handleTunnelDisable(w http.ResponseWriter, r *http.Request) {
	if !gw.tunnelAdminOK(w, r) {
		return
	}
	if err := gw.disableTunnel(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": gw.tunnelMgr.Status()})
}
