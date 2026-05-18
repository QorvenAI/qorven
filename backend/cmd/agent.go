// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// cmd/agent.go — `qorven agent ...` subcommand tree.
//
// Distinct from the existing `qorven agents` (plural) subtree:
//   agents  — inspect/manage agent rows in the running gateway
//   agent   — DX scaffolding for plugin + UI authors (local-only,
//             no DB required)
//
// The singular form is deliberate: `qorven agent init` reads as
// "one agent, scaffolding its starter kit."
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/qorvenai/qorven/cmd/scaffold"
)

var (
	agentInitName        string
	agentInitDescription string
	agentInitPluginOnly  bool
	agentInitUIOnly      bool
	agentInitRuntime     string
)

func init() {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Agent-developer tooling (DX scaffolds, local only)",
		Long: `Commands for developers (human or AI) building on top of Qorven.
These do NOT talk to a running gateway — they emit starter code
onto your local filesystem.

Run "qorven agent init" to generate plugin + UI boilerplate.`,
	}

	agentInitCmd := &cobra.Command{
		Use:   "init [target-dir]",
		Short: "Scaffold a Qorven plugin + optional Next.js UI starter",
		Long: `Generate starter code for a Qorven Wasm plugin, optionally plus a
minimal Next.js UI to drive it.

Output layout (defaults to ./qorven-<plugin>/):
  plugin/   Go source, go.mod, parameters.json, Makefile, README
  ui/       Next.js app, tsconfig, env example, README

The plugin code compiles out-of-the-box under Go 1.21+:
  cd <dir>/plugin && make build
produces <plugin>.wasm ready to upload via POST /v1/wasm-plugins.

Flags:
  --name          required, must match ^[a-z][a-z0-9_]{0,62}$
  --description   optional, shows up in the LLM's tool listing
  --plugin-only   emit only the plugin tree
  --ui-only       emit only the UI tree
  --runtime       plugin toolchain: "go" (default, ~3.2 MiB) or
                  "tinygo" (~50 KiB, requires tinygo on PATH)

If the target directory exists but the expected files don't, init
fills the gaps. If any target file exists, init aborts — it never
overwrites work.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runAgentInit,
	}
	agentInitCmd.Flags().StringVar(&agentInitName, "name", "", "plugin name (required)")
	agentInitCmd.Flags().StringVar(&agentInitDescription, "description", "", "short description shown in the LLM tool listing")
	agentInitCmd.Flags().BoolVar(&agentInitPluginOnly, "plugin-only", false, "scaffold only the plugin tree (skip UI)")
	agentInitCmd.Flags().BoolVar(&agentInitUIOnly, "ui-only", false, "scaffold only the UI tree (skip plugin)")
	agentInitCmd.Flags().StringVar(&agentInitRuntime, "runtime", "go", "plugin toolchain: \"go\" (default) or \"tinygo\"")

	agentCmd.AddCommand(agentInitCmd)
	rootCmd.AddCommand(agentCmd)
}

func runAgentInit(cmd *cobra.Command, args []string) error {
	if agentInitName == "" {
		return fmt.Errorf("--name is required")
	}
	if agentInitPluginOnly && agentInitUIOnly {
		return fmt.Errorf("--plugin-only and --ui-only are mutually exclusive; drop both to scaffold both trees")
	}

	// Default target dir: ./qorven-<name>/. Explicit arg overrides.
	targetDir := fmt.Sprintf("qorven-%s", agentInitName)
	if len(args) == 1 {
		targetDir = args[0]
	}
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("resolve target dir: %w", err)
	}

	// Validate runtime here (before handing to Render) so the error
	// is attributed to the --runtime flag specifically.
	runtime := scaffold.Runtime(agentInitRuntime)
	if runtime != scaffold.RuntimeGo && runtime != scaffold.RuntimeTinyGo {
		return fmt.Errorf("--runtime must be %q or %q; got %q",
			scaffold.RuntimeGo, scaffold.RuntimeTinyGo, agentInitRuntime)
	}

	opts := scaffold.Options{
		Name:        agentInitName,
		Description: agentInitDescription,
		Plugin:      !agentInitUIOnly,
		UI:          !agentInitPluginOnly,
		Runtime:     runtime,
		TargetDir:   absTarget,
	}

	written, err := scaffold.Render(opts)
	if err != nil {
		return fmt.Errorf("scaffold: %w", err)
	}

	// Human + agent-readable summary. Prefix with "qorven agent init:"
	// so tooling grepping stdout can key off a known line.
	fmt.Fprintf(os.Stdout, "qorven agent init: scaffolded %d file(s) under %s\n",
		len(written), absTarget)
	for _, p := range written {
		rel, _ := filepath.Rel(absTarget, p)
		fmt.Fprintf(os.Stdout, "  + %s\n", rel)
	}

	// Next-steps hint. An AI agent reading this can follow the
	// commands directly.
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Next steps:")
	if opts.Plugin {
		fmt.Fprintf(os.Stdout, "  cd %s/plugin && make build\n", strings.TrimSuffix(targetDir, "/"))
		if opts.Runtime == scaffold.RuntimeTinyGo {
			fmt.Fprintln(os.Stdout, "  # requires tinygo >= 0.34 (brew install tinygo / see plugin/README.md)")
		}
		fmt.Fprintln(os.Stdout, "  # then: make upload GATEWAY=<url> TOKEN=<admin-jwt>")
	}
	if opts.UI {
		fmt.Fprintf(os.Stdout, "  cd %s/ui && pnpm install && pnpm dev\n", strings.TrimSuffix(targetDir, "/"))
	}
	fmt.Fprintln(os.Stdout, "  # See AGENTS.md in the Qorven repo for the full platform contract.")
	return nil
}
