// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/qorvenai/qorven/cmd/installer"
	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Qorven on this server (system dependencies, DB, service)",
	Long: `Full server installation wizard.

Installs PostgreSQL and Docker if missing, creates the qorven database
and OS user, copies the binary to /usr/local/bin, runs migrations, and
registers a systemd service so Qorven starts on boot.

Must be run as root (or with sudo). After install completes, run:

  qorven init      # configure provider, admin account, first agent

Examples:
  sudo qorven install
  sudo qorven install --skip-docker
  sudo qorven install --data-dir /opt/qorven`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInstall(cmd)
	},
}

var (
	installSkipDocker    bool
	installSkipPG        bool
	installDataDir       string
	installTailscaleKey  string
	installSkipTailscale bool
)

func init() {
	installCmd.Flags().BoolVar(&installSkipDocker, "skip-docker", false, "Skip Docker installation")
	installCmd.Flags().BoolVar(&installSkipPG, "skip-postgres", false, "Skip PostgreSQL installation")
	installCmd.Flags().StringVar(&installDataDir, "data-dir", "/var/lib/qorven", "Directory for Qorven data files")
	installCmd.Flags().StringVar(&installTailscaleKey, "tailscale-auth-key", "", "Pre-auth key for headless Tailscale setup (tskey-auth-...)")
	installCmd.Flags().BoolVar(&installSkipTailscale, "skip-tailscale", false, "Skip Tailscale installation")
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("qorven install only supports Linux (got %s)", runtime.GOOS)
	}
	if os.Getuid() != 0 {
		return fmt.Errorf("qorven install must be run as root\n\n  Run:  sudo %s install", os.Args[0])
	}

	ok, err := installer.Run(installer.Config{
		Version:          Version,
		DataDir:          installDataDir,
		SkipDocker:       installSkipDocker,
		SkipPG:           installSkipPG,
		TailscaleAuthKey: installTailscaleKey,
		SkipTailscale:    installSkipTailscale,
	})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("installation failed")
	}
	return nil
}
