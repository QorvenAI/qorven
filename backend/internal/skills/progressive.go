// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package skills

import (
	"fmt"
	"strings"
)

// ProgressiveDisclosure implements the skill presentation pattern from Qorven + Qorven.
// Channel/main agent sees: name + description only (token-efficient).
// Worker/subagent sees: full content in system prompt.
// Mandatory scan instruction ensures the agent checks skills before every reply.

// FormatSkillList returns a compact listing for the system prompt.
// Only names + descriptions — full content loaded on demand via skill tool.
func FormatSkillList(skills []Info) string {
	if len(skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("<available_skills>\n")
	for _, s := range skills {
		desc := s.Description
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		b.WriteString(fmt.Sprintf("  <skill name=\"%s\" location=\"%s\">\n", s.Name, s.Path))
		b.WriteString(fmt.Sprintf("    <description>%s</description>\n", desc))
		b.WriteString("  </skill>\n")
	}
	b.WriteString("</available_skills>")
	return b.String()
}

// MandatoryScanInstruction returns the instruction that goes in the system prompt
// BEFORE the skill list. This ensures the agent checks skills on every turn.
func MandatoryScanInstruction() string {
	return `## Skills (mandatory)
Before replying: scan <available_skills> <description> entries.
- If exactly one skill clearly applies: read its SKILL.md at <location> with the read tool, then follow it.
- If multiple could apply: choose the most specific one, then read/follow it.
- If none clearly apply: do not read any SKILL.md.
Constraints: never read more than one skill up front; only read after selecting.`
}

// FormatSkillsSection returns the complete skills section for the system prompt.
func FormatSkillsSection(skills []Info) string {
	if len(skills) == 0 {
		return ""
	}
	return MandatoryScanInstruction() + "\n\n" + FormatSkillList(skills)
}

// FormatFullSkillForWorker returns the complete skill content for injection
// into a worker/subagent system prompt (progressive disclosure tier 2).
func FormatFullSkillForWorker(content string, name string) string {
	return fmt.Sprintf("<skill_instructions name=\"%s\">\n%s\n</skill_instructions>", name, content)
}
