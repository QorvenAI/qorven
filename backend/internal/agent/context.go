// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"encoding/json"
	"context"
	"fmt"
	"strings"

	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/skills"
	"github.com/qorvenai/qorven/internal/tools"
)

// ContextBuilder assembles the full LLM context from multiple sources.
// Order matters: system prompt → SOUL.md → bulletin → skills → tool summary → history → memories.
type ContextBuilder struct {
	agent        *Agent
	skillLoader  *skills.Loader
	skillStore   *skills.Store
	memStore     *memory.Store
	toolReg      *tools.Registry
	teamRoster   []*Agent
	runtime      RuntimeContext
	memResults   []string
	learnedHints string

	// extraTools is the per-run list injected via RunRequest.ExtraTools.
	// Populated by Loop.Run before BuildToolDefs runs. this
	// is how tenant Wasm plugins reach the LLM's tool menu.
	extraTools []tools.Tool
}

func NewContextBuilder(agent *Agent, skillLoader *skills.Loader, memStore *memory.Store, toolReg *tools.Registry) *ContextBuilder {
	return &ContextBuilder{agent: agent, skillLoader: skillLoader, memStore: memStore, toolReg: toolReg}
}

// SetLearnedHints sets dynamic hints from the learning loop.
func (cb *ContextBuilder) SetLearnedHints(hints string) { cb.learnedHints = hints }

// SetTeamRoster sets the list of other agents for team section.
func (cb *ContextBuilder) SetTeamRoster(agents []*Agent) {
	cb.teamRoster = agents
}

// SetSkillStore sets the DB skill store for loading installed marketplace skills.
func (cb *ContextBuilder) SetSkillStore(store *skills.Store) {
	cb.skillStore = store
}

// SetRuntime sets the per-request runtime context.
func (cb *ContextBuilder) SetRuntime(r RuntimeContext) {
	cb.runtime = r
}

// SetMemoryResults passes pre-fetched memories to the PromptBuilder.
func (cb *ContextBuilder) SetMemoryResults(m []string) {
	cb.memResults = m
}

// SetExtraTools registers per-request tools that augment the global
// registry for this invocation only. Used by Loop.Run to propagate
// RunRequest.ExtraTools.
func (cb *ContextBuilder) SetExtraTools(t []tools.Tool) {
	cb.extraTools = t
}

// BuildStableSystem returns the static portion of the system prompt as a
// providers.Message with CacheControl="ephemeral". Anthropic's prompt-cache
// charges 10% read-cost on subsequent turns when this block hashes identically.
// The stable prefix contains: model preamble, platform facts, operating rules,
// safety, and tool posture — all sections that don't change per turn.
//
// The caller is expected to prepend this message before the main system message
// (BuildSystemPrompt) in the messages array. Both messages have Role="system".
func (cb *ContextBuilder) BuildStableSystem() providers.Message {
	pb := NewPromptBuilder(cb.agent, cb.runtime)
	if cb.toolReg != nil {
		pb.SetToolRegistry(cb.toolReg)
	}
	return providers.Message{
		Role:         "system",
		Content:      pb.BuildStablePrefix(),
		CacheControl: "ephemeral",
	}
}

// BuildSystemPrompt assembles the 12-section system prompt via PromptBuilder.
func (cb *ContextBuilder) BuildSystemPrompt(bulletin string) string {
	// Use the new PromptBuilder
	pb := NewPromptBuilder(cb.agent, cb.runtime)
	pb.SetTeam(cb.teamRoster)
	pb.SetSkillStore(cb.skillStore)
	if cb.toolReg != nil { pb.SetToolRegistry(cb.toolReg) }
	if len(cb.memResults) > 0 { pb.SetMemoryResults(cb.memResults) }

	prompt := pb.Build()

	// Append learned preferences from learning loop
	if cb.learnedHints != "" {
		prompt += "\n\n## Learned Preferences\n" + cb.learnedHints
	}

	// Append memory bulletin if present
	if bulletin != "" {
		prompt += "\n\n## Memory Bulletin\n" + bulletin
	}

	// Append filesystem skills (SKILL.md files from workspace)
	if cb.skillLoader != nil {
		var allowList []string
		skillContext := cb.skillLoader.LoadForContext(allowList)
		if skillContext != "" {
			prompt += "\n\n" + skillContext
		}
	}

	// Append installed marketplace skill instructions
	if cb.skillStore != nil && cb.agent.ID != "" {
		installed, _ := cb.skillStore.AgentSkills(context.Background(), cb.agent.ID)
		for _, sk := range installed {
			if sk.SkillMD != "" {
				prompt += fmt.Sprintf("\n\n## Skill: %s\n%s", sk.Name, sk.SkillMD)
			}
		}
	}

	return prompt
}

