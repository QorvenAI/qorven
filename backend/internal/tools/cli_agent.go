// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

//go:build !windows

package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// RunCLIAgentTool spawns a coding CLI agent (claude, codex, kilo) as a
// subprocess, feeds it a prompt, and returns the full output. The agent runs
// non-interactively using each CLI's headless/print flag so Prime can call it
// from within a turn without blocking the event loop.
//
// Supported agents:
//   - claude  → claude --print "<prompt>"      (Claude Code CLI)
//   - codex   → codex --quiet "<prompt>"       (OpenAI Codex CLI)
//   - kilo    → kilo --non-interactive "<prompt>"
type RunCLIAgentTool struct {
	workspace string
}

func NewRunCLIAgentTool(workspace string) *RunCLIAgentTool {
	return &RunCLIAgentTool{workspace: workspace}
}

func (t *RunCLIAgentTool) Name() string { return "run_cli_agent" }
func (t *RunCLIAgentTool) Description() string {
	return `Run a coding CLI agent (claude, codex, or kilo) with a prompt and return its output.

Use this to delegate a focused coding task to a specialist CLI agent:
- claude: Claude Code CLI (great for code writing, refactoring, explanation)
- codex: OpenAI Codex CLI (great for code generation)
- kilo: Kilo code editor agent

The agent runs non-interactively and returns its full output. The working
directory defaults to the current workspace.`
}

func (t *RunCLIAgentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent": map[string]any{
				"type":        "string",
				"enum":        []string{"claude", "codex", "kilo"},
				"description": "Which CLI agent to run: claude, codex, or kilo",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The task or question to send to the agent",
			},
			"dir": map[string]any{
				"type":        "string",
				"description": "Working directory for the agent. Defaults to the workspace root.",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Max seconds to wait for output (default 120, max 600).",
			},
		},
		"required": []string{"agent", "prompt"},
	}
}

// cliAgentCommand maps agent name to (binary, args-before-prompt).
// Each CLI's non-interactive flag is hardcoded here so Execute stays simple.
var cliAgentCommand = map[string]struct {
	bin  string
	args []string
}{
	"claude": {bin: "claude", args: []string{"--print"}},
	"codex":  {bin: "codex", args: []string{"--quiet"}},
	"kilo":   {bin: "kilo", args: []string{"--non-interactive"}},
}

func (t *RunCLIAgentTool) Execute(ctx context.Context, args map[string]any) *Result {
	agentName, _ := args["agent"].(string)
	prompt, _ := args["prompt"].(string)
	dir, _ := args["dir"].(string)

	if agentName == "" {
		return ErrorResult("agent is required (claude, codex, or kilo)")
	}
	if prompt == "" {
		return ErrorResult("prompt is required")
	}

	spec, ok := cliAgentCommand[agentName]
	if !ok {
		return ErrorResult(fmt.Sprintf("unknown agent %q — must be claude, codex, or kilo", agentName))
	}

	if dir == "" {
		dir = WorkspaceFromCtx(ctx)
		if dir == "" {
			dir = t.workspace
		}
	}

	timeoutSec := 120
	if v, ok := args["timeout_seconds"].(float64); ok && v > 0 {
		timeoutSec = int(v)
		if timeoutSec > 600 {
			timeoutSec = 600
		}
	}

	// Verify the binary exists before attempting to run.
	binPath, err := exec.LookPath(spec.bin)
	if err != nil {
		return ErrorResult(fmt.Sprintf("%s CLI not found in PATH — install it first (e.g. npm install -g @anthropic-ai/claude-code)", spec.bin))
	}

	cmdArgs := append(spec.args, prompt)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, binPath, cmdArgs...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	startedAt := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(startedAt).Round(time.Millisecond)

	out := strings.TrimSpace(stdout.String())
	errOut := strings.TrimSpace(stderr.String())

	if runErr != nil {
		msg := fmt.Sprintf("%s failed after %s", agentName, elapsed)
		if errOut != "" {
			msg += ":\n" + errOut
		} else if runErr.Error() != "" {
			msg += ": " + runErr.Error()
		}
		if out != "" {
			msg += "\n\nPartial output:\n" + out
		}
		return ErrorResult(msg)
	}

	if out == "" {
		out = "(no output)"
	}
	// Cap at 40KB — CLI agents can be verbose.
	if len(out) > 40000 {
		out = "[... truncated to last 40KB ...]\n" + out[len(out)-40000:]
	}

	header := fmt.Sprintf("[%s — %s]\n\n", agentName, elapsed)
	return TextResult(header + out)
}
