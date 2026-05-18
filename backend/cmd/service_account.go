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

var serviceAccountCmd = &cobra.Command{
	Use:   "service-account",
	Short: "Manage service accounts",
	Aliases: []string{"sa"},
}

var serviceAccountListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all service accounts",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/service-accounts")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalList(data))
			return nil
		}
		tbl := output.NewTable("ID", "ROLE", "TENANT", "DESCRIPTION", "CREATED", "REVOKED")
		for _, sa := range unmarshalList(data) {
			revoked := ""
			if r := str(sa, "revoked_at"); r != "" && r != "null" {
				revoked = r[:10]
			}
			tenant := str(sa, "tenant_id")
			if tenant == "" {
				tenant = "(global)"
			}
			created := str(sa, "created_at")
			if len(created) > 10 {
				created = created[:10]
			}
			tbl.AddRow(str(sa, "id"), str(sa, "role"), tenant, str(sa, "description"), created, revoked)
		}
		printer.Print(tbl)
		return nil
	},
}

var serviceAccountAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add or reactivate a service account",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, _ := cmd.Flags().GetString("id")
		role, _ := cmd.Flags().GetString("role")
		description, _ := cmd.Flags().GetString("description")
		tenantID, _ := cmd.Flags().GetString("tenant")
		global, _ := cmd.Flags().GetBool("global")
		force, _ := cmd.Flags().GetBool("force")

		c, err := newHTTP()
		if err != nil {
			return err
		}
		body := buildBody("id", id, "role", role, "description", description, "tenant_id", tenantID, "global", global, "force", force)
		data, err := c.Post("/v1/service-accounts", body)
		if err != nil {
			return err
		}
		m := unmarshalMap(data)
		fmt.Printf("Service account created: %s (role=%s)\n", str(m, "id"), str(m, "role"))
		return nil
	},
}

var serviceAccountRevokeCmd = &cobra.Command{
	Use:   "revoke <id>",
	Short: "Revoke a service account",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tui.Confirm(fmt.Sprintf("Revoke service account %s?", args[0]), cfg.Yes) {
			return nil
		}
		c, err := newHTTP()
		if err != nil {
			return err
		}
		_, err = c.Delete("/v1/service-accounts/" + args[0])
		if err != nil {
			return err
		}
		printer.Success("Service account revoked")
		return nil
	},
}

func init() {
	serviceAccountAddCmd.Flags().String("id", "", "Service account ID")
	serviceAccountAddCmd.Flags().String("role", "service", "Role: admin, service, orchestrator")
	serviceAccountAddCmd.Flags().String("description", "", "Human-readable description")
	serviceAccountAddCmd.Flags().String("tenant", "", "Tenant ID (omit for global infra actor)")
	serviceAccountAddCmd.Flags().Bool("global", false, "Create as global (cross-tenant) infra actor")
	serviceAccountAddCmd.Flags().Bool("force", false, "Upsert: update role/description even if account is already active")
	_ = serviceAccountAddCmd.MarkFlagRequired("id")

	serviceAccountCmd.AddCommand(serviceAccountListCmd, serviceAccountAddCmd, serviceAccountRevokeCmd)
	rootCmd.AddCommand(serviceAccountCmd)
}
