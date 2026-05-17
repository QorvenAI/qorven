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

var cronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Manage scheduled jobs",
}

var cronListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cron jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/cron-jobs")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "AGENT", "EXPRESSION", "TASK", "ENABLED", "LAST_RUN")
		for _, j := range unmarshalList(data) {
			task := str(j, "task")
			if len(task) > 40 {
				task = task[:40] + "..."
			}
			tbl.AddRow(
				str(j, "id")[:mmin(8, len(str(j, "id")))],
				str(j, "agent_id")[:mmin(8, len(str(j, "agent_id")))],
				str(j, "cron_expression"),
				task,
				str(j, "enabled"),
				str(j, "last_run_at"),
			)
		}
		printer.Print(tbl)
		return nil
	},
}

var cronCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a cron job",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		agentID, _ := cmd.Flags().GetString("agent")
		expression, _ := cmd.Flags().GetString("expression")
		task, _ := cmd.Flags().GetString("task")

		if task != "" {
			content, err := readContent(task)
			if err != nil {
				return err
			}
			task = content
		}

		body := buildBody(
			"agent_id", agentID,
			"expression", expression,
			"task", task,
			"enabled", true,
		)
		data, err := c.Post("/v1/cron-jobs", body)
		if err != nil {
			return err
		}
		m := unmarshalMap(data)
		printer.Success(fmt.Sprintf("Cron job created: %s", str(m, "id")))
		return nil
	},
}

var cronDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a cron job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tui.Confirm(fmt.Sprintf("Delete cron job %s?", args[0]), cfg.Yes) {
			return nil
		}
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Delete("/v1/cron-jobs/" + args[0])
		if err != nil {
			return err
		}
		printer.Success("Cron job deleted")
		return nil
	},
}

var cronToggleCmd = &cobra.Command{
	Use:   "toggle <id>",
	Short: "Enable or disable a cron job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Post("/v1/cron-jobs/"+args[0]+"/toggle", nil)
		if err != nil {
			return err
		}
		printer.Success("Cron job toggled")
		return nil
	},
}

func init() {
	cronCreateCmd.Flags().String("agent", "", "Agent ID")
	cronCreateCmd.Flags().String("expression", "", "Cron expression (e.g. '0 9 * * *')")
	cronCreateCmd.Flags().String("task", "", "Task prompt (or @file)")
	_ = cronCreateCmd.MarkFlagRequired("agent")
	_ = cronCreateCmd.MarkFlagRequired("expression")
	_ = cronCreateCmd.MarkFlagRequired("task")

	cronCmd.AddCommand(cronListCmd, cronCreateCmd, cronDeleteCmd, cronToggleCmd)
	rootCmd.AddCommand(cronCmd)
}
