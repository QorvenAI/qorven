// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

// CertaintyLevel represents how confident we are in a memory conclusion.
// Inspired by Neuromancer XR's 4-level framework.
type CertaintyLevel string

const (
	CertaintyExplicit  CertaintyLevel = "explicit"  // directly stated by user (100%)
	CertaintyDeductive CertaintyLevel = "deductive"  // necessarily follows from explicit (99%)
	CertaintyInductive CertaintyLevel = "inductive"  // likely pattern from multiple facts (80%)
	CertaintyAbductive CertaintyLevel = "abductive"  // probable explanation for behavior (60%)
)

// CertaintyWeight returns a retrieval weight for ranking memories.
func CertaintyWeight(c CertaintyLevel) float64 {
	switch c {
	case CertaintyExplicit:  return 1.0
	case CertaintyDeductive: return 0.95
	case CertaintyInductive: return 0.75
	case CertaintyAbductive: return 0.55
	default: return 0.5
	}
}

// AtomicConclusion is a single, self-contained memory fact with certainty.
type AtomicConclusion struct {
	ID         string         `json:"id"`
	AgentID    string         `json:"agent_id"`
	PeerID     string         `json:"peer_id"`     // who this is about
	Content    string         `json:"content"`      // one atomic fact
	Type       MemoryType     `json:"type"`         // user/feedback/project/reference
	Certainty  CertaintyLevel `json:"certainty"`    // explicit/deductive/inductive/abductive
	Premises   []string       `json:"premises"`     // IDs of conclusions this builds on
	Source     string         `json:"source"`        // session/message that produced this
	Weight     float64        `json:"weight"`        // retrieval priority
}

// ExtractionPrompt for the 4-level memory system.
const NeuromancerPrompt = `You are a memory extraction specialist. Extract atomic conclusions from this conversation turn.

For each conclusion, classify its certainty level:
- EXPLICIT: Information directly stated by the user. Quote or closely paraphrase.
- DEDUCTIVE: Conclusions that NECESSARILY follow from explicit info. Must be logically certain.
- INDUCTIVE: Patterns likely true based on multiple observations. Prefix with "likely" or "tends to".
- ABDUCTIVE: Probable explanations for observed behavior. Prefix with "probably" or "suggests".

Rules:
- Each conclusion must be ONE atomic fact (under 100 chars)
- Self-contained — understandable without context
- No speculation in EXPLICIT/DEDUCTIVE levels
- INDUCTIVE/ABDUCTIVE must reference their premises
- Skip information derivable from code, git, or docs

Return JSON array:
[{"content": "...", "certainty": "explicit|deductive|inductive|abductive", "premises": []}]

Only return the JSON array.`

// LLMExtractConclusions uses an LLM with NeuromancerPrompt to extract atomic conclusions.
func LLMExtractConclusions(ctx context.Context, provider providers.Provider, model, userMsg, assistantMsg string) []AtomicConclusion {
	if userMsg == "" && assistantMsg == "" {
		return nil
	}
	turn := ""
	if userMsg != "" {
		turn += "User: " + truncateCertainty(userMsg, 500) + "\n"
	}
	if assistantMsg != "" {
		turn += "Assistant: " + truncateCertainty(assistantMsg, 500) + "\n"
	}

	messages := []providers.Message{
		{Role: "system", Content: NeuromancerPrompt},
		{Role: "user", Content: turn},
	}
	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Model:    model,
		Messages: messages,
		Options:  map[string]any{"max_tokens": 500},
	})
	if err != nil {
		return nil
	}
	return parseConclusions(resp.Content)
}

func parseConclusions(raw string) []AtomicConclusion {
	// Find JSON array in response
	start := strings.Index(raw, "[")
	end := strings.LastIndex(raw, "]")
	if start < 0 || end < 0 || end <= start {
		return nil
	}
	var items []struct {
		Content   string   `json:"content"`
		Certainty string   `json:"certainty"`
		Premises  []string `json:"premises"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &items); err != nil {
		return nil
	}
	var conclusions []AtomicConclusion
	for _, item := range items {
		level := CertaintyLevel(item.Certainty)
		if level != CertaintyExplicit && level != CertaintyDeductive &&
			level != CertaintyInductive && level != CertaintyAbductive {
			level = CertaintyInductive // default
		}
		conclusions = append(conclusions, AtomicConclusion{
			Content:   item.Content,
			Certainty: level,
			Premises:  item.Premises,
			Weight:    CertaintyWeight(level),
		})
	}
	return conclusions
}

func truncateCertainty(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "..."
}
