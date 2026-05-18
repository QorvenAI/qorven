// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// monitor.go — `qorven monitor` watches the running gateway and web UI,
// diagnoses outages, and tells the operator what's wrong and how to fix it.
//
// It's the CLI counterpart to the frontend's reconnect banner: when the
// frontend is down we can't show anything in the UI, so the CLI becomes
// the eyes.  When the backend is down the frontend can only show "backend
// unreachable"; `qorven monitor` probes deeper (DB, process, ports) and
// surfaces the real root cause.
//
// Flags:
//   --interval   poll interval (default 10s)
//   --once       probe once and exit (non-zero on any issue)

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var (
	monitorInterval time.Duration
	monitorOnce     bool
)

func init() {
	mc := &cobra.Command{
		Use:   "monitor",
		Short: "Watch gateway + frontend health and diagnose outages",
		Long: `Polls /livez, /readyz, and the web UI port on a fixed interval.

When something is down it diagnoses the root cause (process dead, DB
offline, port conflict) and prints a human-readable fix hint. Keeps
running until Ctrl-C, or exits after one probe with --once.

Exit codes (--once):
  0  everything healthy
  1  one or more services degraded or down`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMonitor()
		},
	}
	mc.Flags().DurationVar(&monitorInterval, "interval", 10*time.Second, "Poll interval")
	mc.Flags().BoolVar(&monitorOnce, "once", false, "Probe once and exit")
	rootCmd.AddCommand(mc)
}

// serviceState tracks the last known state so we only log on transitions.
type serviceState struct {
	apiAlive  *bool
	apiReady  *bool
	webAlive  *bool
}

func runMonitor() error {
	apiBase := cfg.Server
	if apiBase == "" {
		apiBase = "http://localhost:4200"
	}
	apiBase = strings.TrimRight(apiBase, "/")

	// Derive web UI URL from runtime.json if available.
	webBase := discoverWebURL(apiBase)

	fmt.Printf("  qorven monitor\n")
	fmt.Printf("  API:  %s\n", apiBase)
	fmt.Printf("  Web:  %s\n", webBase)
	fmt.Println()

	state := &serviceState{}
	issues := probeAndReport(apiBase, webBase, state)

	if monitorOnce {
		if issues > 0 {
			return fmt.Errorf("%d service(s) need attention", issues)
		}
		return nil
	}

	// Continuous mode — print header on first run, then only on change.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	ticker := time.NewTicker(monitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-sig:
			fmt.Println("\n  Monitor stopped.")
			return nil
		case <-ticker.C:
			probeAndReport(apiBase, webBase, state)
		}
	}
}

// probeAndReport runs all probes, prints changes, returns the count of issues.
func probeAndReport(apiBase, webBase string, state *serviceState) int {
	issues := 0
	ts := time.Now().Format("15:04:05")

	// ── Backend liveness ──────────────────────────────────────────────
	apiAlive, livezErr := probeLivez(apiBase)
	if changed(&state.apiAlive, apiAlive) {
		if apiAlive {
			monitorOK(ts, "Backend", "process alive ("+apiBase+")")
		} else {
			monitorFail(ts, "Backend", "process unreachable")
			printRecoveryHint("backend_down", apiBase)
		}
	}
	if !apiAlive {
		issues++
		_ = livezErr
	}

	// ── Backend readiness (DB) ────────────────────────────────────────
	if apiAlive {
		apiReady, dbStatus, readyzErr := probeReadyz(apiBase)
		if changed(&state.apiReady, apiReady) {
			if apiReady {
				monitorOK(ts, "Database", "ok")
			} else {
				monitorFail(ts, "Database", "unavailable — "+dbStatus)
				printRecoveryHint("db_down", apiBase)
			}
		}
		if !apiReady {
			issues++
			_ = readyzErr
		}
	}

	// ── Web UI ────────────────────────────────────────────────────────
	if webBase != "" {
		webAlive := probeHTTP(webBase)
		if changed(&state.webAlive, webAlive) {
			if webAlive {
				monitorOK(ts, "Web UI", "reachable ("+webBase+")")
			} else {
				monitorFail(ts, "Web UI", "unreachable ("+webBase+")")
				if !apiAlive {
					monitorHint("  → Web UI is served by the backend binary — fix the backend first")
				} else {
					printRecoveryHint("web_down", webBase)
				}
			}
		}
		if !webAlive {
			issues++
		}
	}

	_ = issues // printed above; zero case is silent intentionally

	return issues
}

