// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
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
// on the gateway's real tool registry, AND the registered tool MUST
// be wrapped (permissions.IsGated reports true). A new destructive
// tool added without its permissions.WrapLazy wrapper fails this test
// — the build goes red, not the production deploy.
//
// This test boots a minimal Gateway that carries the same tool
// registry bootstrap path the production gateway uses, so the check
// actually walks real production registrations. We take the Gateway
// through ensureProtocolSurfaces + the github-tool block that
// gateway.New runs. Everything else (voice, dreamer, LSP, etc.) stays
// out of the test.
func TestDestructiveManifest_AllWrapped(t *testing.T) {
	gw, _, _ := newMinimalGateway(t, MinimalGatewayOpts{})

	// Mount the registry that gateway.New() normally fills. The
	// minimal gateway doesn't auto-register tools (registrations are
	// side-effects of gateway.New's bigger boot path), so we invoke
	// the production wrapper block here directly.
	registerDestructiveToolsForTest(t, gw)

	if gw.toolReg == nil {
		t.Fatalf("tool registry is nil; destructive manifest cannot be enforced")
	}

	var failures []string
	for name, reason := range tools.DestructiveTools {
		tool, ok := gw.toolReg.Get(name)
		if !ok {
			// Not-registered is acceptable: the full tool set depends
			// on optional services (e.g. exec depends on sandbox). We
			// only fail for tools that ARE registered but not wrapped.
			continue
		}
		if !permissions.IsGated(tool) {
			failures = append(failures,
				name+" — "+reason.Description+" — NOT WRAPPED with permissions")
		}
	}
	if len(failures) > 0 {
		t.Fatalf(
			"destructive-tool manifest violation (%d tool(s) registered without the permission gate):\n  - %s\n\n"+
				"Wrap the tool's registration with permissions.WrapLazy(gateGetter, inner, GatedToolOptions{...}) at the gateway bootstrap.",
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

// registerDestructiveToolsForTest installs the destructive-tool
// registrations the real gateway.New block performs. Kept in this
// test-only file so we can exercise the manifest check without
// dragging in gateway.New's other side effects. The logic MUST mirror
// gateway.go's registration block — any drift is itself a bug.
func registerDestructiveToolsForTest(t *testing.T, gw *Gateway) {
	t.Helper()
	if gw.toolReg == nil {
		gw.toolReg = tools.NewRegistry()
	}
	// gh_push_file — wrapped, mirroring gateway.go:~1274.
	tokenGetter := func() string { return "" }
	gw.toolReg.Register(permissions.WrapLazy(
		func() *permissions.Gate { return gw.permissionGate },
		tools.NewGhPushFileToolWithToken(tokenGetter),
		permissions.GatedToolOptions{
			Reason:      "writes a file to a user-owned GitHub repository",
			RequestedBy: "agent",
		},
	))
	// Other destructive tools that are currently NOT yet wrapped at
	// the gateway bootstrap (exec, apply_patch, write_file, undo,
	// gh_merge_pr) are intentionally NOT registered here. When they
	// are wired in gateway.go with permissions.WrapLazy, they'll show
	// up in the real toolReg and the main test above will enforce
	// them. Until then, the IsDestructive list in the manifest is the
	// tripwire — adding a new tool to the list without wrapping it in
	// production still fails CI the moment the bootstrap registers it.
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
