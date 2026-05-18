// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

// ReservedCoreToolNames is the closed set of platform tool names that
// tenant-uploaded Wasm plugins may NOT register under. The guard
// exists because the agent loop's tool dispatcher (agent.executeTool)
// applies shadow semantics — if a plugin and a built-in share a name,
// the plugin wins for that request. Left unchecked, a tenant could
// upload a plugin named "exec" or "gh_push_file" and intercept
// destructive actions the platform's permission gate was built to
// protect.
//
// ## What belongs here
//
// Two categories of names are reserved:
//
//   1. Every name in DestructiveTools. A destructive built-in is
//      gated by permissions.WrapLazy; a shadowing plugin would not
//      inherit that wrapping (plugin wrapping is the loader's own
//      gate). Shadow semantics would therefore silently DOWNGRADE
//      the permission posture of a destructive action.
//
//   2. Core orchestration primitives whose wire shape the LLM
//      relies on: room_post, room_list, memory_search, session_*,
//      etc. An AI-generated plugin with a colliding name could
//      poison the plan graph by returning malformed JSON the
//      orchestrator then tries to parse.
//
// ## What does NOT belong here
//
// Read-only tools that do not mutate state (gh_read_issue,
// web_search, list_files) are omitted by design — a tenant who
// writes a better search implementation SHOULD be able to shadow
// the default. The security risk is shadowing *mutations* and
// *orchestration*, not *queries*.
//
// ## Adding a name
//
// Edit this slice directly. The Store.Upload path rejects at write
// time; the Loader rejects at load time (belt + braces). A name
// added after tenants have uploaded a matching plugin will start
// failing those plugins on next load — the operator must revoke +
// rename. Document the addition in the commit message so operators
// auditing plugin health have context.
//
// See also: TestReservedManifest_IncludesDestructiveTools —
// CI-enforced invariant that every destructive tool is reserved.
var ReservedCoreToolNames = map[string]struct{}{
	// Destructive built-ins (mirror DestructiveTools; unit test
	// enforces that these two lists stay in sync).
	"gh_push_file": {},
	"gh_merge_pr":  {},
	"exec":         {},
	"apply_patch":  {},
	"write_file":   {},
	"undo":         {},
	"cron":         {},

	// Core orchestration primitives. A shadow here could poison the
	// plan graph or room message bus.
	"room_post":      {},
	"room_list":      {},
	"room_decide":    {},
	"room_assign":    {},
	"join_room":      {},
	"leave_room":     {},
	"memory_search":  {},
	"memory_get":     {},
	"kg_search":      {},
	"sessions_list":  {},
	"session_status": {},
	"delegate":       {},
	"spawn":          {},
	"clarify":                 {},
	"ask_followup_question":  {},
	"produce_project_brief":  {},
}

// IsReservedCoreToolName reports whether the given name is in the
// reserved set. The plugin store and loader call this before any
// registration; the HTTP upload handler calls it before writing the
// row so tenants get a 400 with a clear code instead of a silent
// skip.
func IsReservedCoreToolName(name string) bool {
	_, ok := ReservedCoreToolNames[name]
	return ok
}