// ── Probers ───────────────────────────────────────────────────────────────

func probeLivez(base string) (bool, error) {
	resp, err := httpGet(base+"/livez", 3*time.Second)
	if err != nil {
		return false, err
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

func probeReadyz(base string) (bool, string, error) {
	resp, err := httpGet(base+"/readyz", 3*time.Second)
	if err != nil {
		return false, "request failed: " + err.Error(), err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, "ok", nil
	}
	// Parse the check details so we can report which dependency is down.
	var body struct {
		Checks map[string]string `json:"checks"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if db, ok := body.Checks["database"]; ok && db != "ok" {
		return false, db, nil
	}
	return false, fmt.Sprintf("HTTP %d", resp.StatusCode), nil
}

func probeHTTP(url string) bool {
	resp, err := httpGet(url, 3*time.Second)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func httpGet(url string, timeout time.Duration) (*http.Response, error) {
	cl := &http.Client{Timeout: timeout}
	return cl.Get(url)
}

// discoverWebURL reads the web port from /__qorven_runtime.
func discoverWebURL(apiBase string) string {
	resp, err := httpGet(apiBase+"/__qorven_runtime", 2*time.Second)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var info struct {
		WebAddr string `json:"web_addr"`
		WebPort int    `json:"web_port"`
		APIAddr string `json:"api_addr"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return ""
	}
	if info.WebPort > 0 && info.WebAddr != "" {
		// Use the same host as the API URL but the web port.
		parts := strings.SplitN(apiBase, "://", 2)
		if len(parts) == 2 {
			host := strings.SplitN(parts[1], ":", 2)[0]
			return fmt.Sprintf("%s://%s:%d", parts[0], host, info.WebPort)
		}
	}
	return ""
}

// printRecoveryHint prints a specific actionable hint for each failure mode.
func printRecoveryHint(reason, addr string) {
	switch reason {
	case "backend_down":
		fmt.Printf("    → Is the gateway running?  Try: qorven start\n")
		fmt.Printf("    → Check logs:              journalctl -u qorven -n 50\n")
		if runtime.GOOS != "windows" {
			if pid := findProcessOnPort(addr); pid != "" {
				fmt.Printf("    → Port held by PID:       %s\n", pid)
			}
		}
	case "db_down":
		fmt.Printf("    → Check Postgres is running:  pg_isready -h localhost\n")
		fmt.Printf("    → Start if using Docker:      docker compose up -d postgres\n")
		fmt.Printf("    → Verify DSN:                 qorven doctor\n")
	case "web_down":
		fmt.Printf("    → Web UI is part of the backend binary — check backend logs\n")
		fmt.Printf("    → In dev mode, is `pnpm dev` running in web/?\n")
	}
}

// findProcessOnPort returns the PID holding the given addr's port (Unix only).
func findProcessOnPort(addr string) string {
	parts := strings.SplitN(addr, "://", 2)
	hostPort := addr
	if len(parts) == 2 {
		hostPort = parts[1]
	}
	port := hostPort
	if idx := strings.LastIndex(hostPort, ":"); idx >= 0 {
		port = hostPort[idx+1:]
	}
	if port == "" || port == hostPort {
		return ""
	}
	out, err := exec.Command("sh", "-c", fmt.Sprintf("lsof -ti tcp:%s 2>/dev/null | head -1", port)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ── Helpers ───────────────────────────────────────────────────────────────

// changed returns true and updates *last when cur != *last.
func changed(last **bool, cur bool) bool {
	if *last == nil || **last != cur {
		*last = &cur
		return true
	}
	return false
}

func monitorOK(ts, label, detail string) {
	fmt.Printf("  %s  ✓ %-12s %s\n", ts, label+":", detail)
}

func monitorFail(ts, label, detail string) {
	fmt.Printf("  %s  ✗ %-12s %s\n", ts, label+":", detail)
}

func monitorHint(msg string) {
	fmt.Println(msg)
}
