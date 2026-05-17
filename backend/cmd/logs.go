// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Stream gateway logs (journalctl -u qorven -f)",
	// Bare `qorven logs` uses journalctl when available, falls back to log file.
	RunE: func(cmd *cobra.Command, args []string) error {
		return streamLogs(cmd)
	},
}

var logsTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Stream gateway logs in real-time",
	RunE: func(cmd *cobra.Command, args []string) error {
		return streamLogs(cmd)
	},
}

func streamLogs(cmd *cobra.Command) error {
	lines, _ := cmd.Flags().GetInt("lines")
	level, _ := cmd.Flags().GetString("level")
	since, _ := cmd.Flags().GetString("since")

	// Prefer journalctl — the standard path on a systemd install.
	if path, err := exec.LookPath("journalctl"); err == nil {
		jArgs := []string{"-u", "qorven", "-f", "--no-pager"}
		if lines > 0 {
			jArgs = append(jArgs, "-n", fmt.Sprintf("%d", lines))
		}
		if since != "" {
			jArgs = append(jArgs, "--since", since)
		}
		c := exec.Command(path, jArgs...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		_ = level // journalctl owns its own output filtering
		return c.Run()
	}

	// Fallback: tail the log file directly.
	return tailLogFile(lines, level)
}

func tailLogFile(lines int, level string) error {
	home, _ := os.UserHomeDir()
	logFile := home + "/.qorven/logs/qorven.log"
	if _, err := os.Stat(logFile); err != nil {
		logFile = "/tmp/qorven.log"
		if _, err := os.Stat(logFile); err != nil {
			return fmt.Errorf("no log file found at ~/.qorven/logs/qorven.log or /tmp/qorven.log")
		}
	}

	f, err := os.Open(logFile)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, _ := f.Stat()
	if stat.Size() > 0 && lines > 0 {
		seekTailLines(f, lines)
	}

	fmt.Printf("Streaming logs from %s (Ctrl+C to stop)\n\n", logFile)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	scanner := bufio.NewScanner(f)
	for {
		select {
		case <-sigCh:
			fmt.Println("\nStopped.")
			return nil
		default:
		}

		if scanner.Scan() {
			line := scanner.Text()
			if level != "" && !strings.Contains(strings.ToLower(line), strings.ToLower(level)) {
				continue
			}
			fmt.Println(colorizeLine(line))
		} else {
			time.Sleep(200 * time.Millisecond)
			scanner = bufio.NewScanner(f)
		}
	}
}

func seekTailLines(f *os.File, n int) {
	stat, _ := f.Stat()
	size := stat.Size()
	buf := make([]byte, 1)
	count := 0
	for pos := size - 1; pos >= 0; pos-- {
		f.ReadAt(buf, pos)
		if buf[0] == '\n' {
			count++
			if count > n {
				f.Seek(pos+1, 0)
				return
			}
		}
	}
	f.Seek(0, 0)
}

func colorizeLine(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "error") || strings.Contains(lower, "fatal") {
		return "\033[31m" + line + "\033[0m"
	}
	if strings.Contains(lower, "warn") {
		return "\033[33m" + line + "\033[0m"
	}
	return line
}

func init() {
	for _, c := range []*cobra.Command{logsCmd, logsTailCmd} {
		c.Flags().StringP("level", "l", "", "Filter: info, warn, error")
		c.Flags().String("since", "", "Show logs since (e.g. '1 hour ago', '2026-01-01')")
		c.Flags().IntP("lines", "n", 50, "Initial lines to show")
	}
	logsCmd.AddCommand(logsTailCmd)
	rootCmd.AddCommand(logsCmd)
}
