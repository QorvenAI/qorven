// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/output"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Manage tools",
}

var toolsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available tools",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/tools/builtin")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("NAME", "DESCRIPTION", "ENABLED")
		for _, t := range unmarshalList(data) {
			desc := str(t, "description")
			if len(desc) > 50 {
				desc = desc[:50] + "..."
			}
			tbl.AddRow(str(t, "name"), desc, str(t, "enabled"))
		}
		printer.Print(tbl)
		return nil
	},
}

func init() {
	toolsCmd.AddCommand(toolsListCmd)
	rootCmd.AddCommand(toolsCmd)
}
