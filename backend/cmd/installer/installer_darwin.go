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

func platformConfigDir() string { return "/usr/local/etc/qorven" }

func platformBinPath() string { return "/usr/local/bin/qorven" }

func platformRestartService(configPath string) {
	label := "ai.qorven.server"
	exec.Command("launchctl", "unload", launchdPlistPath()).Run()
	exec.Command("launchctl", "load", "-w", launchdPlistPath()).Run()
	_ = label
}

func platformMigrate(configPath, dsn string) error {
	cmd := exec.Command("/usr/local/bin/qorven", "migrate", "up",
		"--config", configPath,
	)
	cmd.Env = append(os.Environ(), "QORVEN_POSTGRES_DSN="+dsn)
	return cmd.Run()
}

func probeSocketDSN() string {
	// macOS Homebrew PostgreSQL uses a Unix socket in /tmp
	return "postgres:///qorven?host=/tmp&user=" + currentUser() + "&sslmode=disable"
}

func currentUser() string {
	out, err := exec.Command("id", "-un").Output()
	if err != nil {
		return "postgres"
	}
	return strings.TrimSpace(string(out))
}

func launchdPlistPath() string {
	// Install as a global LaunchDaemon so it runs at boot (requires sudo)
	return "/Library/LaunchDaemons/ai.qorven.server.plist"
}

func platformRequirementsText() string {
	req := func(icon, label, detail string) string {
		return icon + "  " + fgSt.Render(label) + "\n" +
			"   " + mutedSt.Render(detail)
	}
	return req("🍎", "macOS 12 Monterey or later", "Intel or Apple Silicon") + "\n" +
		req("🍺", "Homebrew", "installed at /opt/homebrew or /usr/local") + "\n" +
		req("🔑", "sudo access", "to install the service and write to /usr/local") + "\n" +
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
		dimSt.Render("  PostgreSQL service not starting") + "\n" +
		dimSt.Render("  Port already in use") + "\n" +
		dimSt.Render("  Disk full — needs 5 GB free") + "\n" +
		dimSt.Render("  Not running with sudo")
	logs = mutedSt.Render("  tail -f /usr/local/var/log/qorven.log") + "\n" +
		mutedSt.Render("  brew services list")
	return
}

func executeStep(idx int, cfg Config) (detail string, warn bool, err error) {
	switch idx {
	case 0: // Detect
		out, _ := exec.Command("sw_vers", "-productVersion").Output()
		arch, _ := exec.Command("uname", "-m").Output()
		return "macOS " + strings.TrimSpace(string(out)) + "  " + strings.TrimSpace(string(arch)), false, nil

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
		// Check if any PostgreSQL@N is already running
		if out, e := exec.Command("pg_isready").Output(); e == nil {
			v, _ := exec.Command("psql", "--version").Output()
			_ = out
			return strings.TrimSpace(string(v)), false, nil
		}
		// Install latest PostgreSQL (Homebrew tracks latest major)
		if err = runQuiet("brew", "install", "postgresql@16"); err != nil {
			// Try older name
			if err = runQuiet("brew", "install", "postgresql"); err != nil {
				return "", false, fmt.Errorf("brew install postgresql: %w", err)
			}
		}
		// Start and enable on boot
		runQuiet("brew", "services", "start", "postgresql@16")
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
		return "installed — " + strings.TrimSpace(string(v)), false, nil

	case 3: // pgvector via Homebrew
		if cfg.SkipPG {
			return "skipped (--skip-postgres)", true, nil
		}
		if err = runQuiet("brew", "install", "pgvector"); err != nil {
			return "skipped (pgvector unavailable — vector search disabled)", true, nil
		}
		return "installed", false, nil

	case 4: // Database setup
		if cfg.SkipPG {
			return "skipped (--skip-postgres)", true, nil
		}
		// On macOS Homebrew, the current user is the superuser
		user := currentUser()
		out, _ := exec.Command("psql", "-U", user, "-d", "postgres", "-tAc",
			"SELECT 1 FROM pg_database WHERE datname='qorven'").Output()
		if strings.TrimSpace(string(out)) != "1" {
			if dbOut, dbErr := exec.Command("createdb", "-U", user, "qorven").CombinedOutput(); dbErr != nil {
				return "", false, fmt.Errorf("createdb: %w — %s", dbErr, strings.TrimSpace(string(dbOut)))
			}
		}
		exec.Command("psql", "-U", user, "-d", "qorven", "-c",
			"CREATE EXTENSION IF NOT EXISTS vector;").Run()
		return "ready", false, nil

	case 5: // Directories
		logDir := "/usr/local/var/log"
		dataDir := cfg.DataDir
		if dataDir == "" || dataDir == "/var/lib/qorven" {
			dataDir = "/usr/local/var/qorven"
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
		dataDir := cfg.DataDir
		if dataDir == "" || dataDir == "/var/lib/qorven" {
			dataDir = "/usr/local/var/qorven"
		}
		logFile := "/usr/local/var/log/qorven.log"
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
`, platformBinPath(), platformConfigDir(), dataDir, logFile, logFile)
		plistPath := launchdPlistPath()
		if err = os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
			return "", false, fmt.Errorf("write plist: %w", err)
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
		// On macOS, Tailscale is a menu bar app — just check if it's already running
		if commandExists("tailscale") {
			if out, e := exec.Command("tailscale", "ip", "-4").Output(); e == nil {
				ip := strings.TrimSpace(string(out))
				if strings.HasPrefix(ip, "100.") {
					return "connected:" + ip, false, nil
				}
			}
		}
		// Try brew install as a fallback (installs tailscale CLI only)
		if runQuiet("brew", "install", "tailscale") == nil {
			runQuiet("brew", "services", "start", "tailscale")
			time.Sleep(2 * time.Second)
			if out, e := exec.Command("tailscale", "ip", "-4").Output(); e == nil {
				ip := strings.TrimSpace(string(out))
				if strings.HasPrefix(ip, "100.") {
					return "connected:" + ip, false, nil
				}
			}
			// Need browser auth
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
