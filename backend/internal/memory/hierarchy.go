// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Scope defines the visibility level of a memory.
// Higher scopes are visible to lower scopes. Lower never leaks up without extraction.
type Scope string

const (
	ScopeCompany Scope = "company" // Tenant-wide: all Qors see it (company facts, policies)
	ScopeTeam    Scope = "team"    // Team-level: Qors in the same team see it
	ScopeAgent   Scope = "agent"   // Agent-specific: only this Qor sees it
	ScopeTask    Scope = "task"    // Task/project-specific: archived when task completes
	ScopeSession    Scope = "session"    // Session-specific: only this conversation
	ScopeDiscussion Scope = "discussion" // Discussion-cluster: across sessions in same topic cluster
	ScopePrime      Scope = "prime"      // Supervisor-only: Prime's observations about the system
)

// ScopedMemory is a memory with explicit scope.
type ScopedMemory struct {
	Memory
	Scope     Scope  `json:"scope"`
	TeamID    string `json:"team_id,omitempty"`    // for team-scoped
	TaskID    string `json:"task_id,omitempty"`    // for task-scoped
	SessionID    string `json:"session_id,omitempty"`    // for session-scoped
	DiscussionID string `json:"discussion_id,omitempty"` // for discussion-scoped
}

// HierarchyStore manages the memory hierarchy: company > team > agent > session.
type HierarchyStore struct {
	store    *Store
	tenantID string
}

// NewHierarchyStore creates a hierarchy-aware memory store.
func NewHierarchyStore(store *Store, tenantID string) *HierarchyStore {
	return &HierarchyStore{store: store, tenantID: tenantID}
}

// SaveCompany saves a company-wide memory visible to ALL agents.
func (h *HierarchyStore) SaveCompany(ctx context.Context, content, source string) (string, error) {
	m := Memory{
		AgentID:     "00000000-0000-0000-0000-000000000001",
		Type:        "company",
		Content:     content,
		Source:      source,
		Importance:  0.9, // company memories are high importance
		DecayExempt: true, // never decay company knowledge
	}
	return h.store.Save(ctx, h.tenantID, m)
}

// SaveTeamMemory saves a team-scoped memory.
func (h *HierarchyStore) SaveTeamMemory(ctx context.Context, teamID, content, source string) (string, error) {
	m := Memory{
		AgentID:     "00000000-0000-0000-0000-000000000002",
		Type:        "team",
		Content:     content,
		Source:      source,
		Importance:  0.8,
		DecayExempt: true,
	}
	return h.store.Save(ctx, h.tenantID, m)
}

// SavePrime saves a Prime supervisor observation.
func (h *HierarchyStore) SavePrime(ctx context.Context, content, source string) (string, error) {
	m := Memory{
		AgentID:     "00000000-0000-0000-0000-000000000003",
		Type:        "prime",
		Content:     content,
		Source:      source,
		Importance:  0.85,
		DecayExempt: false, // Prime observations can decay (patterns change)
	}
	return h.store.Save(ctx, h.tenantID, m)
}

// SaveTask saves a task/project-scoped memory. Archived when task completes.
func (h *HierarchyStore) SaveTask(ctx context.Context, taskID, agentID, content, source string) (string, error) {
	m := Memory{
		AgentID:     agentID,
		Type:        "task:" + taskID,
		Content:     content,
		Source:      source,
		Importance:  0.7,
		DecayExempt: true, // task memories persist until task archived
	}
	return h.store.Save(ctx, h.tenantID, m)
}

// SearchTask searches memories scoped to a specific task.
func (h *HierarchyStore) SearchTask(ctx context.Context, taskID, query string, maxResults int) ([]SearchResult, error) {
	return h.store.SearchByTypeQuery(ctx, h.tenantID, "task:"+taskID, query, maxResults)
}

// SaveDiscussion saves a memory scoped to a specific discussion cluster.
func (h *HierarchyStore) SaveDiscussion(ctx context.Context, discussionID, agentID, content, source string) (string, error) {
	m := Memory{
		AgentID:    agentID,
		Type:       "discussion:" + discussionID,
		Content:    content,
		Source:     source,
		Importance: 0.7,
	}
	return h.store.Save(ctx, h.tenantID, m)
}

// SearchDiscussion searches memories scoped to a specific discussion cluster.
func (h *HierarchyStore) SearchDiscussion(ctx context.Context, discussionID, query string, maxResults int) ([]SearchResult, error) {
	return h.store.SearchByTypeQuery(ctx, h.tenantID, "discussion:"+discussionID, query, maxResults)
}

// ArchiveTask marks all task memories as decayable (task completed).
func (h *HierarchyStore) ArchiveTask(ctx context.Context, taskID string) error {
	return h.store.MarkDecayable(ctx, h.tenantID, "task:"+taskID)
}

// SearchHierarchy searches memories across the full hierarchy for an agent.
// Returns: company memories + team memories + agent memories, merged and ranked.
func (h *HierarchyStore) SearchHierarchy(ctx context.Context, agentID, teamID, query string, maxResults int) ([]SearchResult, error) {
	allResults := []SearchResult{}

	// 1. Company memories (visible to all)
	companyResults, _ := h.store.Search(ctx, h.tenantID, "00000000-0000-0000-0000-000000000001", query, maxResults)
	for i := range companyResults {
		companyResults[i].Memory.Type = "company"
	}
	allResults = append(allResults, companyResults...)

	// 2. Team memories (if agent is in a team)
	if teamID != "" {
		teamResults, _ := h.store.Search(ctx, h.tenantID, "00000000-0000-0000-0000-000000000002", query, maxResults)
		for i := range teamResults {
			teamResults[i].Memory.Type = "team"
		}
		allResults = append(allResults, teamResults...)
	}

	// 3. Prime observations (visible to all agents for context)
	primeResults, _ := h.store.Search(ctx, h.tenantID, "00000000-0000-0000-0000-000000000003", query, 3)
	for i := range primeResults {
		primeResults[i].Memory.Type = "prime"
	}
	allResults = append(allResults, primeResults...)

	// 4. Agent-specific memories
	agentResults, _ := h.store.Search(ctx, h.tenantID, agentID, query, maxResults/2)
	allResults = append(allResults, agentResults...)

	// Sort by score descending, limit to maxResults
	sortByScore(allResults)
	if len(allResults) > maxResults {
		allResults = allResults[:maxResults]
	}

	return allResults, nil
}

