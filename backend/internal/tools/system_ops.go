// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build !windows

package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// SystemOpsTool provides structured system operations for privileged agent roles.
// It wraps specific system management commands behind a typed interface.
// All elevated commands run via scoped NOPASSWD sudoers (/etc/sudoers.d/qorven-ops).
//
// Only executes when the tool context has AllowElevated set (sysops/chief role).
// Degrades gracefully on macOS (no systemctl/journalctl) and non-standard Linux distros.
type SystemOpsTool struct{}

func NewSystemOpsTool() *SystemOpsTool { return &SystemOpsTool{} }

func (t *SystemOpsTool) Name() string { return "system_ops" }
func (t *SystemOpsTool) Description() string {
	return "Manage system services, read logs, check network, and install packages. " +
		"Actions: service_status, service_start, service_stop, service_restart, service_reload, " +
		"service_enable, service_disable, service_logs, system_health, install_package, " +
		"network_status, tailscale_status, docker_ps, docker_logs."
}

func (t *SystemOpsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Operation to perform",
				"enum": []string{
					"service_status", "service_start", "service_stop", "service_restart",
					"service_reload", "service_enable", "service_disable",
					"service_logs", "system_health", "install_package",
					"network_status", "tailscale_status", "docker_ps", "docker_logs",
				},
			},
			"service": map[string]any{
				"type":        "string",
				"description": "Service name (for service_* and docker_logs actions)",
			},
			"lines": map[string]any{
				"type":        "integer",
				"description": "Number of log lines to return (default 50, max 500)",
			},
			"package": map[string]any{
				"type":        "string",
				"description": "Package name to install (for install_package action)",
			},
			"since": map[string]any{
				"type":        "string",
				"description": "Time filter for service_logs, e.g. '1h', '30m', '2026-05-25 18:00'",
			},
		},
		"required": []string{"action"},
	}
}

