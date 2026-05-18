// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the Qorven server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServiceCommand("stop")
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "restart",
		Short: "Restart the Qorven server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServiceCommand("restart")
		},
	})
}

// runServiceCommand runs a systemctl action (stop/restart/start) on the
// qorven unit. Falls back to sending SIGTERM to a running process when
// systemd is not available (e.g. Docker, macOS, dev env).
func runServiceCommand(action string) error {
	// Try systemctl first — the normal case for a production install.
	if path, err := exec.LookPath("systemctl"); err == nil {
		out, err := exec.Command(path, action, "qorven").CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemctl %s qorven: %s", action, string(out))
		}
		fmt.Printf("qorven %sed\n", action)
		return nil
	}

	// systemctl not available — use the runtime.json to find the PID.
	if action == "stop" || action == "restart" {
		if err := killRuntimePID(); err != nil {
			fmt.Fprintf(os.Stderr, "warn: %v\n", err)
		} else {
			fmt.Println("qorven stopped")
		}
	}
	if action == "restart" {
		fmt.Println("hint: run 'qorven start' to start again")
	}
	return nil
}

func killRuntimePID() error {
	home, _ := os.UserHomeDir()
	pidFile := home + "/.qorven/runtime.json"
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("runtime.json not found — is qorven running?")
	}
	// Parse pid field without pulling in encoding/json at the top level.
	var pid int
	fmt.Sscanf(string(extractJSONField(data, "pid")), "%d", &pid)
	if pid <= 0 {
		return fmt.Errorf("no PID in runtime.json")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found", pid)
	}
	return p.Signal(os.Interrupt)
}

// extractJSONField does a minimal scan for `"key": <value>` without
// importing encoding/json — the only value we need is pid (a number).
func extractJSONField(data []byte, key string) []byte {
	needle := `"` + key + `":`
	s := string(data)
	idx := 0
	for i := 0; i < len(s)-len(needle); i++ {
		if s[i:i+len(needle)] == needle {
			idx = i + len(needle)
			break
		}
	}
	if idx == 0 {
		return nil
	}
	// Skip whitespace
	for idx < len(s) && (s[idx] == ' ' || s[idx] == '\t' || s[idx] == '\n') {
		idx++
	}
	// Read until comma, newline, or closing brace
	end := idx
	for end < len(s) && s[end] != ',' && s[end] != '\n' && s[end] != '}' {
		end++
	}
	return []byte(s[idx:end])
}
