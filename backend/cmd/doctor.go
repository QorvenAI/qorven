// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/internal/config"
)

var doctorFix      bool
var doctorPreflight bool
var doctorVerify   bool

// preflightPort is the port to test for availability in --preflight mode.
var preflightPort int

var doctorCommand = &cobra.Command{
	Use:   "doctor",
	Short: "Check system environment and configuration health",
	Long: `Full health check of Qorven installation.

Checks: version, config, database, providers, agents, memory,
tools, channels, workspace, external tools, gateway.

Modes:
  (default)    Full post-install health check
  --preflight  Pre-install readiness check — runs on a bare machine before install
  --verify     Post-install liveness check — confirms a fresh install came up

Examples:
  qorven doctor               # check everything
  qorven doctor --fix         # auto-repair common issues
  qorven doctor --preflight   # is this machine ready to install Qorven?
  qorven doctor --verify      # did the install come up correctly?`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if doctorPreflight && doctorVerify {
			return fmt.Errorf("--preflight and --verify are mutually exclusive")
		}
		if doctorPreflight {
			return runDoctorPreflight()
		}
		if doctorVerify {
			return runDoctorVerify()
		}
		return runDoctor()
	},
}

func init() {
	doctorCommand.Flags().BoolVar(&doctorFix, "fix", false, "Auto-repair common issues")
	doctorCommand.Flags().BoolVar(&doctorPreflight, "preflight", false, "Pre-install readiness check (runs on a bare machine)")
	doctorCommand.Flags().BoolVar(&doctorVerify, "verify", false, "Post-install liveness check (confirms a fresh install is healthy)")
	doctorCommand.Flags().IntVar(&preflightPort, "port", 8486, "Port to probe for availability in --preflight mode")
	rootCmd.AddCommand(doctorCommand)
}

// ── Preflight ─────────────────────────────────────────────────────────────────

