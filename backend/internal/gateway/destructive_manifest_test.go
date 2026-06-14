// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/tools"
)

// TestDestructiveManifest_AllWrapped is the CI enforcement point for
// P3E-03 and the Phase 3 operational-standard "destructive actions
// must route through the permission gate."
//
// Contract: every name in tools.DestructiveTools MUST be registered
// via the PRODUCTION gw.registerDestructiveTools path, AND every
// non-exempt tool MUST be wrapped (permissions.IsGated reports true).
//
// This test calls gw.registerDestructiveTools directly — the same
// method registerTools() calls during normal gateway boot — so any
// un-wrapping in the real registration code fails this test. The old
// hand-maintained registerDestructiveToolsForTest mirror has been
// removed; there is now a single source of truth.
//
// cron is the documented exempt tool: it is in the manifest but
// intentionally NOT wrapped (see registerDestructiveTools for the
// rationale). The test asserts this exception explicitly so it cannot
// be silently removed without making the test fail.
func TestDestructiveManifest_AllWrapped(t *testing.T) {
	gw, _, _ := newMinimalGateway(t, MinimalGatewayOpts{})
	gw.toolReg = tools.NewRegistry()

	// Call the PRODUCTION registration method — not a mirror.
	gw.registerDestructiveTools(
		gw.toolReg,
		"/tmp/qorven-workspace-test",
		"/tmp",
		func() string { return "" },
		tools.NewFileHistory(),
	)

	if gw.toolReg == nil {
		t.Fatalf("tool registry is nil; destructive manifest cannot be enforced")
	}

	// cron is the only manifest tool intentionally exempt from the gate.
	// Assert the exception is still present in the manifest and still bare.
	const cronExempt = "cron"
	if _, ok := tools.DestructiveTools[cronExempt]; !ok {
		t.Fatalf("cron is no longer in DestructiveTools — update this test to reflect the new manifest")
	}
	cronTool, cronRegistered := gw.toolReg.Get(cronExempt)
	if cronRegistered && permissions.IsGated(cronTool) {
		t.Errorf("cron is now wrapped — if intentional, remove the explicit exemption from this test and update registerDestructiveTools")
	}

	var failures []string
	for name, reason := range tools.DestructiveTools {
		if name == cronExempt {
			continue // documented exception above
		}
		tool, ok := gw.toolReg.Get(name)
		if !ok {
			failures = append(failures,
				name+" — "+reason.Description+" — NOT REGISTERED (add it to registerDestructiveTools)")
			continue
		}
		if !permissions.IsGated(tool) {
			failures = append(failures,
				name+" — "+reason.Description+" — registered WITHOUT the permission gate wrapper")
		}
	}
	if len(failures) > 0 {
		t.Fatalf(
			"destructive-tool manifest violation (%d tool(s)):\n  - %s\n\n"+
				"Fix: in gateway_tools.go registerDestructiveTools, ensure every non-exempt tool "+
				"is registered with permissions.WrapLazy(gateGetter, inner, GatedToolOptions{...}).",
			len(failures), strings.Join(failures, "\n  - "),
		)
	}
}

// TestDestructiveManifest_DetectsRegressions proves the check itself
// actually fails when an unwrapped destructive tool is registered.
// Without this, a silently-broken IsGated check could pass the main
// test even when tools are unwrapped.
func TestDestructiveManifest_DetectsRegressions(t *testing.T) {
	gw, _, _ := newMinimalGateway(t, MinimalGatewayOpts{})

	// Register a tool whose Name() is in the destructive manifest,
	// bypassing the gate. The check MUST flag it.
	gw.toolReg = tools.NewRegistry()
	gw.toolReg.Register(&stubBareTool{name: "gh_push_file"})

	// Simulate what TestDestructiveManifest_AllWrapped does:
	var failures []string
	for name := range tools.DestructiveTools {
		tool, ok := gw.toolReg.Get(name)
		if !ok {
			continue
		}
		if !permissions.IsGated(tool) {
			failures = append(failures, name)
		}
	}
	if len(failures) == 0 {
		t.Fatalf("regression: manifest check passed when gh_push_file was registered without the gate")
	}
}

// TestDestructiveManifest_AcceptsWrapped proves a wrapped tool passes
// the check (positive case).
func TestDestructiveManifest_AcceptsWrapped(t *testing.T) {
	gw, _, _ := newMinimalGateway(t, MinimalGatewayOpts{})

	gw.toolReg = tools.NewRegistry()
	inner := &stubBareTool{name: "gh_push_file"}
	wrapped := permissions.WrapLazy(
		func() *permissions.Gate { return gw.permissionGate },
		inner,
		permissions.GatedToolOptions{Reason: "test"},
	)
	gw.toolReg.Register(wrapped)

	tool, ok := gw.toolReg.Get("gh_push_file")
	if !ok {
		t.Fatalf("expected tool registered")
	}
	if !permissions.IsGated(tool) {
		t.Fatalf("wrapped tool should report IsGated=true")
	}
}

// stubBareTool is an unwrapped Tool used for the regression test.
type stubBareTool struct {
	name string
}

func (s *stubBareTool) Name() string                            { return s.name }
func (s *stubBareTool) Description() string                     { return "stub" }
func (s *stubBareTool) Parameters() map[string]any              { return map[string]any{} }
func (s *stubBareTool) Execute(_ context.Context, _ map[string]any) *tools.Result {
	return tools.TextResult("stub")
}