func (t *SystemOpsTool) Execute(ctx context.Context, args map[string]any) *Result {
	if !IsElevated(ctx) {
		return ErrorResult("system_ops requires elevated agent role (sysops)")
	}

	action, _ := args["action"].(string)
	service, _ := args["service"].(string)
	pkg, _ := args["package"].(string)
	since, _ := args["since"].(string)
	lines := 50
	if l, ok := toInt(args["lines"]); ok && l > 0 {
		if l > 500 {
			l = 500
		}
		lines = l
	}

	switch action {
	case "service_status":
		if service == "" {
			return ErrorResult("service name required")
		}
		return t.serviceCtl(ctx, "status", "--no-pager", "-l", service)

	case "service_start":
		if service == "" {
			return ErrorResult("service name required")
		}
		return t.serviceCtl(ctx, "start", service)

	case "service_stop":
		if service == "" {
			return ErrorResult("service name required")
		}
		return t.serviceCtl(ctx, "stop", service)

	case "service_restart":
		if service == "" {
			return ErrorResult("service name required")
		}
		return t.serviceCtl(ctx, "restart", service)

	case "service_reload":
		if service == "" {
			return ErrorResult("service name required")
		}
		return t.serviceCtl(ctx, "reload", service)

	case "service_enable":
		if service == "" {
			return ErrorResult("service name required")
		}
		return t.serviceCtl(ctx, "enable", service)

	case "service_disable":
		if service == "" {
			return ErrorResult("service name required")
		}
		return t.serviceCtl(ctx, "disable", service)

	case "service_logs":
		if service == "" {
			return ErrorResult("service name required")
		}
		return t.serviceLogs(ctx, service, lines, since)

	case "system_health":
		return t.systemHealth(ctx)

	case "install_package":
		if pkg == "" {
			return ErrorResult("package name required")
		}
		if strings.ContainsAny(pkg, ";|&$`\\\"'") {
			return ErrorResult("invalid package name")
		}
		return t.installPackage(ctx, pkg)

	case "network_status":
		return t.networkStatus(ctx)

	case "tailscale_status":
		ts, err := exec.LookPath("tailscale")
		if err != nil {
			return ErrorResult("tailscale not installed")
		}
		return t.run(ctx, "sudo", ts, "status")

	case "docker_ps":
		docker, err := exec.LookPath("docker")
		if err != nil {
			return ErrorResult("docker not installed")
		}
		return t.run(ctx, "sudo", docker, "ps", "--format",
			"table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}")

	case "docker_logs":
		if service == "" {
			return ErrorResult("container name required")
		}
		docker, err := exec.LookPath("docker")
		if err != nil {
			return ErrorResult("docker not installed")
		}
		return t.run(ctx, "sudo", docker, "logs", "--tail",
			fmt.Sprintf("%d", lines), service)

	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}

// serviceCtl runs systemctl (Linux) or returns a macOS hint.
func (t *SystemOpsTool) serviceCtl(ctx context.Context, subcmd ...string) *Result {
	sc, err := exec.LookPath("systemctl")
	if err != nil {
		if runtime.GOOS == "darwin" {
			return ErrorResult("systemctl is not available on macOS. " +
				"Use 'brew services' or 'launchctl' via the exec tool instead.")
		}
		return ErrorResult("systemctl not found — is systemd installed?")
	}
	args := append([]string{sc}, subcmd...)
	return t.run(ctx, "sudo", args...)
}

// serviceLogs reads journald (Linux) or system.log (macOS fallback).
func (t *SystemOpsTool) serviceLogs(ctx context.Context, service string, lines int, since string) *Result {
	jc, err := exec.LookPath("journalctl")
	if err != nil {
		if runtime.GOOS == "darwin" {
			// macOS: try log show for the process name
			logBin, lerr := exec.LookPath("log")
			if lerr != nil {
				return ErrorResult("journalctl not available and 'log' not found on this macOS system")
			}
			logArgs := []string{logBin, "show", "--process", service,
				"--last", fmt.Sprintf("%dm", lines)}
			return t.run(ctx, logArgs[0], logArgs[1:]...)
		}
		return ErrorResult("journalctl not found — is systemd installed?")
	}
	jArgs := []string{jc, "-u", service, "--no-pager", "-n", fmt.Sprintf("%d", lines)}
	if since != "" {
		jArgs = append(jArgs, "--since", since)
	}
	return t.run(ctx, "sudo", jArgs...)
}

// installPackage detects the available package manager and installs.
func (t *SystemOpsTool) installPackage(ctx context.Context, pkg string) *Result {
	// Priority: dnf (RHEL/Amazon Linux) → apt-get (Debian/Ubuntu) → yum → brew (macOS)
	for _, pm := range []struct{ bin, flag string }{
		{"dnf", "install -y"},
		{"apt-get", "install -y"},
		{"yum", "install -y"},
	} {
		bin, err := exec.LookPath(pm.bin)
		if err != nil {
			continue
		}
		flagParts := strings.Fields(pm.flag)
		args := append([]string{bin}, append(flagParts, pkg)...)
		return t.run(ctx, "sudo", args...)
	}
	// macOS: brew (no sudo)
	if runtime.GOOS == "darwin" {
		brew, err := exec.LookPath("brew")
		if err != nil {
			return ErrorResult("no package manager found (tried dnf, apt-get, yum, brew)")
		}
		return t.run(ctx, brew, "install", pkg)
	}
	return ErrorResult("no supported package manager found (tried dnf, apt-get, yum)")
}

// systemHealth gathers service state, disk, memory, and load — using OS-appropriate commands.
func (t *SystemOpsTool) systemHealth(ctx context.Context) *Result {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var sb strings.Builder
	sb.WriteString("=== System Health ===\n\n")

	// Services — only meaningful when systemd is present
	if sc, err := exec.LookPath("systemctl"); err == nil {
		services := []string{"nginx", "postgresql", "redis", "tailscaled", "docker", "qorven"}
		for _, svc := range services {
			cmd := exec.CommandContext(ctx, "sudo", sc, "is-active", svc)
			out, _ := cmd.Output()
			status := strings.TrimSpace(string(out))
			icon := "✅"
			if status != "active" {
				icon = "⚠️"
			}
			sb.WriteString(fmt.Sprintf("%s %-20s %s\n", icon, svc, status))
		}
	} else {
		sb.WriteString("(systemctl not available — service status skipped)\n")
	}

	// Disk — df works on Linux and macOS
	sb.WriteString("\n=== Disk ===\n")
	if cmd := exec.CommandContext(ctx, "df", "-h", "/"); cmd != nil {
		if out, err := cmd.Output(); err == nil {
			sb.WriteString(string(out))
		}
	}

	// Memory — OS-specific
	sb.WriteString("\n=== Memory ===\n")
	if runtime.GOOS == "darwin" {
		// macOS: vm_stat for page stats, sysctl for total
		if cmd := exec.CommandContext(ctx, "vm_stat"); cmd != nil {
			if out, err := cmd.Output(); err == nil {
				// Show first 6 lines (page counts)
				lines := strings.SplitN(string(out), "\n", 7)
				sb.WriteString(strings.Join(lines[:min(6, len(lines))], "\n"))
				sb.WriteString("\n")
			}
		}
		if cmd := exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize"); cmd != nil {
			if out, err := cmd.Output(); err == nil {
				sb.WriteString("Total RAM: " + strings.TrimSpace(string(out)) + " bytes\n")
			}
		}
	} else {
		// Linux: free -h
		if cmd := exec.CommandContext(ctx, "free", "-h"); cmd != nil {
			if out, err := cmd.Output(); err == nil {
				sb.WriteString(string(out))
			}
		}
	}

	// Load — uptime works everywhere
	sb.WriteString("\n=== Load ===\n")
	if cmd := exec.CommandContext(ctx, "uptime"); cmd != nil {
		if out, err := cmd.Output(); err == nil {
			sb.WriteString(strings.TrimSpace(string(out)) + "\n")
		}
	}

	return TextResult(sb.String())
}

