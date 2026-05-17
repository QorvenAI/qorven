// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/wizard"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive TUI setup wizard",
	Long: `First-time setup wizard for Qorven.

Walks you through creating an admin account, configuring a workspace,
connecting an LLM provider, and optionally adding channels and voice.

The wizard talks to a running Qorven server. Start the server first:

  qorven start

Then run the wizard:

  qorven setup                           # connects to http://localhost
  qorven setup --server http://1.2.3.4   # connect to a remote server
  qorven setup --inline                  # no alt-screen (for CI / logs)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// --server is a persistent root flag; cfg.Server is already resolved.
		server := cfg.Server
		if server == "" {
			server = "http://localhost"
		}
		inline, _ := cmd.Flags().GetBool("inline")
		if inline {
			return wizard.RunInline(server)
		}
		return wizard.Run(server)
	},
}

func init() {
	setupCmd.Flags().Bool("inline", false, "Run without alt-screen (useful in CI)")
	rootCmd.AddCommand(setupCmd)
}
