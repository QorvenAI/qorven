// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"encoding/json"
)

// AgentRole defines structural execution constraints for a class of agents.
// Unlike per-agent system prompts (personality), roles define hard boundaries:
// which tools are reachable, how many iterations are allowed, and which
// prompt mode drives the system prompt assembly. These constraints are applied
// before and after every LLM call — the LLM cannot negotiate or override them.
//
// The separation is intentional:
//   - Agent identity (system_prompt in DB) = who the agent is, its tone and domain
//   - Agent role (AgentRole) = what the agent can structurally do
//
// When a new agent type is created, assigning a role is all that's needed to
// enforce its operational boundaries. No bespoke prompt debugging required.
type AgentRole struct {
	// AgentKey is the agent_key value this role applies to (e.g. "prime").
	AgentKey string

	// ToolsAllowed is the exclusive set of tools available to this role.
	// nil means unrestricted (all tools from the tool registry).
	// Takes precedence over the per-agent tools_allowed from DB.
	ToolsAllowed []string

	// ToolsDenied is always applied on top of ToolsAllowed.
	// Used to block specific tools when ToolsAllowed is nil.
	ToolsDenied []string

	// MaxIterations caps the think→act→observe loop for this role.
	// 0 means use the agent's configured MaxToolIterations.
	MaxIterations int

	// PromptMode controls which system prompt sections are assembled.
	// Zero value means use the default channel-derived mode.
	PromptMode PromptMode
}

// roleRegistry maps agent_key → AgentRole.
// Built-in roles are registered here; future user-defined roles will extend this.
var roleRegistry = map[string]AgentRole{
	// intake_planner: gathers project requirements, never writes code.
	// Structural enforcement: only ask_followup_question and produce_project_brief
	// are reachable — not via prompt instruction but via tool filter.
	"prime": {
		AgentKey:      "prime",
		ToolsAllowed:  []string{"ask_followup_question", "produce_project_brief"},
		MaxIterations: 5,
		PromptMode:    PromptIntake,
	},

	// code_specialist: full coding environment, blocked from social/workspace tools.
	"code": {
		AgentKey:    "code",
		ToolsDenied: []string{"qorven_social", "workspace_builder", "produce_project_brief", "ask_followup_question"},
		MaxIterations: 25,
	},

	// researcher: web-focused, no code execution, no file writes.
	"researcher": {
		AgentKey:    "researcher",
		ToolsDenied: []string{"code_exec", "shell_exec", "write_file", "delete_file"},
		MaxIterations: 12,
	},

	// support: customer-facing, tight tool surface.
	"support": {
		AgentKey:     "support",
		ToolsAllowed: []string{"knowledge_search", "ticket_create", "escalate", "send_message"},
		MaxIterations: 8,
		PromptMode:   PromptChannel,
	},

	// general: no structural restrictions beyond the agent's own DB config.
	// Registered so callers can detect "known general role" vs unrecognised key.
	"general": {
		AgentKey:      "general",
		MaxIterations: 20,
	},
}

// ResolveRole returns the AgentRole for the given agent_key, if one exists.
// Returns (role, true) when found, (zero, false) when not found.
func ResolveRole(agentKey string) (AgentRole, bool) {
	r, ok := roleRegistry[agentKey]
	return r, ok
}

// ApplyRole enforces role constraints onto the agent before the execution loop
// processes the request. It mutates ag in-place:
//
//   - Overrides ToolsAllowed/ToolsDenied with the role's values (role wins).
//   - Caps MaxToolIterations at the role's limit when the role sets one.
//   - Returns the PromptMode the loop should use (zero if role doesn't set one).
//
// This is called once per request, after the agent is loaded from the DB and
// before BuildToolDefs/BuildSystemPrompt run. The LLM never sees this logic.
func ApplyRole(ag *Agent, role AgentRole) PromptMode {
	if len(role.ToolsAllowed) > 0 {
		b, _ := json.Marshal(role.ToolsAllowed)
		ag.ToolsAllowed = b
		// When we have an explicit allowlist, clear deny-list: the allowlist
		// is the complete surface; no separate deny needed.
		ag.ToolsDenied = nil
	}

	// Apply deny-list on top (when no allowlist, or when caller stacked both).
	if len(role.ToolsDenied) > 0 && len(role.ToolsAllowed) == 0 {
		b, _ := json.Marshal(role.ToolsDenied)
		ag.ToolsDenied = b
	}

	if role.MaxIterations > 0 && (ag.MaxToolIterations <= 0 || role.MaxIterations < ag.MaxToolIterations) {
		ag.MaxToolIterations = role.MaxIterations
	}

	return role.PromptMode
}