// BuildMessages assembles the message history for an LLM call.
// Includes: session history + relevant memories for the current query.
func (cb *ContextBuilder) BuildMessages(history []providers.Message, userMessage string, memoryResults []memory.SearchResult) []providers.Message {
	var msgs []providers.Message

	// Memories are now in system prompt Section 9 — no longer injected as messages
	// (prevents double-injection that wastes tokens)

	// Append session history
	msgs = append(msgs, history...)

	// Append current user message
	msgs = append(msgs, providers.Message{
		Role:    "user",
		Content: userMessage,
	})

	return msgs
}

// BuildToolDefs returns the tool definitions allowed for this agent.
func (cb *ContextBuilder) BuildToolDefs() []providers.ToolDefinition {
	if cb.toolReg == nil && len(cb.extraTools) == 0 {
		return nil
	}

	var allow, deny []string
	// Parse agent's tools_allowed/denied from JSONB
	if cb.agent.ToolsAllowed != nil {
		parseStringSlice(cb.agent.ToolsAllowed, &allow)
	}
	if cb.agent.ToolsDenied != nil {
		parseStringSlice(cb.agent.ToolsDenied, &deny)
	}

	// Convert global tools to provider format
	var provDefs []providers.ToolDefinition
	if cb.toolReg != nil {
		filtered := tools.FilterTools(cb.toolReg, allow, deny, cb.agent.ToolProfile, false)
		provDefs = make([]providers.ToolDefinition, len(filtered))
		for i, td := range filtered {
			provDefs[i] = providers.ToolDefinition{
				Type: "function",
				Function: providers.ToolFunctionSchema{
					Name:        td.Function.Name,
					Description: td.Function.Description,
					Parameters:  td.Function.Parameters,
				},
			}
		}
	}

	// append per-request extra tools (tenant-scoped Wasm
	// plugins injected by the orchestrator). Respect the agent's
	// deny-list — allow-list does NOT apply to extras because the
	// caller already curated them. Shadowing: if an extra tool has
	// the same name as a global one, the extra replaces it for this
	// run (the tool-execution dispatcher applies the same shadow).
	if len(cb.extraTools) > 0 {
		denySet := map[string]bool{}
		for _, d := range deny {
			denySet[d] = true
		}
		// Drop any global def that will be shadowed, so the LLM sees
		// exactly one definition per name.
		shadowed := map[string]bool{}
		for _, t := range cb.extraTools {
			shadowed[t.Name()] = true
		}
		if len(shadowed) > 0 {
			kept := provDefs[:0]
			for _, td := range provDefs {
				if !shadowed[td.Function.Name] {
					kept = append(kept, td)
				}
			}
			provDefs = kept
		}
		for _, t := range cb.extraTools {
			if denySet[t.Name()] {
				continue
			}
			provDefs = append(provDefs, providers.ToolDefinition{
				Type: "function",
				Function: providers.ToolFunctionSchema{
					Name:        t.Name(),
					Description: t.Description(),
					Parameters:  t.Parameters(),
				},
			})
		}
	}
	return provDefs
}

// buildToolSummary creates a brief summary of available tools for the system prompt.
func buildToolSummary(reg *tools.Registry, agent *Agent) string {
	var allow, deny []string
	if agent.ToolsAllowed != nil {
		parseStringSlice(agent.ToolsAllowed, &allow)
	}
	if agent.ToolsDenied != nil {
		parseStringSlice(agent.ToolsDenied, &deny)
	}

	filtered := tools.FilterTools(reg, allow, deny, "", false)
	if len(filtered) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Available Tools\n\n")
	for _, td := range filtered {
		fmt.Fprintf(&b, "- **%s**: %s\n", td.Function.Name, td.Function.Description)
	}
	return b.String()
}

func parseStringSlice(raw []byte, out *[]string) {
	if raw == nil {
		return
	}
	var s []string
	if err := json.Unmarshal(raw, &s); err == nil {
		*out = s
	}
}
