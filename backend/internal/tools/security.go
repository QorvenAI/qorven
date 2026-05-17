// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// --- Path Security (defends against CVE-2026-32013, CVE-2026-32060, CVE-2026-32061, CVE-2026-26321, CVE-2026-28457) ---

// SafePath resolves a path within a workspace, preventing traversal and symlink escape.
// Returns the cleaned absolute path or an error.
func SafePath(workspace, requestedPath string) (string, error) {
	if workspace == "" {
		return "", fmt.Errorf("no workspace configured")
	}
	workspace, _ = filepath.Abs(workspace)

	// URL-decode the path to prevent %2e%2e%2f traversal
	if decoded, err := url.PathUnescape(requestedPath); err == nil {
		requestedPath = decoded
	}

	// Block tilde expansion — prevents ~/.ssh/id_rsa access
	if strings.HasPrefix(requestedPath, "~") {
		return "", fmt.Errorf("path outside workspace: tilde paths not allowed")
	}

	// Resolve the requested path
	var abs string
	if filepath.IsAbs(requestedPath) {
		abs = filepath.Clean(requestedPath)
	} else {
		abs = filepath.Clean(filepath.Join(workspace, requestedPath))
	}

	// Check relative path doesn't escape (catches ".." tricks)
	rel, err := filepath.Rel(workspace, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path outside workspace: %s", requestedPath)
	}

	// Resolve symlinks and verify the REAL path is still inside workspace
	// (defense against CVE-2026-32013 symlink attack)
	real, err := filepath.EvalSymlinks(abs)
	if err == nil {
		// File exists — verify resolved path
		realRel, err := filepath.Rel(workspace, real)
		if err != nil || strings.HasPrefix(realRel, "..") {
			return "", fmt.Errorf("symlink escapes workspace: %s → %s", requestedPath, real)
		}
		return real, nil
	}

	// File doesn't exist yet (write_file) — verify parent exists and is inside workspace
	parent := filepath.Dir(abs)
	if realParent, err := filepath.EvalSymlinks(parent); err == nil {
		parentRel, err := filepath.Rel(workspace, realParent)
		if err != nil || strings.HasPrefix(parentRel, "..") {
			return "", fmt.Errorf("parent symlink escapes workspace: %s", requestedPath)
		}
	}

	return abs, nil
}

// SafeSlug validates a slug (skill name, tool name) — defense against CVE-2026-28457.
var safeSlugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*[a-z0-9]$`)

func IsSafeSlug(s string) bool {
	return len(s) >= 2 && len(s) <= 100 && safeSlugRe.MatchString(s)
}

// --- Shell Security (defends against CVE-2026-24763, CVE-2026-32032, CVE-2026-25157) ---

// ShellDenyPatterns blocks dangerous shell commands.
// Matches Qorven CVEs: command injection, reverse shells, eval attacks.
var ShellDenyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)curl\s.*\|\s*(ba)?sh`),           // curl|sh
	regexp.MustCompile(`(?i)wget\s.*\|\s*(ba)?sh`),           // wget|sh
	regexp.MustCompile(`(?i)(bash|sh|zsh)\s+-[ci]\s+`),       // sh -c (direct shell)
	regexp.MustCompile(`(?i)eval\s*\$\(`),                     // eval $(...)
	regexp.MustCompile(`(?i)base64\s.*\|\s*(ba)?sh`),          // base64|sh
	regexp.MustCompile(`(?i)/dev/tcp/`),                       // bash reverse shell
	regexp.MustCompile(`(?i)nc\s+-[elp]`),                     // netcat listener
	regexp.MustCompile(`(?i)mkfifo\s+/tmp/`),                  // named pipe reverse shell
	regexp.MustCompile(`(?i)python[23]?\s+-c\s+.*socket`),     // python reverse shell
	regexp.MustCompile(`(?i)perl\s+-e\s+.*socket`),            // perl reverse shell
	regexp.MustCompile(`(?i)ruby\s+-e\s+.*TCPSocket`),         // ruby reverse shell
	regexp.MustCompile(`(?i)rm\s+-rf\s+/`),                    // rm -rf /
	regexp.MustCompile(`(?i)chmod\s+[0-7]*777`),               // chmod 777
	regexp.MustCompile(`(?i)>\s*/etc/`),                       // write to /etc
	regexp.MustCompile(`(?i)dd\s+if=.*of=/dev/`),              // dd to device
	regexp.MustCompile(`(?i):\(\)\{.*:\|:&.*\};:`),              // fork bomb
	regexp.MustCompile(`(?i)\bsudo\b`),                        // sudo
	regexp.MustCompile(`(?i)\bsu\s+-?\s*root`),                // su root
}

// IsShellDenied checks if a command matches any deny pattern.
func IsShellDenied(cmd string) (bool, string) {
	for _, p := range ShellDenyPatterns {
		if p.MatchString(cmd) {
			return true, p.String()
		}
	}
	return false, ""
}

// SafeShell is the hardcoded shell binary — NEVER use os.Getenv("SHELL").
// Defense against CVE-2026-32032 (malicious SHELL env var).
const SafeShell = "/bin/sh"

// --- SSRF Protection (defense against server-side request forgery) ---

// IsPrivateIP checks if an IP is private/reserved (blocks SSRF to internal services).
func IsPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	privateRanges := []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"127.0.0.0/8", "169.254.0.0/16", // link-local + metadata
		"::1/128", "fc00::/7", "fe80::/10",
	}
	for _, cidr := range privateRanges {
		_, network, _ := net.ParseCIDR(cidr)
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateURL checks a URL is safe to fetch (no SSRF, no file://, etc.).
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("blocked scheme: %s (only http/https allowed)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty hostname")
	}
	// Block metadata endpoints (AWS, GCP, Azure)
	blocked := []string{"metadata.google.internal", "metadata.google", "169.254.169.254"}
	for _, b := range blocked {
		if strings.EqualFold(host, b) {
			return fmt.Errorf("blocked host: %s", host)
		}
	}
	// Block internal service ports
	port := u.Port()
	if port != "" && BlockedPorts[port] {
		return fmt.Errorf("blocked: internal service port %s", port)
	}
	// DNS resolve and check for private IPs
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed: %s", host)
	}
	for _, ip := range ips {
		if IsPrivateIP(ip) {
			return fmt.Errorf("blocked: %s resolves to private IP %s", host, ip)
		}
	}
	return nil
}

// --- Output Truncation ---

const MaxToolOutput = 4000 // 4KB — prevents context bloat (was 60KB) // 60KB — matches Qorven's limit

func TruncateOutput(s string, max int) string {
	if len(s) <= max { return s }
	return s[:max] + fmt.Sprintf("\n\n[truncated — %d bytes total, showing first %d]", len(s), max)
}

// --- File Size Check ---

func FileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil { return 0, err }
	return info.Size(), nil
}

// --- Workspace Quota ---

// CheckQuota verifies a write won't exceed file or workspace limits.
func CheckQuota(workspace string, writeSize int64) error {
	if writeSize > MaxFileSize {
		return fmt.Errorf("file too large: %d bytes exceeds %d MB limit", writeSize, MaxFileSize/(1024*1024))
	}
	total, err := dirSize(workspace)
	if err != nil { return nil } // fail open if can't check
	if total+writeSize > MaxWorkspaceSize {
		return fmt.Errorf("workspace quota exceeded: %d MB used, %d MB limit", total/(1024*1024), MaxWorkspaceSize/(1024*1024))
	}
	return nil
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil { return nil } // skip errors
		if !info.IsDir() { size += info.Size() }
		return nil
	})
	return size, err
}
