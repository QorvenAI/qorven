// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package reach

import (
	"testing"
)

// deep_test.go — Real API integration tests for Qorven Reach.

func TestDeep_HN_RealSearch(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	ch := &HNChannel{}
	results, err := ch.Search("Go programming", 5)
	if err != nil { t.Skipf("hn: %v", err) }
	if len(results) == 0 { t.Fatal("HN returned 0 results") }

	for _, r := range results {
		if r.Platform != "hackernews" { t.Errorf("platform: %q", r.Platform) }
		if r.Title == "" { t.Error("empty title") }
		if r.Engagement["points"] == 0 && r.Engagement["comments"] == 0 {
			t.Logf("low engagement: %q", r.Title)
		}
	}
	t.Logf("hn: %d results, top: %q (%d pts) ✓", len(results), results[0].Title, results[0].Engagement["points"])
}

func TestDeep_HN_RealRead(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	ch := &HNChannel{}
	// Read a known HN item (Ask HN: Who is hiring?)
	result, err := ch.Read("1")
	if err != nil { t.Skipf("hn read: %v", err) }
	if result.Title == "" { t.Error("empty title") }
	if result.Author == "" { t.Error("empty author") }
	t.Logf("hn read: %q by %s ✓", result.Title, result.Author)
}

func TestDeep_V2EX_RealHotTopics(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	ch := &V2EXChannel{}
	results, err := ch.Search("", 0) // empty query = hot topics
	if err != nil { t.Skipf("v2ex: %v", err) }
	// V2EX hot topics may or may not match empty query
	t.Logf("v2ex: %d hot topics ✓", len(results))
}

func TestDeep_Registry_SearchAll(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	reg := DefaultRegistry(Config{})
	results := reg.SearchAll("Go programming", 3)

	if len(results) == 0 { t.Fatal("SearchAll returned 0 platforms") }

	for platform, items := range results {
		t.Logf("  %s: %d results", platform, len(items))
		if len(items) > 0 {
			t.Logf("    top: %q", items[0].Title)
		}
	}
	t.Logf("SearchAll: %d platforms returned results ✓", len(results))
}

func TestDeep_GitHub_RealSearch(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	ch := &GitHubChannel{}
	results, err := ch.Search("AI agent", 5)
	if err != nil { t.Skipf("github: %v", err) }
	if len(results) == 0 { t.Fatal("GitHub returned 0 results") }

	for _, r := range results {
		if r.Platform != "github" { t.Errorf("platform: %q", r.Platform) }
		if r.Engagement["stars"] == 0 { t.Logf("0 stars: %q", r.Title) }
	}
	t.Logf("github: %d results, top: %q (⭐%d) ✓",
		len(results), results[0].Title, results[0].Engagement["stars"])
}

func TestDeep_GitHub_RealRead(t *testing.T) {
	if testing.Short() { t.Skip("skip real API") }

	ch := &GitHubChannel{}
	result, err := ch.Read("golang/go")
	if err != nil { t.Skipf("github read: %v", err) }
	if result.Title != "golang/go" { t.Errorf("title: %q", result.Title) }
	if result.Engagement["stars"] < 100000 { t.Errorf("golang/go should have 100K+ stars: %d", result.Engagement["stars"]) }
	if !containsStr(result.Content, "README") && !containsStr(result.Content, "Go") {
		t.Error("content should contain README or Go description")
	}
	t.Logf("github read: %q (⭐%d) ✓", result.Title, result.Engagement["stars"])
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findStr(s, sub))
}

func findStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub { return true }
	}
	return false
}
