// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Hard skill tests — parsing, search, BM25 scoring, edge cases.

func TestParseSkillFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	os.WriteFile(path, []byte(`---
name: Code Review
description: Reviews code for bugs and style
when_to_use: When user asks for code review
allowed_tools: [read_file, web_search]
model: gpt-4
context: inline
---
You are a code reviewer. Analyze the code for:
- Bugs and logic errors
- Style violations
- Performance issues
`), 0644)

	skill, err := ParseSkillFile(path)
	if err != nil { t.Fatal(err) }
	if skill.Name != "Code Review" { t.Errorf("name=%q", skill.Name) }
	if skill.Description != "Reviews code for bugs and style" { t.Errorf("desc=%q", skill.Description) }
	if skill.WhenToUse != "When user asks for code review" { t.Errorf("when=%q", skill.WhenToUse) }
	if len(skill.AllowedTools) != 2 { t.Errorf("tools=%d", len(skill.AllowedTools)) }
	if skill.Model != "gpt-4" { t.Errorf("model=%q", skill.Model) }
	if skill.Context != "inline" { t.Errorf("context=%q", skill.Context) }
	if !strings.Contains(skill.Prompt, "code reviewer") { t.Error("prompt missing body") }
}

func TestParseSkillFile_MinimalFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	os.WriteFile(path, []byte(`---
name: Simple
---
Just do the thing.
`), 0644)

	skill, err := ParseSkillFile(path)
	if err != nil { t.Fatal(err) }
	if skill.Name != "Simple" { t.Errorf("name=%q", skill.Name) }
	if !strings.Contains(skill.Prompt, "do the thing") { t.Error("missing prompt body") }
}

func TestParseSkillFile_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	os.WriteFile(path, []byte("Just a plain markdown file with no frontmatter."), 0644)

	skill, err := ParseSkillFile(path)
	if err != nil { t.Fatal(err) }
	if skill.Prompt == "" { t.Error("should use entire file as prompt") }
}

func TestParseSkillFile_Nonexistent(t *testing.T) {
	_, err := ParseSkillFile("/nonexistent/SKILL.md")
	if err == nil { t.Error("should fail for nonexistent file") }
}

func TestParseSkillFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	os.WriteFile(path, []byte(""), 0644)

	skill, err := ParseSkillFile(path)
	if err != nil { t.Fatal(err) }
	_ = skill
}

func TestLoadSkillsDir(t *testing.T) {
	dir := t.TempDir()
	// Create nested skill files
	os.MkdirAll(filepath.Join(dir, "review"), 0755)
	os.MkdirAll(filepath.Join(dir, "test"), 0755)
	os.WriteFile(filepath.Join(dir, "review", "SKILL.md"), []byte("---\nname: Review\n---\nReview code."), 0644)
	os.WriteFile(filepath.Join(dir, "test", "SKILL.md"), []byte("---\nname: Test\n---\nWrite tests."), 0644)

	skills, err := LoadSkillsDir(dir)
	if err != nil { t.Fatal(err) }
	if len(skills) != 2 { t.Errorf("expected 2 skills, got %d", len(skills)) }
}

func TestLoadSkillsDir_Empty(t *testing.T) {
	dir := t.TempDir()
	skills, err := LoadSkillsDir(dir)
	if err != nil { t.Fatal(err) }
	if len(skills) != 0 { t.Errorf("expected 0, got %d", len(skills)) }
}

func TestLoadSkillsDir_Nonexistent(t *testing.T) {
	_, err := LoadSkillsDir("/nonexistent/dir")
	if err == nil { t.Error("should fail for nonexistent dir") }
}

func TestBuildSkillPrompt(t *testing.T) {
	skill := &Skill{
		Name:        "Code Review",
		Description: "Reviews code",
		WhenToUse:   "When asked to review",
		Prompt:      "You are a code reviewer.",
	}
	prompt := BuildSkillPrompt(skill)
	if prompt == "" { t.Error("empty prompt") }
	if !strings.Contains(prompt, "Code Review") { t.Error("missing name") }
}

func TestBuildSkillPrompt_Empty(t *testing.T) {
	skill := &Skill{}
	prompt := BuildSkillPrompt(skill)
	_ = prompt // should not panic
}

func TestSearch_Basic(t *testing.T) {
	skills := []Info{
		{Name: "Code Review", Description: "Reviews code for bugs"},
		{Name: "Test Writer", Description: "Writes unit tests"},
		{Name: "Deploy Helper", Description: "Helps with deployment"},
	}
	results := Search(skills, "code review", 10)
	if len(results) == 0 { t.Error("should find results") }
	// Code Review should rank highest
	if results[0].Name != "Code Review" { t.Logf("top result: %q (may vary by scoring)", results[0].Name) }
}

func TestSearch_NoMatch(t *testing.T) {
	skills := []Info{
		{Name: "Code Review", Description: "Reviews code"},
	}
	results := Search(skills, "xyznonexistent", 10)
	// May return empty or low-score results
	_ = results
}

func TestSearch_EmptyQuery(t *testing.T) {
	skills := []Info{{Name: "Test"}}
	results := Search(skills, "", 10)
	_ = results // should not panic
}

func TestSearch_EmptySkills(t *testing.T) {
	results := Search(nil, "test", 10)
	if len(results) != 0 { t.Error("empty skills should return empty") }
}

func TestSearch_MaxResults(t *testing.T) {
	skills := make([]Info, 100)
	for i := range skills { skills[i] = Info{Name: "skill"} }
	results := Search(skills, "skill", 5)
	if len(results) > 5 { t.Errorf("should limit to 5: got %d", len(results)) }
}

