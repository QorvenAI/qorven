// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/output"
	"github.com/qorvenai/qorven/cmd/tui"
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Manage chat sessions",
}

var sessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/sessions")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "AGENT", "CHANNEL", "LABEL", "TOKENS", "UPDATED")
		for _, s := range unmarshalList(data) {
			tokens := fmt.Sprintf("%s/%s",
				str(s, "input_tokens"), str(s, "output_tokens"))
			tbl.AddRow(
				str(s, "id"),
				str(s, "agent_id")[:8],
				str(s, "channel"),
				str(s, "label"),
				tokens,
				str(s, "updated_at")[:10],
			)
		}
		printer.Print(tbl)
		return nil
	},
}

var sessionsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get session details and messages",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/sessions/" + args[0] + "/messages")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalMap(data))
			return nil
		}
		// Print messages in readable format
		var resp struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
				Channel string `json:"channel"`
			} `json:"messages"`
		}
		json.Unmarshal(data, &resp)
		for _, m := range resp.Messages {
			label := m.Role
			if m.Channel != "" {
				label += " [" + m.Channel + "]"
			}
			content := m.Content
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			fmt.Printf("%s: %s\n\n", label, content)
		}
		return nil
	},
}

var sessionsDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tui.Confirm(fmt.Sprintf("Delete session %s?", args[0]), cfg.Yes) {
			return nil
		}
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Delete("/v1/sessions/" + args[0])
		if err != nil {
			return err
		}
		printer.Success("Session deleted")
		return nil
	},
}

func init() {
	sessionsCmd.AddCommand(sessionsListCmd, sessionsGetCmd, sessionsDeleteCmd)
	rootCmd.AddCommand(sessionsCmd)
}