// runDoctorPreflight checks whether this machine is ready to install Qorven.
// It intentionally does NOT load the Qorven config or touch any database —
// it runs on a bare machine before any install has taken place.
func runDoctorPreflight() error {
	blockers := 0
	warnings := 0

	fmt.Println()
	fmt.Println("  qorven doctor --preflight")
	fmt.Printf("  Version:    %s\n", Version)
	fmt.Printf("  OS:         %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Println()
	fmt.Println("  Preflight checks:")
	fmt.Println()

	// ── 1. OS / arch / distro ──────────────────────────────────────────────
	distro := detectDistro()
	if distro != "" {
		doctorOK("  OS/Distro", distro)
	} else {
		doctorOK("  OS/Distro", fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH))
	}

	// ── 2. Privileges ─────────────────────────────────────────────────────
	// On Linux/macOS a system install needs root. On Windows the PS1 handles it.
	switch runtime.GOOS {
	case "linux", "darwin":
		if os.Geteuid() == 0 {
			doctorOK("  Privileges", "running as root")
		} else {
			doctorFail("  Privileges", "NOT running as root — system install requires sudo")
			doctorHint("Re-run: sudo qorven doctor --preflight")
			blockers++
		}
	default:
		doctorOK("  Privileges", "Windows — handled by PowerShell installer")
	}

	// ── 3. Package manager (Linux only) ───────────────────────────────────
	if runtime.GOOS == "linux" {
		pm := detectPackageManager()
		if pm != "" {
			doctorOK("  Pkg manager", pm)
		} else {
			doctorFail("  Pkg manager", "no supported package manager (apt/dnf/yum) found")
			doctorHint("Install PostgreSQL 16+ manually from https://postgresql.org/download, then re-run")
			blockers++
		}
	}

	// ── 4. Port availability ───────────────────────────────────────────────
	port := preflightPort
	ln, listenErr := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if listenErr == nil {
		ln.Close()
		doctorOK("  Port", fmt.Sprintf("%d is available", port))
	} else {
		doctorWarn("  Port", fmt.Sprintf("%d is already in use — use --port to pick another", port))
		warnings++
	}

	// ── 5. Existing PostgreSQL ─────────────────────────────────────────────
	pgState := preflightDetectPostgres()
	if pgState == "running" {
		doctorOK("  PostgreSQL", "server found and running — will reuse")
	} else if pgState == "installed" {
		doctorWarn("  PostgreSQL", "server installed but not running — installer will start it")
		warnings++
	} else {
		doctorOK("  PostgreSQL", "not found — installer will install PostgreSQL 16+")
	}

	// ── 6. Disk space ─────────────────────────────────────────────────────
	// Check the data directory's filesystem (or / if not writable yet).
	dataDir := preflightDataDir()
	freeBytes := diskFreeBytes(dataDir)
	const minBytes = 2 * 1024 * 1024 * 1024 // 2 GiB
	if freeBytes == 0 {
		// diskFreeBytes returns 0 on Windows or on stat error
		if runtime.GOOS == "windows" {
			doctorOK("  Disk space", "check skipped on Windows")
		} else {
			doctorWarn("  Disk space", fmt.Sprintf("could not stat %s — verify ≥2 GB free manually", dataDir))
			warnings++
		}
	} else if freeBytes < minBytes {
		doctorFail("  Disk space",
			fmt.Sprintf("%.1f GB free on %s — need at least 2 GB",
				float64(freeBytes)/float64(1<<30), dataDir))
		blockers++
	} else {
		doctorOK("  Disk space",
			fmt.Sprintf("%.1f GB free on %s", float64(freeBytes)/float64(1<<30), dataDir))
	}

	// ── 7. Internet reachability (best-effort) ────────────────────────────
	if canReachInternet() {
		doctorOK("  Internet", "reachable")
	} else {
		doctorWarn("  Internet", "could not reach release host — offline install still possible if binary is local")
		warnings++
	}

	// ── Summary ────────────────────────────────────────────────────────────
	fmt.Println()
	if blockers == 0 && warnings == 0 {
		fmt.Println("  ✓ Preflight: ready")
	} else if blockers == 0 {
		fmt.Printf("  ○ Preflight: ready with %d warning(s)\n", warnings)
	} else {
		fmt.Printf("  ✗ Preflight: %d blocker(s), %d warning(s)\n", blockers, warnings)
	}
	fmt.Println()

	if blockers > 0 {
		return fmt.Errorf("preflight: %d blocker(s) must be resolved before installing", blockers)
	}
	return nil
}

// detectDistro parses /etc/os-release on Linux to get the pretty distro name.
// Returns "" on non-Linux or if the file cannot be read.
func detectDistro() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return fmt.Sprintf("linux/%s", runtime.GOARCH)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			name := strings.TrimPrefix(line, "PRETTY_NAME=")
			name = strings.Trim(name, `"'`)
			return fmt.Sprintf("%s (%s)", name, runtime.GOARCH)
		}
	}
	return fmt.Sprintf("linux/%s", runtime.GOARCH)
}

// detectPackageManager returns the name of the first known package manager
// found in PATH, or "" if none is available.
func detectPackageManager() string {
	for _, pm := range []string{"apt-get", "dnf", "yum"} {
		if _, err := exec.LookPath(pm); err == nil {
			return pm
		}
	}
	return ""
}

// preflightDetectPostgres checks whether a PostgreSQL server is present/running
// without requiring any credentials. Returns "running", "installed", or "absent".
func preflightDetectPostgres() string {
	// pg_isready: fastest signal — succeeds only when the server is actively listening.
	if _, err := exec.Command("pg_isready", "-q").CombinedOutput(); err == nil {
		return "running"
	}
	// pg_lsclusters on Debian/Ubuntu: lists clusters even when stopped.
	if path, err := exec.LookPath("pg_lsclusters"); err == nil {
		out, lerr := exec.Command(path, "-h").Output()
		if lerr == nil && strings.TrimSpace(string(out)) != "" {
			return "installed"
		}
	}
	// systemd unit file present (any platform that has systemctl).
	if path, err := exec.LookPath("systemctl"); err == nil {
		out, _ := exec.Command(path, "list-unit-files", "--no-legend", "--no-pager", "postgresql*").Output()
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, ".service") {
				return "installed"
			}
		}
	}
	return "absent"
}

