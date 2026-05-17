// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/tools"
)

// TestPromptBuilder_IntakeMode verifies the sections included (and excluded)
// when PromptIntake mode is used.
func TestPromptBuilder_IntakeMode(t *testing.T) {
	tests := []struct {
		name          string
		memResults    []string
		wantContains  []string
		wantAbsent    []string
	}{
		{
			name:       "intake_no_memory",
			memResults: nil,
			// No memory results → sectionMemory returns "" → no "Relevant Memories" header
			wantContains: []string{},
			wantAbsent: []string{
				"mandatory_tool_use",
				"act_dont_ask",
			},
		},
		{
			name:       "intake_with_memory",
			memResults: []string{"User prefers async communication"},
			wantContains: []string{
				"Relevant Memories",
			},
			wantAbsent: []string{
				"mandatory_tool_use",
				"act_dont_ask",
			},
		},
		{
			name:       "intake_channel_label",
			memResults: nil,
			wantContains: []string{
				"Project Intake",
			},
			wantAbsent: []string{
				"mandatory_tool_use",
				"act_dont_ask",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ag := &Agent{ID: "intake-agent", DisplayName: "IntakeBot", AgentKey: "intakebot"}
			rc := RuntimeContext{
				Mode:    PromptIntake,
				Channel: "intake",
			}
			pb := NewPromptBuilder(ag, rc)
			if tc.memResults != nil {
				pb.SetMemoryResults(tc.memResults)
			}

			prompt := pb.Build()
			if prompt == "" {
				t.Fatal("Build() returned empty prompt")
			}

			for _, want := range tc.wantContains {
				if !strings.Contains(prompt, want) {
					t.Errorf("expected prompt to contain %q, but it did not.\nPrompt:\n%s", want, prompt)
				}
			}

			for _, absent := range tc.wantAbsent {
				if strings.Contains(prompt, absent) {
					t.Errorf("expected prompt NOT to contain %q, but it did.\nPrompt:\n%s", absent, prompt)
				}
			}
		})
	}
}

// TestSectionTools_NoProseDump verifies that sectionTools() never inlines a
// per-tool name:description list — regardless of how many tools are registered.
// The tool schemas travel on the wire in tools:[...]; duplicating them in prose
// costs ~1,100 tokens and degrades tool-selection accuracy (RAG-MCP, 2025).
func TestSectionTools_NoProseDump(t *testing.T) {
	reg := tools.NewRegistry()

	// Register 10 fake tools — names that would appear if DescribeAll() were called.
	fakeNames := []string{
		"fake_alpha", "fake_beta", "fake_gamma", "fake_delta", "fake_epsilon",
		"fake_zeta", "fake_eta", "fake_theta", "fake_iota", "fake_kappa",
	}
	for _, name := range fakeNames {
		n := name // capture
		reg.Register(&fakeTool{name: n, desc: "description of " + n})
	}

	ag := &Agent{ID: "t", DisplayName: "Bot"}
	pb := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull})
	pb.SetToolRegistry(reg)

	prompt := pb.Build()

	// None of the fake tool names should appear in the system prompt prose.
	for _, name := range fakeNames {
		if strings.Contains(prompt, name) {
			t.Errorf("sectionTools inlined tool name %q in system prompt prose — this is the bloat anti-pattern", name)
		}
	}

	// The prompt must still contain a "## Tools" header (posture line).
	if !strings.Contains(prompt, "## Tools") {
		t.Error("expected '## Tools' header in prompt")
	}

	// Size check: sectionTools() output must be ≤400 chars (≈100 tokens) regardless of registry size.
	// The old code produced ~85 chars × N tools. With 10 tools that was 850 chars.
	pb2 := NewPromptBuilder(ag, RuntimeContext{Mode: PromptFull})
	pb2.SetToolRegistry(reg)
	section := pb2.sectionTools()
	if len(section) > 400 {
		t.Errorf("sectionTools() returned %d chars (>400 limit) — still bloating the prompt", len(section))
	}
}

// fakeTool is a minimal Tool implementation for prompt builder tests.
type fakeTool struct {
	name string
	desc string
}

func (f *fakeTool) Name() string        { return f.name }
func (f *fakeTool) Description() string { return f.desc }
func (f *fakeTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (f *fakeTool) Execute(_ context.Context, _ map[string]any) *tools.Result {
	return &tools.Result{ForLLM: "ok"}
}
