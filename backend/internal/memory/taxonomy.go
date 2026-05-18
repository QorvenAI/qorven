// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MemoryType — 4 types from Claude Code's taxonomy.
// Only stores what's NOT derivable from current project state.
type MemoryType string

const (
	MemUser      MemoryType = "user"      // user's role, goals, expertise, preferences
	MemFeedback  MemoryType = "feedback"   // corrections AND confirmations (both failure + success)
	MemProject   MemoryType = "project"    // ongoing work, deadlines, initiatives
	MemReference MemoryType = "reference"  // pointers to external systems (Linear, Grafana, Slack)
)

// TypedMemory is a structured memory record.
type TypedMemory struct {
	ID          string     `json:"id"`
	AgentID     string     `json:"agent_id"`
	Type        MemoryType `json:"type"`
	Name        string     `json:"name"`
	Description string     `json:"description"` // one-line, used for relevance matching
	Body        string     `json:"body"`         // structured: rule/fact + Why + How to apply
	Scope       string     `json:"scope"`        // "private" or "team"
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Stale       bool       `json:"stale"`        // drift detected
}

// AntiSaveRules — things that should NOT be saved as memories.
var AntiSaveRules = []string{
	"code patterns or conventions (derivable from reading code)",
	"git history or who-changed-what (use git log/blame)",
	"debugging solutions (the fix is in the code)",
	"anything in CLAUDE.md or README files",
	"ephemeral task details or current conversation context",
	"activity logs or PR lists (ask what was surprising instead)",
}

// ShouldSave checks if content violates anti-save rules.
func ShouldSave(content string) bool {
	lower := strings.ToLower(content)
	antiPatterns := []string{
		"function ", "class ", "import ", "package ", // code patterns
		"git log", "git blame", "commit history",     // git history
		"console.log", "fmt.Println", "debug",         // debugging
	}
	for _, p := range antiPatterns {
		if strings.Contains(lower, p) { return false }
	}
	return true
}

// StructuredBody formats a memory with Why + How to apply.
func StructuredBody(fact, why, howToApply string) string {
	return fmt.Sprintf("%s\n\n**Why:** %s\n\n**How to apply:** %s", fact, why, howToApply)
}

// DriftCheck verifies a memory against current state.
// Returns true if the memory appears stale.
func DriftCheck(ctx context.Context, mem *TypedMemory, currentState string) bool {
	// If memory references a file path, check if it still exists
	if mem.Type == MemReference || mem.Type == MemProject {
		if time.Since(mem.UpdatedAt) > 7*24*time.Hour { return true }
	}
	// If memory references a function/file name, check against current state
	if currentState != "" && mem.Body != "" {
		// Extract key terms from memory
		terms := extractKeyTerms(mem.Body)
		for _, term := range terms {
			if !strings.Contains(currentState, term) { return true }
		}
	}
	return false
}

func extractKeyTerms(body string) []string {
	var terms []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		// Extract file paths
		if strings.Contains(line, "/") && !strings.HasPrefix(line, "http") {
			for _, word := range strings.Fields(line) {
				if strings.Contains(word, "/") && !strings.HasPrefix(word, "http") {
					terms = append(terms, strings.Trim(word, "`,\"'()"))
				}
			}
		}
	}
	return terms
}


// ExtractMemoryPrompt — see certainty.go NeuromancerPrompt for the enhanced version.
