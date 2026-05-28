// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadSteeringRules loads project-level steering configuration that directs
// agent behavior. This is the equivalent of .cursorrules / .windsurfrules / Kiro steering/.
//
// Load order (merged, later overrides earlier):
//  1. .qorven/RULES.md — primary project rules
//  2. .qorven/context/*.md — additional context files (architecture, conventions, API docs)
//
// The result is injected into EVERY agent's system prompt when working on this project,
// not just Prime Coder. Sub-agents, coder agents, and delegated tasks all see these rules.
func LoadSteeringRules(projectPath string) string {
	if projectPath == "" {
		return ""
	}

	var sections []string

	// 1. Primary rules file
	rulesPath := filepath.Join(projectPath, ".qorven", "RULES.md")
	if data, err := os.ReadFile(rulesPath); err == nil {
		content := strings.TrimSpace(string(data))
		if content != "" {
			sections = append(sections, "## Project Rules\n"+content)
		}
	}

	// 2. Context directory — all .md files loaded alphabetically
	ctxDir := filepath.Join(projectPath, ".qorven", "context")
	entries, err := os.ReadDir(ctxDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(ctxDir, e.Name()))
			if err != nil {
				continue
			}
			content := strings.TrimSpace(string(data))
			if content == "" {
				continue
			}
			label := strings.TrimSuffix(e.Name(), ".md")
			label = strings.ReplaceAll(label, "-", " ")
			label = strings.ReplaceAll(label, "_", " ")
			sections = append(sections, "## Context: "+label+"\n"+content)
		}
	}

	if len(sections) == 0 {
		return ""
	}

	return "# Steering Rules\n\nThese rules define how you must work in this project. Follow them strictly.\n\n" +
		strings.Join(sections, "\n\n")
}

// DefaultRulesTemplate returns a starter RULES.md for a new project.
func DefaultRulesTemplate(stack string) string {
	base := `# Project Rules

## Code Style
- Write clean, readable code with meaningful variable names
- Keep functions small and focused (under 50 lines)
- Add comments only for non-obvious logic

## Testing
- Write tests for new functionality
- Run tests before committing

## Git
- Use conventional commits (feat:, fix:, refactor:, docs:, test:)
- Keep commits atomic — one logical change per commit
`

	switch {
	case strings.Contains(stack, "react") || strings.Contains(stack, "next"):
		base += `
## React/Next.js Conventions
- Use functional components with hooks
- Prefer server components where possible
- Use Tailwind CSS for styling
- Keep components under 200 lines — extract sub-components when larger
- Use TypeScript strict mode
`
	case strings.Contains(stack, "go"):
		base += `
## Go Conventions
- Follow effective Go guidelines
- Use table-driven tests
- Handle all errors explicitly — no underscore discards
- Keep packages focused — one responsibility per package
- Use context.Context for cancellation propagation
`
	case strings.Contains(stack, "python"):
		base += `
## Python Conventions
- Follow PEP 8
- Use type hints on all public functions
- Use dataclasses or Pydantic for structured data
- Prefer composition over inheritance
`
	case strings.Contains(stack, "rust"):
		base += `
## Rust Conventions
- Prefer Result<T, E> over unwrap/expect in library code
- Use clippy lints
- Derive common traits (Debug, Clone) on structs
- Document public APIs with doc comments
`
	}

	return base
}
