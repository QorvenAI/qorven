// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

//go:build !windows

package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/sandbox"
)

// bgProcessRegistry tracks background processes started by the exec
// tool so the gateway can SIGTERM them on shutdown. Before this, a
// detached child kept running after its parent exited — an
// `npm run dev &` spawned by an agent would keep holding port 3000
// long after the gateway had been stopped and restarted. Setsid
// creates a new session, so the usual "parent death → child SIGHUP"
// doesn't happen.
//
// The registry is a package-level singleton because exec tools are
// instantiated per-request but the processes they spawn are long-
// lived and host-scoped. A shared map + mutex keeps the API flat.
var bgProcessRegistry = struct {
	mu   sync.Mutex
	pids map[int]string // pid → description for logging
}{pids: make(map[int]string)}

// registerBackgroundProcess adds a spawned-in-background PID to the
// registry. Callers should also start a goroutine that unregisters
// when the process exits.
func registerBackgroundProcess(pid int, description string) {
	bgProcessRegistry.mu.Lock()
	bgProcessRegistry.pids[pid] = description
	bgProcessRegistry.mu.Unlock()
}

func unregisterBackgroundProcess(pid int) {
	bgProcessRegistry.mu.Lock()
	delete(bgProcessRegistry.pids, pid)
	bgProcessRegistry.mu.Unlock()
}

// ShutdownBackgroundProcesses sends SIGTERM to every tracked process,
// waits briefly for them to exit, then force-kills any that remain.
// Called from the gateway's graceful-shutdown handler so an operator
// stopping the service doesn't leak dev servers and ad-hoc pipelines.
func ShutdownBackgroundProcesses(grace time.Duration) {
	bgProcessRegistry.mu.Lock()
	pids := make([]int, 0, len(bgProcessRegistry.pids))
	for pid := range bgProcessRegistry.pids {
		pids = append(pids, pid)
	}
	bgProcessRegistry.mu.Unlock()
	if len(pids) == 0 {
		return
	}
	slog.Info("exec.background.shutdown", "count", len(pids), "grace", grace)
	for _, pid := range pids {
		// Signal the whole process group — Setsid gave each child its
		// own group, and `npm run dev` style commands spawn children
		// of their own.
		_ = syscall.Kill(-pid, syscall.SIGTERM)
	}
	time.Sleep(grace)
	for _, pid := range pids {
		if err := syscall.Kill(pid, 0); err == nil {
			// Still alive — escalate.
			slog.Warn("exec.background.sigkill", "pid", pid)
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	}
}

const (
	defaultExecTimeout = 120 * time.Second
	sandboxExecTimeout = 300 * time.Second
	maxExecOutput      = 100000 // 100KB
)

// ExecTool executes shell commands, optionally inside a sandbox container.
type ExecTool struct {
	workspace  string
	timeout    time.Duration
	sandboxMgr sandbox.Manager // nil = no sandbox, execute on host
	restrict   bool            // restrict paths to workspace
}

// NewExecTool creates an exec tool that runs commands directly on the host.
func NewExecTool(workspace string, restrict bool) *ExecTool {
	return &ExecTool{
		workspace: workspace,
		timeout:   defaultExecTimeout,
		restrict:  restrict,
	}
}

// NewSandboxedExecTool creates an exec tool that routes commands through a sandbox container.
func NewSandboxedExecTool(workspace string, restrict bool, mgr sandbox.Manager) *ExecTool {
	return &ExecTool{
		workspace:  workspace,
		timeout:    sandboxExecTimeout,
		restrict:   restrict,
		sandboxMgr: mgr,
	}
}

func (t *ExecTool) Name() string { return "exec" }
func (t *ExecTool) Description() string {
	return "Execute a shell command. For servers/long-running processes, use nohup or & to run in background."
}
func (t *ExecTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to execute",
			},
			"working_dir": map[string]any{
				"type":        "string",
				"description": "Working directory for the command (default: workspace root)",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 120, max 300)",
			},
			"background": map[string]any{
				"type":        "boolean",
				"description": "Run in background — starts process detached, returns PID immediately. Use for servers, dev servers, long builds.",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ExecTool) Execute(ctx context.Context, args map[string]any) *Result {
	command, _ := args["command"].(string)
	if command == "" {
		return ErrorResult("command is required")
	}

	// Reject NUL bytes — they cause silent shell truncation enabling injection.
	if strings.ContainsRune(command, '\x00') {
		return ErrorResult("command contains invalid NUL byte")
	}

	// Security: check deny patterns
	if denied, pattern := IsShellDenied(command); denied {
		return ErrorResult(fmt.Sprintf("command blocked by security policy (matched: %s)", pattern))
	}

	// Resolve workspace
	ws := WorkspaceFromCtx(ctx)
	if ws == "" {
		ws = t.workspace
	}

	// Resolve working directory
	cwd := ws
	if wd, _ := args["working_dir"].(string); wd != "" {
		if t.restrict {
			resolved, err := resolvePath(wd, ws, true)
			if err != nil {
				return ErrorResult(err.Error())
			}
			cwd = resolved
		} else {
			cwd = wd
		}
	}

	// Timeout
	timeout := t.timeout
	if to, ok := toInt(args["timeout"]); ok && to > 0 {
		if to > 300 {
			to = 300
		}
		timeout = time.Duration(to) * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Background mode: start process detached, return PID immediately
	if bg, _ := args["background"].(bool); bg {
		cmd := exec.Command(SafeShell, "-c", command)
		cmd.Dir = cwd
		cmd.Env = safeEnv(cwd)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return ErrorResult(fmt.Sprintf("background start failed: %v", err))
		}
		pid := cmd.Process.Pid
		registerBackgroundProcess(pid, command)
		// Reap zombie AND unregister on exit so the shutdown sweep
		// doesn't target an already-dead PID (which would either be a
		// no-op or, worse, kill a recycled PID).
		go func() {
			_ = cmd.Wait()
			unregisterBackgroundProcess(pid)
		}()
		return TextResult(fmt.Sprintf("Started in background. PID: %d\nUse exec with command \"kill -0 %d && echo running || echo dead\" to check if it is still alive. Use \"kill %d\" to stop it.", pid, pid, pid))
	}

	// Sandbox routing
	sandboxKey := SandboxKeyFromCtx(ctx)
	if t.sandboxMgr != nil && sandboxKey != "" {
		return t.executeInSandbox(ctx, command, cwd, sandboxKey)
	}

	// Host execution
	return t.executeOnHost(ctx, command, cwd, timeout)
}

