// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
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
	"4200": true, // Qorven backend
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

	// Block private IP ranges
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return true, fmt.Sprintf("private IP %s blocked", host)
		}
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

	return false, ""
}
