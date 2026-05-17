// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// SafetyLimits enforces execution boundaries for browser and exec tools.
type SafetyLimits struct {
	ExecTimeout    time.Duration // max execution time (default 30s)
	MaxOutputBytes int           // max stdout+stderr (default 1MB)
	MaxPages       int           // max browser pages (default 5)
	IdleTimeout    time.Duration // auto-close idle browser (default 60s)
	DenyPatterns   []string      // blocked command patterns
}

var DefaultLimits = SafetyLimits{
	ExecTimeout:    30 * time.Second,
	MaxOutputBytes: 1024 * 1024, // 1MB
	MaxPages:       5,
	IdleTimeout:    60 * time.Second,
	DenyPatterns: []string{
		"rm -rf /", "mkfs", "dd if=", "> /dev/sd",
		":(){ :|:& };:", "chmod 777 /", "curl|sh", "wget|sh",
		"curl | bash", "wget | bash",
	},
}

// CheckCommand validates a command against safety limits.
func (sl *SafetyLimits) CheckCommand(cmd string) error {
	lower := strings.ToLower(cmd)
	for _, p := range sl.DenyPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return fmt.Errorf("blocked: command matches deny pattern '%s'", p)
		}
	}
	return nil
}

// ExecWithLimits runs a command with timeout and output limits.
func (sl *SafetyLimits) ExecWithLimits(ctx context.Context, command, workDir string) (string, int, error) {
	if err := sl.CheckCommand(command); err != nil {
		return "", 1, err
	}

	timeout := sl.ExecTimeout
	if timeout == 0 {
		timeout = DefaultLimits.ExecTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()

	// Truncate output
	maxBytes := sl.MaxOutputBytes
	if maxBytes == 0 {
		maxBytes = DefaultLimits.MaxOutputBytes
	}
	if len(output) > maxBytes {
		output = append(output[:maxBytes], []byte("\n... (truncated)")...)
		slog.Warn("exec.output_truncated", "command", command[:min(len(command), 60)], "bytes", len(output))
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() != nil {
			return string(output) + "\n⏰ Command timed out after " + timeout.String(), 124, fmt.Errorf("timeout")
		} else {
			exitCode = 1
		}
	}

	return string(output), exitCode, nil
}


// DangerousEnvVars are environment variables that could be injected via exec to compromise builds.
// Ref: Qorven security patches for Maven, Gradle, .NET injection.
var DangerousEnvVars = []string{
	"MAVEN_OPTS", "GRADLE_OPTS", "JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS",
	"DOTNET_STARTUP_HOOKS", "NODE_OPTIONS", "PYTHONSTARTUP",
	"LD_PRELOAD", "LD_LIBRARY_PATH", "DYLD_INSERT_LIBRARIES",
}

// CheckEnvInjection detects attempts to set dangerous env vars in commands.
func CheckEnvInjection(cmd string) error {
	upper := strings.ToUpper(cmd)
	for _, env := range DangerousEnvVars {
		if strings.Contains(upper, env+"=") {
			return fmt.Errorf("blocked: setting %s is not allowed (security)", env)
		}
	}
	return nil
}

// secretPatterns matches common API key formats for exfiltration detection.
var secretPatterns = []string{
	"sk-", "sk-proj-", "Bearer ", "token=sk-", "key=sk-",
	"fc-", "tvly-", "pplx-", "ghp_", "gho_", "github_pat_",
	"xoxb-", "xapp-", "AIza", "AKIA", "aws_secret",
}

// ContainsSecretInURL checks if a URL contains embedded API keys.
func ContainsSecretInURL(url string) bool {
	lower := strings.ToLower(url)
	for _, p := range secretPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	// Check for base64-encoded secrets
	if strings.Contains(url, "base64,") {
		return true // suspicious — could be encoding secrets
	}
	return false
}

// ScanOutputForSecrets checks tool output for leaked secrets.
func ScanOutputForSecrets(output string) string {
	for _, p := range secretPatterns {
		if idx := strings.Index(output, p); idx >= 0 {
			// Redact the secret
			end := idx + len(p)
			for end < len(output) && output[end] != ' ' && output[end] != '"' && output[end] != '\n' {
				end++
			}
			output = output[:idx+len(p)] + "***REDACTED***" + output[end:]
		}
	}
	return output
}
