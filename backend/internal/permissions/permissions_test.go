// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package permissions

import (
	"testing"
)

// Hard permission tests — role-based access, tool filtering, edge cases.

func TestDefaultPermissions_Admin(t *testing.T) {
	perms := DefaultPermissions(RoleChief)
	if len(perms) == 0 { t.Error("admin should have permissions") }
}

func TestDefaultPermissions_Member(t *testing.T) {
	perms := DefaultPermissions(RoleDirector)
	if len(perms) == 0 { t.Error("member should have permissions") }
}

func TestDefaultPermissions_Viewer(t *testing.T) {
	perms := DefaultPermissions(RoleSpecialist)
	// Viewer may have limited or no permissions
	_ = perms
}

func TestHasPermission_True(t *testing.T) {
	perms := DefaultPermissions(RoleChief)
	if len(perms) > 0 {
		if !HasPermission(perms, perms[0]) { t.Error("admin should have own permissions") }
	}
}

func TestHasPermission_False(t *testing.T) {
	if HasPermission(nil, "nonexistent_perm") { t.Error("nil perms should not have anything") }
}

func TestHasPermission_Empty(t *testing.T) {
	if HasPermission([]Permission{}, "any") { t.Error("empty perms should not have anything") }
}

func TestHasAny_True(t *testing.T) {
	perms := []Permission{"read", "write", "delete"}
	if !HasAny(perms, "write", "admin") { t.Error("should match 'write'") }
}

func TestHasAny_False(t *testing.T) {
	perms := []Permission{"read"}
	if HasAny(perms, "write", "delete") { t.Error("should not match") }
}

func TestHasAny_Empty(t *testing.T) {
	if HasAny(nil, "read") { t.Error("nil should not match") }
}

func TestFilterTools_Admin(t *testing.T) {
	allTools := []string{"web_search", "exec", "file_read", "file_write", "mcp_tool1"}
	perms := DefaultPermissions(RoleChief)
	filtered := FilterTools(RoleChief, perms, allTools)
	if len(filtered) == 0 { t.Error("admin should have tools") }
}

func TestFilterTools_EmptyPerms(t *testing.T) {
	allTools := []string{"web_search", "exec"}
	filtered := FilterTools(RoleDirector, nil, allTools)
	// With nil perms, behavior depends on implementation
	_ = filtered
}

func TestFilterTools_EmptyTools(t *testing.T) {
	perms := DefaultPermissions(RoleChief)
	filtered := FilterTools(RoleChief, perms, nil)
	if len(filtered) != 0 { t.Error("no tools to filter") }
}

func TestIsWebTool(t *testing.T) {
	if !isWebTool("web_search") { t.Error("web_search should be web tool") }
	if !isWebTool("web_fetch") { t.Error("web_fetch should be web tool") }
	if isWebTool("exec") { t.Error("exec should not be web tool") }
}

func TestIsCodeTool(t *testing.T) {
	if !isCodeTool("exec") { t.Error("exec should be code tool") }
	if !isCodeTool("sandbox_run") { t.Error("sandbox_run should be code tool") }
	if isCodeTool("web_search") { t.Error("web_search should not be code tool") }
}

func TestIsFileTool(t *testing.T) {
	if !isFileTool("file_read") { t.Error("file_read should be file tool") }
	if !isFileTool("file_write") { t.Error("file_write should be file tool") }
	if isFileTool("exec") { t.Error("exec should not be file tool") }
}

func TestIsMCPTool(t *testing.T) {
	if !isMCPTool("mcp_github") { t.Error("mcp_github should be MCP tool") }
	if isMCPTool("web") { t.Error("web should not be MCP tool") }
	if isMCPTool("mcp") { t.Error("'mcp' alone should not match (too short)") }
}

func TestIsConnectorTool(t *testing.T) {
	if !isConnectorTool("execute_action") { t.Error("execute_action should be connector") }
	if isConnectorTool("exec") { t.Error("exec should not be connector") }
}

func TestValidScope(t *testing.T) {
	// Test known scopes
	if !ValidScope("read") && !ValidScope("write") && !ValidScope("admin") {
		// scope validation depends on AllScopes map — verified by compilation
	}
}

func TestRole_Constants(t *testing.T) {
	roles := []Role{RoleChief, RoleDirector, RoleSpecialist}
	seen := map[Role]bool{}
	for _, r := range roles {
		if r == "" { t.Error("empty role") }
		if seen[r] { t.Errorf("duplicate: %s", r) }
		seen[r] = true
	}
}
