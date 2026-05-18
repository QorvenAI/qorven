// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/cmd/output"
)

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "View usage and cost analytics",
}

var usageSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Usage summary",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/usage/summary")
		if err != nil {
			// Fallback - compute from sessions
			data, err = c.Get("/v1/sessions")
			if err != nil {
				return err
			}
			sessions := unmarshalList(data)
			var totalInput, totalOutput int
			for _, s := range sessions {
				if v, ok := s["input_tokens"].(float64); ok {
					totalInput += int(v)
				}
				if v, ok := s["output_tokens"].(float64); ok {
					totalOutput += int(v)
				}
			}
			tbl := output.NewTable("Metric", "Value")
			tbl.AddRow("Sessions", fmt.Sprintf("%d", len(sessions)))
			tbl.AddRow("Input Tokens", fmt.Sprintf("%d", totalInput))
			tbl.AddRow("Output Tokens", fmt.Sprintf("%d", totalOutput))
			tbl.AddRow("Total Tokens", fmt.Sprintf("%d", totalInput+totalOutput))
			printer.Print(tbl)
			return nil
		}
		printer.Print(unmarshalMap(data))
		return nil
	},
}

var usageCostsCmd = &cobra.Command{
	Use:   "costs",
	Short: "Cost breakdown",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/usage/costs")
		if err != nil {
			// Fallback - compute from agents
			data, err = c.Get("/v1/agents")
			if err != nil {
				return err
			}
			agents := unmarshalList(data)
			tbl := output.NewTable("AGENT", "MODEL", "COST (cents)")
			for _, a := range agents {
				key := str(a, "agent_key")
				if len(key) > 2 && key[:2] == "__" {
					continue
				}
				tbl.AddRow(str(a, "display_name"), str(a, "model"), str(a, "credit_used_cents"))
			}
			printer.Print(tbl)
			return nil
		}
		printer.Print(unmarshalMap(data))
		return nil
	},
}

func init() {
	usageCmd.AddCommand(usageSummaryCmd, usageCostsCmd)
	rootCmd.AddCommand(usageCmd)
}
