// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent_test

import (
	"testing"

	"github.com/qorvenai/qorven/internal/agent"
)

func TestShellSecurity_DenyGroups_StillBlock(t *testing.T) {
	s := agent.NewShellSecurity()

	// Deny group: system destruction
	r := s.CheckCommand("rm -rf /")
	if r.Allowed {
		t.Fatal("rm -rf / should be blocked by deny groups")
	}

	// Deny group: credential theft
	r = s.CheckCommand("cat /etc/passwd")
	if r.Allowed {
		t.Fatal("cat /etc/passwd should be blocked by deny groups")
	}

	// Safe bin: should be allowed
	r = s.CheckCommand("git status")
	if !r.Allowed {
		t.Fatalf("git status should be allowed, got blocked with: %s", r.Reason)
	}

	// AskMode off means unknown binaries pass through (not "ask")
	if s.AskMode != "off" {
		t.Fatalf("AskMode should be off, got: %s", s.AskMode)
	}
}
