// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/internal/config"
	"github.com/qorvenai/qorven/internal/gateway"
)

var gwCmd = &cobra.Command{
	Use:   "gateway",
	Short: "Manage the Qorven gateway service",
}

var gwStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start gateway in background",
	RunE: func(cmd *cobra.Command, args []string) error {
		if isGatewayRunning() {
			printer.Success("Gateway is already running")
			return nil
		}
		home, _ := os.UserHomeDir()
		logDir := filepath.Join(home, ".qorven", "logs")
		os.MkdirAll(logDir, 0755)
		logFile := filepath.Join(logDir, "gateway.log")

		binary, _ := os.Executable()
		proc := exec.Command(binary, "start")
		proc.Stdout, _ = os.Create(logFile)
		proc.Stderr = proc.Stdout
		proc.SysProcAttr = daemonSysProcAttr()
		if err := proc.Start(); err != nil {
			return fmt.Errorf("failed to start: %w", err)
		}

		// Save PID
		pidFile := filepath.Join(home, ".qorven", "gateway.pid")
		os.WriteFile(pidFile, []byte(strconv.Itoa(proc.Process.Pid)), 0644)

		printer.Success(fmt.Sprintf("Gateway started (PID: %d)\n  Log: %s", proc.Process.Pid, logFile))
		return nil
	},
}

var gwStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running gateway",
	RunE: func(cmd *cobra.Command, args []string) error {
		pid := readPID()
		if pid == 0 {
			return fmt.Errorf("gateway is not running (no PID file)")
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("process %d not found", pid)
		}
		proc.Signal(syscall.SIGTERM)
		removePID()
		printer.Success(fmt.Sprintf("Gateway stopped (PID: %d)", pid))
		return nil
	},
}

var gwStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show gateway status",
	RunE: func(cmd *cobra.Command, args []string) error {
		pid := readPID()
		running := isGatewayRunning()

		fmt.Printf("  Status:  ")
		if running {
			fmt.Println("running")
		} else {
			fmt.Println("stopped")
		}
		if pid > 0 {
			fmt.Printf("  PID:     %d\n", pid)
		}
		fmt.Printf("  URL:     %s\n", cfg.Server)

		if running {
			resp, err := http.Get(cfg.Server + "/health")
			if err == nil {
				defer resp.Body.Close()
				fmt.Printf("  Health:  %s\n", resp.Status)
			}
		}
		return nil
	},
}

var gwInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install as systemd user service",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		binary, _ := os.Executable()
		unitDir := filepath.Join(home, ".config", "systemd", "user")
		os.MkdirAll(unitDir, 0755)

		unit := fmt.Sprintf(`[Unit]
Description=Qorven Gateway
After=network.target

[Service]
Type=simple
ExecStart=%s start
Restart=on-failure
RestartSec=5
Environment=QORVEN_CONFIG=%s/.qorven/config.toml

[Install]
WantedBy=default.target
`, binary, home)

		unitFile := filepath.Join(unitDir, "qorven.service")
		if err := os.WriteFile(unitFile, []byte(unit), 0644); err != nil {
			return err
		}

		exec.Command("systemctl", "--user", "daemon-reload").Run()
		exec.Command("systemctl", "--user", "enable", "qorven").Run()
		exec.Command("systemctl", "--user", "start", "qorven").Run()

		printer.Success(fmt.Sprintf("Installed: %s\n  Start: systemctl --user start qorven\n  Logs:  journalctl --user -u qorven -f", unitFile))
		return nil
	},
}

var gwUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove systemd service",
	RunE: func(cmd *cobra.Command, args []string) error {
		exec.Command("systemctl", "--user", "stop", "qorven").Run()
		exec.Command("systemctl", "--user", "disable", "qorven").Run()
		home, _ := os.UserHomeDir()
		os.Remove(filepath.Join(home, ".config", "systemd", "user", "qorven.service"))
		exec.Command("systemctl", "--user", "daemon-reload").Run()
		printer.Success("Service removed")
		return nil
	},
}

func init() {
	// Keep the existing "start" command for foreground mode
	// gateway subcommands are for service management
	gwCmd.AddCommand(gwStartCmd, gwStopCmd, gwStatusCmd, gwInstallCmd, gwUninstallCmd)
	rootCmd.AddCommand(gwCmd)

	// Suppress unused import
	_ = config.Load
	_ = gateway.New
	_ = signal.Notify
	_ = strings.TrimSpace
}

func isGatewayRunning() bool {
	resp, err := http.Get(cfg.Server + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func readPID() int {
	home, _ := os.UserHomeDir()
	data, err := os.ReadFile(filepath.Join(home, ".qorven", "gateway.pid"))
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

func removePID() {
	home, _ := os.UserHomeDir()
	os.Remove(filepath.Join(home, ".qorven", "gateway.pid"))
}
