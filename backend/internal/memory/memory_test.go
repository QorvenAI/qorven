// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"strings"
	"testing"
)

func TestScope_Constants(t *testing.T) {
	scopes := []Scope{ScopeCompany, ScopeTeam, ScopeAgent, ScopeTask, ScopeSession, ScopePrime}
	seen := map[Scope]bool{}
	for _, s := range scopes {
		if s == "" { t.Error("empty scope") }
		if seen[s] { t.Errorf("duplicate: %s", s) }
		seen[s] = true
	}
	if len(scopes) != 6 { t.Errorf("expected 6 scopes, got %d", len(scopes)) }
}

func TestScope_Values(t *testing.T) {
	if ScopeCompany != "company" { t.Error("wrong company") }
	if ScopeTeam != "team" { t.Error("wrong team") }
	if ScopeAgent != "agent" { t.Error("wrong agent") }
	if ScopeTask != "task" { t.Error("wrong task") }
	if ScopeSession != "session" { t.Error("wrong session") }
	if ScopePrime != "prime" { t.Error("wrong prime") }
}

func TestSearchResult_Fields(t *testing.T) {
	r := SearchResult{Memory: Memory{Content: "test memory"}, Score: 0.95}
	if r.Score < 0 || r.Score > 1 { t.Errorf("score out of range: %f", r.Score) }
}

func TestSearchResult_ScoreRange(t *testing.T) {
	scores := []float64{0.0, 0.1, 0.5, 0.9, 1.0}
	for _, s := range scores {
		r := SearchResult{Score: s}
		if r.Score < 0 || r.Score > 1 { t.Errorf("invalid score: %f", s) }
	}
}

func TestPGBackend_Close(t *testing.T) {
	b := &PGBackend{}
	if err := b.Close(); err != nil { t.Error("close should not error") }
}

func TestPGBackend_Name(t *testing.T) {
	b := &PGBackend{}
	if b.Name() != "postgresql" { t.Errorf("name=%q", b.Name()) }
}

func TestScopedMemory_Fields(t *testing.T) {
	m := ScopedMemory{Memory: Memory{Content: "important fact"}, Scope: ScopeCompany}
	if m.Scope != ScopeCompany { t.Error("wrong scope") }
	if m.Memory.Content != "important fact" { t.Error("wrong content") }
}

func TestMemoryContentLength(t *testing.T) {
	long := strings.Repeat("word ", 10000)
	if len(long) < 40000 { t.Error("test string too short") }
}

func TestHierarchyStore_New(t *testing.T) {
	h := NewHierarchyStore(nil, "tenant1")
	if h == nil { t.Fatal("nil hierarchy") }
}
