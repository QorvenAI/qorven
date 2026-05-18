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

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage agent memory",
}

var memorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Semantic search agent memory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		agentID, _ := cmd.Flags().GetString("agent")
		limit, _ := cmd.Flags().GetInt("limit")
		path := fmt.Sprintf("/v1/memory/search?q=%s&limit=%d", args[0], limit)
		if agentID != "" {
			path += "&agent_id=" + agentID
		}
		data, err := c.Get(path)
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalMap(data))
			return nil
		}
		var resp struct {
			Results []struct {
				Content string  `json:"content"`
				Score   float64 `json:"score"`
				Type    string  `json:"type"`
			} `json:"results"`
		}
		json.Unmarshal(data, &resp)
		tbl := output.NewTable("SCORE", "TYPE", "CONTENT")
		for _, r := range resp.Results {
			content := r.Content
			if len(content) > 80 {
				content = content[:80] + "..."
			}
			tbl.AddRow(fmt.Sprintf("%.2f", r.Score), r.Type, content)
		}
		printer.Print(tbl)
		return nil
	},
}

var memoryGraphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Query knowledge graph",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := newHTTP()
		if err != nil {
			return err
		}
		data, err := c.Get("/v1/graph")
		if err != nil {
			return err
		}
		if cfg.OutputFormat != "table" {
			printer.Print(unmarshalMap(data))
			return nil
		}
		var resp struct {
			Stats struct {
				TotalNodes int     `json:"total_nodes"`
				TotalEdges int     `json:"total_edges"`
				AvgDegree  float64 `json:"avg_degree"`
			} `json:"stats"`
		}
		json.Unmarshal(data, &resp)
		tbl := output.NewTable("Metric", "Value")
		tbl.AddRow("Nodes", fmt.Sprintf("%d", resp.Stats.TotalNodes))
		tbl.AddRow("Edges", fmt.Sprintf("%d", resp.Stats.TotalEdges))
		tbl.AddRow("Avg Degree", fmt.Sprintf("%.1f", resp.Stats.AvgDegree))
		printer.Print(tbl)
		return nil
	},
}

func init() {
	memorySearchCmd.Flags().String("agent", "", "Filter by agent ID")
	memorySearchCmd.Flags().Int("limit", 10, "Max results")

	memoryCmd.AddCommand(memorySearchCmd, memoryGraphCmd)
	rootCmd.AddCommand(memoryCmd)
}
