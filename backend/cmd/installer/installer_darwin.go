//go:build darwin

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func platformSteps() []installStep {
	return []installStep{
		{label: "Detect system"},
		{label: "Check Homebrew"},
		{label: "Install PostgreSQL"},
		{label: "Install pgvector"},
		{label: "Setup database"},
		{label: "Create data directories"},
		{label: "Install binary to /usr/local/bin"},
		{label: "Register launchd service"},
		{label: "Tailscale (optional)"},
	}
}

// brewPrefix returns the actual Homebrew prefix — /opt/homebrew on Apple Silicon,
// /usr/local on Intel. Never hardcode /usr/local on macOS.
func brewPrefix() string {
	out, err := exec.Command("brew", "--prefix").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		// Fallback detection based on architecture
		arch, _ := exec.Command("uname", "-m").Output()
		if strings.TrimSpace(string(arch)) == "arm64" {
			return "/opt/homebrew"
		}
		return "/usr/local"
	}
	return strings.TrimSpace(string(out))
}

func platformConfigDir() string { return "/usr/local/etc/qorven" }

func platformBinPath() string {
	// Binary lives under the Homebrew prefix so it's on PATH by default
	return brewPrefix() + "/bin/qorven"
}

func platformRestartService(configPath string) {
	exec.Command("launchctl", "unload", launchdPlistPath()).Run()
	exec.Command("launchctl", "load", "-w", launchdPlistPath()).Run()
}

func platformMigrate(configPath, dsn string) error {
	cmd := exec.Command(platformBinPath(), "migrate", "up",
		"--config", configPath,
	)
	cmd.Env = append(os.Environ(), "QORVEN_POSTGRES_DSN="+dsn)
	return cmd.Run()
}

// probeSocketDSN detects the correct Unix socket path for macOS.
// Homebrew PG uses /tmp; postgres.app uses a different path.
func probeSocketDSN() string {
	user := currentUser()

	// Try /tmp first (Homebrew default)
	if _, err := os.Stat("/tmp/.s.PGSQL.5432"); err == nil {
		return "postgres:///qorven?host=/tmp&user=" + user + "&sslmode=disable"
	}
	// Try /var/run/postgresql (postgres.app style)
	if _, err := os.Stat("/var/run/postgresql/.s.PGSQL.5432"); err == nil {
		return "postgres:///qorven?host=/var/run/postgresql&user=" + user + "&sslmode=disable"
	}
	// Try Homebrew prefix socket directory
	prefix := brewPrefix()
	sockPath := prefix + "/var/run/postgresql"
	if entries, err := os.ReadDir(sockPath); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".s.PGSQL.") {
				return "postgres:///qorven?host=" + sockPath + "&user=" + user + "&sslmode=disable"
			}
		}
	}
	// Fall back to TCP
	return "postgres://" + user + "@127.0.0.1:5432/qorven?sslmode=disable"
}

func currentUser() string {
	out, err := exec.Command("id", "-un").Output()
	if err != nil {
		return "postgres"
	}
	return strings.TrimSpace(string(out))
}

func launchdPlistPath() string {
	return "/Library/LaunchDaemons/ai.qorven.server.plist"
}

// detectRunningPGVersion returns the major version of the currently running
// PostgreSQL on macOS (needed to match pgvector to the right version).
func detectRunningPGVersion() string {
	out, err := exec.Command("psql", "--version").Output()
	if err != nil {
		return ""
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) >= 3 {
		return strings.Split(parts[2], ".")[0]
	}
	return ""
}

// linkedBrewPGService returns the brew service name for the currently active PG.
func linkedBrewPGService() string {
	// Try postgresql@16, then postgresql@15, then generic postgresql
	for _, candidate := range []string{"postgresql@17", "postgresql@16", "postgresql@15", "postgresql@14", "postgresql"} {
		out, err := exec.Command("brew", "services", "list").Output()
		if err == nil && strings.Contains(string(out), candidate) {
			return candidate
		}
	}
	return "postgresql@16"
}

