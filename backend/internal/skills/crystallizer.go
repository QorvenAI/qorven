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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/providers"
)

// EvolutionMode describes how a skill was created.
type EvolutionMode string

const (
	ModeFix      EvolutionMode = "FIX"      // Repaired broken skill
	ModeDerived  EvolutionMode = "DERIVED"  // Enhanced from parent skill
	ModeCaptured EvolutionMode = "CAPTURED" // Extracted from successful execution
)

// CrystallizedSkill is an auto-generated skill from an agent run.
type CrystallizedSkill struct {
	ID          string        `json:"id"`
	AgentID     string        `json:"agent_id"`
	Name        string        `json:"name"`
	Slug        string        `json:"slug"`
	Description string        `json:"description"`
	Procedure   string        `json:"procedure"`   // Step-by-step reusable procedure
	Scope       string        `json:"scope"`        // "private", "shared", "marketplace"
	Mode        EvolutionMode `json:"mode"`
	ParentID    string        `json:"parent_id,omitempty"`
	ReuseCount  int           `json:"reuse_count"`
	SuccessRate float64       `json:"success_rate"`
	TokenSaved  int           `json:"token_saved"`
	CreatedAt   time.Time     `json:"created_at"`
}

// Crystallizer extracts reusable skills from successful agent runs.
type Crystallizer struct {
	pool     *pgxpool.Pool
	provider providers.Provider
	model    string
}

func NewCrystallizer(pool *pgxpool.Pool, provider providers.Provider, model string) *Crystallizer {
	return &Crystallizer{pool: pool, provider: provider, model: model}
}

// MaybeExtract attempts to crystallize a skill from a successful run.
// Only triggers when: success + 2+ tool calls + no similar skill exists.
func (c *Crystallizer) MaybeExtract(ctx context.Context, agentID, userMessage, response string, toolsUsed []string) (*CrystallizedSkill, error) {
	// Gate: only crystallize multi-step runs
	if len(toolsUsed) < 2 {
		return nil, nil
	}

	// Gate: check if similar skill already exists
	if c.hasSimilarSkill(ctx, agentID, userMessage) {
		return nil, nil
	}

	// Ask LLM to extract a generalized procedure
	prompt := fmt.Sprintf(`Extract a reusable skill from this successful agent run.

User request: %s
Tools used: %s
Agent response (first 500 chars): %s

Create a generalized, reusable procedure. Remove specific names/dates but keep the pattern.
Respond with JSON:
{
  "name": "short skill name",
  "description": "what this skill does",
  "procedure": "step-by-step procedure that can be reused for similar tasks"
}`, userMessage, strings.Join(toolsUsed, ", "), response[:min(len(response), 500)])

	resp, err := c.provider.Chat(ctx, providers.ChatRequest{
		Model:    c.model,
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options: map[string]any{
			"temperature":     0.2,
			"max_tokens":      500,
			"response_format": map[string]string{"type": "json_object"},
		},
	})
	if err != nil {
		return nil, err
	}

	var extracted struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Procedure   string `json:"procedure"`
	}
	if json.Unmarshal([]byte(resp.Content), &extracted) != nil || extracted.Name == "" {
		return nil, nil
	}

	slug := slugify(extracted.Name) + "-" + hashShort(userMessage)

	skill := &CrystallizedSkill{
		AgentID:     agentID,
		Name:        extracted.Name,
		Slug:        slug,
		Description: extracted.Description,
		Procedure:   extracted.Procedure,
		Scope:       "private",
		Mode:        ModeCaptured,
		ReuseCount:  0,
		SuccessRate: 1.0,
	}

	// Store in DB
	err = c.pool.QueryRow(ctx,
		`INSERT INTO crystallized_skills (agent_id, name, slug, description, procedure, scope, mode, reuse_count, success_rate)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 0, 1.0) RETURNING id`,
		skill.AgentID, skill.Name, skill.Slug, skill.Description, skill.Procedure, skill.Scope, skill.Mode,
	).Scan(&skill.ID)
	if err != nil {
		return nil, err
	}

	slog.Info("skill.crystallized", "agent", agentID, "name", extracted.Name, "mode", ModeCaptured, "tools", len(toolsUsed))
	return skill, nil
}

