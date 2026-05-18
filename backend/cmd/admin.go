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

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Administration commands",
}

// --- Approvals ---

var adminApprovalsCmd = &cobra.Command{Use: "approvals", Short: "Manage execution approvals"}

var adminApprovalsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending approvals",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/approvals")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "AGENT", "TOOL", "STATUS", "CREATED")
		for _, a := range unmarshalList(data) {
			tbl.AddRow(str(a, "id"), str(a, "agent_id"),
				str(a, "tool_name"), str(a, "status"), str(a, "created_at")[:10])
		}
		printer.Print(tbl)
		return nil
	},
}

var adminApprovalsApproveCmd = &cobra.Command{
	Use:  "approve <id>",
	Short: "Approve an execution",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Post("/v1/approvals/"+args[0]+"/approve", nil)
		if err != nil {
			return err
		}
		printer.Success("Approved")
		return nil
	},
}

var adminApprovalsDenyCmd = &cobra.Command{
	Use:  "deny <id>",
	Short: "Deny an execution",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Post("/v1/approvals/"+args[0]+"/deny", nil)
		if err != nil {
			return err
		}
		printer.Success("Denied")
		return nil
	},
}

// --- Activity / Audit ---

var adminActivityCmd = &cobra.Command{
	Use:   "activity",
	Short: "View audit log",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/audit")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("TIME", "ACTOR", "ACTION", "RESOURCE", "DETAIL")
		for _, e := range unmarshalList(data) {
			ts := str(e, "created_at")
			if len(ts) > 16 {
				ts = ts[:16]
			}
			detail := str(e, "detail")
			if len(detail) > 40 {
				detail = detail[:40] + "..."
			}
			tbl.AddRow(ts, str(e, "actor"), str(e, "action"), str(e, "resource"), detail)
		}
		printer.Print(tbl)
		return nil
	},
}

// --- API Keys ---

var adminKeysCmd = &cobra.Command{Use: "api-keys", Short: "Manage API keys"}

var adminKeysListCmd = &cobra.Command{
	Use:   "list",
	Short: "List API keys (masked)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/api-keys")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "NAME", "PREFIX", "CREATED")
		for _, k := range unmarshalList(data) {
			tbl.AddRow(str(k, "id"), str(k, "name"), str(k, "prefix"), str(k, "created_at")[:10])
		}
		printer.Print(tbl)
		return nil
	},
}

var adminKeysCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new API key",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		name, _ := cmd.Flags().GetString("name")
		body := buildBody("name", name)
		data, err := c.Post("/v1/api-keys", body)
		if err != nil {
			return err
		}
		m := unmarshalMap(data)
		fmt.Printf("API Key created: %s\n", str(m, "name"))
		fmt.Printf("Key: %s\n", str(m, "key"))
		fmt.Println("(Save this key - it won't be shown again)")
		return nil
	},
}

var adminKeysRevokeCmd = &cobra.Command{
	Use:   "revoke <id>",
	Short: "Revoke an API key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tui.Confirm(fmt.Sprintf("Revoke API key %s?", args[0]), cfg.Yes) {
			return nil
		}
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Delete("/v1/api-keys/" + args[0])
		if err != nil {
			return err
		}
		printer.Success("API key revoked")
		return nil
	},
}

// --- Dead-letter queue ---

var adminDeadLettersCmd = &cobra.Command{
	Use:   "dead-letters",
	Short: "List dead-lettered wakeup requests",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/admin/dead-letters")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "CAUSE", "PLAN", "ATTEMPTS", "REASON", "DEAD_AT")
		for _, dl := range unmarshalList(data) {
			planID := str(dl, "plan_id")
			if len(planID) > 8 {
				planID = planID[:8]
			}
			deadAt := str(dl, "consumed_at")
			if len(deadAt) > 16 {
				deadAt = deadAt[:16]
			}
			reason := str(dl, "dead_letter_reason")
			if len(reason) > 40 {
				reason = reason[:40] + "..."
			}
			tbl.AddRow(str(dl, "id"), str(dl, "cause"), planID, str(dl, "attempts"), reason, deadAt)
		}
		printer.Print(tbl)
		return nil
	},
}

func init() {
	adminKeysCreateCmd.Flags().String("name", "", "Key name")
	_ = adminKeysCreateCmd.MarkFlagRequired("name")

	adminApprovalsCmd.AddCommand(adminApprovalsListCmd, adminApprovalsApproveCmd, adminApprovalsDenyCmd)
	adminKeysCmd.AddCommand(adminKeysListCmd, adminKeysCreateCmd, adminKeysRevokeCmd)
	adminCmd.AddCommand(adminApprovalsCmd, adminActivityCmd, adminKeysCmd, adminDeadLettersCmd)
	rootCmd.AddCommand(adminCmd)
}
