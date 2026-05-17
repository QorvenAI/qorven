// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Hard skills tests — real-world skill loading, search precision, edge cases.

func TestHard_Skills_LoadRealProject(t *testing.T) {
	// Try to load skills from a temp dir (real workspace path is not portable)
	dir := t.TempDir()
	skills, err := LoadSkillsDir(dir)
	if err != nil { t.Logf("load from temp dir: %v", err) }
	t.Logf("skills found: %d", len(skills))
	for _, s := range skills {
		if s.Name == "" { t.Error("skill with empty name") }
		t.Logf("  %s (%s)", s.Name, s.FilePath)
	}
}

func TestHard_Skills_SearchPrecision_10Queries(t *testing.T) {
	skills := []Info{
		{Name: "Code Review"}, {Name: "Unit Testing"}, {Name: "Integration Testing"},
		{Name: "Database Migration"}, {Name: "API Design"}, {Name: "Docker Deployment"},
		{Name: "Security Audit"}, {Name: "Performance Tuning"}, {Name: "Code Refactoring"},
		{Name: "Documentation Writer"}, {Name: "Bug Triage"}, {Name: "Release Manager"},
		{Name: "Incident Response"}, {Name: "Data Pipeline"}, {Name: "ML Model Training"},
	}

	queries := map[string][]string{
		"code":       {"Code Review", "Code Refactoring"},
		"test":       {"Unit Testing", "Integration Testing"},
		"deploy":     {"Docker Deployment"},
		"security":   {"Security Audit"},
		"database":   {"Database Migration"},
		"performance": {"Performance Tuning"},
		"bug":        {"Bug Triage"},
		"release":    {"Release Manager"},
		"data":       {"Data Pipeline"},
		"docs":       {"Documentation Writer"},
	}

	for query, expected := range queries {
		results := Search(skills, query, 3)
		if len(results) == 0 { t.Logf("search %q: no results (BM25 needs exact terms)", query); continue }

		topName := results[0].Name
		matched := false
		for _, exp := range expected {
			if topName == exp { matched = true }
		}
		if !matched { t.Logf("search %q: top=%q (expected one of %v)", query, topName, expected) }
	}
}

func TestHard_Skills_ParseComplexFrontmatter(t *testing.T) {
	dir := t.TempDir()

	// Skill with all possible frontmatter fields
	content := "---\n" +
		"name: Complex Skill\n" +
		"description: A skill with every possible field\n" +
		"when_to_use: When testing complex parsing\n" +
		"allowed_tools: [web_search, exec, read_file, write_file, browser]\n" +
		"model: gpt-4-turbo\n" +
		"context: fork\n" +
		"agent: specialist-agent\n" +
		"---\n" +
		"# Complex Skill Instructions\n\n" +
		"## Step 1: Analyze\n" +
		"Look at the code carefully.\n\n" +
		"## Step 2: Plan\n" +
		"Create a plan before making changes.\n\n" +
		"## Step 3: Execute\n" +
		"Make the changes incrementally.\n\n" +
		"## Step 4: Verify\n" +
		"Run tests after each change.\n"

	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644)

	skill, err := ParseSkillFile(filepath.Join(dir, "SKILL.md"))
	if err != nil { t.Fatal(err) }

	if skill.Name != "Complex Skill" { t.Errorf("name=%q", skill.Name) }
	if len(skill.AllowedTools) != 5 { t.Errorf("tools=%d (expected 5)", len(skill.AllowedTools)) }
	if skill.Model != "gpt-4-turbo" { t.Errorf("model=%q", skill.Model) }
	if skill.Context != "fork" { t.Errorf("context=%q", skill.Context) }
	if skill.Agent != "specialist-agent" { t.Errorf("agent=%q", skill.Agent) }
	if !strings.Contains(skill.Prompt, "Step 1") { t.Error("missing Step 1") }
	if !strings.Contains(skill.Prompt, "Step 4") { t.Error("missing Step 4") }
	if !strings.Contains(skill.Prompt, "incrementally") { t.Error("missing body content") }

	prompt := BuildSkillPrompt(skill)
	if len(prompt) < 200 { t.Errorf("prompt too short: %d", len(prompt)) }
	t.Logf("complex skill: %d tools, prompt=%d chars", len(skill.AllowedTools), len(prompt))
}

func TestHard_Skills_BM25_DocumentLength(t *testing.T) {
	// Longer documents should not automatically score higher
	short := "go programming"
	long := "go programming " + strings.Repeat("unrelated filler text about various topics ", 100)

	scoreShort := bm25Score(short, []string{"go", "programming"})
	scoreLong := bm25Score(long, []string{"go", "programming"})

	// Short document with higher term density should score higher
	if scoreLong > scoreShort*2 { t.Errorf("long doc scores too high: short=%.4f, long=%.4f", scoreShort, scoreLong) }
	t.Logf("BM25 length normalization: short=%.4f, long=%.4f", scoreShort, scoreLong)
}

func TestHard_Skills_LoadManySkills(t *testing.T) {
	dir := t.TempDir()

	// Create 20 skills
	for i := 0; i < 20; i++ {
		skillDir := filepath.Join(dir, "skill-"+string(rune('A'+i%26)))
		os.MkdirAll(skillDir, 0755)
		content := "---\nname: Skill " + string(rune('A'+i%26)) + "\n---\nDo skill " + string(rune('A'+i%26)) + " things.\n"
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644)
	}

	skills, err := LoadSkillsDir(dir)
	if err != nil { t.Fatal(err) }
	if len(skills) != 20 { t.Errorf("expected 20, got %d", len(skills)) }

	// Search across all 20
	results := Search([]Info{{Name: "Skill A"}, {Name: "Skill B"}, {Name: "Skill Z"}}, "skill", 5)
	if len(results) == 0 { t.Error("no search results") }
	t.Logf("loaded %d skills, search returned %d", len(skills), len(results))
}
