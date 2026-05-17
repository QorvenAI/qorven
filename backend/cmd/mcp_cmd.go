// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/output"
	"github.com/qorvenai/qorven/cmd/tui"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage MCP servers",
}

var mcpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List MCP servers",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/mcp/servers")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "NAME", "URL", "STATUS")
		for _, s := range unmarshalList(data) {
			tbl.AddRow(str(s, "id"), str(s, "name"), str(s, "url"), str(s, "status"))
		}
		printer.Print(tbl)
		return nil
	},
}

var mcpGetCmd = &cobra.Command{
	Use:  "get <id>",
	Short: "Get MCP server details",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/mcp/servers/" + args[0])
		if err != nil {
			return err
		}
		printer.Print(unmarshalMap(data))
		return nil
	},
}

var mcpCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Register a new MCP server",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		name, _ := cmd.Flags().GetString("name")
		url, _ := cmd.Flags().GetString("url")
		transport, _ := cmd.Flags().GetString("transport")
		body := buildBody("name", name, "url", url, "transport", transport)
		data, err := c.Post("/v1/mcp/servers", body)
		if err != nil {
			return err
		}
		m := unmarshalMap(data)
		printer.Success(fmt.Sprintf("MCP server registered: %s", str(m, "id")))
		return nil
	},
}

var mcpDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete MCP server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tui.Confirm("Delete this MCP server?", cfg.Yes) {
			return nil
		}
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Delete("/v1/mcp/servers/" + args[0])
		if err != nil {
			return err
		}
		printer.Success("MCP server deleted")
		return nil
	},
}

var mcpTestCmd = &cobra.Command{
	Use:   "test <id>",
	Short: "Test MCP server connection",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Post("/v1/mcp/servers/"+args[0]+"/test", nil)
		if err != nil {
			return err
		}
		printer.Success("MCP server connection OK")
		return nil
	},
}

var mcpToolsCmd = &cobra.Command{
	Use:   "tools <id>",
	Short: "List tools from MCP server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/mcp/servers/" + args[0] + "/tools")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("NAME", "DESCRIPTION")
		for _, t := range unmarshalList(data) {
			desc := str(t, "description")
			if len(desc) > 60 {
				desc = desc[:60] + "..."
			}
			tbl.AddRow(str(t, "name"), desc)
		}
		printer.Print(tbl)
		return nil
	},
}

func init() {
	mcpCreateCmd.Flags().String("name", "", "Server name")
	mcpCreateCmd.Flags().String("url", "", "Server URL")
	mcpCreateCmd.Flags().String("transport", "stdio", "Transport: stdio, sse")
	_ = mcpCreateCmd.MarkFlagRequired("name")
	_ = mcpCreateCmd.MarkFlagRequired("url")

	mcpCmd.AddCommand(mcpListCmd, mcpGetCmd, mcpCreateCmd, mcpDeleteCmd, mcpTestCmd, mcpToolsCmd)
	rootCmd.AddCommand(mcpCmd)
}
