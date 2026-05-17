// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools_test

import (
	"testing"

	"github.com/qorvenai/qorven/internal/tools"
)

// TestReservedManifest_IncludesDestructiveTools is the CI-enforced
// invariant: every destructive tool MUST also be reserved. If a new
// destructive tool is added to the manifest without a matching
// reservation, tenant plugins could register under the same name
// and SHADOW the destructive built-in — bypassing its permission
// gate.
//
// This test is intentionally narrow: it does not enforce the
// opposite (not every reserved name must be destructive). Core
// orchestration primitives like `room_post` are reserved but are
// not themselves destructive — they just must not be shadowed.
func TestReservedManifest_IncludesDestructiveTools(t *testing.T) {
	for name, reason := range tools.DestructiveTools {
		if !tools.IsReservedCoreToolName(name) {
			t.Errorf("destructive tool %q (%s) is NOT reserved — tenant plugins could shadow it",
				name, reason.Description)
		}
	}
}

// TestIsReservedCoreToolName_SpotChecks is a readable tripwire for
// the most likely hijack targets. If a reviewer accidentally deletes
// an entry from the list, this flags exactly what disappeared.
func TestIsReservedCoreToolName_SpotChecks(t *testing.T) {
	mustBeReserved := []string{
		// destructive
		"exec", "apply_patch", "write_file",
		"gh_push_file", "gh_merge_pr", "undo",
		// orchestration
		"room_post", "memory_search", "sessions_list", "spawn",
	}
	for _, name := range mustBeReserved {
		if !tools.IsReservedCoreToolName(name) {
			t.Errorf("%q dropped from reserved list — tenant plugins can now hijack it", name)
		}
	}
}

// TestIsReservedCoreToolName_AllowsOrdinaryNames — the gate must
// not accidentally reserve every name. Tenants with legitimate
// plugin names should pass through.
func TestIsReservedCoreToolName_AllowsOrdinaryNames(t *testing.T) {
	ordinary := []string{
		"my_custom_tool",
		"echo",
		"stripe_refund",
		"internal_helper",
	}
	for _, name := range ordinary {
		if tools.IsReservedCoreToolName(name) {
			t.Errorf("ordinary name %q is reserved — false positive blocks legitimate uploads", name)
		}
	}
}
