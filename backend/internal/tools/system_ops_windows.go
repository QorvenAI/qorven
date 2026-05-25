// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

//go:build windows

package tools

import "context"

// SystemOpsTool stub for Windows.
// Full system operations (systemctl, journalctl, apt) are Linux/macOS-only.
// On Windows, agents should use the exec tool with PowerShell commands instead.
type SystemOpsTool struct{}

func NewSystemOpsTool() *SystemOpsTool { return &SystemOpsTool{} }

func (t *SystemOpsTool) Name() string { return "system_ops" }
func (t *SystemOpsTool) Description() string {
	return "Manage system services, read logs, check network, and install packages. " +
		"Note: on Windows, service management uses PowerShell/sc.exe via the exec tool."
}
func (t *SystemOpsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "Operation to perform",
			},
			"service": map[string]any{"type": "string"},
			"lines":   map[string]any{"type": "integer"},
			"package": map[string]any{"type": "string"},
			"since":   map[string]any{"type": "string"},
		},
		"required": []string{"action"},
	}
}

func (t *SystemOpsTool) Execute(_ context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)
	switch action {
	case "system_health":
		return TextResult(windowsSystemHealth())
	case "network_status":
		return TextResult("Use exec tool with PowerShell: Get-NetAdapter | Format-Table Name,Status,LinkSpeed")
	case "install_package":
		pkg, _ := args["package"].(string)
		if pkg == "" {
			return ErrorResult("package name required")
		}
		return TextResult("Use exec tool: winget install " + pkg +
			"\nOr with Chocolatey: choco install " + pkg)
	case "service_status", "service_start", "service_stop", "service_restart":
		svc, _ := args["service"].(string)
		if svc == "" {
			return ErrorResult("service name required")
		}
		hint := map[string]string{
			"service_status":  "Get-Service -Name " + svc,
			"service_start":   "Start-Service -Name " + svc,
			"service_stop":    "Stop-Service -Name " + svc,
			"service_restart": "Restart-Service -Name " + svc,
		}[action]
		return TextResult("Use exec tool with PowerShell: " + hint)
	case "service_logs":
		svc, _ := args["service"].(string)
		return TextResult("Use exec tool: Get-EventLog -LogName Application -Source " + svc + " -Newest 50")
	default:
		return ErrorResult("system_ops action '" + action + "' is not supported on Windows. " +
			"Use the exec tool with PowerShell or cmd commands instead.")
	}
}

func windowsSystemHealth() string {
	return "=== Windows System Health ===\n\n" +
		"Use exec tool with PowerShell for Windows health checks:\n\n" +
		"Services:  Get-Service | Where-Object {$_.Status -eq 'Running'}\n" +
		"Disk:      Get-PSDrive C | Select-Object Used,Free\n" +
		"Memory:    Get-CimInstance Win32_OperatingSystem | Select FreePhysicalMemory,TotalVisibleMemorySize\n" +
		"CPU:       Get-CimInstance Win32_Processor | Select LoadPercentage\n" +
		"Uptime:    (Get-Date) - (gcim Win32_OperatingSystem).LastBootUpTime\n"
}
