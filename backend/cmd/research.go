// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"context"
	"os"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/internal/qor/social"
)

func init() {
	var platforms, depth string
	var jsonOutput bool

	researchCmd := &cobra.Command{
		Use:   "research <topic>",
		Short: "Search social platforms for trends and intelligence",
		Long: `Search Reddit, HN, GitHub, and more in parallel. Score by engagement.

Examples:
  qorven research "AI agents"
  qorven research "Go vs Rust" --platforms reddit,hackernews
  qorven research "Bitcoin" --depth deep --json`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			topic := strings.Join(args, " ")
			return runResearch(topic, platforms, depth, jsonOutput)
		},
	}
	researchCmd.Flags().StringVar(&platforms, "platforms", "", "filter: reddit,hackernews,github")
	researchCmd.Flags().StringVar(&depth, "depth", "default", "quick/default/deep")
	researchCmd.Flags().BoolVar(&jsonOutput, "json", false, "output as JSON")
	rootCmd.AddCommand(researchCmd)
}

func runResearch(topic, platformFilter, depth string, jsonOut bool) error {
	engine := social.NewIntelligenceEngine("", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Fprintf(os.Stderr, "  Searching for %q...\n", topic)

	opts := social.SearchOpts{MaxResults: 15, TimeRange: 30 * 24 * time.Hour}
	if depth == "quick" { opts.MaxResults = 5 }
	if depth == "deep" { opts.MaxResults = 30 }

	var results []social.ScoredResult
	var err error

	if platformFilter != "" {
		var plats []social.Platform
		for _, p := range strings.Split(platformFilter, ",") {
			plats = append(plats, social.Platform(strings.TrimSpace(p)))
		}
		results, err = engine.SearchPlatforms(ctx, topic, plats, opts)
	} else {
		results, err = engine.SearchAll(ctx, topic, opts)
	}
	if err != nil { return fmt.Errorf("search failed: %v", err) }

	if jsonOut {
		data, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(results) == 0 {
		fmt.Println("  No results found.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "  Found %d results\n\n", len(results))

	for i, r := range results {
		if i >= 20 { break }
		eng := ""
		if r.Upvotes > 0 { eng += fmt.Sprintf("↑%d ", r.Upvotes) }
		if r.Likes > 0 { eng += fmt.Sprintf("♥%d ", r.Likes) }
		if r.Comments > 0 { eng += fmt.Sprintf("💬%d ", r.Comments) }
		if r.Views > 0 { eng += fmt.Sprintf("👁%d ", r.Views) }

		fmt.Printf("  %d. [%s] %s %s\n", i+1, r.Platform, r.Title, eng)
		if r.URL != "" { fmt.Printf("     %s\n", r.URL) }
		fmt.Println()
	}
	return nil
}
