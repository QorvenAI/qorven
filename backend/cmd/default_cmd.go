// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/tui"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qorvenai/qorven/internal/config"
)

func init() {
	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runDefaultCommand()
	}
}

func runDefaultCommand() error {
	home, _ := os.UserHomeDir()
	qHome := filepath.Join(home, ".qorven")

	// 1. No config? → guide to init
	cfgFile := os.Getenv("QORVEN_CONFIG")
	if cfgFile == "" {
		cfgFile = filepath.Join(qHome, "config.toml")
	}
	serverCfg, err := config.Load(cfgFile)
	if err != nil || serverCfg == nil {
		fmt.Println()
		fmt.Println("  Welcome to Qorven!")
		fmt.Println()
		fmt.Println("  Run:  qorven init")
		fmt.Println()
		return nil
	}

	// 2. Gateway not running?
	c, err := newHTTP()
	if err != nil || c.HealthCheck() != nil {
		fmt.Println()
		fmt.Println("  Gateway is not running.")
		fmt.Println()
		fmt.Println("  Start it:  qorven start")
		fmt.Println("  Then:      qorven chat")
		fmt.Println()
		return nil
	}

	// 3. Everything ready → launch TUI chat
	agentID, agentName, agentModel, _ := resolveAgent(c, "")
	if agentID == "" {
		fmt.Println("  No agents configured. Run: qorven init")
		return nil
	}
	sessionID := ""
	if sid, err := createSession(c, agentID); err == nil {
		sessionID = sid
	}
	_ = qHome
	return tui.Run(agentName, agentID, agentModel, sessionID)
}
