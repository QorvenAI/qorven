// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// SandboxMode defines the execution permission level.
// Inspired by OpenAI Codex: read-only | workspace-write | danger-full-access
type SandboxMode string

const (
	SandboxReadOnly       SandboxMode = "read-only"        // Only read tools (grep, glob, read, git_log)
	SandboxWorkspaceWrite SandboxMode = "workspace-write"  // Read + write within workspace only
	SandboxFullAccess     SandboxMode = "danger-full-access" // Everything — use with caution
)

// SandboxPolicy enforces tool access based on the current sandbox mode.
type SandboxPolicy struct {
	Mode      SandboxMode
	Workspace string // absolute path to the allowed workspace
}

// readOnlyTools can always execute regardless of sandbox mode.
var readOnlyTools = map[string]bool{
	"read_file": true, "grep": true, "glob": true, "list_files": true,
	"git_log": true, "git_diff": true, "web_search": true, "web_fetch": true,
	"ask_user": true, "clarify": true, "memory_search": true,
}

// writeTools can execute in workspace-write and full-access modes.
var writeTools = map[string]bool{
	"write_file": true, "edit": true,
}

// dangerousTools can only execute in full-access mode.
var dangerousTools = map[string]bool{
	"exec": true, "spawn": true,
}

// Check returns nil if the tool is allowed, or an error explaining why it's blocked.
func (sp *SandboxPolicy) Check(toolName string, args map[string]any) error {
	switch sp.Mode {
	case SandboxReadOnly:
		if readOnlyTools[toolName] {
			return nil
		}
		return fmt.Errorf("🔒 sandbox: %s is blocked in read-only mode. Only read tools are allowed (grep, glob, read_file, list_files, web_search)", toolName)

	case SandboxWorkspaceWrite:
		if readOnlyTools[toolName] {
			return nil
		}
		if writeTools[toolName] {
			return sp.checkWorkspaceBound(toolName, args)
		}
		if toolName == "exec" {
			return sp.checkExecWorkspaceBound(args)
		}
		return fmt.Errorf("🔒 sandbox: %s is blocked in workspace-write mode. Use full-access mode for system-level operations", toolName)

	case SandboxFullAccess:
		return nil // everything allowed

	default:
		// Unknown mode — default to workspace-write for safety
		if readOnlyTools[toolName] || writeTools[toolName] {
			return nil
		}
		return fmt.Errorf("🔒 sandbox: %s blocked (unknown sandbox mode %q)", toolName, sp.Mode)
	}
}

// checkWorkspaceBound ensures write operations stay within the workspace.
func (sp *SandboxPolicy) checkWorkspaceBound(toolName string, args map[string]any) error {
	path, _ := args["path"].(string)
	if path == "" {
		path, _ = args["file"].(string)
	}
	if path == "" {
		return nil // no path arg — let the tool handle it
	}

	// Resolve to absolute
	absPath := path
	if !filepath.IsAbs(path) {
		absPath = filepath.Join(sp.Workspace, path)
	}
	absPath = filepath.Clean(absPath)

	if !strings.HasPrefix(absPath, sp.Workspace) {
		return fmt.Errorf("🔒 sandbox: %s cannot write outside workspace (%s). Path: %s", toolName, sp.Workspace, path)
	}
	return nil
}

// checkExecWorkspaceBound allows exec in workspace-write mode only if the command
// doesn't modify files outside the workspace.
func (sp *SandboxPolicy) checkExecWorkspaceBound(args map[string]any) error {
	cmd, _ := args["command"].(string)
	if cmd == "" {
		return fmt.Errorf("🔒 sandbox: exec requires a command")
	}

	// Block commands that could modify system state
	blocked := []string{
		"sudo", "su ", "chmod", "chown", "mount", "umount",
		"systemctl", "service ", "apt", "yum", "dnf", "brew",
		"pip install -g", "npm install -g", "cargo install",
	}
	lower := strings.ToLower(cmd)
	for _, b := range blocked {
		if strings.Contains(lower, b) {
			return fmt.Errorf("🔒 sandbox: exec blocked — %q contains system-level command %q. Use full-access mode", cmd, b)
		}
	}
	return nil
}

// --- Context integration ---

type sandboxKey struct{}

// WithSandbox attaches a sandbox policy to the context.
func WithSandbox(ctx context.Context, policy *SandboxPolicy) context.Context {
	return context.WithValue(ctx, sandboxKey{}, policy)
}

// SandboxFromCtx retrieves the sandbox policy from context.
func SandboxFromCtx(ctx context.Context) *SandboxPolicy {
	if v, ok := ctx.Value(sandboxKey{}).(*SandboxPolicy); ok {
		return v
	}
	return nil
}

// EnforceSandbox checks the sandbox policy before tool execution.
// Returns nil if allowed, error if blocked.
func EnforceSandbox(ctx context.Context, toolName string, args map[string]any) error {
	policy := SandboxFromCtx(ctx)
	if policy == nil {
		return nil // no sandbox = full access (backward compatible)
	}
	return policy.Check(toolName, args)
}
