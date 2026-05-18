// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/output"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "health",
		Short: "Check gateway health",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newHTTP()
			if err != nil {
				return err
			}
			if err := c.HealthCheck(); err != nil {
				return err
			}
			printer.Success("Gateway is healthy")
			return nil
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show gateway status and metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := newHTTP()
			if err != nil {
				return err
			}
			resp, err := c.Get("/health/detailed")
			if err != nil {
				// Fallback to basic health
				resp, err = c.Get("/health")
				if err != nil {
					return err
				}
			}

			if cfg.OutputFormat == "json" || cfg.OutputFormat == "yaml" {
				var data any
				json.Unmarshal(resp, &data)
				printer.Print(data)
				return nil
			}

			var status map[string]any
			json.Unmarshal(resp, &status)

			table := output.NewTable("Key", "Value")
			for _, key := range []string{"status", "version", "uptime", "agents", "providers", "tools", "channels", "sessions"} {
				if v, ok := status[key]; ok {
					table.AddRow(key, fmt.Sprintf("%v", v))
				}
			}
			printer.Print(table)
			return nil
		},
	})
}
