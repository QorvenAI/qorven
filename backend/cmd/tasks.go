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

var tasksCmd = &cobra.Command{
	Use:   "tasks",
	Short: "Manage background tasks",
}

var tasksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		path := "/v1/tasks"
		if v, _ := cmd.Flags().GetString("agent"); v != "" {
			path += "?agent_id=" + v
		}
		data, err := c.Get(path)
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "TITLE", "STATUS", "PRIORITY", "ASSIGNED", "UPDATED")
		for _, t := range unmarshalList(data) {
			title := str(t, "title")
			if len(title) > 40 {
				title = title[:40] + "..."
			}
			updated := str(t, "updated_at")
			if len(updated) > 10 {
				updated = updated[:10]
			}
			tbl.AddRow(
				str(t, "id"),
				title,
				str(t, "status"),
				str(t, "priority"),
				truncate(str(t, "assigned_to"), 8),
				updated,
			)
		}
		printer.Print(tbl)
		return nil
	},
}

var tasksCancelCmd = &cobra.Command{
	Use:   "cancel <id>",
	Short: "Cancel a running task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tui.Confirm(fmt.Sprintf("Cancel task %s?", args[0]), cfg.Yes) {
			return nil
		}
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Post("/v1/tasks/"+args[0]+"/cancel", nil)
		if err != nil {
			return err
		}
		printer.Success("Task cancelled")
		return nil
	},
}

var tasksGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get task details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/tasks/" + args[0])
		if err != nil {
			return err
		}
		printer.Print(unmarshalMap(data))
		return nil
	},
}

func init() {
	tasksListCmd.Flags().String("agent", "", "Filter by agent ID")
	tasksCmd.AddCommand(tasksListCmd, tasksGetCmd, tasksCancelCmd)
	rootCmd.AddCommand(tasksCmd)
}
