// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package permissions

import "strings"

// PermissionMode controls how tool execution is authorized.
// Inspired by Claude Code's 5-mode system.
type PermissionMode string

const (
	ModeDefault PermissionMode = "default" // prompt user for dangerous tools
	ModePlan    PermissionMode = "plan"    // read-only tools only, no writes
	ModeBypass  PermissionMode = "bypass"  // skip all permission checks (admin)
	ModeAuto    PermissionMode = "auto"    // auto-approve safe tools, prompt for dangerous
	ModeAsk     PermissionMode = "ask"     // always prompt user for every tool
)

// ToolRisk classifies tools by risk level.
type ToolRisk string

const (
	RiskSafe      ToolRisk = "safe"      // read-only, no side effects
	RiskModerate  ToolRisk = "moderate"  // writes files, sends messages
	RiskDangerous ToolRisk = "dangerous" // shell exec, delete, deploy
)

// ToolRiskMap classifies each tool.
var ToolRiskMap = map[string]ToolRisk{
	// Safe (read-only)
	"web_search": RiskSafe, "web_fetch": RiskSafe, "file_read": RiskSafe,
	"grep": RiskSafe, "glob": RiskSafe, "git_log": RiskSafe, "git_diff": RiskSafe,
	"knowledge_query": RiskSafe, "ask_user": RiskSafe, "memory_read": RiskSafe,
	// Moderate (writes)
	"file_write": RiskModerate, "file_edit": RiskModerate, "memory_write": RiskModerate,
	"send_email": RiskModerate, "send_message": RiskModerate, "create_task": RiskModerate,
	"git_commit": RiskModerate, "execute_action": RiskModerate,
	// Dangerous (exec/delete/deploy)
	"shell_exec": RiskDangerous, "file_delete": RiskDangerous, "deploy": RiskDangerous,
	"git_push": RiskDangerous, "agent_spawn": RiskDangerous,
}

// ToolPermission records a user's approval/denial for a specific tool.
type ToolPermission struct {
	Tool     string `json:"tool"`
	Approved bool   `json:"approved"`
	Scope    string `json:"scope"` // "once", "session", "always"
}

// CheckPermission determines if a tool can execute under the given mode.
// Returns: allowed bool, needsPrompt bool
func CheckPermission(tool string, mode PermissionMode, approvals map[string]*ToolPermission) (bool, bool) {
	risk := ToolRiskMap[tool]
	if risk == "" { risk = RiskModerate } // unknown tools default to moderate

	switch mode {
	case ModeBypass:
		return true, false
	case ModePlan:
		return risk == RiskSafe, false
	case ModeAsk:
		if ap, ok := approvals[tool]; ok { return ap.Approved, false }
		return false, true
	case ModeAuto:
		if risk == RiskSafe { return true, false }
		if ap, ok := approvals[tool]; ok { return ap.Approved, false }
		return false, true
	default: // ModeDefault
		if risk == RiskSafe { return true, false }
		if risk == RiskModerate { return true, false }
		if ap, ok := approvals[tool]; ok { return ap.Approved, false }
		return false, true // dangerous tools need approval
	}
}

// FilterToolsByMode returns only the tools allowed under the given mode.
func FilterToolsByMode(tools []string, mode PermissionMode) []string {
	if mode == ModeBypass { return tools }
	var allowed []string
	for _, t := range tools {
		ok, _ := CheckPermission(t, mode, nil)
		if ok { allowed = append(allowed, t) }
	}
	return allowed
}

// IsDangerousCommand checks if a shell command is dangerous.
func IsDangerousCommand(cmd string) bool {
	lower := strings.ToLower(cmd)
	dangerous := []string{
		"rm -rf", "mkfs", "dd if=", "> /dev/sd", "chmod 777 /",
		"curl | sh", "wget | sh", ":(){ :|:& };:",
		"DROP TABLE", "DELETE FROM", "TRUNCATE",
		"shutdown", "reboot", "init 0",
	}
	for _, d := range dangerous {
		if strings.Contains(lower, d) { return true }
	}
	return false
}
