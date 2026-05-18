// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package tools — destructive tool manifest.
//
// DestructiveTools is the closed set of tool names whose unchecked
// execution can harm user data, infrastructure, or third-party state.
// Every tool in this set MUST be wrapped with permissions.WrapLazy (or
// Wrap) before it reaches a registered state in production. The CI
// check at internal/tools/destructive_manifest_test.go walks the
// registered tool table and FAILS the build if any manifest tool
// is unwrapped.
//
// Adding a new destructive tool: add its Name() to DestructiveTools
// AND wrap its registration with permissions.WrapLazy at the gateway
// bootstrap. The CI check enforces both sides.
//
// Rationale:
//
//   - gh_push_file    — writes code to a user's GitHub repo.
//   - gh_merge_pr     — merges a PR (irreversible on `main`).
//   - exec            — runs arbitrary shell commands on the host.
//   - apply_patch     — mutates workspace files.
//   - write_file      — creates or overwrites workspace files.
//   - undo            — applies snapshot reverts; operator-visible but
//                       can lose work if triggered without consent.
//
// Tools that read data (gh_read_issue, gh_list_prs, lsp diagnostics)
// are intentionally NOT in this set — read operations don't need a
// user prompt and shouldn't block the agent loop.
package tools

// DestructiveTools is the manifest. Modify in one place only.
var DestructiveTools = map[string]DestructiveReason{
	"gh_push_file":  {Description: "writes code to a user-owned GitHub repository"},
	"gh_merge_pr":   {Description: "merges a pull request (irreversible on protected branches)"},
	"exec":          {Description: "executes arbitrary shell commands on the host"},
	"apply_patch":   {Description: "mutates workspace files in-place"},
	"write_file":    {Description: "creates or overwrites workspace files"},
	"undo":          {Description: "applies snapshot reverts; can discard user work"},
	"cron":          {Description: "creates, enables, or deletes recurring scheduled tasks"},
}

// DestructiveReason documents why a tool is considered destructive.
// The Description surfaces in the permission.requested event's Reason
// field when callers pass it through.
type DestructiveReason struct {
	Description string
}

// IsDestructive reports whether the tool name is in the manifest.
func IsDestructive(name string) bool {
	_, ok := DestructiveTools[name]
	return ok
}
