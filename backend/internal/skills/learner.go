// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package skills

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Learner watches agent behavior and creates/improves skills from experience.
// This is the self-improving loop: solve problem → detect pattern → save skill → reuse → improve.
type Learner struct {
	store     *Store
	loader    *Loader
	tenantID  string
	agentID   string
	threshold int // minimum tool calls before considering skill creation
}

// NewLearner creates a skill learner for an agent.
func NewLearner(store *Store, loader *Loader, tenantID, agentID string) *Learner {
	return &Learner{store: store, loader: loader, tenantID: tenantID, agentID: agentID, threshold: 3}
}

// ToolTrace records a tool call sequence from a session.
type ToolTrace struct {
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args,omitempty"`
	Result    string         `json:"result,omitempty"`
	Success   bool           `json:"success"`
	Timestamp time.Time      `json:"timestamp"`
}

// SessionOutcome captures what happened in a session for skill extraction.
type SessionOutcome struct {
	SessionID   string      `json:"session_id"`
	AgentID     string      `json:"agent_id"`
	UserQuery   string      `json:"user_query"`
	ToolTraces  []ToolTrace `json:"tool_traces"`
	FinalAnswer string      `json:"final_answer"`
	Success     bool        `json:"success"`
	Duration    time.Duration `json:"duration"`
}

// AnalyzeAndLearn examines a completed session and decides whether to create or improve a skill.
func (l *Learner) AnalyzeAndLearn(ctx context.Context, outcome SessionOutcome) error {
	if !outcome.Success || len(outcome.ToolTraces) < l.threshold {
		return nil // only learn from successful, non-trivial sessions
	}

	pattern := l.extractPattern(outcome)
	if pattern == "" {
		return nil
	}

	existing := l.findSimilarSkill(ctx, pattern)
	if existing != nil {
		return l.improveSkill(ctx, existing, outcome)
	}
	return l.createSkill(ctx, pattern, outcome)
}

// extractPattern identifies the reusable pattern from tool traces.
func (l *Learner) extractPattern(outcome SessionOutcome) string {
	if len(outcome.ToolTraces) == 0 {
		return ""
	}
	var tools []string
	seen := map[string]bool{}
	for _, t := range outcome.ToolTraces {
		if !seen[t.Tool] {
			tools = append(tools, t.Tool)
			seen[t.Tool] = true
		}
	}
	return strings.Join(tools, " → ")
}

// findSimilarSkill checks if we already have a skill for this pattern.
func (l *Learner) findSimilarSkill(ctx context.Context, pattern string) *SkillDetail {
	skills, err := l.store.AgentSkills(ctx, l.agentID)
	if err != nil {
		return nil
	}
	patternHash := hashPattern(pattern)
	for _, s := range skills {
		for _, tag := range s.Tags {
			if tag == "auto:"+patternHash {
				return &s
			}
		}
	}
	return nil
}

// createSkill generates a new skill from the session outcome and saves to DB.
func (l *Learner) createSkill(ctx context.Context, pattern string, outcome SessionOutcome) error {
	slug := "auto-" + hashPattern(pattern)[:12]
	name := fmt.Sprintf("Learned: %s", truncate(outcome.UserQuery, 50))
	description := fmt.Sprintf("Auto-created skill from successful session. Pattern: %s", pattern)
	content := l.buildSkillContent(pattern, outcome)

	// Save to learned_skills table
	_, err := l.store.Pool().Exec(ctx, `
		INSERT INTO learned_skills (agent_id, tenant_id, slug, name, description, pattern_hash, tool_sequence, skill_content, usage_count, success_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 1, 1)
		ON CONFLICT (agent_id, pattern_hash) DO UPDATE SET
			usage_count = learned_skills.usage_count + 1,
			success_count = learned_skills.success_count + 1,
			last_used_at = now(),
			updated_at = now()`,
		l.agentID, l.tenantID, slug, name, description, hashPattern(pattern),
		toolNames(outcome.ToolTraces), content)
	if err != nil {
		return fmt.Errorf("save learned skill: %w", err)
	}

	slog.Info("skill learned from experience",
		"agent", l.agentID, "slug", slug, "pattern", pattern,
		"tools", len(outcome.ToolTraces), "query", truncate(outcome.UserQuery, 80))
	return nil
}

// improveSkill updates an existing skill based on new experience.
func (l *Learner) improveSkill(ctx context.Context, existing *SkillDetail, outcome SessionOutcome) error {
	_, err := l.store.Pool().Exec(ctx, `
		UPDATE learned_skills SET
			usage_count = usage_count + 1,
			success_count = CASE WHEN $1 THEN success_count + 1 ELSE success_count END,
			last_used_at = now(),
			updated_at = now()
		WHERE agent_id = $2 AND slug = $3`,
		outcome.Success, l.agentID, existing.Slug)
	if err != nil {
		return err
	}
	slog.Info("skill reinforced by experience",
		"agent", l.agentID, "slug", existing.Slug,
		"query", truncate(outcome.UserQuery, 80))
	return nil
}

func toolNames(traces []ToolTrace) []string {
	names := make([]string, len(traces))
	for i, t := range traces {
		names[i] = t.Tool
	}
	return names
}

// buildSkillContent generates the skill markdown from a session outcome.
func (l *Learner) buildSkillContent(pattern string, outcome SessionOutcome) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("name: %s\n", truncate(outcome.UserQuery, 50)))
	b.WriteString(fmt.Sprintf("description: Auto-learned skill. Pattern: %s\n", pattern))
	b.WriteString("version: 1.0.0\nauthor: auto-learner\n")
	b.WriteString("---\n\n")
	b.WriteString("# Learned Skill\n\n")
	b.WriteString(fmt.Sprintf("**Original query:** %s\n\n", outcome.UserQuery))
	b.WriteString("## Tool Sequence\n\n")
	for i, t := range outcome.ToolTraces {
		status := "✅"
		if !t.Success {
			status = "❌"
		}
		b.WriteString(fmt.Sprintf("%d. %s `%s`\n", i+1, status, t.Tool))
	}
	b.WriteString(fmt.Sprintf("\n## Approach\n\n%s\n", outcome.FinalAnswer))
	return b.String()
}

func hashPattern(pattern string) string {
	h := sha256.Sum256([]byte(pattern))
	return fmt.Sprintf("%x", h)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func tagsToSlice(raw json.RawMessage) []string {
	var tags []string
	json.Unmarshal(raw, &tags)
	return tags
}
