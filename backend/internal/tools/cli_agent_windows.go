// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build windows

package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type RunCLIAgentTool struct{ workspace string }

func NewRunCLIAgentTool(workspace string) *RunCLIAgentTool {
	return &RunCLIAgentTool{workspace: workspace}
}
func (t *RunCLIAgentTool) Name() string { return "run_cli_agent" }
func (t *RunCLIAgentTool) Description() string {
	return "Run a CLI agent (claude, codex, kilo) with a prompt and return output."
}
func (t *RunCLIAgentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent":           map[string]any{"type": "string"},
			"prompt":          map[string]any{"type": "string"},
			"dir":             map[string]any{"type": "string"},
			"timeout_seconds": map[string]any{"type": "integer"},
		},
		"required": []string{"agent", "prompt"},
	}
}

var cliAgentCommand = map[string]struct {
	bin  string
	args []string
}{
	"claude": {"claude", []string{"-p", "--output-format", "text"}},
	"codex":  {"codex", []string{"-q"}},
	"kilo":   {"kilo", []string{"-p"}},
}

func (t *RunCLIAgentTool) Execute(ctx context.Context, args map[string]any) *Result {
	agentName, _ := args["agent"].(string)
	prompt, _ := args["prompt"].(string)
	dir, _ := args["dir"].(string)

	if agentName == "" {
		return ErrorResult("agent is required")
	}
	if prompt == "" {
		return ErrorResult("prompt is required")
	}
	spec, ok := cliAgentCommand[agentName]
	if !ok {
		return ErrorResult(fmt.Sprintf("unknown agent %q", agentName))
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
	binPath, err := exec.LookPath(spec.bin)
	if err != nil {
		return ErrorResult(fmt.Sprintf("%s CLI not found in PATH", spec.bin))
	}
	cmdArgs := append(spec.args, prompt)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, binPath, cmdArgs...)
	cmd.Dir = dir
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
		}
		return ErrorResult(msg)
	}
	if out == "" {
		out = "(no output)"
	}
	if len(out) > 40000 {
		out = out[:40000] + "\n…[truncated]"
	}
	return &Result{ForLLM: fmt.Sprintf("agent=%s  elapsed=%s\n\n%s", agentName, elapsed, out)}
}
