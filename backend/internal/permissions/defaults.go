package permissions

// DefaultProfile is a (tool, scope) pair used to seed permission_policies
// for a new Qor. Seeded once via LoadDefaults when the Qor first appears
// in a session.
type DefaultProfile struct {
	Tool  string
	Scope PermScope
}

// roleDefaults maps agent_key → default permission profiles.
// Anything not listed here defaults to ScopeAskFirst at runtime.
// Immutable floor: ShellSecurity deny groups always win regardless of these defaults.
var roleDefaults = map[string][]DefaultProfile{
	"general": {
		{Tool: "cron",          Scope: ScopeAutoApproved},
		{Tool: "read_file",     Scope: ScopeAutoApproved},
		{Tool: "list_files",    Scope: ScopeAutoApproved},
		{Tool: "web_search",    Scope: ScopeAutoApproved},
		{Tool: "web_fetch",     Scope: ScopeAutoApproved},
		{Tool: "memory_search", Scope: ScopeAutoApproved},
		{Tool: "write_file",    Scope: ScopeAskFirst},
		{Tool: "exec",          Scope: ScopeAskFirst},
		{Tool: "gh_push_file",  Scope: ScopeAskFirst},
		{Tool: "gh_merge_pr",   Scope: ScopeAskFirst},
	},
	"researcher": {
		{Tool: "web_search",    Scope: ScopeAutoApproved},
		{Tool: "web_fetch",     Scope: ScopeAutoApproved},
		{Tool: "read_file",     Scope: ScopeAutoApproved},
		{Tool: "memory_search", Scope: ScopeAutoApproved},
		{Tool: "cron",          Scope: ScopeAutoApproved},
		{Tool: "exec",          Scope: ScopeBlocked},
		{Tool: "write_file",    Scope: ScopeBlocked},
		{Tool: "gh_push_file",  Scope: ScopeBlocked},
		{Tool: "gh_merge_pr",   Scope: ScopeBlocked},
	},
	"code": {
		{Tool: "read_file",    Scope: ScopeAutoApproved},
		{Tool: "list_files",   Scope: ScopeAutoApproved},
		{Tool: "web_search",   Scope: ScopeAutoApproved},
		{Tool: "write_file",   Scope: ScopeAutoApproved},
		{Tool: "exec",         Scope: ScopeAutoApproved},
		{Tool: "cron",         Scope: ScopeAutoApproved},
		{Tool: "gh_push_file", Scope: ScopeAskFirst},
		{Tool: "gh_merge_pr",  Scope: ScopeAskFirst},
	},
	"support": {
		{Tool: "web_search",    Scope: ScopeAutoApproved},
		{Tool: "web_fetch",     Scope: ScopeAutoApproved},
		{Tool: "memory_search", Scope: ScopeAutoApproved},
		{Tool: "cron",          Scope: ScopeAskFirst},
		{Tool: "exec",          Scope: ScopeBlocked},
		{Tool: "write_file",    Scope: ScopeBlocked},
		{Tool: "gh_push_file",  Scope: ScopeBlocked},
		{Tool: "gh_merge_pr",   Scope: ScopeBlocked},
	},
	"prime": {
		{Tool: "read_file",     Scope: ScopeAutoApproved},
		{Tool: "list_files",    Scope: ScopeAutoApproved},
		{Tool: "web_search",    Scope: ScopeAutoApproved},
		{Tool: "web_fetch",     Scope: ScopeAutoApproved},
		{Tool: "memory_search", Scope: ScopeAutoApproved},
		{Tool: "cron",          Scope: ScopeAutoApproved},
		{Tool: "write_file",    Scope: ScopeAutoApproved},
		{Tool: "exec",          Scope: ScopeAutoApproved},
		{Tool: "gh_push_file",  Scope: ScopeAskFirst},
		{Tool: "gh_merge_pr",   Scope: ScopeAskFirst},
	},
}

// DefaultsForRole returns the default permission profiles for the given
// agent_key. Falls back to "general" when the key is not recognised.
func DefaultsForRole(agentKey string) []DefaultProfile {
	if defs, ok := roleDefaults[agentKey]; ok {
		return defs
	}
	return roleDefaults["general"]
}
