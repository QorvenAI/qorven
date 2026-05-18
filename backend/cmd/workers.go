// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"

	"github.com/qorvenai/qorven/cmd/output"
	"github.com/spf13/cobra"
)

var workersCmd = &cobra.Command{
	Use:   "workers",
	Short: "Manage daemon agent workers",
}

var workersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered daemon agents, active tasks, and pending plans",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}

		// Agents
		agentsData, err := c.Get("/v1/daemon/agents")
		if err != nil {
			return fmt.Errorf("agents: %w", err)
		}
		agentsTbl := output.NewTable("ID", "NAME", "PROVIDER", "STATUS", "CAPABILITIES")
		agents := unmarshalList(agentsData)
		for _, a := range agents {
			caps := ""
			if c, ok := a["capabilities"].([]any); ok {
				for i, v := range c {
					if i > 0 {
						caps += ","
					}
					caps += fmt.Sprintf("%v", v)
				}
			}
			id := str(a, "id")
			if len(id) > 8 {
				id = id[:8]
			}
			agentsTbl.AddRow(id, str(a, "name"), str(a, "provider"), str(a, "status"), caps)
		}
		fmt.Println("=== Agents ===")
		printer.Print(agentsTbl)

		// Tasks
		tasksData, err := c.Get("/v1/daemon/tasks")
		if err != nil {
			return fmt.Errorf("tasks: %w", err)
		}
		tasksTbl := output.NewTable("ID", "TITLE", "OWNER", "PRIORITY", "STATUS", "%")
		for _, t := range unmarshalList(tasksData) {
			id := str(t, "id")
			if len(id) > 8 {
				id = id[:8]
			}
			pct := ""
			if p, ok := t["percent"].(float64); ok && p > 0 {
				pct = fmt.Sprintf("%.0f", p)
			}
			tasksTbl.AddRow(id, str(t, "title"), str(t, "owner"), str(t, "priority"), str(t, "status"), pct)
		}
		fmt.Println("\n=== Tasks ===")
		printer.Print(tasksTbl)

		// Plans
		plansData, err := c.Get("/v1/daemon/plans")
		if err != nil {
			return fmt.Errorf("plans: %w", err)
		}
		plansTbl := output.NewTable("ID", "TITLE", "PROPOSED BY", "STATUS")
		for _, p := range unmarshalList(plansData) {
			id := str(p, "id")
			if len(id) > 8 {
				id = id[:8]
			}
			plansTbl.AddRow(id, str(p, "title"), str(p, "proposed_by"), str(p, "status"))
		}
		fmt.Println("\n=== Plans ===")
		printer.Print(plansTbl)

		return nil
	},
}

var workersApproveCmd = &cobra.Command{
	Use:   "approve <plan-id>",
	Short: "Approve a pending plan",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Post("/v1/daemon/plans/"+args[0]+"/approve", map[string]string{})
		if err != nil {
			return err
		}
		fmt.Println("Plan approved.")
		return nil
	},
}

var workersRejectCmd = &cobra.Command{
	Use:   "reject <plan-id>",
	Short: "Reject a pending plan",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		reason, _ := cmd.Flags().GetString("reason")
		_, err = c.Post("/v1/daemon/plans/"+args[0]+"/reject", map[string]string{"reason": reason})
		if err != nil {
			return err
		}
		fmt.Println("Plan rejected.")
		return nil
	},
}

func init() {
	workersRejectCmd.Flags().String("reason", "", "Rejection reason")
	workersCmd.AddCommand(workersListCmd, workersApproveCmd, workersRejectCmd)
	rootCmd.AddCommand(workersCmd)
}
