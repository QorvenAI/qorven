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

var teamsCmd = &cobra.Command{
	Use:   "teams",
	Short: "Manage agent teams",
}

var teamsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List teams",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/teams")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "NAME", "MEMBERS", "TASKS")
		for _, t := range unmarshalList(data) {
			tbl.AddRow(str(t, "id"), str(t, "name"),
				str(t, "member_count"), str(t, "task_count"))
		}
		printer.Print(tbl)
		return nil
	},
}

var teamsGetCmd = &cobra.Command{
	Use:  "get <id>",
	Short: "Get team details",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/teams/" + args[0])
		if err != nil {
			return err
		}
		printer.Print(unmarshalMap(data))
		return nil
	},
}

var teamsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a team",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		name, _ := cmd.Flags().GetString("name")
		desc, _ := cmd.Flags().GetString("description")
		body := buildBody("name", name, "description", desc)
		data, err := c.Post("/v1/teams", body)
		if err != nil {
			return err
		}
		m := unmarshalMap(data)
		printer.Success(fmt.Sprintf("Team created: %s (ID: %s)", str(m, "name"), str(m, "id")))
		return nil
	},
}

var teamsDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a team",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tui.Confirm(fmt.Sprintf("Delete team %s?", args[0]), cfg.Yes) {
			return nil
		}
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Delete("/v1/teams/" + args[0])
		if err != nil {
			return err
		}
		printer.Success("Team deleted")
		return nil
	},
}

// --- Members ---

var teamsMembersCmd = &cobra.Command{Use: "members", Short: "Manage team members"}

var teamsMembersListCmd = &cobra.Command{
	Use:  "list <teamID>",
	Short: "List team members",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/teams/" + args[0] + "/members")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("AGENT_ID", "NAME", "ROLE")
		for _, m := range unmarshalList(data) {
			tbl.AddRow(str(m, "agent_id")[:8], str(m, "display_name"), str(m, "role"))
		}
		printer.Print(tbl)
		return nil
	},
}

var teamsMembersAddCmd = &cobra.Command{
	Use:   "add <teamID>",
	Short: "Add team member",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		agent, _ := cmd.Flags().GetString("agent")
		role, _ := cmd.Flags().GetString("role")
		_, err = c.Post("/v1/teams/"+args[0]+"/members",
			buildBody("agent_id", agent, "role", role))
		if err != nil {
			return err
		}
		printer.Success("Member added")
		return nil
	},
}

var teamsMembersRemoveCmd = &cobra.Command{
	Use:   "remove <teamID>",
	Short: "Remove team member",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		agent, _ := cmd.Flags().GetString("agent")
		_, err = c.Delete("/v1/teams/" + args[0] + "/members/" + agent)
		if err != nil {
			return err
		}
		printer.Success("Member removed")
		return nil
	},
}

func init() {
	teamsCreateCmd.Flags().String("name", "", "Team name")
	teamsCreateCmd.Flags().String("description", "", "Team description")
	_ = teamsCreateCmd.MarkFlagRequired("name")

	teamsMembersAddCmd.Flags().String("agent", "", "Agent ID to add")
	teamsMembersAddCmd.Flags().String("role", "member", "Role: lead, member")
	_ = teamsMembersAddCmd.MarkFlagRequired("agent")
	teamsMembersRemoveCmd.Flags().String("agent", "", "Agent ID to remove")
	_ = teamsMembersRemoveCmd.MarkFlagRequired("agent")

	teamsMembersCmd.AddCommand(teamsMembersListCmd, teamsMembersAddCmd, teamsMembersRemoveCmd)
	teamsCmd.AddCommand(teamsListCmd, teamsGetCmd, teamsCreateCmd, teamsDeleteCmd, teamsMembersCmd)
	rootCmd.AddCommand(teamsCmd)
}