func platformRequirementsText() string {
	req := func(icon, label, detail string) string {
		return icon + "  " + fgSt.Render(label) + "\n" +
			"   " + mutedSt.Render(detail)
	}
	return req("🍎", "macOS 12 Monterey or later", "Intel or Apple Silicon") + "\n" +
		req("🍺", "Homebrew", "brew.sh — auto-detected at /opt/homebrew or /usr/local") + "\n" +
		req("🔑", "sudo access", "to install the service and write to system dirs") + "\n" +
		req("💾", "2 GB RAM  ·  5 GB disk", "minimum recommended") + "\n" +
		req("🌐", "Internet access", "to pull packages on first install")
}

func platformServiceCommands() string {
	return mutedSt.Render("  sudo launchctl list ai.qorven.server") + "\n" +
		mutedSt.Render("  tail -f /usr/local/var/log/qorven.log") + "\n" +
		mutedSt.Render("  sudo qorven migrate up")
}

func platformErrorHints() (common, logs string) {
	common = dimSt.Render("  Homebrew not installed (brew.sh)") + "\n" +
		dimSt.Render("  Multiple PostgreSQL versions — check brew services list") + "\n" +
		dimSt.Render("  Port already in use") + "\n" +
		dimSt.Render("  Disk full — needs 5 GB free") + "\n" +
		dimSt.Render("  Not running with sudo")
	logs = mutedSt.Render("  brew services list") + "\n" +
		mutedSt.Render("  tail -f /usr/local/var/log/qorven.log")
	return
}

