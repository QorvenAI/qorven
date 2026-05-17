// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import "regexp"

// DenyGroup is a named set of shell command deny patterns.
// Defense-in-depth: patterns complement Docker hardening (cap-drop ALL,
// no-new-privileges, pids-limit, memory limit).
type DenyGroup struct {
	Name        string           // machine name: "package_install"
	Description string           // human label: "Package Installation"
	Default     bool             // true = denied by default
	Patterns    []*regexp.Regexp // deny patterns for this group
}

// DenyGroupRegistry holds all known deny groups, keyed by name.
// All groups are ON (denied) by default — admin must explicitly allow.
var DenyGroupRegistry = map[string]*DenyGroup{
	"destructive_ops": {
		Name: "destructive_ops", Description: "Destructive Operations", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\brm\s+-[rf]{1,2}\b`),
			regexp.MustCompile(`\brm\s+.*--recursive`),
			regexp.MustCompile(`\brm\s+.*--force`),
			regexp.MustCompile(`\bdel\s+/[fq]\b`),
			regexp.MustCompile(`\brmdir\s+/s\b`),
			regexp.MustCompile(`\b(mkfs|diskpart)\b|\bformat\s`),
			regexp.MustCompile(`\bdd\s+if=`),
			regexp.MustCompile(`>\s*/dev/sd[a-z]\b`),
			regexp.MustCompile(`\b(shutdown|reboot|poweroff|halt)\b`),
			regexp.MustCompile(`\b(init|telinit)\s+[06]\b`),
			regexp.MustCompile(`\bsystemctl\s+(suspend|hibernate)\b`),
			regexp.MustCompile(`:\(\)\s*\{.*\};\s*:`), // fork bomb
		},
	},
	"data_exfiltration": {
		Name: "data_exfiltration", Description: "Data Exfiltration", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\bcurl\b.*\|\s*(ba)?sh\b`),
			regexp.MustCompile(`\bcurl\b.*(-d\b|-F\b|--data|--upload|--form|-T\b|(-X|--request)\s*P(UT|OST|ATCH))`),
			regexp.MustCompile(`\bwget\b.*-O\s*-\s*\|\s*(ba)?sh\b`),
			regexp.MustCompile(`\bwget\b.*(--post-(data|file)|--method=P(UT|OST|ATCH)|--body-data)`),
			regexp.MustCompile(`\b(nslookup|dig|host)\b`),
			regexp.MustCompile(`/dev/tcp/`),
			regexp.MustCompile(`\b(curl|wget)\b.*\blocalhost\b`),
			regexp.MustCompile(`\b(curl|wget)\b.*\b127\.0\.0\.1\b`),
			regexp.MustCompile(`\b(curl|wget)\b.*\b0\.0\.0\.0\b`),
		},
	},
	"reverse_shell": {
		Name: "reverse_shell", Description: "Reverse Shell", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\b(nc|ncat|netcat)\b.*(\s+-[a-z]|\s+\d+\.\d+\.\d+\.\d+|\s+localhost\b)`),
			regexp.MustCompile(`\bsocat\b`),
			regexp.MustCompile(`\bopenssl\b.*s_client`),
			regexp.MustCompile(`\btelnet\b.*\d+`),
			regexp.MustCompile(`\bpython[23]?\b.*(import|from)\s+(socket|http|urllib|requests|httpx|aiohttp)\b`),
			regexp.MustCompile(`\bperl\b.*-e\s*.*\b[Ss]ocket\b`),
			regexp.MustCompile(`\bruby\b.*-e\s*.*\b(TCPSocket|Socket)\b`),
			regexp.MustCompile(`\bnode\b.*-e\s*.*\b(net\.|http\.|https\.|fetch\(|axios|got\(|undici)\b`),
			regexp.MustCompile(`\bnode\b.*-e\s*.*require\s*\(\s*['"]https?['"]\s*\)`),
			regexp.MustCompile(`\bawk\b.*/inet/`),
			regexp.MustCompile(`\bmkfifo\b`),
		},
	},
	"code_injection": {
		Name: "code_injection", Description: "Code Injection & Eval", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\beval\s*\$`),
			regexp.MustCompile(`\bbase64\s+(-d\w*|--decode)\b.*\|\s*(ba)?sh\b`),
		},
	},
	"privilege_escalation": {
		Name: "privilege_escalation", Description: "Privilege Escalation", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\bsudo\b`),
			regexp.MustCompile(`\bsu\b`),
			regexp.MustCompile(`\bdoas\b`),
			regexp.MustCompile(`\bpkexec\b`),
			regexp.MustCompile(`\brunuser\b`),
			regexp.MustCompile(`\bnsenter\b`),
			regexp.MustCompile(`\bunshare\b`),
			regexp.MustCompile(`\b(mount|umount)\b`),
			regexp.MustCompile(`\b(capsh|setcap|getcap)\b`),
		},
	},
	"dangerous_paths": {
		Name: "dangerous_paths", Description: "Dangerous Path Operations", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\bchmod\s+[0-7]{3,4}\s+/`),
			regexp.MustCompile(`\bchown\b.*\s+/`),
			regexp.MustCompile(`\bchmod\b.*\+x.*/tmp/`),
			regexp.MustCompile(`\bchmod\b.*\+x.*/var/tmp/`),
			regexp.MustCompile(`\bchmod\b.*\+x.*/dev/shm/`),
		},
	},
	"env_injection": {
		Name: "env_injection", Description: "Environment Variable Injection", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\bLD_PRELOAD\s*=`),
			regexp.MustCompile(`\bDYLD_INSERT_LIBRARIES\s*=`),
			regexp.MustCompile(`\bLD_LIBRARY_PATH\s*=`),
			regexp.MustCompile(`/etc/ld\.so\.preload`),
			regexp.MustCompile(`\bGIT_EXTERNAL_DIFF\s*=`),
			regexp.MustCompile(`\bGIT_DIFF_OPTS\s*=`),
			regexp.MustCompile(`\bBASH_ENV\s*=`),
			regexp.MustCompile(`\bENV\s*=.*\bsh\b`),
		},
	},
	"container_escape": {
		Name: "container_escape", Description: "Container Escape", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`/var/run/docker\.sock|docker\.(sock|socket)`),
			regexp.MustCompile(`/proc/sys/(kernel|fs|net)/`),
			regexp.MustCompile(`/sys/(kernel|fs|class|devices)/`),
		},
	},
	"crypto_mining": {
		Name: "crypto_mining", Description: "Crypto Mining", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\b(xmrig|cpuminer|minerd|cgminer|bfgminer|ethminer|nbminer|t-rex|phoenixminer|lolminer|gminer|claymore)\b`),
			regexp.MustCompile(`stratum\+tcp://|stratum\+ssl://`),
		},
	},
	"filter_bypass": {
		Name: "filter_bypass", Description: "Filter Bypass (CVE mitigations)", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\bsed\b.*['"]/e\b`),
			regexp.MustCompile(`\bsort\b.*--compress-program`),
			regexp.MustCompile(`\bgit\b.*(--upload-pack|--receive-pack|--exec)=`),
			regexp.MustCompile(`\b(rg|grep)\b.*--pre=`),
			regexp.MustCompile(`\bman\b.*--html=`),
			regexp.MustCompile(`\bhistory\b.*-[saw]\b`),
			regexp.MustCompile(`\$\{[^}]*@[PpEeAaKk]\}`),
		},
	},
	"network_recon": {
		Name: "network_recon", Description: "Network Reconnaissance & Tunneling", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\b(nmap|masscan|zmap|rustscan)\b`),
			regexp.MustCompile(`\b(ssh|scp|sftp)\b.*@`),
			regexp.MustCompile(`\b(chisel|frp|ngrok|cloudflared|bore|localtunnel)\b`),
		},
	},
	"package_install": {
		Name: "package_install", Description: "Package Installation", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\bpip3?\s+install\b`),
			regexp.MustCompile(`\bnpm\s+install\b`),
			regexp.MustCompile(`\bnpm\s+i\b`),
			regexp.MustCompile(`\bapk\s+(add|del)\b`),
			regexp.MustCompile(`\bdoas\s+apk\b`),
			regexp.MustCompile(`\byarn\s+(add|global)\b`),
			regexp.MustCompile(`\bpnpm\s+(add|install)\b`),
			regexp.MustCompile(`\bpip3?\s+uninstall\b`),
			regexp.MustCompile(`\bnpm\s+uninstall\b`),
			regexp.MustCompile(`\bpython[23]?\b.*-m\s+pip\b`),
		},
	},
	"persistence": {
		Name: "persistence", Description: "Persistence Mechanisms", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\bcrontab\b`),
			regexp.MustCompile(`>\s*~/?\.(bashrc|bash_profile|profile|zshrc)`),
			regexp.MustCompile(`\btee\b.*\.(bashrc|bash_profile|profile|zshrc)`),
		},
	},
	"process_control": {
		Name: "process_control", Description: "Process Manipulation", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`\bkill\s+-9\s`),
			regexp.MustCompile(`\b(killall|pkill)\b`),
		},
	},
	"env_dump": {
		Name: "env_dump", Description: "Environment Variable Dumping", Default: true,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`^\s*env\s*$`),
			regexp.MustCompile(`^\s*env\s*\|`),
			regexp.MustCompile(`^\s*env\s*>\s`),
			regexp.MustCompile(`\bprintenv\b`),
			regexp.MustCompile(`^\s*(set|export\s+-p|declare\s+-x)\s*($|\|)`),
			regexp.MustCompile(`\bcompgen\s+-e\b`),
			regexp.MustCompile(`/proc/[^/]+/environ`),
			regexp.MustCompile(`/proc/self/environ`),
			regexp.MustCompile(`(?i)\bstrings\b.*/proc/`),
			regexp.MustCompile(`(?i)\becho\b.*\$\{?QORVEN_(GATEWAY_TOKEN|ENCRYPTION_KEY|POSTGRES_DSN)`),
			regexp.MustCompile(`(?i)\bprintf\b.*\$\{?QORVEN_(GATEWAY_TOKEN|ENCRYPTION_KEY|POSTGRES_DSN)`),
			regexp.MustCompile(`\bpython[23]?\b.*os\.(environ|getenv).*QORVEN_`),
			regexp.MustCompile(`\bnode\b.*-e.*process\.env\.QORVEN_`),
		},
	},
}

// DenyGroupNames returns all registered group names in stable order.
func DenyGroupNames() []string {
	return []string{
		"destructive_ops", "data_exfiltration", "reverse_shell", "code_injection",
		"privilege_escalation", "dangerous_paths", "env_injection", "container_escape",
		"crypto_mining", "filter_bypass", "network_recon", "package_install",
		"persistence", "process_control", "env_dump",
	}
}

// ResolveDenyPatterns merges group defaults with overrides and returns the effective deny patterns.
// overrides: map[groupName]enabled — true=deny, false=allow. nil = all defaults.
func ResolveDenyPatterns(overrides map[string]bool) []*regexp.Regexp {
	var patterns []*regexp.Regexp
	for _, name := range DenyGroupNames() {
		group := DenyGroupRegistry[name]
		enabled := group.Default
		if overrides != nil {
			if v, ok := overrides[name]; ok {
				enabled = v
			}
		}
		if enabled {
			patterns = append(patterns, group.Patterns...)
		}
	}
	return patterns
}

// DefaultDenyPatterns returns all patterns from groups where Default=true.
func DefaultDenyPatterns() []*regexp.Regexp {
	return ResolveDenyPatterns(nil)
}

// IsGroupDenied checks if a specific deny group is active for the given context.
func IsGroupDenied(overrides map[string]bool, group string) bool {
	if overrides != nil {
		if v, ok := overrides[group]; ok {
			return v
		}
	}
	if g, ok := DenyGroupRegistry[group]; ok {
		return g.Default
	}
	return true // unknown group = denied
}

// matchesAny checks if command matches any of the given patterns.
func matchesAny(command string, patterns []*regexp.Regexp) bool {
	for _, p := range patterns {
		if p.MatchString(command) {
			return true
		}
	}
	return false
}
