// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// BlockedPorts are internal service ports that should never be accessed by agents.
var BlockedPorts = map[string]bool{
	"8486": true, // Qorven backend
	"4100": true, // LiteLLM
	"5432": true, // PostgreSQL
	"8881": true, // STT server
	"6379": true, // Redis
	"9090": true, // Prometheus
}

// IsInternalURL checks if a URL points to an internal/private IP address.
// Returns true if the URL should be blocked (SSRF protection).
func IsInternalURL(rawURL string) (bool, string) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false, ""
	}

	host := parsed.Hostname()
	port := parsed.Port()

	// Block localhost variants
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0" {
		return true, "localhost access blocked"
	}

	// Block internal service ports
	if port != "" && BlockedPorts[port] {
		return true, fmt.Sprintf("internal port %s blocked", port)
	}

	// Block metadata endpoints (cloud provider)
	if host == "169.254.169.254" || host == "metadata.google.internal" {
		return true, "cloud metadata endpoint blocked"
	}

	// Block internal hostnames
	lowerHost := strings.ToLower(host)
	internalSuffixes := []string{".internal", ".local", ".localhost", ".svc.cluster.local"}
	for _, suffix := range internalSuffixes {
		if strings.HasSuffix(lowerHost, suffix) {
			return true, fmt.Sprintf("internal hostname %s blocked", host)
		}
	}

	// Literal IP — check the address directly.
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return true, fmt.Sprintf("private IP %s blocked", host)
		}
		return false, ""
	}

	// Hostname — RESOLVE it and check every address. This closes the
	// DNS-rebinding / decoy-domain vector where a public name resolves to an
	// internal/metadata IP. Fail CLOSED on resolution error.
	ips, err := net.LookupIP(host)
	if err != nil {
		return true, fmt.Sprintf("DNS resolution failed for %s — blocked", host)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return true, fmt.Sprintf("hostname %s resolves to blocked IP %s", host, ip)
		}
	}

	return false, ""
}

// isBlockedIP reports whether an IP is loopback/private/link-local or the
// IPv4/IPv6 cloud-metadata address.
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// AWS/GCP/Azure metadata.
	if ip.Equal(net.ParseIP("169.254.169.254")) || ip.Equal(net.ParseIP("fd00:ec2::254")) {
		return true
	}
	return false
}

// SafeNavigateURL rejects non-http(s) schemes and any URL that resolves to an
// internal/private/metadata target. Shared by the browse agent and the
// browser_goto primitive so every navigation entry point is guarded uniformly.
func SafeNavigateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("blocked: only http(s) URLs allowed")
	}
	if blocked, reason := IsInternalURL(raw); blocked {
		return fmt.Errorf("blocked: %s", reason)
	}
	return nil
}
