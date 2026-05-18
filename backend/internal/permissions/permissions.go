// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package permissions

// Role defines the agent hierarchy level.
type Role string

const (
	RoleChief      Role = "chief"      // Full access — all tools, all actions, all agents
	RoleDirector   Role = "director"   // Department tools + delegation within department
	RoleSpecialist Role = "specialist" // Minimal — only assigned tools
)

// Permission is a granular capability.
type Permission string

const (
	PermAll             Permission = "*"                  // Chief only
	PermToolExecute     Permission = "tool.execute"       // Can execute tools
	PermToolWeb         Permission = "tool.web"           // Web search/fetch
	PermToolConnector   Permission = "tool.connector"     // External service actions
	PermToolCode        Permission = "tool.code"          // Code execution
	PermToolFile        Permission = "tool.file"          // File operations
	PermToolMCP         Permission = "tool.mcp"           // MCP server tools
	PermAgentDelegate   Permission = "agent.delegate"     // Can delegate to other agents
	PermAgentCreate     Permission = "agent.create"       // Can create new agents
	PermAgentManage     Permission = "agent.manage"       // Can update/delete agents
	PermTaskCreate      Permission = "task.create"        // Can create tasks
	PermTaskAssign      Permission = "task.assign"        // Can assign tasks to agents
	PermDashboardEdit   Permission = "dashboard.edit"     // Can modify dashboard blocks
	PermConnectionManage Permission = "connection.manage" // Can add/remove connections
	PermBudgetView      Permission = "budget.view"        // Can view cost data
	PermBudgetManage    Permission = "budget.manage"      // Can set budgets
	PermAuditView       Permission = "audit.view"         // Can view audit log
)

// DefaultPermissions returns the default permission set for a role.
func DefaultPermissions(role Role) []Permission {
	switch role {
	case RoleChief:
		return []Permission{PermAll}
	case RoleDirector:
		return []Permission{
			PermToolExecute, PermToolWeb, PermToolConnector, PermToolFile,
			PermAgentDelegate, PermTaskCreate, PermTaskAssign,
			PermDashboardEdit, PermBudgetView,
		}
	case RoleSpecialist:
		return []Permission{
			PermToolExecute, PermToolWeb,
			PermTaskCreate,
		}
	default:
		return []Permission{PermToolExecute}
	}
}

// HasPermission checks if a set of permissions includes the required one.
func HasPermission(perms []Permission, required Permission) bool {
	for _, p := range perms {
		if p == PermAll || p == required {
			return true
		}
	}
	return false
}

// HasAny checks if any of the required permissions are present.
func HasAny(perms []Permission, required ...Permission) bool {
	for _, r := range required {
		if HasPermission(perms, r) {
			return true
		}
	}
	return false
}

// FilterTools returns only the tool names the role is allowed to use.
func FilterTools(role Role, perms []Permission, allTools []string) []string {
	if HasPermission(perms, PermAll) {
		return allTools
	}

	allowed := make([]string, 0, len(allTools))
	for _, t := range allTools {
		switch {
		case isWebTool(t) && HasPermission(perms, PermToolWeb):
			allowed = append(allowed, t)
		case isConnectorTool(t) && HasPermission(perms, PermToolConnector):
			allowed = append(allowed, t)
		case isCodeTool(t) && HasPermission(perms, PermToolCode):
			allowed = append(allowed, t)
		case isFileTool(t) && HasPermission(perms, PermToolFile):
			allowed = append(allowed, t)
		case isMCPTool(t) && HasPermission(perms, PermToolMCP):
			allowed = append(allowed, t)
		case HasPermission(perms, PermToolExecute):
			allowed = append(allowed, t)
		}
	}
	return allowed
}

func isWebTool(t string) bool       { return t == "web_search" || t == "web_fetch" }
func isConnectorTool(t string) bool  { return t == "execute_action" }
func isCodeTool(t string) bool       { return t == "exec" || t == "sandbox_run" }
func isFileTool(t string) bool       { return t == "file_read" || t == "file_write" || t == "file_upload" }
func isMCPTool(t string) bool        { return len(t) > 4 && t[:4] == "mcp_" }
