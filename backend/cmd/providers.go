// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"net/http"
	"time"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var providersCmd = &cobra.Command{
	Use:   "providers",
	Short: "Manage LLM providers and API keys",
}

var providersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := apiGet("/v1/providers")
		if err != nil {
			return err
		}
		var resp struct {
			Providers []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Type string `json:"provider_type"`
				Base string `json:"api_base"`
			} `json:"providers"`
		}
		json.Unmarshal(data, &resp)
		if len(resp.Providers) == 0 {
			fmt.Println("No providers configured. Add one with: qorven providers add")
			return nil
		}
		for _, p := range resp.Providers {
			fmt.Printf("  %s  %-12s  %-15s  %s\n", p.ID[:8], p.Name, p.Type, p.Base)
		}
		return nil
	},
}

var providersAddCmd = &cobra.Command{
	Use:   "add <name> <type> <api-base> <api-key>",
	Short: "Add a new provider",
	Long: `Add a new LLM provider. Types: openai_compat, bedrock, anthropic.

Examples:
  qorven providers add deepseek openai_compat https://api.deepseek.com/v1 sk-xxx
  qorven providers add openai openai_compat https://api.openai.com/v1 sk-xxx
  qorven providers add gemini openai_compat https://generativelanguage.googleapis.com/v1beta/openai AIza...`,
	Args: cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, _ := json.Marshal(map[string]string{
			"name":          args[0],
			"provider_type": args[1],
			"api_base":      args[2],
			"api_key":       args[3],
		})
		data, err := apiPost("/v1/providers", body)
		if err != nil {
			return err
		}
		fmt.Printf("Provider added: %s\n", string(data))
		return nil
	},
}

var modelsCmd = &cobra.Command{
	Use:   "models [search]",
	Short: "Search the model registry (1700+ models with costs)",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "/v1/providers/model-registry"
		if len(args) > 0 {
			path += "?search=" + strings.Join(args, "+")
		}
		data, err := apiGet(path)
		if err != nil {
			return err
		}
		var resp struct {
			Models []struct {
				Name      string  `json:"name"`
				MaxInput  int     `json:"max_input_tokens"`
				InputCost float64 `json:"input_cost_per_1m"`
				Tools     bool    `json:"supports_tools"`
				Vision    bool    `json:"supports_vision"`
				Reasoning bool    `json:"supports_reasoning"`
				Provider  string  `json:"provider"`
			} `json:"models"`
			Total int `json:"total"`
		}
		json.Unmarshal(data, &resp)

		fmt.Printf("Found %d models:\n\n", resp.Total)
		fmt.Printf("%-40s %8s %8s %5s %5s %5s  %s\n", "MODEL", "CONTEXT", "$/1M IN", "TOOL", "VIS", "REAS", "PROVIDER")
		fmt.Println(strings.Repeat("─", 100))

		limit := 50
		if len(resp.Models) < limit {
			limit = len(resp.Models)
		}
		for _, m := range resp.Models[:limit] {
			ctx := fmt.Sprintf("%dK", m.MaxInput/1000)
			cost := fmt.Sprintf("$%.2f", m.InputCost)
			tools, vis, reas := "  ", "  ", "  "
			if m.Tools { tools = " ✓" }
			if m.Vision { vis = " ✓" }
			if m.Reasoning { reas = " ✓" }
			fmt.Printf("%-40s %8s %8s %5s %5s %5s  %s\n", m.Name, ctx, cost, tools, vis, reas, m.Provider)
		}
		if len(resp.Models) > limit {
			fmt.Printf("\n... and %d more. Use 'qorven models <search>' to filter.\n", len(resp.Models)-limit)
		}
		return nil
	},
}

func init() {
	providersCmd.AddCommand(providersListCmd)
	providersCmd.AddCommand(providersAddCmd)
	rootCmd.AddCommand(providersCmd)
	rootCmd.AddCommand(modelsCmd)
}

// apiGet/apiPost helpers
func apiGet(path string) ([]byte, error) {
	server := os.Getenv("QORVEN_SERVER")
	if server == "" { server = "http://localhost" }
	
	resp, err := defaultHTTP().Get(server + path)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	data := make([]byte, 1<<20)
	n, _ := resp.Body.Read(data)
	return data[:n], nil
}

func apiPost(path string, body []byte) ([]byte, error) {
	server := os.Getenv("QORVEN_SERVER")
	if server == "" { server = "http://localhost" }
	
	resp, err := defaultHTTP().Post(server+path, "application/json", strings.NewReader(string(body)))
	if err != nil { return nil, err }
	defer resp.Body.Close()
	data := make([]byte, 1<<20)
	n, _ := resp.Body.Read(data)
	return data[:n], nil
}

func defaultHTTP() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}
