// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"regexp"
	"strings"
)

// ShellSecurity enforces command execution safety.
// 8 deny groups block dangerous commands before they reach the shell.
type ShellSecurity struct {
	DenyGroups    []DenyGroup
	SafeBins      map[string]bool
	Allowlist     map[string]bool // persistent user approvals
	AskMode       string          // "off", "on-miss", "always"
}

// DenyGroup is a category of blocked shell patterns.
type DenyGroup struct {
	Name     string
	Patterns []*regexp.Regexp
	Message  string
}

// ShellCheckResult is the outcome of a security check.
type ShellCheckResult struct {
	Allowed bool
	Reason  string
	AskUser bool   // true if user approval needed
	Group   string // which deny group matched
}

// NewShellSecurity creates security with default deny groups.
func NewShellSecurity() *ShellSecurity {
	return &ShellSecurity{
		DenyGroups: defaultDenyGroups(),
		SafeBins:   defaultSafeBins(),
		Allowlist:  make(map[string]bool),
		AskMode:    "off",
	}
}

// CheckCommand validates a command against deny groups, safe bins, and allowlist.
func (s *ShellSecurity) CheckCommand(command string) ShellCheckResult {
	normalized := strings.TrimSpace(command)
	if normalized == "" {
		return ShellCheckResult{Allowed: false, Reason: "empty command"}
	}

	// 1. Check deny groups first (always blocked, no override)
	for _, group := range s.DenyGroups {
		for _, pattern := range group.Patterns {
			if pattern.MatchString(normalized) {
				return ShellCheckResult{
					Allowed: false,
					Reason:  group.Message,
					Group:   group.Name,
				}
			}
		}
	}

	// 2. Check persistent allowlist
	if s.Allowlist[normalized] {
		return ShellCheckResult{Allowed: true}
	}

	// 3. Check safe bins
	binary := extractBinary(normalized)
	if s.SafeBins[binary] {
		return ShellCheckResult{Allowed: true}
	}

	// 4. Apply ask mode
	switch s.AskMode {
	case "off":
		return ShellCheckResult{Allowed: true}
	case "always":
		return ShellCheckResult{Allowed: false, AskUser: true, Reason: "approval required"}
	default: // "on-miss"
		return ShellCheckResult{Allowed: false, AskUser: true, Reason: "command not in allowlist"}
	}
}

// ApproveCommand adds a command to the persistent allowlist.
func (s *ShellSecurity) ApproveCommand(command string) {
	s.Allowlist[strings.TrimSpace(command)] = true
}

// extractBinary gets the first word of a command (the binary name).
func extractBinary(command string) string {
	// Handle common prefixes
	cmd := command
	for _, prefix := range []string{"sudo ", "env ", "nice ", "nohup ", "time "} {
		cmd = strings.TrimPrefix(cmd, prefix)
	}
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}
	// Strip path: /usr/bin/git → git
	binary := parts[0]
	if idx := strings.LastIndex(binary, "/"); idx >= 0 {
		binary = binary[idx+1:]
	}
	return binary
}

// --- 8 Deny Groups (from Qorven) ---

