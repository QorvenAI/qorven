// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/internal/qor/reach"
)

func init() {
	var platform string
	readCmd := &cobra.Command{
		Use:   "read <url>",
		Short: "Read any URL — auto-detects platform",
		Long: `Fetch and display clean content from any URL.

Examples:
  qorven read https://github.com/golang/go
  qorven read https://news.ycombinator.com/item?id=12345
  qorven read https://example.com --platform web`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRead(args[0], platform)
		},
	}
	readCmd.Flags().StringVar(&platform, "platform", "", "override platform detection")
	rootCmd.AddCommand(readCmd)
}

func runRead(rawURL, platformOverride string) error {
	reg := reach.DefaultRegistry(reach.Config{})
	p := platformOverride
	if p == "" { p = detectPlatformFromURL(rawURL) }

	ch, ok := reg.Get(p)
	if !ok { ch, _ = reg.Get("web") }
	if ch == nil { return fmt.Errorf("no reader available") }

	readID := rawURL
	if p == "github" && strings.Contains(rawURL, "github.com/") {
		parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://github.com/"), "http://github.com/"), "/")
		if len(parts) >= 2 { readID = parts[0] + "/" + parts[1] }
	}

	fmt.Fprintf(os.Stderr, "  Reading from %s...\n\n", p)
	result, err := ch.Read(readID)
	if err != nil { return fmt.Errorf("read failed: %v", err) }

	fmt.Printf("# %s\n\n", result.Title)
	if result.Author != "" { fmt.Printf("By: %s\n", result.Author) }
	if len(result.Engagement) > 0 {
		var eng []string
		for k, v := range result.Engagement { eng = append(eng, fmt.Sprintf("%s:%d", k, v)) }
		fmt.Printf("Engagement: %s\n", strings.Join(eng, " "))
	}
	fmt.Println()
	fmt.Println(result.Content)
	return nil
}

func detectPlatformFromURL(url string) string {
	switch {
	case strings.Contains(url, "reddit.com"): return "reddit"
	case strings.Contains(url, "github.com"): return "github"
	case strings.Contains(url, "news.ycombinator.com"): return "hackernews"
	case strings.Contains(url, "v2ex.com"): return "v2ex"
	case strings.Contains(url, "bilibili.com"): return "bilibili"
	default: return "web"
	}
}
