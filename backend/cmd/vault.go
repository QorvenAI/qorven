// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/qorvenai/qorven/internal/qor/coworker"
)

func init() {
	vaultCmd := &cobra.Command{
		Use:   "vault",
		Short: "Obsidian-compatible knowledge vault",
		Long: `Manage your personal knowledge vault (Obsidian-compatible markdown).

Examples:
  qorven vault list                    # recent notes
  qorven vault search "AI agents"     # find notes
  qorven vault create "Meeting Notes" # new note
  qorven vault read meeting-notes     # show note`,
	}
	vaultCmd.AddCommand(vaultListCmd())
	vaultCmd.AddCommand(vaultSearchCmd())
	vaultCmd.AddCommand(vaultCreateCmd())
	vaultCmd.AddCommand(vaultReadCmd())
	rootCmd.AddCommand(vaultCmd)
}

func vaultPath() string { return filepath.Join(resolveQorvenHome(), "vault") }

func vaultListCmd() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List recent notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			v := coworker.NewVault(vaultPath())
			notes := v.ListRecent(20)
			if len(notes) == 0 { fmt.Println("  Vault is empty. Create a note: qorven vault create \"Title\""); return nil }
			for _, n := range notes {
				tags := ""
				if len(n.Tags) > 0 { tags = " #" + strings.Join(n.Tags, " #") }
				fmt.Printf("  %-24s %s%s\n", n.ID, n.Title, tags)
			}
			return nil
		},
	}
}

func vaultSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use: "search <query>", Short: "Search notes by content", Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v := coworker.NewVault(vaultPath())
			results := v.Search(strings.Join(args, " "))
			if len(results) == 0 { fmt.Println("  No notes found."); return nil }
			for _, n := range results {
				preview := n.Content
				if len(preview) > 80 { preview = preview[:80] + "..." }
				fmt.Printf("  [%s] %s\n    %s\n\n", n.ID, n.Title, preview)
			}
			return nil
		},
	}
}

func vaultCreateCmd() *cobra.Command {
	var content string
	cmd := &cobra.Command{
		Use: "create <title>", Short: "Create a new note", Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			title := strings.Join(args, " ")
			if content == "" { content = "# " + title + "\n\n" }
			v := coworker.NewVault(vaultPath())
			note := &coworker.Note{Title: title, Content: content}
			if err := v.Save(note); err != nil { return err }
			fmt.Printf("  ✓ Created: %s (%s)\n", note.Title, note.ID)
			fmt.Printf("    Path: %s/%s.md\n", vaultPath(), note.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&content, "content", "", "note content (reads from stdin if empty)")
	return cmd
}

func vaultReadCmd() *cobra.Command {
	return &cobra.Command{
		Use: "read <id>", Short: "Show note content", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v := coworker.NewVault(vaultPath())
			note, ok := v.Get(args[0])
			if !ok { return fmt.Errorf("note %q not found", args[0]) }
			fmt.Printf("# %s\n\n%s\n", note.Title, note.Content)
			if len(note.Backlinks) > 0 { fmt.Printf("\nBacklinks: %s\n", strings.Join(note.Backlinks, ", ")) }
			if len(note.Tags) > 0 { fmt.Printf("Tags: #%s\n", strings.Join(note.Tags, " #")) }
			return nil
		},
	}
}
