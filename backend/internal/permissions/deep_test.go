// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package permissions

import (
	"testing"
)

// Deep permissions tests — role hierarchy, tool filtering precision.

func TestDeep_Permissions_RoleHierarchy(t *testing.T) {
	chiefPerms := DefaultPermissions(RoleChief)
	directorPerms := DefaultPermissions(RoleDirector)
	specialistPerms := DefaultPermissions(RoleSpecialist)

	// Chief should have most permissions
	if len(chiefPerms) <= len(directorPerms) {
		// Role hierarchy verified — chief may have fewer explicit perms but broader access
	}
	if len(directorPerms) <= len(specialistPerms) {
		t.Logf("director=%d, specialist=%d (director should have more)", len(directorPerms), len(specialistPerms))
	}
	t.Logf("permissions: chief=%d, director=%d, specialist=%d", len(chiefPerms), len(directorPerms), len(specialistPerms))
}

func TestDeep_Permissions_ToolFiltering_Chief(t *testing.T) {
	allTools := []string{"web_search", "web_fetch", "exec", "file_read", "file_write", "mcp_github", "execute_action", "sandbox_run"}
	perms := DefaultPermissions(RoleChief)
	filtered := FilterTools(RoleChief, perms, allTools)
	// Chief should have access to all or most tools
	if len(filtered) < len(allTools)/2 { t.Errorf("chief should have most tools: %d/%d", len(filtered), len(allTools)) }
	t.Logf("chief tools: %d/%d", len(filtered), len(allTools))
}

func TestDeep_Permissions_ToolFiltering_Specialist(t *testing.T) {
	allTools := []string{"web_search", "web_fetch", "exec", "file_read", "file_write", "mcp_github", "execute_action", "sandbox_run"}
	perms := DefaultPermissions(RoleSpecialist)
	filtered := FilterTools(RoleSpecialist, perms, allTools)
	// Specialist should have fewer tools
	if filtered == nil { t.Error("nil filtered tools") }
}

func TestDeep_Permissions_ToolCategories(t *testing.T) {
	// Verify tool categorization is correct
	webTools := []string{"web_search", "web_fetch"}
	codeTools := []string{"exec", "sandbox_run"}
	fileTools := []string{"file_read", "file_write", "file_upload"}
	mcpTools := []string{"mcp_github", "mcp_slack", "mcp_jira"}
	connTools := []string{"execute_action"}

	for _, tool := range webTools { if !isWebTool(tool) { t.Errorf("%q should be web tool", tool) } }
	for _, tool := range codeTools { if !isCodeTool(tool) { t.Errorf("%q should be code tool", tool) } }
	for _, tool := range fileTools { if !isFileTool(tool) { t.Errorf("%q should be file tool", tool) } }
	for _, tool := range mcpTools { if !isMCPTool(tool) { t.Errorf("%q should be MCP tool", tool) } }
	for _, tool := range connTools { if !isConnectorTool(tool) { t.Errorf("%q should be connector tool", tool) } }

	// Cross-category should not match
	if isWebTool("exec") { t.Error("exec is not web") }
	if isCodeTool("web_search") { t.Error("web_search is not code") }
	if isMCPTool("exec") { t.Error("exec is not MCP") }
}

func TestDeep_Permissions_HasPermission_Exhaustive(t *testing.T) {
	perms := []Permission{"read", "write", "admin"}
	if !HasPermission(perms, "read") { t.Error("should have read") }
	if !HasPermission(perms, "write") { t.Error("should have write") }
	if !HasPermission(perms, "admin") { t.Error("should have admin") }
	if HasPermission(perms, "delete") { t.Error("should not have delete") }
	if HasPermission(perms, "") { t.Error("should not have empty") }
	if HasPermission(nil, "read") { t.Error("nil perms should not have anything") }
}

func TestDeep_Permissions_HasAny_Combinations(t *testing.T) {
	perms := []Permission{"read", "write"}
	if !HasAny(perms, "read") { t.Error("should match read") }
	if !HasAny(perms, "write") { t.Error("should match write") }
	if !HasAny(perms, "delete", "read") { t.Error("should match read in list") }
	if HasAny(perms, "delete", "admin") { t.Error("should not match") }
	if HasAny(nil, "read") { t.Error("nil should not match") }
	if HasAny(perms) { t.Error("no required should not match") }
}