// preflightDataDir returns a reasonable directory to check disk space against.
func preflightDataDir() string {
	switch runtime.GOOS {
	case "linux":
		return "/var/lib"
	case "darwin":
		return "/usr/local"
	default:
		return "."
	}
}

// canReachInternet tries a short HTTP GET to a known stable endpoint.
// Returns true if the request succeeds within 3 seconds.
func canReachInternet() bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("https://github.com")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// ── Verify ────────────────────────────────────────────────────────────────────

// runDoctorVerify confirms that a just-completed install is healthy.
// It reuses the DB/schema checks from runDoctor and adds a service-active check.
// pgvector absent is a WARN, not a failure. Exit non-zero only on real failures:
// service down, DB unreachable, or migrations dirty.
func runDoctorVerify() error {
	issues := 0

	fmt.Println()
	fmt.Println("  qorven doctor --verify")
	fmt.Printf("  Version:    %s", Version)
	if Commit != "none" {
		fmt.Printf(" (commit: %s)", Commit)
	}
	fmt.Println()
	fmt.Printf("  OS:         %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Println()
	fmt.Println("  Verify checks:")
	fmt.Println()

	// ── 1. Service active ─────────────────────────────────────────────────
	svc := checkServiceActive()
	switch {
	case svc.skipped:
		doctorWarn("  Service", "check not available on this platform — skip")
	case svc.active:
		doctorOK("  Service", svc.detail)
	default:
		doctorFail("  Service", svc.detail+" — run: sudo systemctl start qorven")
		issues++
	}

	// ── 2. DB reachable + schema + extensions ─────────────────────────────
	fmt.Println()
	fmt.Println("  Database:")
	cfg, err := config.Load(os.Getenv("QORVEN_CONFIG"))
	if err != nil {
		doctorFail("  Config", fmt.Sprintf("%v", err))
		doctorHint("Run: qorven init")
		issues++
		fmt.Println()
		fmt.Printf("  ✗ Verify: %d issue(s)\n\n", issues)
		return fmt.Errorf("verify: %d issue(s)", issues)
	}

	dsn := cfg.Database.DSN
	if dsn == "" {
		doctorFail("  Connection", "NOT CONFIGURED — set QORVEN_POSTGRES_DSN")
		issues++
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		pool, connErr := pgxpool.New(ctx, dsn)
		if connErr != nil {
			doctorFail("  Connection", fmt.Sprintf("FAILED (%v)", connErr))
			issues++
		} else {
			defer pool.Close()
			if pingErr := pool.Ping(ctx); pingErr != nil {
				doctorFail("  Connection", fmt.Sprintf("PING FAILED (%v)", pingErr))
				issues++
			} else {
				var pgVer string
				pool.QueryRow(ctx, "SELECT version()").Scan(&pgVer)
				if idx := strings.Index(pgVer, ","); idx > 0 {
					pgVer = pgVer[:idx]
				}
				doctorOK("  Connection", pgVer)

				// Schema: version + dirty flag
				var version int
				var dirty bool
				schemaErr := pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version, &dirty)
				if schemaErr != nil {
					doctorWarn("  Schema", "no migrations table — run: qorven migrate up")
					issues++
				} else if dirty {
					doctorFail("  Schema", fmt.Sprintf("v%d DIRTY — run: qorven migrate force %d", version, version-1))
					issues++
				} else {
					doctorOK("  Schema", fmt.Sprintf("v%d (clean)", version))
				}

				// Required extensions (created by 001_schema.up.sql): pgcrypto, uuid-ossp
				for _, ext := range []string{"pgcrypto", "uuid-ossp"} {
					var present bool
					pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname=$1)", ext).Scan(&present)
					if present {
						doctorOK("  "+ext, "installed")
					} else {
						doctorFail("  "+ext, "NOT installed (required) — run: qorven migrate up")
						issues++
					}
				}

				// pgvector: WARN only — vector search is optional
				var hasVector bool
				pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector')").Scan(&hasVector)
				if hasVector {
					doctorOK("  pgvector", "installed")
				} else {
					doctorWarn("  pgvector", "not installed (vector search disabled — optional)")
					// Not counted as an issue
				}
			}
		}
	}

	// ── 3. Health endpoint ─────────────────────────────────────────────────
	fmt.Println()
	gw := "http://" + cfg.Server.Listen
	healthURL := gw + "/health"
	client := &http.Client{Timeout: 5 * time.Second}
	resp, httpErr := client.Get(healthURL)
	if httpErr == nil && resp.StatusCode == 200 {
		resp.Body.Close()
		doctorOK("  Health", healthURL+" → 200 OK")
	} else if httpErr == nil {
		resp.Body.Close()
		doctorFail("  Health", fmt.Sprintf("%s → HTTP %d", healthURL, resp.StatusCode))
		issues++
	} else {
		doctorFail("  Health", fmt.Sprintf("%s unreachable (%v)", healthURL, httpErr))
		doctorHint("Start with: qorven start  or  sudo systemctl start qorven")
		issues++
	}

	// ── Summary ────────────────────────────────────────────────────────────
	fmt.Println()
	if issues == 0 {
		fmt.Println("  ✓ Verify: healthy")
	} else {
		fmt.Printf("  ✗ Verify: %d issue(s)\n", issues)
	}
	fmt.Println()

	if issues > 0 {
		return fmt.Errorf("verify: %d issue(s)", issues)
	}
	return nil
}

