// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuiltInCatalog_Structure: every builtin must have the minimum
// fields populated. If someone adds a new entry and forgets Description,
// this catches it before it ships.
func TestBuiltInCatalog_Structure(t *testing.T) {
	if len(BuiltInCatalog) < 10 {
		t.Fatalf("catalog shrunk suspiciously to %d entries", len(BuiltInCatalog))
	}
	seen := make(map[string]bool)
	for i, b := range BuiltInCatalog {
		if b.Slug == "" {
			t.Errorf("entry %d: empty slug", i)
		}
		if b.Name == "" {
			t.Errorf("entry %d (%s): empty name", i, b.Slug)
		}
		if b.Description == "" {
			t.Errorf("entry %d (%s): empty description", i, b.Slug)
		}
		if b.WhenToUse == "" {
			t.Errorf("entry %d (%s): empty when_to_use — LLM will struggle to pick this skill", i, b.Slug)
		}
		if b.Category == "" {
			t.Errorf("entry %d (%s): empty category — breaks Settings UI grouping", i, b.Slug)
		}
		if len(b.Body) < 50 {
			t.Errorf("entry %d (%s): body too short to be useful (%d chars)", i, b.Slug, len(b.Body))
		}
		// No duplicate slugs — the store's slug uniqueness is a DB
		// constraint so a dup here would make the install loop spam
		// unique-violation errors on every boot.
		if seen[b.Slug] {
			t.Errorf("duplicate slug: %q", b.Slug)
		}
		seen[b.Slug] = true
	}
}

// TestBuiltInCatalog_SlugFormat: slugs must match the package-level
// SlugRegexp, because Store.Create rejects anything else.
func TestBuiltInCatalog_SlugFormat(t *testing.T) {
	for _, b := range BuiltInCatalog {
		if !SlugRegexp.MatchString(b.Slug) {
			t.Errorf("slug %q does not match SlugRegexp — Store.Create will reject it", b.Slug)
		}
	}
}

// TestRenderBuiltInMD_ParseRoundtrip: the file we write must be
// parseable by ParseSkillFile. If the renderer ever drifts from the
// parser, every builtin silently breaks at install time.
func TestRenderBuiltInMD_ParseRoundtrip(t *testing.T) {
	for _, b := range BuiltInCatalog {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "SKILL.md")
		if err := os.WriteFile(path, []byte(renderBuiltInMD(b)), 0o644); err != nil {
			t.Fatal(err)
		}
		skill, err := ParseSkillFile(path)
		if err != nil {
			t.Errorf("%s: ParseSkillFile failed: %v", b.Slug, err)
			continue
		}
		if skill.Name != b.Name {
			t.Errorf("%s: name round-trip mismatch: %q vs %q", b.Slug, skill.Name, b.Name)
		}
		if skill.Description != b.Description {
			t.Errorf("%s: description round-trip mismatch", b.Slug)
		}
		if skill.WhenToUse != b.WhenToUse {
			t.Errorf("%s: when_to_use round-trip mismatch", b.Slug)
		}
		if !strings.Contains(skill.Prompt, "{{input}}") && strings.Contains(b.Body, "{{input}}") {
			t.Errorf("%s: {{input}} placeholder lost in round-trip", b.Slug)
		}
	}
}

// TestYAMLQuote_RoundtripChoice: yamlQuote must pick a quote style
// that the parser can strip cleanly — no interior escapes.
func TestYAMLQuote_RoundtripChoice(t *testing.T) {
	cases := []struct {
		in   string
		want string // what yamlQuote should emit
	}{
		{"no quotes", `"no quotes"`},
		{`has "quote"`, `'has "quote"'`},           // prefer single-wrap when value has double
		{"can't", `"can't"`},                        // single in value → double wrap
		{`both "and" 'chars'`, `both "and" 'chars'`}, // both → bare scalar
		{"", `""`},
	}
	for _, c := range cases {
		if got := yamlQuote(c.in); got != c.want {
			t.Errorf("yamlQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestIsUniqueViolation_Patterns: the reinstall path relies on this
// function to swallow "already exists" errors silently. A regression
// that makes it return false for real PG errors would flood the log.
func TestIsUniqueViolation_Patterns(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"SQLSTATE 23505: duplicate key value violates unique constraint \"skills_slug_key\"", true},
		{"duplicate key value violates unique constraint", true},
		{"23505 duplicate", true},
		{"some other error", false},
		{"", false},
	}
	for _, c := range cases {
		got := isUniqueViolation(errOrNil(c.msg))
		if got != c.want {
			t.Errorf("isUniqueViolation(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}

type stubErr string

func (e stubErr) Error() string { return string(e) }

func errOrNil(s string) error {
	if s == "" {
		return nil
	}
	return stubErr(s)
}