func executeStep(idx int, cfg Config) (detail string, warn bool, err error) {
	switch idx {
	case 0: // Detect
		out, _ := exec.Command("sw_vers", "-productVersion").Output()
		arch, _ := exec.Command("uname", "-m").Output()
		osVer := strings.TrimSpace(string(out))
		archStr := strings.TrimSpace(string(arch))
		prefix := brewPrefix()
		return fmt.Sprintf("macOS %s  %s  (brew prefix: %s)", osVer, archStr, prefix), false, nil

	case 1: // Homebrew
		brew, brewErr := exec.LookPath("brew")
		if brewErr != nil {
			return "", false, fmt.Errorf("Homebrew not found — install from https://brew.sh then re-run")
		}
		v, _ := exec.Command(brew, "--version").Output()
		return strings.SplitN(strings.TrimSpace(string(v)), "\n", 2)[0], false, nil

	case 2: // PostgreSQL via Homebrew
		if cfg.SkipPG {
			return "skipped (--skip-postgres)", true, nil
		}

		// Check if PostgreSQL is already running (any version, any installation method)
		if _, e := exec.Command("pg_isready").Output(); e == nil {
			v, _ := exec.Command("psql", "--version").Output()
			pgVer := detectRunningPGVersion()
			detail = strings.TrimSpace(string(v))
			if pgVer != "" {
				detail += fmt.Sprintf(" (version %s — already running)", pgVer)
			}
			return detail, false, nil
		}

		// Detect which PG versions are installed via Homebrew
		brewListOut, _ := exec.Command("brew", "list", "--formula").Output()
		brewList := string(brewListOut)

		installed := ""
		for _, candidate := range []string{"postgresql@17", "postgresql@16", "postgresql@15", "postgresql@14"} {
			if strings.Contains(brewList, candidate) {
				installed = candidate
				break
			}
		}
		if installed == "" && strings.Contains(brewList, "postgresql") {
			installed = "postgresql"
		}

		if installed != "" {
			// Existing Homebrew PG — just start it
			runQuiet("brew", "services", "start", installed)
		} else {
			// Install PostgreSQL 16 (current stable)
			if err = runQuiet("brew", "install", "postgresql@16"); err != nil {
				return "", false, fmt.Errorf("brew install postgresql@16: %w", err)
			}
			installed = "postgresql@16"
			runQuiet("brew", "link", "--force", installed)
			runQuiet("brew", "services", "start", installed)
		}

		// Wait for readiness
		ready := false
		for i := 0; i < 20; i++ {
			if _, e := exec.Command("pg_isready").Output(); e == nil {
				ready = true
				break
			}
			time.Sleep(time.Second)
		}
		if !ready {
			return "", false, fmt.Errorf("postgresql did not become ready — check: brew services list")
		}
		v, _ := exec.Command("psql", "--version").Output()
		return strings.TrimSpace(string(v)), false, nil

	case 3: // pgvector — must match the running PG version
		if cfg.SkipPG {
			return "skipped (--skip-postgres)", true, nil
		}
		pgVer := detectRunningPGVersion()

		// brew install pgvector installs for the linked PG — verify versions match
		if pgVer != "" {
			// Force link correct version before installing pgvector
			svc := linkedBrewPGService()
			if svc != "" {
				runQuiet("brew", "link", "--force", "--overwrite", svc)
			}
		}

		if err = runQuiet("brew", "install", "pgvector"); err != nil {
			return "skipped (pgvector unavailable — vector search disabled)", true, nil
		}

		// Verify pgvector shared library is findable by the running PG
		prefix := brewPrefix()
		pgShareDir := fmt.Sprintf("%s/opt/postgresql@%s/share/postgresql@%s/extension", prefix, pgVer, pgVer)
		if pgVer == "" || !fileExists(pgShareDir+"/vector.control") {
			// Try generic path
			altPath := prefix + "/share/postgresql/extension/vector.control"
			if !fileExists(altPath) {
				return "installed (pgvector library path not verified — run: CREATE EXTENSION vector; manually)", true, nil
			}
		}
		return "installed", false, nil

	case 4: // Database setup
		if cfg.SkipPG {
			return "skipped (--skip-postgres)", true, nil
		}
		user := currentUser()

		// Verify we can actually connect
		if _, e := exec.Command("psql", "-U", user, "-d", "postgres", "-c", "SELECT 1").Output(); e != nil {
			return "", false, fmt.Errorf("cannot connect to PostgreSQL as '%s' — is it running? (brew services list)", user)
		}

		// Create DB
		out, _ := exec.Command("psql", "-U", user, "-d", "postgres", "-tAc",
			"SELECT 1 FROM pg_database WHERE datname='qorven'").Output()
		if strings.TrimSpace(string(out)) != "1" {
			if dbOut, dbErr := exec.Command("createdb", "-U", user, "qorven").CombinedOutput(); dbErr != nil {
				return "", false, fmt.Errorf("createdb: %w — %s", dbErr, strings.TrimSpace(string(dbOut)))
			}
		}

		// Enable pgvector (non-fatal)
		exec.Command("psql", "-U", user, "-d", "qorven", "-c",
			"CREATE EXTENSION IF NOT EXISTS vector;").Run()

		// Confirm whether vector is actually enabled
		extOut, _ := exec.Command("psql", "-U", user, "-d", "qorven", "-tAc",
			"SELECT 1 FROM pg_extension WHERE extname='vector'").Output()
		if strings.TrimSpace(string(extOut)) == "1" {
			return "ready — pgvector enabled", false, nil
		}
		return "ready — pgvector not available (vector search disabled, Qorven works without it)", true, nil

	case 5: // Directories
		prefix := brewPrefix()
		logDir := prefix + "/var/log"
		dataDir := cfg.DataDir
		if dataDir == "" || dataDir == "/var/lib/qorven" {
			dataDir = prefix + "/var/qorven"
		}
		dirs := []string{dataDir, dataDir + "/workspaces", dataDir + "/tls", logDir, platformConfigDir()}
		for _, d := range dirs {
			if err = os.MkdirAll(d, 0755); err != nil {
				return "", false, fmt.Errorf("mkdir %s: %w", d, err)
			}
		}
		return dataDir, false, nil

	case 6: // Binary
		target := platformBinPath()
		self, _ := os.Executable()
		selfReal, _ := filepath.EvalSymlinks(self)
		targetReal, evalErr := filepath.EvalSymlinks(target)
		if evalErr == nil && selfReal == targetReal {
			return "already in place", false, nil
		}
		// Unload service before replacing binary
		exec.Command("launchctl", "unload", launchdPlistPath()).Run()
		data, readErr := os.ReadFile(self)
		if readErr != nil {
			return "", false, fmt.Errorf("read binary: %w", readErr)
		}
		// Ensure parent dir exists (brew prefix may differ by arch)
		if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return "", false, fmt.Errorf("create bin dir: %w", err)
		}
		tmp := target + ".installing"
		if err = os.WriteFile(tmp, data, 0755); err != nil {
			return "", false, fmt.Errorf("write temp binary: %w", err)
		}
		if err = os.Rename(tmp, target); err != nil {
			os.Remove(tmp)
			return "", false, fmt.Errorf("rename binary: %w", err)
		}
		return target, false, nil

	case 7: // launchd service
		prefix := brewPrefix()
		dataDir := cfg.DataDir
		if dataDir == "" || dataDir == "/var/lib/qorven" {
			dataDir = prefix + "/var/qorven"
		}
		logFile := prefix + "/var/log/qorven.log"
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>ai.qorven.server</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>start</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>QORVEN_CONFIG</key>
        <string>%s/config.toml</string>
        <key>PATH</key>
        <string>%s/bin:/usr/local/bin:/usr/bin:/bin</string>
    </dict>
    <key>WorkingDirectory</key>
    <string>%s</string>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
`, platformBinPath(), platformConfigDir(), prefix, dataDir, logFile, logFile)
		plistPath := launchdPlistPath()
		if err = os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
			// Try user LaunchAgent as fallback (no sudo needed)
			userPlistDir := os.ExpandEnv("$HOME/Library/LaunchAgents")
			os.MkdirAll(userPlistDir, 0755)
			plistPath = userPlistDir + "/ai.qorven.server.plist"
			if werr := os.WriteFile(plistPath, []byte(plist), 0644); werr != nil {
				return "", false, fmt.Errorf("write plist (tried system and user location): %w", err)
			}
			exec.Command("launchctl", "unload", plistPath).Run()
			if err = runQuiet("launchctl", "load", "-w", plistPath); err != nil {
				return "", false, fmt.Errorf("launchctl load (user agent): %w", err)
			}
			return "registered as user LaunchAgent (not system-wide)", true, nil
		}
		exec.Command("launchctl", "unload", plistPath).Run()
		if err = runQuiet("launchctl", "load", "-w", plistPath); err != nil {
			return "", false, fmt.Errorf("launchctl load: %w", err)
		}
		return "registered — auto-start on boot", false, nil

	case 8: // Tailscale (optional)
		if cfg.SkipTailscale {
			return "skipped (--skip-tailscale)", true, nil
		}
		if commandExists("tailscale") {
			if out, e := exec.Command("tailscale", "ip", "-4").Output(); e == nil {
				ip := strings.TrimSpace(string(out))
				if strings.HasPrefix(ip, "100.") {
					return "connected:" + ip, false, nil
				}
			}
		}
		// Try brew install CLI
		if runQuiet("brew", "install", "tailscale") == nil {
			runQuiet("brew", "services", "start", "tailscale")
			time.Sleep(2 * time.Second)
			if out, e := exec.Command("tailscale", "ip", "-4").Output(); e == nil {
				ip := strings.TrimSpace(string(out))
				if strings.HasPrefix(ip, "100.") {
					return "connected:" + ip, false, nil
				}
			}
			out, startErr := exec.Command("tailscale", "up", "--ssh").CombinedOutput()
			if startErr == nil {
				for _, line := range strings.Split(string(out), "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "https://login.tailscale.com/") {
						return "url:" + line, false, nil
					}
				}
			}
		}
		return "skipped — install Tailscale from tailscale.com/download/mac", true, nil
	}
	return "", false, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
