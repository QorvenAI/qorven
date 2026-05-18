// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/client"
	"github.com/qorvenai/qorven/cmd/output"
)

var (
	cfg     *Config
	printer *output.Printer
	rootCmd = &cobra.Command{
		Use:   "qorven",
		Short: "Self-hosted AI agent platform",
		Long: `⚡ Qorven — Self-hosted AI Agent Platform

GETTING STARTED:
  qorven start          Start the Qorven server
  qorven init           Interactive setup wizard
  qorven chat           Chat with an agent

SERVER:
  qorven start          Start server (foreground)
  qorven stop           Stop the running server
  qorven restart        Restart the server
  qorven status         Show server status
  qorven logs           Stream server logs (journalctl -u qorven -f)
  qorven monitor        Watch service health in real-time

AGENTS:
  qorven chat           Chat with Prime (or specify --agent)
  qorven agents         List all agents
  qorven agent create   Create a new agent

ADMIN:
  qorven auth login     Login to get a token
  qorven auth setup     Create admin account
  qorven backup         Backup database + config
  qorven restore        Restore from backup
  qorven doctor         Run diagnostics
  qorven update         Check for and install updates
  qorven uninstall      Remove Qorven completely

CONFIG:
  qorven config         Show current config
  qorven version        Show version

Run 'qorven <command> --help' for details on any command.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg = loadConfig(cmd)
			printer = output.NewPrinter(cfg.OutputFormat)
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}
)

// Config holds resolved CLI configuration.
type Config struct {
	Server       string
	Token        string
	OutputFormat string
	Verbose      bool
	Yes          bool
}

func loadConfig(cmd *cobra.Command) *Config {
	c := &Config{OutputFormat: "table"}

	// 1. Env vars
	if v := os.Getenv("QORVEN_SERVER"); v != "" {
		c.Server = v
	} else {
		c.Server = "http://localhost"
	}
	if v := os.Getenv("QORVEN_TOKEN"); v != "" {
		c.Token = v
	}
	if v := os.Getenv("QORVEN_GATEWAY_TOKEN"); v != "" && c.Token == "" {
		c.Token = v
	}

	// 2. Auto-load ~/.qorven/.env
	loadQorvenEnv()

	// Re-check after env load — .env may have set QORVEN_SERVER / QORVEN_TOKEN
	if v := os.Getenv("QORVEN_SERVER"); v != "" {
		c.Server = v
	}
	if c.Token == "" {
		if v := os.Getenv("QORVEN_TOKEN"); v != "" {
			c.Token = v
		}
		if v := os.Getenv("QORVEN_GATEWAY_TOKEN"); v != "" {
			c.Token = v
		}
	}

	// 2b. Saved profile token (from `qorven auth login`)
	if c.Token == "" {
		pf, _ := loadProfiles()
		profile := pf.Active
		if profile == "" {
			profile = "default"
		}
		if tok := loadToken(profile); tok != "" {
			c.Token = tok
		}
		if c.Server == "http://localhost" {
			for _, p := range pf.Profiles {
				if p.Name == profile && p.Server != "" {
					c.Server = p.Server
				}
			}
		}
	}

	// 3. Flags override
	if cmd.Flags().Changed("server") {
		c.Server, _ = cmd.Flags().GetString("server")
	}
	if cmd.Flags().Changed("token") {
		c.Token, _ = cmd.Flags().GetString("token")
	}
	if cmd.Flags().Changed("output") {
		c.OutputFormat, _ = cmd.Flags().GetString("output")
	}
	if cmd.Flags().Changed("verbose") {
		c.Verbose, _ = cmd.Flags().GetBool("verbose")
	}
	c.Yes, _ = cmd.Flags().GetBool("yes")

	return c
}

// loadQorvenEnv sources ~/.qorven/.env if it exists.
func loadQorvenEnv() {
	home, _ := os.UserHomeDir()
	envFile := home + "/.qorven/.env"
	data, err := os.ReadFile(envFile)
	if err != nil {
		return
	}
	for _, line := range splitLines(string(data)) {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		// Strip "export " prefix
		if len(line) > 7 && line[:7] == "export " {
			line = line[7:]
		}
		if idx := indexOf(line, '='); idx > 0 {
			key := line[:idx]
			val := line[idx+1:]
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

// newHTTP creates an authenticated HTTP client from config.
func newHTTP() (*client.HTTPClient, error) {
	if cfg.Server == "" {
		return nil, client.ErrServerRequired
	}
	if cfg.Token == "" {
		return nil, client.ErrNotAuthenticated
	}
	return client.NewHTTPClient(cfg.Server, cfg.Token, false), nil
}

// Execute runs the root command.
func Execute() {
	// NO_COLOR is no longer needed — TUI uses alt-screen which handles
	// terminal queries properly. Plain chat mode uses no ANSI escapes.
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.String("server", "", "Gateway URL (env: QORVEN_SERVER)")
	pf.String("token", "", "Auth token (env: QORVEN_TOKEN)")
	pf.StringP("output", "o", "table", "Output format: table, json, yaml")
	pf.BoolP("yes", "y", false, "Skip confirmation prompts")
	pf.BoolP("verbose", "v", false, "Verbose output")
}

// Helpers - avoid importing strings for tiny ops
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
