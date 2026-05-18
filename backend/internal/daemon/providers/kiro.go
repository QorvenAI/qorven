// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package providers contains AgentProvider implementations for each external
// agent type. Kiro CLI is an SSE-pull agent: it connects to /v1/daemon/stream
// and receives task_assigned events. Dispatch here just ensures the task is
// visible in the queue; the actual execution happens client-side.
package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/qorvenai/qorven/internal/daemon"
)

// KiroProvider implements daemon.AgentProvider for a kiro-cli instance.
// Kiro connects via SSE push; we don't exec it in-process.
// The Dispatch implementation is a no-op because the SSE stream is the
// delivery mechanism — the task is already in the queue when Dispatch is called.
type KiroProvider struct {
	agentID   string
	agentName string
	lastSeen  time.Time
}

// NewKiroProvider creates a provider stub for a connected kiro-cli instance.
func NewKiroProvider(agentID, agentName string) *KiroProvider {
	return &KiroProvider{agentID: agentID, agentName: agentName, lastSeen: time.Now()}
}

// ProviderID satisfies daemon.AgentProvider.
func (k *KiroProvider) ProviderID() string { return "kiro_cli" }

// Name satisfies daemon.AgentProvider.
func (k *KiroProvider) Name() string { return k.agentName }

// Capabilities returns the default capability set for Kiro CLI.
// Registered agents can override this on registration.
func (k *KiroProvider) Capabilities() []string {
	return []string{"code", "frontend", "review", "test"}
}

// Dispatch is a no-op for SSE-pull agents. The task_assigned event has
// already been broadcast to all SSE subscribers; kiro will pick it up.
func (k *KiroProvider) Dispatch(_ context.Context, _ *daemon.Task) error {
	return nil
}

// Cancel cannot forcibly stop a kiro-cli process from the server side.
// We record the intent; kiro should poll /v1/daemon/tasks/:id and check
// the cancelled status, then abort its local execution.
func (k *KiroProvider) Cancel(_ context.Context, taskID string) error {
	// The task status is set to cancelled by Registry.Fail before this is called.
	// Kiro will see the status change on its next task-status poll.
	return nil
}

// Ping checks liveness by inspecting the last-seen timestamp.
// If kiro's heartbeat has been silent for >30s, treat it as offline.
func (k *KiroProvider) Ping(_ context.Context) error {
	if time.Since(k.lastSeen) > 30*time.Second {
		return fmt.Errorf("kiro agent %s: no heartbeat for %s", k.agentID, time.Since(k.lastSeen).Round(time.Second))
	}
	return nil
}

// Touch updates the last-seen timestamp (called on heartbeat).
func (k *KiroProvider) Touch() { k.lastSeen = time.Now() }

// ─── Claude Code provider (stub) ──────────────────────────────────────────────

// ClaudeCodeProvider implements daemon.AgentProvider for a claude-code CLI instance.
// Identical pattern to KiroProvider — SSE-pull, no in-process exec.
type ClaudeCodeProvider struct {
	agentID   string
	agentName string
	lastSeen  time.Time
}

// NewClaudeCodeProvider creates a provider stub for a connected claude-code instance.
func NewClaudeCodeProvider(agentID, agentName string) *ClaudeCodeProvider {
	return &ClaudeCodeProvider{agentID: agentID, agentName: agentName, lastSeen: time.Now()}
}

func (c *ClaudeCodeProvider) ProviderID() string { return "claude_code" }
func (c *ClaudeCodeProvider) Name() string       { return c.agentName }
func (c *ClaudeCodeProvider) Capabilities() []string {
	return []string{"code", "backend", "review", "plan", "research"}
}
func (c *ClaudeCodeProvider) Dispatch(_ context.Context, _ *daemon.Task) error  { return nil }
func (c *ClaudeCodeProvider) Cancel(_ context.Context, _ string) error          { return nil }
func (c *ClaudeCodeProvider) Ping(_ context.Context) error {
	if time.Since(c.lastSeen) > 30*time.Second {
		return fmt.Errorf("claude agent %s: no heartbeat for %s", c.agentID, time.Since(c.lastSeen).Round(time.Second))
	}
	return nil
}
func (c *ClaudeCodeProvider) Touch() { c.lastSeen = time.Now() }
