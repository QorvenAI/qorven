// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/qorvenai/qorven/internal/config"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove Qorven completely",
	Long: `Stops the service and removes the binary and systemd unit (default).

--purge ALSO removes:
  • All config files (/etc/qorven, config.toml, .env)
  • Data directory and logs (/var/lib/qorven, /var/log/qorven)
  • PostgreSQL database and role (qorven)
  • nginx reverse-proxy config (/etc/nginx/conf.d/qorven.conf)
  • The 'qorven' OS system user

Note: PostgreSQL itself is NOT uninstalled — it is a shared system resource.
To remove it: sudo apt-get remove --purge postgresql* (Debian/Ubuntu)`,
	RunE: runUninstall,
}

func runUninstall(cmd *cobra.Command, args []string) error {
	purge, _ := cmd.Flags().GetBool("purge")
	yes, _ := cmd.Flags().GetBool("yes")

	if !yes {
		msg := "This will stop Qorven, remove the binary, and unregister the service."
		if purge {
			msg = "This will stop Qorven and remove the binary, service, config, data, database, nginx config, and the 'qorven' OS user."
		}
		fmt.Println(msg)
		fmt.Print("Continue? [y/N]: ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		reply := strings.TrimSpace(strings.ToLower(scanner.Text()))
		if reply != "y" && reply != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// 1. Stop and disable the service
	fmt.Print("  Stopping service… ")
	if _, err := exec.LookPath("systemctl"); err == nil {
		exec.Command("systemctl", "stop", "qorven").Run()
		exec.Command("systemctl", "disable", "qorven").Run()
		os.Remove("/etc/systemd/system/qorven.service")
		exec.Command("systemctl", "daemon-reload").Run()
		exec.Command("systemctl", "reset-failed").Run()
	} else {
		// Non-systemd fallback (macOS launchd or direct PID)
		exec.Command("launchctl", "unload", "-w",
			"/Library/LaunchDaemons/ai.qorven.server.plist").Run()
		exec.Command("launchctl", "unload", "-w",
			os.ExpandEnv("$HOME/Library/LaunchAgents/ai.qorven.server.plist")).Run()
		os.Remove("/Library/LaunchDaemons/ai.qorven.server.plist")
		os.Remove(os.ExpandEnv("$HOME/Library/LaunchAgents/ai.qorven.server.plist"))
		killRuntimePID()
	}
	fmt.Println("done")

	// 2. Remove binary and symlink
	fmt.Print("  Removing binary… ")
	binaryPaths := []string{
		"/opt/qorven/bin/qorven",
		"/usr/local/bin/qorven",
		"/usr/bin/qorven",
	}
	// macOS Homebrew paths
	for _, prefix := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		binaryPaths = append(binaryPaths, prefix+"/qorven")
	}
	removed := 0
	seen := map[string]bool{}
	for _, p := range binaryPaths {
		if seen[p] {
			continue
		}
		seen[p] = true
		if _, err := os.Lstat(p); err == nil {
			if err := os.Remove(p); err != nil {
				fmt.Printf("\n    warn: could not remove %s: %v\n", p, err)
			} else {
				removed++
			}
		}
	}
	os.RemoveAll("/opt/qorven/bin")
	if removed > 0 {
		fmt.Println("done")
	} else {
		fmt.Println("(not found — already removed)")
	}

	if purge {
		// 3. Remove config, data, and logs
		fmt.Print("  Removing config and data… ")
		for _, d := range []string{
			config.DataDir(),
			"/etc/qorven",
			"/var/lib/qorven",
			"/var/log/qorven",
		} {
			os.RemoveAll(d)
		}
		fmt.Println("done")

		// 4. Remove nginx reverse-proxy config
		fmt.Print("  Removing nginx config… ")
		nginxConf := "/etc/nginx/conf.d/qorven.conf"
		if _, err := os.Stat(nginxConf); err == nil {
			if err := os.Remove(nginxConf); err != nil {
				fmt.Printf("warn: %v\n", err)
			} else {
				// Reload nginx so it stops serving the old config.
				if _, lookErr := exec.LookPath("nginx"); lookErr == nil {
					exec.Command("nginx", "-s", "reload").Run()
				}
				fmt.Println("done")
			}
		} else {
			fmt.Println("(not found — skipped)")
		}

		// 5. Drop PostgreSQL database and role (best-effort)
		fmt.Print("  Dropping database qorven… ")
		if dropdbPath, err := exec.LookPath("dropdb"); err == nil {
			out, err := exec.Command("sudo", "-u", "postgres",
				dropdbPath, "--if-exists", "qorven").CombinedOutput()
			if err != nil {
				fmt.Printf("warn: %s\n", strings.TrimSpace(string(out)))
			} else {
				fmt.Println("done")
			}
		} else {
			fmt.Println("skipped (dropdb not found)")
		}
		fmt.Print("  Dropping role qorven… ")
		if psqlPath, err := exec.LookPath("psql"); err == nil {
			out, err := exec.Command("sudo", "-u", "postgres",
				psqlPath, "-c", "DROP ROLE IF EXISTS qorven;").CombinedOutput()
			if err != nil {
				fmt.Printf("warn: %s\n", strings.TrimSpace(string(out)))
			} else {
				fmt.Println("done")
			}
		} else {
			fmt.Println("skipped (psql not found)")
		}

		// 6. Remove the 'qorven' OS system user
		// userdel removes the user; --remove also wipes its home dir (none here,
		// since the user was created with --no-create-home, but --remove is safe).
		fmt.Print("  Removing OS user 'qorven'… ")
		if _, lookErr := exec.LookPath("userdel"); lookErr == nil {
			out, err := exec.Command("userdel", "--remove", "qorven").CombinedOutput()
			if err != nil {
				msg := strings.TrimSpace(string(out))
				if strings.Contains(msg, "does not exist") {
					fmt.Println("(not found — skipped)")
				} else {
					fmt.Printf("warn: %s\n", msg)
				}
			} else {
				fmt.Println("done")
			}
		} else {
			// macOS
			exec.Command("dscl", ".", "-delete", "/Users/qorven").Run()
			fmt.Println("done")
		}
	}

	fmt.Println()
	fmt.Println("  Qorven has been uninstalled.")
	if !purge {
		fmt.Printf("  Config and data preserved at %s\n", config.DataDir())
		fmt.Println("  Re-run with --purge to also remove config, data, database, nginx config, and the OS user.")
		fmt.Println("  Note: PostgreSQL itself is not removed. To remove it:")
		fmt.Println("    sudo apt-get remove --purge postgresql*   (Debian/Ubuntu)")
		fmt.Println("    brew uninstall postgresql@16              (macOS/Homebrew)")
	}
	return nil
}

func init() {
	uninstallCmd.Flags().Bool("purge", false,
		"Also remove config, data, database, nginx config, and the 'qorven' OS user")
	uninstallCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	rootCmd.AddCommand(uninstallCmd)
}