// executeOnHost runs a command directly on the host.
func (t *ExecTool) executeOnHost(ctx context.Context, command, cwd string, timeout time.Duration) *Result {
	cmd := exec.CommandContext(ctx, SafeShell, "-c", command)
	cmd.Dir = cwd
	cmd.Env = safeEnv(cwd)

	stdout := &limitedBuffer{max: 1 << 20}
	stderr := &limitedBuffer{max: 1 << 20}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()

	var result string
	if stdout.Len() > 0 {
		result = stdout.String()
	}
	if stderr.Len() > 0 {
		if result != "" {
			result += "\n"
		}
		result += "STDERR:\n" + stderr.String()
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ErrorResult(fmt.Sprintf("command timed out after %s\n%s", timeout, result))
		}
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		result = ScanOutputForSecrets(result)
		if result == "" {
			result = err.Error()
		}
		return &Result{
			ForLLM:  fmt.Sprintf("❌ Exit code: %d\n%s\nFix the command or code and try again.", exitCode, result),
			ForUser: fmt.Sprintf("exit code %d\n%s", exitCode, result),
			IsError: true,
		}
	}

	if result == "" {
		result = "(no output)"
	}
	result = ScanOutputForSecrets(result)

	// Save to sandbox_runs for GUI visibility
	if OnExecComplete != nil {
		OnExecComplete(ctx, AgentIDFromCtx(ctx), command, result, 0, 0)
	}

	return &Result{
		ForLLM:  fmt.Sprintf("✅ Exit code: 0\n%s", capOutput(result, maxExecOutput)),
		ForUser: capOutput(result, maxExecOutput),
	}
}

