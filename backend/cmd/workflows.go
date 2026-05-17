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

var workflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "Manage workflows",
}

var workflowsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workflows",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/workflows")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "NAME", "TRIGGER", "STEPS", "ENABLED")
		for _, w := range unmarshalList(data) {
			steps := "0"
			if s, ok := w["steps"].([]any); ok {
				steps = fmt.Sprintf("%d", len(s))
			}
			tbl.AddRow(
				str(w, "id"),
				str(w, "name"),
				str(w, "trigger_type"),
				steps,
				str(w, "enabled"),
			)
		}
		printer.Print(tbl)
		return nil
	},
}

var workflowsGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get workflow details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/workflows/" + args[0])
		if err != nil {
			return err
		}
		printer.Print(unmarshalMap(data))
		return nil
	},
}

var workflowsDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a workflow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tui.Confirm(fmt.Sprintf("Delete workflow %s?", args[0]), cfg.Yes) {
			return nil
		}
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Delete("/v1/workflows/" + args[0])
		if err != nil {
			return err
		}
		printer.Success("Workflow deleted")
		return nil
	},
}

var workflowsToggleCmd = &cobra.Command{
	Use:   "toggle <id>",
	Short: "Enable or disable a workflow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Post("/v1/workflows/"+args[0]+"/toggle", nil)
		if err != nil {
			return err
		}
		printer.Success("Workflow toggled")
		return nil
	},
}

func init() {
	workflowsCmd.AddCommand(workflowsListCmd, workflowsGetCmd, workflowsDeleteCmd, workflowsToggleCmd)
	rootCmd.AddCommand(workflowsCmd)
}
