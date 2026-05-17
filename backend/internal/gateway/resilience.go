// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"nhooyr.io/websocket"
)

// WS heartbeat constants — tuned for the common "laptop on residential
// wifi behind a NAT" case. 20s is short enough that a dead connection
// is detected well before the OS-level TCP keepalive (2h default),
// long enough that we don't flood a browser with traffic when it's
// actually fine. 10s timeout gives slow mobile links room to ack.
const (
	wsPingInterval = 20 * time.Second
	wsPingTimeout  = 10 * time.Second
)

// runWSHeartbeat starts a background goroutine that pings the given
// WS conn on a fixed interval. When ping fails (ie peer is dead), it
// cancels the shared context, which drops every other goroutine wired
// to the same conn — writer, reader, piping loops, etc.
//
// The caller MUST pass a cancellable context that wraps the conn's
// lifecycle. When the caller's teardown runs, canceling that context
// stops the heartbeat too — no separate stop channel needed.
//
// Non-blocking: returns immediately. Safe to call exactly once per
// conn at accept time; calling twice would double-ping.
func runWSHeartbeat(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, tag string) {
	go func() {
		ticker := time.NewTicker(wsPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pctx, pcancel := context.WithTimeout(ctx, wsPingTimeout)
				err := conn.Ping(pctx)
				pcancel()
				if err != nil {
					slog.Debug("ws.ping_failed", "conn", tag, "error", err)
					cancel()
					return
				}
			}
		}
	}()
}

// signalNotify installs SIGINT/SIGTERM handlers on the given channel.
// Pulled into a package-private helper so the shutdown wiring in
// gateway.go stays readable and this package centralises the syscall
// import.
func signalNotify(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
}

// resilience.go — port probing, runtime discovery, and graceful shutdown
// helpers. Kept together because they interlock: the port we actually
// bind to (which may differ from the configured port if it was busy) is
// what we write to runtime.json, and it's also what /__qorven_runtime
// reports back to the web client.
//
// Design goals:
//   • A user running `qorven start` with port 4200 occupied must NOT
//     see a crash. The server should walk +1..+10 and bind somewhere.
//   • The web client must be able to discover the actual bound port
//     without the user editing NEXT_PUBLIC_API_URL.
//   • SIGTERM/SIGINT drain in-flight requests instead of killing
//     streaming agent responses mid-token.
//
// runtime.json lives at ~/.qorven/runtime.json and is rewritten every
// time the gateway binds. It's read by the web client's port-discovery
// endpoint at `/__qorven_runtime`.

const portProbeRange = 10 // scan configured+1 .. configured+10

// bindListener probes `preferred` first, then preferred+1..preferred+N.
// Returns the net.Listener that succeeded plus the concrete addr used.
// The caller hands this to http.Server.Serve() — we deliberately do NOT
// use ListenAndServe() because that hides which port we actually got.
func bindListener(preferred string) (net.Listener, string, error) {
	host, portStr, err := net.SplitHostPort(preferred)
	if err != nil {
		return nil, "", fmt.Errorf("invalid listen addr %q: %w", preferred, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, "", fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	var lastErr error
	for offset := 0; offset <= portProbeRange; offset++ {
		candidate := net.JoinHostPort(host, strconv.Itoa(port+offset))
		ln, err := net.Listen("tcp", candidate)
		if err == nil {
			if offset > 0 {
				slog.Warn("port bind: preferred busy, using fallback",
					"preferred", preferred, "actual", candidate, "offset", offset,
					"hint", "the web client will discover this via /__qorven_runtime")
			}
			return ln, candidate, nil
		}
		lastErr = err
		// Only walk on EADDRINUSE. Permission-denied or other errors
		// won't get better on an incremented port.
		if !isAddrInUse(err) {
			break
		}
	}
	return nil, "", fmt.Errorf("could not bind on %s or next %d ports: %w",
		preferred, portProbeRange, lastErr)
}

func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	var syscallErr *os.SyscallError
	if errors.As(err, &syscallErr) {
		if errno, ok := syscallErr.Err.(syscall.Errno); ok {
			return errno == syscall.EADDRINUSE
		}
	}
	// Fallback for wrapped errors that don't preserve the syscall type.
	msg := err.Error()
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "bind: address already in use")
}

