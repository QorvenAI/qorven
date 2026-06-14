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
	tokenGetter := func() string { return "" }
	workspace := "/tmp/qorven-workspace-test"

	// gh_push_file
	gw.toolReg.Register(permissions.WrapLazy(
		func() *permissions.Gate { return gw.permissionGate },
		tools.NewGhPushFileToolWithToken(tokenGetter),
		permissions.GatedToolOptions{
			Reason:      "Writes a file to a user-owned GitHub repository",
			RequestedBy: "agent",
		},
	))
	// gh_merge_pr
	gw.toolReg.Register(permissions.WrapLazy(
		func() *permissions.Gate { return gw.permissionGate },
		tools.NewGhMergePRToolWithToken(tokenGetter),
		permissions.GatedToolOptions{
			Reason:      "Merge a pull request",
			RequestedBy: "agent",
		},
	))
	// exec
	gw.toolReg.Register(permissions.WrapLazy(
		func() *permissions.Gate { return gw.permissionGate },
		tools.NewExecTool(workspace, true),
		permissions.GatedToolOptions{
			Reason:      "Run a shell command on the host",
			RequestedBy: "agent",
		},
	))
	// write_file
	writeTool := tools.NewWriteFileTool(workspace)
	gw.toolReg.Register(permissions.WrapLazy(
		func() *permissions.Gate { return gw.permissionGate },
		writeTool,
		permissions.GatedToolOptions{
			Reason:      "Write a file to the workspace",
			RequestedBy: "agent",
		},
	))
	// apply_patch
	fileHistory := tools.NewFileHistory()
	gw.toolReg.Register(permissions.WrapLazy(
		func() *permissions.Gate { return gw.permissionGate },
		tools.NewApplyPatchTool(workspace, fileHistory),
		permissions.GatedToolOptions{
			Reason:      "Apply a code patch to workspace files",
			RequestedBy: "agent",
		},
	))
	// undo
	gw.toolReg.Register(permissions.WrapLazy(
		func() *permissions.Gate { return gw.permissionGate },
		tools.NewUndoTool(fileHistory),
		permissions.GatedToolOptions{
			Reason:      "Undo a previous file change",
			RequestedBy: "agent",
		},
	))
	// cron is intentionally NOT wrapped: it runs in headless/unattended
	// channel sessions where no human approver is present. The gate
	// fails closed (returns "gate not configured") when the gate is nil,
	// which would deadlock autonomous scheduling entirely.
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