// serviceCheckResult holds the result of a service liveness probe.
type serviceCheckResult struct {
	active  bool   // true = definitely up
	skipped bool   // true = check not applicable on this platform
	detail  string // human-readable status
}

// checkServiceActive probes whether the qorven service is running.
// On non-Linux/macOS platforms it returns skipped=true.
func checkServiceActive() serviceCheckResult {
	switch runtime.GOOS {
	case "linux":
		out, err := exec.Command("systemctl", "is-active", "qorven").Output()
		status := strings.TrimSpace(string(out))
		if err == nil && status == "active" {
			return serviceCheckResult{active: true, detail: "active (systemd)"}
		}
		if status == "" {
			// systemctl not available (e.g. container without systemd)
			return serviceCheckResult{skipped: true, detail: "systemctl unavailable"}
		}
		return serviceCheckResult{detail: fmt.Sprintf("systemd status: %s", status)}
	case "darwin":
		out, err := exec.Command("launchctl", "list", "ai.qorven.server").Output()
		if err == nil && len(out) > 0 {
			return serviceCheckResult{active: true, detail: "loaded (launchd)"}
		}
		return serviceCheckResult{detail: "not loaded (launchd) — run: sudo launchctl load /Library/LaunchDaemons/ai.qorven.server.plist"}
	default:
		return serviceCheckResult{skipped: true, detail: "not applicable on " + runtime.GOOS}
	}
}

// ── Full doctor (existing) ────────────────────────────────────────────────────