// OnExecComplete is called after exec runs a command. Set by gateway to save to sandbox_runs.
var OnExecComplete func(ctx context.Context, agentID, command, output string, exitCode int, durationMs int64)

// executeInSandbox routes a command through a Docker sandbox container.
func (t *ExecTool) executeInSandbox(ctx context.Context, command, cwd, sandboxKey string) *Result {
	sb, err := t.sandboxMgr.Get(ctx, sandboxKey, t.workspace, nil)
	if err != nil {
		if errors.Is(err, sandbox.ErrSandboxDisabled) {
			return t.executeOnHost(ctx, command, cwd, t.timeout)
		}
		// Docker unavailable — fail closed, do NOT fallback to host
		slog.Warn("sandbox unavailable", "error", err, "command", truncateCmd(command, 80))
		return ErrorResult(fmt.Sprintf("sandbox unavailable: %v (will not fall back to unsandboxed host execution)", err))
	}

	// Map host workdir to container workdir
	containerCwd := sandbox.DefaultContainerWorkdir

	result, err := sb.Exec(ctx, []string{"sh", "-c", command}, containerCwd)
	if err != nil {
		return ErrorResult(fmt.Sprintf("sandbox exec: %v", err))
	}

	// Format output
	output := result.Stdout
	if result.Stderr != "" {
		if output != "" {
			output += "\n"
		}
		output += "STDERR:\n" + result.Stderr
	}

	output = ScanOutputForSecrets(output)

	if result.ExitCode != 0 {
		if output == "" {
			output = fmt.Sprintf("command exited with code %d", result.ExitCode)
		}
		output += sandbox.MaybeSandboxHint(result.ExitCode, output)
		return &Result{
			ForLLM:  fmt.Sprintf("❌ [sandbox] Exit code: %d\n%s", result.ExitCode, capOutput(output, maxExecOutput)),
			ForUser: capOutput(output, maxExecOutput),
			IsError: true,
		}
	}

	if output == "" {
		output = "(no output)"
	}
	return &Result{
		ForLLM:  fmt.Sprintf("✅ [sandbox] Exit code: 0\n%s", capOutput(output, maxExecOutput)),
		ForUser: capOutput(output, maxExecOutput),
	}
}

// safeEnv builds a minimal environment.
func safeEnv(workspace string) []string {
	return []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=" + workspace,
		"LANG=en_US.UTF-8",
		"TERM=dumb",
	}
}

// limitedBuffer caps output to prevent OOM.
type limitedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (lb *limitedBuffer) Write(p []byte) (int, error) {
	if lb.truncated {
		return len(p), nil
	}
	remaining := lb.max - lb.buf.Len()
	if remaining <= 0 {
		lb.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		lb.buf.Write(p[:remaining])
		lb.truncated = true
		return len(p), nil
	}
	return lb.buf.Write(p)
}

func (lb *limitedBuffer) String() string {
	s := lb.buf.String()
	if lb.truncated {
		s += "\n[output truncated at 1MB]"
	}
	return s
}

func (lb *limitedBuffer) Len() int { return lb.buf.Len() }

// Helper functions

func truncateCmd(cmd string, max int) string {
	if len(cmd) <= max {
		return cmd
	}
	return cmd[:max] + "..."
}

func capOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n[truncated — %d bytes total]", len(s))
}

func resolvePath(path, base string, mustExist bool) (string, error) {
	if path == "" {
		return base, nil
	}
	// Simple path resolution — expand relative paths
	if !strings.HasPrefix(path, "/") {
		path = base + "/" + path
	}
	// Check if within workspace
	if !strings.HasPrefix(path, base) {
		return "", fmt.Errorf("path %s is outside workspace", path)
	}
	if mustExist {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("path does not exist: %s", path)
		}
	}
	return path, nil
}

// Context helpers are in types.go