// SearchSimilar finds crystallized skills matching a query.
func (c *Crystallizer) SearchSimilar(ctx context.Context, agentID, query string, limit int) ([]CrystallizedSkill, error) {
	rows, err := c.pool.Query(ctx,
		`SELECT id, agent_id, name, slug, description, procedure, scope, mode, reuse_count, success_rate, created_at
		 FROM crystallized_skills
		 WHERE (agent_id = $1 OR scope IN ('shared', 'marketplace'))
		   AND (name ILIKE '%' || $2 || '%' OR description ILIKE '%' || $2 || '%')
		 ORDER BY reuse_count DESC, success_rate DESC
		 LIMIT $3`, agentID, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []CrystallizedSkill
	for rows.Next() {
		var s CrystallizedSkill
		rows.Scan(&s.ID, &s.AgentID, &s.Name, &s.Slug, &s.Description, &s.Procedure, &s.Scope, &s.Mode, &s.ReuseCount, &s.SuccessRate, &s.CreatedAt)
		skills = append(skills, s)
	}
	return skills, nil
}

// IncrementReuse bumps the reuse counter and returns true if promotion threshold reached.
func (c *Crystallizer) IncrementReuse(ctx context.Context, skillID string) (promoteReady bool, err error) {
	var count int
	err = c.pool.QueryRow(ctx,
		`UPDATE crystallized_skills SET reuse_count = reuse_count + 1 WHERE id = $1 RETURNING reuse_count`,
		skillID).Scan(&count)
	return count >= 3, err
}

// Promote changes a skill's scope.
func (c *Crystallizer) Promote(ctx context.Context, skillID, newScope string) error {
	_, err := c.pool.Exec(ctx,
		`UPDATE crystallized_skills SET scope = $1 WHERE id = $2`, newScope, skillID)
	return err
}

func (c *Crystallizer) hasSimilarSkill(ctx context.Context, agentID, query string) bool {
	// Simple keyword check — first 3 words
	words := strings.Fields(query)
	if len(words) > 3 {
		words = words[:3]
	}
	searchTerm := strings.Join(words, " ")
	var count int
	c.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM crystallized_skills WHERE agent_id = $1 AND description ILIKE '%' || $2 || '%'`,
		agentID, searchTerm).Scan(&count)
	return count > 0
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		if r == ' ' {
			return '-'
		}
		return -1
	}, s)
	return s
}

func hashShort(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:4])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ImproveSkill updates a skill's procedure if the current execution was better.
// Called after a skill is used and the result is successful.
func (c *Crystallizer) ImproveSkill(ctx context.Context, skillID, newProcedure string, toolsUsed []string) error {
	// Get current skill
	var currentProcedure string
	var reuseCount int
	err := c.pool.QueryRow(ctx,
		`SELECT procedure, reuse_count FROM crystallized_skills WHERE id = $1`, skillID,
	).Scan(&currentProcedure, &reuseCount)
	if err != nil { return err }

	// Only improve after 3+ uses (enough data to know it works)
	if reuseCount < 3 { return nil }

	// If new procedure is significantly different and longer (more detailed), update
	if len(newProcedure) > int(float64(len(currentProcedure))*1.2) && newProcedure != currentProcedure {
		_, err = c.pool.Exec(ctx,
			`UPDATE crystallized_skills SET procedure = $2, updated_at = NOW() WHERE id = $1`,
			skillID, newProcedure)
		if err != nil { return err }
		slog.Info("skill.self_improved", "skill_id", skillID, "old_len", len(currentProcedure), "new_len", len(newProcedure))
	}
	return nil
}