func runDoctor() error {
	issues := 0

	// ── Header ──
	fmt.Println()
	fmt.Println("  qorven doctor")
	fmt.Printf("  Version:    %s", Version)
	if Commit != "none" { fmt.Printf(" (commit: %s)", Commit) }
	if BuildTime != "unknown" { fmt.Printf(" built: %s", BuildTime) }
	fmt.Println()
	fmt.Printf("  OS:         %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  Go:         %s\n", runtime.Version())

	// ── Config ──
	fmt.Println()
	cfg, err := config.Load(os.Getenv("QORVEN_CONFIG"))
	if err != nil {
		doctorFail("Config", fmt.Sprintf("%v", err))
		doctorHint("Run: qorven init")
		issues++
		fmt.Println("\n  Doctor check complete (config failed).")
		return nil
	}
	if cfg.ConfigPath != "" {
		doctorOK("Config", cfg.ConfigPath)
	} else {
		doctorWarn("Config", "using defaults (no config file found)")
		doctorHint("Run: qorven init")
	}

	// Check .env
	qHome := resolveQorvenHome()
	envPath := filepath.Join(qHome, ".env")
	if _, err := os.Stat(envPath); err == nil {
		doctorOK("Secrets", envPath)
	} else {
		doctorWarn("Secrets", ".env not found")
		if doctorFix {
			os.MkdirAll(qHome, 0755)
			os.WriteFile(envPath, []byte("# Qorven secrets\n"), 0600)
			doctorFixed("Created " + envPath)
		}
	}

	// ── Database ──
	fmt.Println()
	fmt.Println("  Database:")
	dsn := cfg.Database.DSN
	if dsn == "" {
		doctorFail("  Connection", "NOT CONFIGURED")
		doctorHint("Set QORVEN_POSTGRES_DSN or run: qorven init")
		issues++
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		start := time.Now()
		pool, connErr := pgxpool.New(ctx, dsn)
		if connErr != nil {
			doctorFail("  Connection", fmt.Sprintf("FAILED (%v)", connErr))
			issues++
		} else {
			defer pool.Close()
			if pingErr := pool.Ping(ctx); pingErr != nil {
				doctorFail("  Connection", fmt.Sprintf("PING FAILED (%v)", pingErr))
				issues++
			} else {
				latency := time.Since(start)
				var pgVer string
				pool.QueryRow(ctx, "SELECT version()").Scan(&pgVer)
				if idx := strings.Index(pgVer, ","); idx > 0 { pgVer = pgVer[:idx] }
				doctorOK("  Connection", fmt.Sprintf("%s (%dms)", pgVer, latency.Milliseconds()))

				// Schema
				var version int
				var dirty bool
				err := pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version, &dirty)
				if err != nil {
					doctorWarn("  Schema", "no migrations table")
					if doctorFix {
						doctorHint("Run: qorven migrate up")
					}
				} else if dirty {
					doctorFail("  Schema", fmt.Sprintf("v%d (DIRTY)", version))
					doctorHint("Run: qorven migrate force %d", version-1)
					issues++
				} else {
					doctorOK("  Schema", fmt.Sprintf("v%d", version))
				}

				// pgvector
				var hasVector bool
				pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector')").Scan(&hasVector)
				if hasVector {
					doctorOK("  pgvector", "installed")
				} else {
					doctorWarn("  pgvector", "not installed (vector search disabled)")
				}

				// Tables
				var tableCount int
				pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public'").Scan(&tableCount)
				doctorOK("  Tables", fmt.Sprintf("%d", tableCount))

				// ── Providers (from DB) ──
				fmt.Println()
				fmt.Println("  Providers:")
				rows, _ := pool.Query(ctx, "SELECT name, COALESCE(display_name,name), enabled, COALESCE(api_key,'') FROM llm_providers ORDER BY name")
				if rows != nil {
					provCount := 0
					for rows.Next() {
						var name, display string
						var enabled bool
						var key string
						rows.Scan(&name, &display, &enabled, &key)
						provCount++
						status := "enabled"
						if !enabled { status = "disabled" }
						if key != "" {
							masked := key[:4] + "..." + key[max(4, len(key)-4):]
							doctorOK("  "+display, fmt.Sprintf("%s (%s)", masked, status))
						} else {
							doctorWarn("  "+display, fmt.Sprintf("no API key (%s)", status))
						}
					}
					rows.Close()
					if provCount == 0 {
						doctorWarn("  (none)", "no providers in database")
					}
				}

				// Config providers
				for _, p := range cfg.Providers {
					if p.APIKey != "" {
						masked := p.APIKey[:4] + "..." + p.APIKey[max(4, len(p.APIKey)-4):]
						doctorOK("  "+p.Name+" (config)", masked)
					}
				}

				// ── Agents ──
				fmt.Println()
				var agentCount int
				pool.QueryRow(ctx, "SELECT count(*) FROM agents").Scan(&agentCount)
				if agentCount > 0 {
					doctorOK("Agents", fmt.Sprintf("%d", agentCount))
					rows, _ := pool.Query(ctx, "SELECT agent_key, model FROM agents ORDER BY created_at LIMIT 5")
					if rows != nil {
						for rows.Next() {
							var key, model string
							rows.Scan(&key, &model)
							fmt.Printf("    %-16s %s\n", key, model)
						}
						rows.Close()
					}
				} else {
					doctorWarn("Agents", "none — run: qorven agent create")
				}

				// ── Memory ──
				fmt.Println()
				var memCount int
				pool.QueryRow(ctx, "SELECT count(*) FROM memories").Scan(&memCount)
				doctorOK("Memory", fmt.Sprintf("%d memories", memCount))

				// ── Channels ──
				fmt.Println()
				fmt.Println("  Channels:")
				chRows, _ := pool.Query(ctx, "SELECT name, channel_type, enabled FROM channel_instances ORDER BY channel_type")
				if chRows != nil {
					chCount := 0
					for chRows.Next() {
						var name, chType string
						var enabled bool
						chRows.Scan(&name, &chType, &enabled)
						chCount++
						status := "enabled"
						if !enabled { status = "disabled" }
						doctorOK("  "+chType+"/"+name, status)
					}
					chRows.Close()
					if chCount == 0 { fmt.Println("    (none configured)") }
				}
			}
		}
	}

	// ── Workspace ──
	fmt.Println()
	wsPath := filepath.Join(qHome, "workspaces")
	if info, err := os.Stat(wsPath); err == nil && info.IsDir() {
		free := diskFree(wsPath)
		doctorOK("Workspace", fmt.Sprintf("%s (%s free)", wsPath, free))
	} else {
		doctorWarn("Workspace", wsPath+" (not found)")
		if doctorFix {
			os.MkdirAll(wsPath, 0755)
			doctorFixed("Created " + wsPath)
		}
	}

	// ── External Tools ──
	fmt.Println()
	fmt.Println("  External Tools:")
	for _, tool := range []string{"docker", "curl", "git", "python3", "node"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			fmt.Printf("    %-12s not found\n", tool)
		} else {
			doctorOK("  "+tool, path)
		}
	}

	// ── Gateway ──
	fmt.Println()
	// Normalize the bind address for an OUTBOUND probe: 0.0.0.0 means "listen on
	// all interfaces" but is not a valid connect target on macOS/Windows — dial
	// loopback instead so the health check doesn't false-fail.
	gw := "http://" + strings.Replace(cfg.Server.Listen, "0.0.0.0", "127.0.0.1", 1)
	resp, err := http.Get(gw + "/health")
	if err == nil && resp.StatusCode == 200 {
		resp.Body.Close()
		doctorOK("Gateway", gw+" (running)")
	} else {
		doctorWarn("Gateway", gw+" (not running)")
		doctorHint("Start with: qorven start")
	}

	// ── Summary ──
	fmt.Println()
	if issues == 0 {
		fmt.Println("  ✓ All checks passed")
	} else {
		fmt.Printf("  ✗ %d issue(s) found\n", issues)
		if !doctorFix {
			fmt.Println("  Run: qorven doctor --fix")
		}
	}
	fmt.Println()
	return nil
}

func doctorOK(label, detail string) {
	fmt.Printf("  %-14s ✓ %s\n", label+":", detail)
}

func doctorWarn(label, detail string) {
	fmt.Printf("  %-14s ⚠ %s\n", label+":", detail)
}

func doctorFail(label, detail string) {
	fmt.Printf("  %-14s ✗ %s\n", label+":", detail)
}

func doctorHint(format string, args ...any) {
	fmt.Printf("    → "+format+"\n", args...)
}

func doctorFixed(msg string) {
	fmt.Printf("    ✓ Fixed: %s\n", msg)
}


func max(a, b int) int { if a > b { return a }; return b }
