// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import "testing"

func TestSafeNavigateURL_BlocksInternal(t *testing.T) {
	blocked := []string{
		"http://169.254.169.254/",
		"http://localhost:8486/",
		"http://127.0.0.1/",
		"http://10.0.0.1/",
		"http://192.168.1.1/",
		"http://172.16.0.1/",
		"http://0.0.0.0/",
		"http://::1/",
		"file:///etc/passwd",
		"ftp://example.com/",
	}
	for _, bad := range blocked {
		if err := SafeNavigateURL(bad); err == nil {
			t.Errorf("SafeNavigateURL(%q) should have been blocked but was allowed", bad)
		}
	}
}

func TestSafeNavigateURL_AllowsExternal(t *testing.T) {
	// Use a literal public IP to avoid DNS lookup flakiness in CI.
	// 93.184.216.34 = example.com (IANA-assigned, publicly routable).
	allowed := []string{
		"http://93.184.216.34/",
		"https://93.184.216.34/",
	}
	for _, good := range allowed {
		if err := SafeNavigateURL(good); err != nil {
			t.Errorf("SafeNavigateURL(%q) should have been allowed but was blocked: %v", good, err)
		}
	}
}

func TestIsInternalURL_BlocksMetadata(t *testing.T) {
	blocked := []struct {
		url    string
		reason string
	}{
		{"http://169.254.169.254/latest/meta-data/", "AWS metadata"},
		{"http://metadata.google.internal/computeMetadata/v1/", "GCP metadata"},
		{"http://localhost/", "localhost"},
		{"http://127.0.0.1:5432/", "loopback IP"},
	}
	for _, tc := range blocked {
		ok, reason := IsInternalURL(tc.url)
		if !ok {
			t.Errorf("IsInternalURL(%q) should be blocked (%s) but got allowed", tc.url, tc.reason)
		}
		if reason == "" {
			t.Errorf("IsInternalURL(%q) returned empty reason", tc.url)
		}
	}
}

func TestSafeNavigateURL_BlockedPorts(t *testing.T) {
	// Internal service ports should be blocked regardless of host.
	for port := range BlockedPorts {
		url := "http://93.184.216.34:" + port + "/"
		if err := SafeNavigateURL(url); err == nil {
			t.Errorf("SafeNavigateURL with port %s should be blocked", port)
		}
	}
}
