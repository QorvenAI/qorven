// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Show current configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgFile := os.Getenv("QORVEN_CONFIG")
		if cfgFile == "" {
			cfgFile = "config.toml"
		}
		// Try ~/.qorven/config.toml as fallback
		if _, err := os.Stat(cfgFile); err != nil {
			home, _ := os.UserHomeDir()
			cfgFile = home + "/.qorven/config.toml"
		}
		data, err := os.ReadFile(cfgFile)
		if err != nil {
			return fmt.Errorf("config file not found: %s", cfgFile)
		}
		fmt.Print(string(data))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a dotted key path in config.toml.

Examples:
  qorven config set server.listen 0.0.0.0:80
  qorven config set auth.gateway_token mytoken`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// For now, show the instruction
		fmt.Printf("To set %s = %s, edit your config.toml:\n", args[0], args[1])
		cfgFile := os.Getenv("QORVEN_CONFIG")
		if cfgFile == "" {
			home, _ := os.UserHomeDir()
			cfgFile = home + "/.qorven/config.toml"
		}
		fmt.Printf("  %s\n", cfgFile)
		return nil
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open config file in editor",
	RunE: func(cmd *cobra.Command, args []string) error {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "nano"
		}
		cfgFile := os.Getenv("QORVEN_CONFIG")
		if cfgFile == "" {
			home, _ := os.UserHomeDir()
			cfgFile = home + "/.qorven/config.toml"
		}
		fmt.Printf("Run: %s %s\n", editor, cfgFile)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configGetCmd, configSetCmd, configEditCmd)
	rootCmd.AddCommand(configCmd)
}