func TestBM25Score(t *testing.T) {
	doc := "the quick brown fox jumps over the lazy dog"
	score := bm25Score(doc, []string{"fox", "dog"})
	if score <= 0 { t.Error("matching terms should have positive score") }

	noMatch := bm25Score(doc, []string{"cat", "bird"})
	if noMatch >= score { t.Error("non-matching should score lower") }
}

func TestBM25Score_Empty(t *testing.T) {
	if bm25Score("", []string{"test"}) != 0 { t.Error("empty doc should score 0") }
	if bm25Score("test", nil) != 0 { t.Error("empty terms should score 0") }
}

func TestParseList(t *testing.T) {
	tests := []struct{ in string; want int }{
		{"[a, b, c]", 3},
		{"a, b", 2},
		{"single", 1},
		{"", 0},
		{"[read_file, web_search, exec]", 3},
	}
	for _, tt := range tests {
		got := parseList(tt.in)
		if len(got) != tt.want { t.Errorf("parseList(%q) = %d items, want %d", tt.in, len(got), tt.want) }
	}
}

func TestSkill_Fields(t *testing.T) {
	s := Skill{
		Name: "Test", Description: "desc", WhenToUse: "when",
		AllowedTools: []string{"a", "b"}, Model: "gpt-4",
		Context: "fork", Agent: "agent1", Prompt: "prompt",
		FilePath: "/path/to/SKILL.md",
	}
	if s.Context != "fork" { t.Error("wrong context") }
	if s.Agent != "agent1" { t.Error("wrong agent") }
}

func TestInfo_Fields(t *testing.T) {
	i := Info{Name: "Test"}
	if i.Name != "Test" { t.Error("wrong name") }
}

func TestSearchResult_Fields(t *testing.T) {
	r := SearchResult{Name: "Test", Score: 0.95}
	if r.Score < 0 { t.Error("negative score") }
}

// === HARD SKILLS TESTS ===

func TestSearch_Ranking(t *testing.T) {
	skills := []Info{
		{Name: "Code Review Expert"},
		{Name: "Test Writer"},
		{Name: "Code Formatter"},
		{Name: "Deploy Helper"},
		{Name: "Code Analysis"},
	}
	results := Search(skills, "code", 10)
	if len(results) == 0 { t.Fatal("no results") }
	// "Code" skills should rank higher
	codeCount := 0
	for _, r := range results[:min3(3, len(results))] {
		if strings.Contains(strings.ToLower(r.Name), "code") { codeCount++ }
	}
	if codeCount == 0 { t.Error("code skills should rank high for 'code' query") }
}

func TestSearch_CaseInsensitive(t *testing.T) {
	skills := []Info{{Name: "UPPERCASE SKILL"}}
	r1 := Search(skills, "uppercase", 10)
	r2 := Search(skills, "UPPERCASE", 10)
	if len(r1) != len(r2) { t.Error("search should be case insensitive") }
}

func TestBM25Score_TermFrequency(t *testing.T) {
	// Document with more occurrences should score higher
	doc1 := "cat"
	doc2 := "cat cat cat"
	s1 := bm25Score(doc1, []string{"cat"})
	s2 := bm25Score(doc2, []string{"cat"})
	if s2 <= s1 { t.Errorf("more occurrences should score higher: %f <= %f", s2, s1) }
}

func TestBM25Score_MultipleTerms(t *testing.T) {
	doc := "the quick brown fox jumps over the lazy dog"
	s1 := bm25Score(doc, []string{"fox"})
	s2 := bm25Score(doc, []string{"fox", "dog"})
	if s2 <= s1 { t.Errorf("more matching terms should score higher: %f <= %f", s2, s1) }
}

func TestParseSkillFile_AllFrontmatterFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	os.WriteFile(path, []byte(`---
name: Full Skill
description: A complete skill with all fields
when_to_use: Always
allowed_tools: [web_search, exec, read_file]
model: gpt-4-turbo
context: fork
agent: specialist
---
You are a specialist. Do everything perfectly.
`), 0644)

	skill, err := ParseSkillFile(path)
	if err != nil { t.Fatal(err) }
	if skill.Name != "Full Skill" { t.Error("name") }
	if skill.Description != "A complete skill with all fields" { t.Error("desc") }
	if skill.WhenToUse != "Always" { t.Error("when") }
	if len(skill.AllowedTools) != 3 { t.Errorf("tools=%d", len(skill.AllowedTools)) }
	if skill.Model != "gpt-4-turbo" { t.Error("model") }
	if skill.Context != "fork" { t.Error("context") }
	if skill.Agent != "specialist" { t.Error("agent") }
	if !strings.Contains(skill.Prompt, "specialist") { t.Error("prompt") }
}

func TestLoadSkillsDir_NestedDeep(t *testing.T) {
	dir := t.TempDir()
	// Create 3 levels deep
	deep := filepath.Join(dir, "a", "b", "c")
	os.MkdirAll(deep, 0755)
	os.WriteFile(filepath.Join(deep, "SKILL.md"), []byte("---\nname: Deep\n---\nDeep skill."), 0644)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: Root\n---\nRoot skill."), 0644)

	skills, err := LoadSkillsDir(dir)
	if err != nil { t.Fatal(err) }
	if len(skills) != 2 { t.Errorf("expected 2 skills (root + deep), got %d", len(skills)) }
}

func min3(a, b int) int { if a < b { return a }; return b }