// networkStatus shows interface addresses, tailscale state, and listening ports.
func (t *SystemOpsTool) networkStatus(ctx context.Context) *Result {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var sb strings.Builder
	sb.WriteString("=== Network Interfaces ===\n")

	if runtime.GOOS == "darwin" {
		// macOS: ifconfig is the standard tool
		if cmd := exec.CommandContext(ctx, "ifconfig", "-a"); cmd != nil {
			if out, err := cmd.Output(); err == nil {
				sb.WriteString(string(out))
			}
		}
	} else {
		// Linux: prefer 'ip' (iproute2), fall back to 'ifconfig'
		if ip, err := exec.LookPath("ip"); err == nil {
			cmd := exec.CommandContext(ctx, ip, "-brief", "addr")
			if out, err := cmd.Output(); err == nil {
				sb.WriteString(string(out))
			}
		} else if ifc, err := exec.LookPath("ifconfig"); err == nil {
			cmd := exec.CommandContext(ctx, ifc)
			if out, err := cmd.Output(); err == nil {
				sb.WriteString(string(out))
			}
		}
	}

	// Tailscale
	sb.WriteString("\n=== Tailscale ===\n")
	if ts, err := exec.LookPath("tailscale"); err == nil {
		cmd := exec.CommandContext(ctx, "sudo", ts, "status", "--peers=false")
		if out, err := cmd.Output(); err == nil {
			sb.WriteString(string(out))
		} else {
			sb.WriteString("tailscale: not connected\n")
		}
	} else {
		sb.WriteString("tailscale: not installed\n")
	}

	// Listening ports — ss (Linux) or netstat (macOS/Linux fallback)
	sb.WriteString("\n=== Listening Ports ===\n")
	if runtime.GOOS != "darwin" {
		if ss, err := exec.LookPath("ss"); err == nil {
			cmd := exec.CommandContext(ctx, ss, "-tlnp")
			if out, err := cmd.Output(); err == nil {
				sb.WriteString(string(out))
				return TextResult(sb.String())
			}
		}
	}
	// macOS or ss unavailable: netstat
	if ns, err := exec.LookPath("netstat"); err == nil {
		cmd := exec.CommandContext(ctx, ns, "-an")
		if out, err := cmd.Output(); err == nil {
			// Filter to LISTEN lines only to keep output short
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "LISTEN") || strings.HasPrefix(line, "Proto") {
					sb.WriteString(line + "\n")
				}
			}
		}
	}

	return TextResult(sb.String())
}

// run executes a command and returns its combined output as a Result.
func (t *SystemOpsTool) run(ctx context.Context, program string, args ...string) *Result {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, program, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := stdout.String()
	if stderr.Len() > 0 {
		if out != "" {
			out += "\n"
		}
		out += stderr.String()
	}
	out = ScanOutputForSecrets(capOutput(out, 20000))

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ErrorResult("command timed out after 30s")
		}
		if out == "" {
			out = err.Error()
		}
		return &Result{ForLLM: fmt.Sprintf("❌ %s", out), ForUser: out, IsError: true}
	}
	if out == "" {
		out = "(no output)"
	}
	return TextResult(out)
}

