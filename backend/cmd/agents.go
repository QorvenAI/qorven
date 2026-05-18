// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/output"
	"github.com/qorvenai/qorven/cmd/tui"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Manage agents",
}

var agentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agents",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/agents")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "KEY", "NAME", "MODEL", "TOOLS", "MEMORY", "STATUS")
		for _, a := range unmarshalList(data) {
			// Skip system agents in table view
			key := str(a, "agent_key")
			if len(key) > 2 && key[:2] == "__" {
				continue
			}
			tbl.AddRow(
				str(a, "id"),
				key,
				str(a, "display_name"),
				str(a, "model"),
				str(a, "tool_profile"),
				str(a, "memory_enabled"),
				str(a, "status"),
			)
		}
		printer.Print(tbl)
		return nil
	},
}

var agentsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get agent details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/agents/" + args[0])
		if err != nil {
			return err
		}
		printer.Print(unmarshalMap(data))
		return nil
	},
}

var agentsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		key, _ := cmd.Flags().GetString("key")
		name, _ := cmd.Flags().GetString("name")
		model, _ := cmd.Flags().GetString("model")
		prompt, _ := cmd.Flags().GetString("prompt")
		toolProfile, _ := cmd.Flags().GetString("tools")

		// @file support for prompt
		if prompt != "" {
			p, err := readContent(prompt)
			if err != nil {
				return err
			}
			prompt = p
		}

		body := buildBody(
			"agent_key", key,
			"display_name", name,
			"model", model,
			"system_prompt", prompt,
			"tool_profile", toolProfile,
		)
		data, err := c.Post("/v1/agents", body)
		if err != nil {
			return err
		}
		m := unmarshalMap(data)
		printer.Success(fmt.Sprintf("Agent created: %s (ID: %s)", str(m, "display_name"), str(m, "id")))
		return nil
	},
}

var agentsUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update agent configuration",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		body := make(map[string]any)
		flagMap := map[string]string{
			"name":  "display_name",
			"model": "model",
			"tools": "tool_profile",
			"key":   "agent_key",
		}
		for flag, apiKey := range flagMap {
			if cmd.Flags().Changed(flag) {
				val, _ := cmd.Flags().GetString(flag)
				body[apiKey] = val
			}
		}
		if cmd.Flags().Changed("prompt") {
			val, _ := cmd.Flags().GetString("prompt")
			content, err := readContent(val)
			if err != nil {
				return err
			}
			body["system_prompt"] = content
		}
		if len(body) == 0 {
			return fmt.Errorf("no fields to update - use flags like --name, --model, --prompt")
		}
		_, err = c.Put("/v1/agents/"+args[0], body)
		if err != nil {
			return err
		}
		printer.Success("Agent updated")
		return nil
	},
}

var agentsDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete an agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tui.Confirm(fmt.Sprintf("Delete agent %s?", args[0]), cfg.Yes) {
			return nil
		}
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Delete("/v1/agents/" + args[0])
		if err != nil {
			return err
		}
		printer.Success("Agent deleted")
		return nil
	},
}

func init() {
	// Shared flags for create and update
	for _, cmd := range []*cobra.Command{agentsCreateCmd, agentsUpdateCmd} {
		cmd.Flags().String("key", "", "Agent key (unique identifier)")
		cmd.Flags().String("name", "", "Display name")
		cmd.Flags().String("model", "deepseek-chat", "Model identifier")
		cmd.Flags().String("prompt", "", "System prompt (or @filepath)")
		cmd.Flags().String("tools", "full", "Tool profile: full, minimal, code")
	}

	agentsCmd.AddCommand(agentsListCmd, agentsGetCmd, agentsCreateCmd, agentsUpdateCmd, agentsDeleteCmd)
	rootCmd.AddCommand(agentsCmd)
}
