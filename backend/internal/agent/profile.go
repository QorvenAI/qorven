// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

// AgentProfile defines behavior for different agent roles.
type AgentProfile struct {
	Name              string
	Role              string
	MaxIterations     int
	MaxToolRetries    int
	RefundOnToolError bool
	PrimaryModel      string
	FallbackModels    []string
	NeedsToolHints    bool
	DefaultToolChoice string // "auto", "required", "none"
}

var ResearchProfile = AgentProfile{
	Name:              "Researcher",
	Role:              "Deep web research and synthesis",
	MaxIterations:     24,
	MaxToolRetries:    1,
	RefundOnToolError: true,
	PrimaryModel:      "",  // use agent's configured model
	FallbackModels:    nil,
	NeedsToolHints:    true,
	DefaultToolChoice: "auto",
}

var DefaultProfile = AgentProfile{
	Name:              "Default",
	Role:              "General assistant",
	MaxIterations:     20,
	MaxToolRetries:    1,
	RefundOnToolError: true,
	NeedsToolHints:    false,
	DefaultToolChoice: "auto",
}

// ProfileForIntent returns the appropriate profile based on chat intent.
func ProfileForIntent(intent string) AgentProfile {
	switch intent {
	case "research", "deep_research":
		return ResearchProfile
	default:
		return DefaultProfile
	}
}
