// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// PlanMode manages the plan-before-execute workflow.
// Flow: EnterPlanMode → explore codebase → write plan → ExitPlanMode → user approval → execute
type PlanMode struct {
	Active    bool   `json:"active"`
	PlanFile  string `json:"plan_file"`
	SessionID string `json:"session_id"`
	StartedAt time.Time `json:"started_at"`
}

// PlanModeConfig controls when plan mode triggers.
var PlanModeConfig = struct {
	AutoTriggerMultiFile int  // auto-enter if task touches > N files
	RequireApproval      bool // require user approval before executing
}{AutoTriggerMultiFile: 3, RequireApproval: true}

// EnterPlanMode transitions the agent into planning mode.
// In plan mode, the agent can only use read-only tools (grep, glob, read).
func (l *Loop) EnterPlanMode(ctx context.Context, sessionID, reason string) (*PlanMode, error) {
	planDir := filepath.Join(os.TempDir(), "qorven-plans")
	os.MkdirAll(planDir, 0755)
	planFile := filepath.Join(planDir, fmt.Sprintf("plan-%s-%d.md", sessionID[:8], time.Now().Unix()))

	// Write initial plan template
	template := fmt.Sprintf(`# Implementation Plan
> Session: %s
> Created: %s
> Reason: %s

## Context
<!-- Explore the codebase and document what you find -->

## Approach
<!-- Describe the implementation strategy -->

## Files to Modify
<!-- List files that will be changed -->

## Steps
<!-- Numbered implementation steps -->

## Risks
<!-- Potential issues or edge cases -->
`, sessionID, time.Now().Format(time.RFC3339), reason)

	os.WriteFile(planFile, []byte(template), 0644)

	return &PlanMode{
		Active: true, PlanFile: planFile,
		SessionID: sessionID, StartedAt: time.Now(),
	}, nil
}

// ExitPlanMode reads the plan file and returns it for user approval.
func (l *Loop) ExitPlanMode(ctx context.Context, pm *PlanMode) (string, error) {
	if pm == nil || !pm.Active { return "", fmt.Errorf("not in plan mode") }
	content, err := os.ReadFile(pm.PlanFile)
	if err != nil { return "", fmt.Errorf("plan file not found: %w", err) }
	return string(content), nil
}

// ShouldEnterPlanMode heuristic — returns true if the task is complex enough.
func ShouldEnterPlanMode(userMessage string) bool {
	complex := []string{
		"implement", "refactor", "redesign", "migrate", "add feature",
		"authentication", "architecture", "restructure", "overhaul",
	}
	lower := strings.ToLower(userMessage)
	for _, kw := range complex {
		if strings.Contains(lower, kw) { return true }
	}
	return false
}

// PlanModeReadOnlyTools restricts tools during planning.
var PlanModeReadOnlyTools = map[string]bool{
	"web_search": true, "web_fetch": true, "file_read": true,
	"grep": true, "glob": true, "git_log": true, "git_diff": true,
	"ask_user": true,
}

// FilterPlanModeTools returns only read-only tools for plan mode.
func FilterPlanModeTools(defs []providers.ToolDefinition) []providers.ToolDefinition {
	var out []providers.ToolDefinition
	for _, d := range defs {
		if PlanModeReadOnlyTools[d.Function.Name] {
			out = append(out, d)
		}
	}
	return out
}

// PlanModeSystemPrompt is injected when Mode == "plan".
const PlanModeSystemPrompt = `You are in PLAN MODE. Your job is to explore the codebase and create a detailed implementation plan.

Rules:
- You can ONLY use read-only tools (file_read, grep, glob, git_log, git_diff, web_search, web_fetch)
- You CANNOT modify any files or run commands
- Explore the relevant code, then write a structured plan with:
  1. Context: What exists today
  2. Approach: How to implement the change
  3. Files to modify: List every file
  4. Steps: Numbered implementation steps
  5. Risks: Edge cases and potential issues
- End with "Plan complete. Approve to proceed?" so the user can review`

// SupervisorProtocolPrompt is injected into worker Qors when Prime is active.
const SupervisorProtocolPrompt = `## Supervisor Protocol
You are monitored by Prime (the supervisor agent). After completing tasks:
- Your outputs are automatically reviewed for quality
- Tool calls with side effects (write_file, exec) trigger review requests
- If Prime detects issues, it may auto-fix or escalate to the human operator
- You do not need to interact with Prime directly — the protocol is automatic
- Focus on doing your best work. Prime handles quality assurance.`