// SearchHierarchyScoped is identical to SearchHierarchy but respects per-conversation
// scope filters.  scopeAllow limits search to those tier names; scopeDeny excludes
// them.  When both are nil the behaviour is identical to SearchHierarchy.
// Tier names: "company", "team", "prime", "agent".
func (h *HierarchyStore) SearchHierarchyScoped(ctx context.Context, agentID, teamID, query string, maxResults int, scopeAllow, scopeDeny []string) ([]SearchResult, error) {
	allowSet := make(map[string]bool, len(scopeAllow))
	for _, s := range scopeAllow { allowSet[s] = true }
	denySet := make(map[string]bool, len(scopeDeny))
	for _, s := range scopeDeny { denySet[s] = true }

	want := func(scope string) bool {
		if denySet[scope] { return false }
		if len(allowSet) > 0 { return allowSet[scope] }
		return true
	}

	allResults := []SearchResult{}

	if want("company") {
		r, _ := h.store.Search(ctx, h.tenantID, "00000000-0000-0000-0000-000000000001", query, maxResults)
		for i := range r { r[i].Memory.Type = "company" }
		allResults = append(allResults, r...)
	}
	if want("team") && teamID != "" {
		r, _ := h.store.Search(ctx, h.tenantID, "00000000-0000-0000-0000-000000000002", query, maxResults)
		for i := range r { r[i].Memory.Type = "team" }
		allResults = append(allResults, r...)
	}
	if want("prime") {
		r, _ := h.store.Search(ctx, h.tenantID, "00000000-0000-0000-0000-000000000003", query, 3)
		for i := range r { r[i].Memory.Type = "prime" }
		allResults = append(allResults, r...)
	}
	if want("agent") {
		r, _ := h.store.Search(ctx, h.tenantID, agentID, query, maxResults/2)
		allResults = append(allResults, r...)
	}

	sortByScore(allResults)
	if len(allResults) > maxResults { allResults = allResults[:maxResults] }
	return allResults, nil
}

// FormatHierarchyForPrompt formats hierarchical memories for the system prompt.
// Adds staleness warnings for old memories (Claude Code pattern).
func FormatHierarchyForPrompt(results []SearchResult) string {
	if len(results) == 0 {
		return ""
	}

	var sections = map[string][]string{
		"company":    {},
		"team":       {},
		"prime":      {},
		"discussion": {},
		"agent":      {},
	}

	for _, r := range results {
		bucket := "agent"
		switch {
		case r.Memory.Type == "company":
			bucket = "company"
		case r.Memory.Type == "team":
			bucket = "team"
		case r.Memory.Type == "prime":
			bucket = "prime"
		case strings.HasPrefix(r.Memory.Type, "discussion:"):
			bucket = "discussion"
		}

		entry := r.Memory.Content
		// Add staleness warning (Claude Code pattern)
		age := memoryStaleness(r.Memory.CreatedAt)
		if age != "" {
			entry += " " + age
		}
		sections[bucket] = append(sections[bucket], entry)
	}

	var out string
	if len(sections["company"]) > 0 {
		out += "## Company Knowledge\n"
		for _, s := range sections["company"] {
			out += "- " + s + "\n"
		}
		out += "\n"
	}
	if len(sections["team"]) > 0 {
		out += "## Team Knowledge\n"
		for _, s := range sections["team"] {
			out += "- " + s + "\n"
		}
		out += "\n"
	}
	if len(sections["prime"]) > 0 {
		out += "## Supervisor Notes\n"
		for _, s := range sections["prime"] {
			out += "- " + s + "\n"
		}
		out += "\n"
	}
	if len(sections["discussion"]) > 0 {
		out += "## Discussion Context\n"
		for _, s := range sections["discussion"] {
			out += "- " + s + "\n"
		}
		out += "\n"
	}
	if len(sections["agent"]) > 0 {
		out += "## Your Memories\n"
		for _, s := range sections["agent"] {
			out += "- " + s + "\n"
		}
	}

	return out
}

// memoryStaleness returns a staleness warning for old memories.
// Inspired by Claude Code's memoryFreshnessText.
func memoryStaleness(createdAt time.Time) string {
	if createdAt.IsZero() {
		return ""
	}
	days := int(time.Since(createdAt).Hours() / 24)
	if days <= 1 {
		return ""
	}
	if days <= 7 {
		return fmt.Sprintf("[%d days ago]", days)
	}
	return fmt.Sprintf("[%d days ago — may be outdated, verify before asserting as fact]", days)
}

func sortByScore(results []SearchResult) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// ScheduleDecay runs memory decay on a daily schedule.
// Reduces importance of old memories, exempts pinned/company ones.
func ScheduleDecay(ctx context.Context, store *Store, tenantID string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			agents := []string{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000003"} // these are decay-exempt by default
			// Decay all agent memories
			count, err := store.Decay(ctx, "") // empty = all agents
			if err != nil {
				slog.Warn("memory.decay.failed", "error", err)
			} else {
				slog.Info("memory.decay.complete", "decayed", count, "exempt", agents)
			}
		}
	}
}
