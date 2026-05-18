// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

// ProviderCapabilities describes what a provider/model supports.
type ProviderCapabilities struct {
	SupportsTools     bool
	SupportsVision    bool
	ParallelToolCalls bool
	NeedsToolHints    bool // weak tool callers need extra enforcement
	MaxToolsPerCall   int
}

// Known model capabilities
var ModelCapabilities = map[string]ProviderCapabilities{
	// Strong tool callers
	"gpt-4o":           {SupportsTools: true, SupportsVision: true, ParallelToolCalls: true, MaxToolsPerCall: 128},
	"gpt-4o-mini":      {SupportsTools: true, SupportsVision: true, ParallelToolCalls: true, MaxToolsPerCall: 128},
	"claude-4.6-opus":  {SupportsTools: true, SupportsVision: true, ParallelToolCalls: true, MaxToolsPerCall: 64},
	"claude-4.6-sonnet":{SupportsTools: true, SupportsVision: true, ParallelToolCalls: true, MaxToolsPerCall: 64},
	"gemini-2.5-flash": {SupportsTools: true, SupportsVision: true, ParallelToolCalls: true, MaxToolsPerCall: 64},
	// Bedrock models (via LiteLLM)
	"nemotron-nano-30b":  {SupportsTools: true, NeedsToolHints: true, MaxToolsPerCall: 16},
	"nemotron-super-120b":{SupportsTools: true, MaxToolsPerCall: 32},
	"nemotron-nano-12b":  {SupportsTools: true, NeedsToolHints: true, MaxToolsPerCall: 16},
	// Weak tool callers — need enforcement
	"deepseek-v3.2":    {SupportsTools: true, NeedsToolHints: true, MaxToolsPerCall: 16},
	"deepseek-chat":    {SupportsTools: true, NeedsToolHints: true, MaxToolsPerCall: 16},
	"qwen3-235b":       {SupportsTools: true, NeedsToolHints: true, MaxToolsPerCall: 16},
	"kimi-k2.5":        {SupportsTools: true, NeedsToolHints: true, MaxToolsPerCall: 16},
}

// GetCapabilities returns capabilities for a model, defaulting to basic support.
func GetCapabilities(model string) ProviderCapabilities {
	if cap, ok := ModelCapabilities[model]; ok { return cap }
	// Default: assume tools work but may need hints
	return ProviderCapabilities{SupportsTools: true, NeedsToolHints: false, MaxToolsPerCall: 32}
}

// ToolEnforcementPrompt is injected for NeedsToolHints models.
const ToolEnforcementPrompt = `## CRITICAL: Tool Calling Rules
- NEVER say "I will search", "I'll look up", "Let me find" — CALL the tool directly.
- When you need external data, you MUST respond with a tool_call, not text describing the action.
- If you're unsure whether to search, SEARCH. It's better to verify than guess.
- Do NOT describe what you plan to do. Just do it by calling the tool.`
