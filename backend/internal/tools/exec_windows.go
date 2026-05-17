// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

//go:build windows

package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/sandbox"
)

const maxExecOutput = 100000 // 100KB

var bgProcessRegistry = struct {
	sync.Mutex
	pids map[int]string
}{pids: make(map[int]string)}

func registerBackgroundProcess(pid int, description string) {
	bgProcessRegistry.Lock()
	bgProcessRegistry.pids[pid] = description
	bgProcessRegistry.Unlock()
}

func unregisterBackgroundProcess(pid int) {
	bgProcessRegistry.Lock()
	delete(bgProcessRegistry.pids, pid)
	bgProcessRegistry.Unlock()
}

func ShutdownBackgroundProcesses(_ time.Duration) {}

type ExecTool struct {
	workspace string
	restrict  bool
	sandbox   sandbox.Manager
}

func NewExecTool(workspace string, restrict bool) *ExecTool {
	return &ExecTool{workspace: workspace, restrict: restrict}
}

func NewSandboxedExecTool(workspace string, restrict bool, mgr sandbox.Manager) *ExecTool {
	return &ExecTool{workspace: workspace, restrict: restrict, sandbox: mgr}
}

func (t *ExecTool) Name() string { return "exec" }
func (t *ExecTool) Description() string {
	return "Execute a shell command and return stdout+stderr."
}
func (t *ExecTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command":    map[string]any{"type": "string"},
			"timeout_ms": map[string]any{"type": "integer"},
		},
		"required": []string{"command"},
	}
}

var OnExecComplete func(ctx context.Context, agentID, command, output string, exitCode int, durationMs int64)

func (t *ExecTool) Execute(ctx context.Context, args map[string]any) *Result {
	command, _ := args["command"].(string)
	if command == "" {
		return ErrorResult("command is required")
	}
	timeoutMs, _ := args["timeout_ms"].(float64)
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond

	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx2, "cmd", "/C", command)
	cmd.Dir = t.workspace
	cmd.Env = safeEnv(t.workspace)
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start).Milliseconds()

	output := string(out)
	exitCode := 0
	if err != nil {
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		} else {
			exitCode = 1
		}
	}

	if OnExecComplete != nil {
		agentID, _ := ctx.Value("agent_id").(string)
		OnExecComplete(ctx, agentID, command, output, exitCode, elapsed)
	}

	if exitCode != 0 {
		return &Result{ForLLM: fmt.Sprintf("exit %d\n%s", exitCode, output), IsError: true}
	}
	return &Result{ForLLM: output}
}

func safeEnv(workspace string) []string {
	env := os.Environ()
	safe := make([]string, 0, len(env))
	for _, e := range env {
		k := strings.SplitN(e, "=", 2)[0]
		switch k {
		case "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "QORVEN_ENCRYPTION_KEY":
			continue
		}
		safe = append(safe, e)
	}
	safe = append(safe, "QORVEN_WORKSPACE="+workspace)
	return safe
}

type limitedBuffer struct{}

func (lb *limitedBuffer) Write(p []byte) (int, error) { return len(p), nil }
func (lb *limitedBuffer) String() string              { return "" }
func (lb *limitedBuffer) Len() int                    { return 0 }

func truncateCmd(cmd string, max int) string {
	if len(cmd) <= max {
		return cmd
	}
	return cmd[:max] + "…"
}
