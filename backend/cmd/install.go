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
	Long: `Install Qorven on this server.

Runs non-interactively, streaming one line of progress per step — safe to run
over ssh, cloud-init, or CI. Installs PostgreSQL if missing, creates the qorven
database, copies the binary, and registers a system service so Qorven starts on
boot. Re-running is safe: it resumes a partial install and repairs an existing one.

  Linux / macOS: must be run as root (or with sudo)
  Windows:       use the PowerShell installer instead:
                   iwr https://get.qorven.ai/win | iex

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
	installCmd.Flags().BoolVar(&installSkipDocker, "skip-docker", false, "Skip Docker installation (Linux only)")
	installCmd.Flags().BoolVar(&installSkipPG, "skip-postgres", false, "Skip PostgreSQL installation")
	installCmd.Flags().StringVar(&installDataDir, "data-dir", "", "Directory for Qorven data files (default: /var/lib/qorven on Linux, /usr/local/var/qorven on macOS)")
	installCmd.Flags().StringVar(&installTailscaleKey, "tailscale-auth-key", "", "Pre-auth key for headless Tailscale setup (tskey-auth-...)")
	installCmd.Flags().BoolVar(&installSkipTailscale, "skip-tailscale", false, "Skip Tailscale installation")
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command) error {
	switch runtime.GOOS {
	case "windows":
		return fmt.Errorf("windows installation uses the PowerShell installer:\n\n  iwr https://get.qorven.ai/win | iex\n\nsee https://docs.qorven.ai/getting-started/install for details")
	case "linux", "darwin":
		// continue below
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	if os.Getuid() != 0 {
		return fmt.Errorf("qorven install must be run as root\n\n  Run:  sudo %s install", os.Args[0])
	}

	// Default data dir varies by platform
	dataDir := installDataDir
	if dataDir == "" {
		if runtime.GOOS == "darwin" {
			dataDir = "/usr/local/var/qorven"
		} else {
			dataDir = "/var/lib/qorven"
		}
	}

	ok, err := installer.Run(installer.Config{
		Version:          Version,
		DataDir:          dataDir,
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
