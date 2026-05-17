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

func writeTestSkill(dir string) {
	content := "---\nname: Database Migration Expert\n" +
		"description: Helps plan and execute database schema migrations safely\n" +
		"when_to_use: When user needs to modify database tables\n" +
		"allowed_tools: [exec, read_file, write_file]\n" +
		"model: gpt-4\ncontext: fork\n---\n" +
		"You are a database migration expert.\n\n" +
		"1. Always create a backup plan first\n" +
		"2. Never drop columns without checking dependencies\n" +
		"3. Use ALTER TABLE for column changes\n" +
		"4. Use pg_dump for backups before destructive changes\n"
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644)
}

func TestDeep_Skills_RealWorldSkillFile(t *testing.T) {
	dir := t.TempDir()
	writeTestSkill(dir)

	skill, err := ParseSkillFile(filepath.Join(dir, "SKILL.md"))
	if err != nil { t.Fatal(err) }
	if skill.Name != "Database Migration Expert" { t.Errorf("name=%q", skill.Name) }
	if !strings.Contains(skill.Description, "database schema") { t.Error("description") }
	if len(skill.AllowedTools) != 3 { t.Errorf("tools=%d", len(skill.AllowedTools)) }
	if skill.Model != "gpt-4" { t.Error("model") }
	if skill.Context != "fork" { t.Error("context") }
	if !strings.Contains(skill.Prompt, "backup plan") { t.Error("missing backup plan in prompt") }
	if !strings.Contains(skill.Prompt, "ALTER TABLE") { t.Error("missing ALTER TABLE in prompt") }

	prompt := BuildSkillPrompt(skill)
	if !strings.Contains(prompt, "Database Migration Expert") { t.Error("name not in prompt") }
	t.Logf("skill prompt: %d chars", len(prompt))
}

func TestDeep_Skills_SearchRanking_Precision(t *testing.T) {
	skills := []Info{
		{Name: "Code Review"}, {Name: "Database Migration"}, {Name: "API Design"},
		{Name: "Code Formatting"}, {Name: "Code Testing"}, {Name: "Deploy to AWS"},
		{Name: "Docker Setup"}, {Name: "Code Refactoring"}, {Name: "Security Audit"},
		{Name: "Performance Tuning"},
	}
	results := Search(skills, "code", 5)
	if len(results) == 0 { t.Fatal("no results") }
	codeResults := 0
	for _, r := range results {
		if strings.Contains(strings.ToLower(r.Name), "code") { codeResults++ }
	}
	if codeResults < 3 { t.Errorf("expected 3+ code results in top 5, got %d", codeResults) }
	t.Logf("search 'code': %d code-related in top %d", codeResults, len(results))
}

func TestDeep_Skills_DirectoryWithMixedFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "review"), 0755)
	os.MkdirAll(filepath.Join(dir, "test"), 0755)
	os.MkdirAll(filepath.Join(dir, "deploy"), 0755)
	os.WriteFile(filepath.Join(dir, "review", "SKILL.md"), []byte("---\nname: Code Review\n---\nReview code."), 0644)
	os.WriteFile(filepath.Join(dir, "test", "SKILL.md"), []byte("---\nname: Test Writer\n---\nWrite tests."), 0644)
	os.WriteFile(filepath.Join(dir, "deploy", "SKILL.md"), []byte("---\nname: Deploy Helper\n---\nDeploy."), 0644)

	skills, err := LoadSkillsDir(dir)
	if err != nil { t.Fatal(err) }
	if len(skills) != 3 { t.Errorf("expected 3, got %d", len(skills)) }
	names := map[string]bool{}
	for _, s := range skills { names[s.Name] = true }
	if !names["Code Review"] { t.Error("missing Code Review") }
	if !names["Test Writer"] { t.Error("missing Test Writer") }
	if !names["Deploy Helper"] { t.Error("missing Deploy Helper") }
}

func TestDeep_Skills_BM25_TermFrequency(t *testing.T) {
	doc1 := "go programming"
	doc2 := "go programming go concurrency go channels go goroutines"
	s1 := bm25Score(doc1, []string{"go"})
	s2 := bm25Score(doc2, []string{"go"})
	if s2 <= s1 { t.Errorf("more occurrences should score higher: %f <= %f", s2, s1) }
	t.Logf("BM25: 1 occ=%.4f, 4 occ=%.4f", s1, s2)
}

func TestDeep_Skills_BM25_MultiTermBoost(t *testing.T) {
	doc := "go programming language with goroutines and channels"
	s1 := bm25Score(doc, []string{"go"})
	s2 := bm25Score(doc, []string{"go", "goroutines"})
	s3 := bm25Score(doc, []string{"go", "goroutines", "channels"})
	if s2 <= s1 { t.Errorf("2 terms > 1: %f <= %f", s2, s1) }
	if s3 <= s2 { t.Errorf("3 terms > 2: %f <= %f", s3, s2) }
	t.Logf("BM25: 1t=%.4f, 2t=%.4f, 3t=%.4f", s1, s2, s3)
}