// runtimeInfo is the JSON payload written to ~/.qorven/runtime.json and
// served at /__qorven_runtime. Kept minimal on purpose — consumers only
// need to know where to connect.
type runtimeInfo struct {
	APIAddr   string    `json:"api_addr"`   // e.g. "127.0.0.1:4201"
	APIPort   int       `json:"api_port"`   // 4201 (convenience)
	WebAddr   string    `json:"web_addr,omitempty"`
	WebPort   int       `json:"web_port,omitempty"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Version   string    `json:"version"`
}

// evictStalePID reads runtime.json and, if the recorded PID is still alive
// and is NOT the current process, sends it SIGTERM and waits up to 5 s for
// it to exit. This ensures only one qorven instance runs at a time on any
// platform — dev machines, Docker, customer VMs — without relying on systemd.
func evictStalePID() {
	data, err := os.ReadFile(runtimePath())
	if err != nil {
		return // no runtime.json → nothing to evict
	}
	var info runtimeInfo
	if err := json.Unmarshal(data, &info); err != nil || info.PID <= 0 {
		return
	}
	if info.PID == os.Getpid() {
		return // that's us — shouldn't happen, but be safe
	}
	proc, err := os.FindProcess(info.PID)
	if err != nil {
		return // process doesn't exist
	}
	// On Unix, FindProcess always succeeds; signal(0) checks liveness.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return // process is already gone
	}
	slog.Warn("single_instance.evicting_stale", "pid", info.PID)
	_ = proc.Signal(syscall.SIGTERM)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			slog.Info("single_instance.stale_exited", "pid", info.PID)
			return
		}
	}
	// Still alive after 5 s — escalate to SIGKILL.
	slog.Warn("single_instance.force_killing", "pid", info.PID)
	_ = proc.Signal(syscall.SIGKILL)
	time.Sleep(200 * time.Millisecond)
}

func runtimePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// Fall back to /tmp — better than crashing when HOME is unset
		// (e.g. `sudo systemctl start qorven` without User= directive).
		return filepath.Join(os.TempDir(), "qorven-runtime.json")
	}
	return filepath.Join(home, ".qorven", "runtime.json")
}

// writeRuntimeInfo persists bound addresses so the web client and any
// CLI tooling can discover them. Failures are warnings, not fatals —
// the server still works without runtime.json, discovery just falls
// back to NEXT_PUBLIC_API_URL.
func writeRuntimeInfo(info runtimeInfo) {
	path := runtimePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		slog.Warn("runtime.json: mkdir failed", "path", path, "error", err)
		return
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		slog.Warn("runtime.json: marshal failed", "error", err)
		return
	}
	// 0o600 — file contains the port the local API listens on, which is
	// not a secret, but stay conservative.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		slog.Warn("runtime.json: write failed", "path", path, "error", err)
		return
	}
	slog.Info("runtime.json written", "path", path, "api_addr", info.APIAddr)
}

// portFromAddr extracts the numeric port from "host:port". Returns 0
// on parse failure — safe to embed in runtime.json as a sentinel.
func portFromAddr(addr string) int {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(p)
	return n
}

// handleRuntimeInfo serves /__qorven_runtime — the web client calls
// this once at boot to learn the actual backend port. Unauthenticated
// by design: the only thing it reveals is the port we're already
// listening on, and anyone who can reach us already knows that.
func (gw *Gateway) handleRuntimeInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	// CORS for dev: web at :3000 polls backend at :4200. Same-origin in
	// prod means this header is harmless there.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	info := runtimeInfo{
		APIAddr:   gw.server.Addr,
		APIPort:   portFromAddr(gw.server.Addr),
		PID:       os.Getpid(),
		StartedAt: gw.startTime,
		Version:   "dev",
	}
	if gw.cfg != nil && gw.cfg.Server.WebListen != "" {
		info.WebAddr = gw.cfg.Server.WebListen
		info.WebPort = portFromAddr(gw.cfg.Server.WebListen)
	}
	_ = json.NewEncoder(w).Encode(info)
}

// handleCADownload serves the local CA certificate for download.
// Unauthenticated — it's a public cert, not a secret. Browsers on
// other machines hit this to get the CA so they can install it and
// stop showing the "connection not private" warning.
// Content-Disposition: attachment so browsers save rather than render.
func (gw *Gateway) handleCADownload(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	caPath := filepath.Join(home, ".qorven", "tls", "ca.pem")
	data, err := os.ReadFile(caPath)
	if err != nil {
		http.Error(w, "CA certificate not found — run: qorven tls generate", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="qorven-ca.pem"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// handleLivez — process is alive. Never touches the DB. Used by k8s
// liveness probes and docker healthchecks — must stay fast and cheap,
// because a slow /livez triggers a pod restart.
func (gw *Gateway) handleLivez(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"alive"}`))
}

// handleReadyz — ready to serve real traffic. Checks the DB connection
// and the voice manager (if configured). Returns 503 if any dependency
// is down, which tells k8s to pull us out of the service until we
// recover. Distinct from /livez so a transient DB blip doesn't restart
// the whole process.
func (gw *Gateway) handleReadyz(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{}
	ready := true

	// DB ping — short timeout so a hung DB doesn't hang the probe.
	if gw.db != nil && gw.db.Pool != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := gw.db.Pool.Ping(ctx); err != nil {
			checks["database"] = "unavailable: " + err.Error()
			ready = false
		} else {
			checks["database"] = "ok"
		}
	} else {
		checks["database"] = "not_configured"
	}

	// Voice manager — only checked if one is configured. Not being
	// configured is NOT an error; voice is optional.
	if gw.voiceMgr != nil {
		checks["voice"] = "ok"
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ready":  ready,
		"checks": checks,
	})
}
