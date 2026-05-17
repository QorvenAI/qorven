// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/skills"
)

// SkillEvolver detects user corrections and evolves Skills accordingly.
// When a user says "no, do it this way" or "that's wrong, use X instead",
// the correction is applied to the relevant Skill permanently.
type SkillEvolver struct {
	provider    providers.Provider
	model       string
	crystallizer *skills.Crystallizer
}

func NewSkillEvolver(provider providers.Provider, model string, crys *skills.Crystallizer) *SkillEvolver {
	return &SkillEvolver{provider: provider, model: model, crystallizer: crys}
}

// DetectAndEvolve checks if the user's message is a correction and updates Skills.
// Called from the Learning Loop after each task.
func (se *SkillEvolver) DetectAndEvolve(ctx context.Context, agentID, userMsg, prevResponse, newResponse string) {
	if se.provider == nil || se.crystallizer == nil {
		return
	}

	// Quick heuristic: is this a correction?
	if !looksLikeCorrection(userMsg) {
		return
	}

	// Find which Skill was used (search by similarity to the task)
	matchedSkills, _ := se.crystallizer.SearchSimilar(ctx, agentID, userMsg, 1)
	if len(matchedSkills) == 0 {
		return
	}

	skill := matchedSkills[0]

	// Ask LLM to generate the improvement
	improvement := se.generateImprovement(ctx, skill.Procedure, userMsg, newResponse)
	if improvement == "" {
		return
	}

	// Apply the improvement
	err := se.crystallizer.ImproveSkill(ctx, skill.ID, improvement, nil)
	if err != nil {
		slog.Warn("skill.evolve_failed", "skill", skill.ID, "error", err)
		return
	}

	slog.Info("skill.evolved", "skill", skill.ID, "trigger", userMsg[:min(len(userMsg), 50)])
}

func looksLikeCorrection(msg string) bool {
	lower := strings.ToLower(msg)
	corrections := []string{
		"no,", "wrong", "don't", "instead", "actually", "not like that",
		"use this", "change it", "fix this", "that's incorrect", "should be",
		"prefer", "always use", "never use", "stop doing", "i told you",
	}
	for _, c := range corrections {
		if strings.Contains(lower, c) {
			return true
		}
	}
	return false
}

func (se *SkillEvolver) generateImprovement(ctx context.Context, currentProcedure, correction, newResponse string) string {
	if len(currentProcedure) > 500 { currentProcedure = currentProcedure[:500] }
	if len(correction) > 300 { correction = correction[:300] }

	resp, err := se.provider.Chat(ctx, providers.ChatRequest{
		Model: se.model,
		Messages: []providers.Message{
			{Role: "system", Content: "You improve Skill procedures based on user corrections. Return ONLY the improved procedure text, nothing else."},
			{Role: "user", Content: "Current procedure:\n" + currentProcedure + "\n\nUser correction: " + correction + "\n\nGenerate the improved procedure that incorporates this correction:"},
		},
		Options: map[string]any{"temperature": 0.2, "max_tokens": 500},
	})
	if err != nil {
		return ""
	}

	improved := strings.TrimSpace(resp.Content)
	// Sanity check: improvement should be reasonable length
	if len(improved) < 20 || len(improved) > 2000 {
		return ""
	}

	// Don't accept if it's just echoing the correction
	if improved == correction {
		return ""
	}

	return improved
}


// SkillFeedback tracks usage and success of Skills for evolution decisions.
type SkillFeedback struct {
	SkillID    string
	Used       bool
	Successful bool
	Correction string // user's correction if any
}

// TrackSkillUsage records that a Skill was used and whether it succeeded.
func TrackSkillUsage(ctx context.Context, crys *skills.Crystallizer, feedback SkillFeedback) {
	if crys == nil || feedback.SkillID == "" {
		return
	}
	// Increment reuse count
	crys.IncrementReuse(ctx, feedback.SkillID)

	// If there was a correction, log it for future improvement
	if feedback.Correction != "" {
		slog.Info("skill.correction_received", "skill", feedback.SkillID, "correction", feedback.Correction[:min(len(feedback.Correction), 100)])
	}
}

func init() {
	// Ensure json import is used
	_ = json.Marshal
}
