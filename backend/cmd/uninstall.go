// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove Qorven completely",
	Long: `Stops the service, removes the binary, and optionally drops the database
and configuration. Data (database, config, logs) is preserved by default
unless --purge is given.`,
	RunE: runUninstall,
}

func runUninstall(cmd *cobra.Command, args []string) error {
	purge, _ := cmd.Flags().GetBool("purge")
	yes, _ := cmd.Flags().GetBool("yes")

	if !yes {
		msg := "This will stop Qorven and remove the binary."
		if purge {
			msg = "This will stop Qorven, remove the binary, configuration, and database."
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

	// 1. Stop service
	fmt.Print("  Stopping service... ")
	if path, err := exec.LookPath("systemctl"); err == nil {
		exec.Command(path, "stop", "qorven").Run()
		exec.Command(path, "disable", "qorven").Run()
		exec.Command("rm", "-f", "/etc/systemd/system/qorven.service").Run()
		if path2, err2 := exec.LookPath("systemctl"); err2 == nil {
			exec.Command(path2, "daemon-reload").Run()
		}
	} else {
		killRuntimePID()
	}
	fmt.Println("done")

	// 2. Remove binary
	fmt.Print("  Removing binary... ")
	for _, p := range []string{"/usr/local/bin/qorven", "/usr/bin/qorven"} {
		if _, err := os.Stat(p); err == nil {
			if err := os.Remove(p); err != nil {
				fmt.Printf("warn: %v\n", err)
			} else {
				fmt.Printf("removed %s\n", p)
				break
			}
		}
	}

	if purge {
		// 3. Remove config + data
		fmt.Print("  Removing config and logs... ")
		home, _ := os.UserHomeDir()
		for _, d := range []string{
			filepath.Join(home, ".qorven"),
			"/etc/qorven",
			"/var/lib/qorven",
			"/var/log/qorven",
		} {
			os.RemoveAll(d)
		}
		fmt.Println("done")

		// 4. Drop database (best-effort)
		fmt.Print("  Dropping database... ")
		if path, err := exec.LookPath("dropdb"); err == nil {
			out, err := exec.Command("sudo", "-u", "postgres", path, "--if-exists", "qorven").CombinedOutput()
			if err != nil {
				fmt.Printf("warn: %s\n", strings.TrimSpace(string(out)))
			} else {
				fmt.Println("done")
			}
		} else {
			fmt.Println("skipped (dropdb not found)")
		}
	}

	fmt.Println()
	fmt.Println("  Qorven has been uninstalled.")
	if !purge {
		home, _ := os.UserHomeDir()
		fmt.Printf("  Config and data preserved at %s/.qorven\n", home)
		fmt.Println("  Re-run with --purge to remove everything.")
	}
	return nil
}

func init() {
	uninstallCmd.Flags().Bool("purge", false, "Also remove config, logs, and database")
	rootCmd.AddCommand(uninstallCmd)
}