func defaultDenyGroups() []DenyGroup {
	return []DenyGroup{
		{
			Name: "system_destruction",
			Patterns: compilePatterns(
				`rm\s+(-[a-zA-Z]*f[a-zA-Z]*\s+)?/`,             // rm -rf /path
				`rm\s+(-[a-zA-Z]*[fr][a-zA-Z]*\s+)+[~$]`,       // rm -rf ~ or $HOME
				`mkfs\.`,
				`dd\s+.*of=/dev/`,
				`:+\(\)\s*\{.*\|.*&.*\}.*:`, // fork bomb variants
			),
			Message: "Blocked: command could destroy system files or devices.",
		},
		{
			Name: "credential_theft",
			Patterns: compilePatterns(
				`cat\s+.*\.(env|pem|key|secret)`,
				`cat\s+.*/\.ssh/`,
				`cat\s+.*/\.aws/`,
				`cat\s+.*/\.gnupg/`,
				`printenv\s+.*(_KEY|_SECRET|_TOKEN|_PASSWORD)`,
			),
			Message: "Blocked: command could expose credentials or secrets.",
		},
		{
			Name: "network_exfiltration",
			Patterns: compilePatterns(
				`curl\s+.*\$\{?\w*(KEY|TOKEN|SECRET|PASSWORD)`,
				`wget\s+.*\$\{?\w*(KEY|TOKEN|SECRET|PASSWORD)`,
				`(wget|curl)\s+.*\|\s*(ba)?sh`, // download-and-execute
				`nc\s+-.*\s+\d+`,               // netcat with port
				`ncat\s+`,
			),
			Message: "Blocked: command could exfiltrate data over the network.",
		},
		{
			Name: "privilege_escalation",
			Patterns: compilePatterns(
				`chmod\s+[0-7]*[67][0-7][0-7]\s+/`, // setuid/setgid on system files
				`chmod\s+(-[a-zA-Z]+\s+)?777\s+/`,  // world-writable on system root
				`chown\s+root`,
				`passwd\b`,
				`usermod\b`,
				`visudo\b`,
			),
			Message: "Blocked: command could escalate privileges.",
		},
		{
			Name: "service_disruption",
			Patterns: compilePatterns(
				`systemctl\s+(stop|disable|mask)\s+`,
				`service\s+\S+\s+stop`,
				`kill\s+-9\s+1\b`,  // kill init
				`shutdown\b`,
				`reboot\b`,
				`halt\b`,
			),
			Message: "Blocked: command could disrupt system services.",
		},
		{
			Name: "package_install",
			Patterns: compilePatterns(
				`apt(-get)?\s+install\b`,
				`yum\s+install\b`,
				`dnf\s+install\b`,
				`pacman\s+-S\b`,
				`brew\s+install\b`,
				`pip\s+install\b`,
				`npm\s+install\s+-g\b`,
			),
			Message: "Blocked: package installation requires approval. Use the approval system.",
		},
		{
			Name: "container_escape",
			Patterns: compilePatterns(
				`docker\s+run\s+.*--privileged`,
				`docker\s+run\s+.*-v\s+/:/`,
				`nsenter\b`,
				`unshare\b`,
			),
			Message: "Blocked: command could escape container isolation.",
		},
		{
			Name: "history_manipulation",
			Patterns: compilePatterns(
				`history\s+-c`,
				`history\s+-w`,
				`unset\s+HISTFILE`,
				`export\s+HISTSIZE=0`,
			),
			Message: "Blocked: command could manipulate shell history.",
		},
	}
}

func compilePatterns(patterns ...string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, regexp.MustCompile(`(?i)`+p))
	}
	return compiled
}

// --- Safe Bins (commands that never need approval) ---

func defaultSafeBins() map[string]bool {
	bins := []string{
		// File inspection
		"ls", "cat", "head", "tail", "wc", "sort", "uniq", "find", "which",
		"file", "stat", "du", "df", "tree",
		// Text processing
		"grep", "awk", "sed", "cut", "tr", "diff", "comm",
		// System info
		"echo", "printf", "date", "pwd", "whoami", "uname", "hostname",
		"id", "groups", "env", "printenv",
		// Logic
		"true", "false", "test",
		// Hashing
		"md5sum", "sha256sum", "sha1sum",
		// Git (read-only)
		"git",
		// Build tools (read-only operations)
		"go", "node", "python", "python3", "ruby", "cargo", "make",
		// Additional safe bins for agentic workloads
		"curl", "wget", "jq", "tar", "gzip", "gunzip", "unzip", "zip",
		"pip", "pip3", "npm", "npx", "yarn", "pnpm",
		"rustc",
		"java", "mvn", "gradle",
		"docker",
		"kubectl",
		"terraform",
		"crontab",
		"systemctl",
		"journalctl",
		"ps", "top", "htop", "free", "vmstat", "iostat",
		"ping", "dig", "nslookup", "traceroute",
		"openssl",
		"ssh", "scp", "rsync",
		"ffmpeg", "convert",
		"gofmt", "cmake",
		"grep", "awk", "sed", "find", "sort", "uniq", "wc", "cut",
		"head", "tail", "less", "more",
		"cp", "mv", "mkdir", "rmdir", "ln", "chmod", "chown",
		"printenv", "which", "whereis", "type",
		"date", "sleep", "echo", "printf",
		"diff", "patch",
		"base64", "md5sum", "sha256sum",
	}
	m := make(map[string]bool, len(bins))
	for _, b := range bins {
		m[b] = true
	}
	return m
}
