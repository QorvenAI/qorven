// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/providers"
)

// Subconscious runs a self-improvement loop for an agent.
// It inspects recent performance, generates improvement ideas,
// debates them, and writes the winning direction back into memory.
type Subconscious struct {
	agentStore   *Store
	memStore     *memory.Store
	providerReg  interface{ Default() providers.Provider }
	tenantID     string
}

// SubconsciousConfig controls the self-improvement loop.
type SubconsciousConfig struct {
	MaxIdeas     int    `json:"max_ideas"`      // candidates to generate (default 5)
	DebateRounds int    `json:"debate_rounds"`  // challenge/defense rounds (default 2)
	AutoApply    bool   `json:"auto_apply"`     // apply without approval
	ModelIdeate  string `json:"model_ideation"` // cheap model for ideas
	ModelDebate  string `json:"model_debate"`   // premium model for critique
}

func DefaultSubconsciousConfig() SubconsciousConfig {
	return SubconsciousConfig{MaxIdeas: 5, DebateRounds: 2, AutoApply: false}
}

func NewSubconscious(agentStore *Store, memStore *memory.Store, providerReg interface{ Default() providers.Provider }, tenantID string) *Subconscious {
	return &Subconscious{agentStore: agentStore, memStore: memStore, providerReg: providerReg, tenantID: tenantID}
}

// RunLoop executes one self-improvement cycle for an agent.
func (s *Subconscious) RunLoop(ctx context.Context, agentID string, cfg SubconsciousConfig) (*SubconsciousResult, error) {
	start := time.Now()
	slog.Info("subconscious.start", "agent", agentID[:8])

	provider := s.providerReg.Default()
	if provider == nil {
		return nil, fmt.Errorf("no LLM provider")
	}

	// 1. INSPECT — gather evidence from recent runs
	evidence, err := s.gatherEvidence(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("evidence: %w", err)
	}
	slog.Info("subconscious.evidence", "agent", agentID[:8], "items", len(evidence))

	// 2. IDEATE — generate improvement candidates
	ideas, err := s.ideate(ctx, provider, agentID, evidence, cfg.MaxIdeas)
	if err != nil {
		return nil, fmt.Errorf("ideate: %w", err)
	}
	slog.Info("subconscious.ideas", "agent", agentID[:8], "count", len(ideas))

	// 3. DEBATE — challenge each idea
	debated, err := s.debate(ctx, provider, ideas, evidence, cfg.DebateRounds)
	if err != nil {
		return nil, fmt.Errorf("debate: %w", err)
	}

	// 4. SYNTHESIZE — pick the winner
	winner, backlog, err := s.synthesize(ctx, provider, debated, evidence)
	if err != nil {
		return nil, fmt.Errorf("synthesize: %w", err)
	}
	slog.Info("subconscious.winner", "agent", agentID[:8], "direction", winner[:min(len(winner), 80)])

	// 5. PERSIST — write back into memory
	if cfg.AutoApply || true { // always persist learnings, approval gate is for actions
		s.persist(ctx, agentID, winner, backlog, debated)
	}

	duration := time.Since(start)
	slog.Info("subconscious.complete", "agent", agentID[:8], "duration_ms", duration.Milliseconds())

	return &SubconsciousResult{
		AgentID:   agentID,
		Winner:    winner,
		Backlog:   backlog,
		Ideas:     ideas,
		Duration:  duration,
	}, nil
}

// SubconsciousResult is the output of one self-improvement cycle.
type SubconsciousResult struct {
	AgentID  string        `json:"agent_id"`
	Winner   string        `json:"winner"`
	Backlog  string        `json:"backlog"`
	Ideas    []string      `json:"ideas"`
	Duration time.Duration `json:"duration"`
}

// gatherEvidence collects recent performance data.
func (s *Subconscious) gatherEvidence(ctx context.Context, agentID string) (string, error) {
	var evidence []string

	// Recent memories
	memories, err := s.memStore.SearchByType(ctx, agentID, "explicit", 10)
	if err == nil {
		for _, m := range memories {
			evidence = append(evidence, fmt.Sprintf("[memory] %s", m.Content))
		}
	}

	// Agent info
	agent, err := s.agentStore.Get(ctx, agentID)
	if err == nil {
		evidence = append(evidence, fmt.Sprintf("[agent] %s (%s) — model: %s, tools: %s", agent.DisplayName, derefStr(agent.Role), agent.Model, agent.ToolProfile))
	}

	if len(evidence) == 0 {
		return "No recent evidence available. This is the first run.", nil
	}

	result := ""
	for _, e := range evidence {
		result += e + "\n"
	}
	return result, nil
}

// ideate generates improvement candidates.
func (s *Subconscious) ideate(ctx context.Context, provider providers.Provider, agentID, evidence string, maxIdeas int) ([]string, error) {
	prompt := fmt.Sprintf(`You are a self-improvement analyst for an AI agent. Based on the evidence below, generate %d specific, actionable improvement ideas.

Evidence:
%s

Generate exactly %d ideas. Each should be:
- Specific (not vague)
- Actionable (can be implemented)
- Measurable (we can tell if it worked)

Format: one idea per line, numbered.`, maxIdeas, evidence, maxIdeas)

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options:  map[string]any{"temperature": 0.8, "max_tokens": 1000},
	})
	if err != nil {
		return nil, err
	}

	// Parse numbered ideas
	var ideas []string
	for _, line := range splitLines(resp.Content) {
		if len(line) > 3 {
			ideas = append(ideas, line)
		}
	}
	return ideas, nil
}

// debate challenges ideas against objections.
func (s *Subconscious) debate(ctx context.Context, provider providers.Provider, ideas []string, evidence string, rounds int) ([]string, error) {
	if len(ideas) == 0 {
		return nil, fmt.Errorf("no ideas to debate")
	}

	ideasText := ""
	for i, idea := range ideas {
		ideasText += fmt.Sprintf("%d. %s\n", i+1, idea)
	}

	prompt := fmt.Sprintf(`You are a critical reviewer. Challenge these improvement ideas. For each, give:
- One strong objection
- Whether it survives the objection (YES/NO)
- If YES, how to make it stronger

Ideas:
%s

Evidence context:
%s

Be harsh but fair. Only ideas that survive should be recommended.`, ideasText, evidence[:min(len(evidence), 2000)])

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options:  map[string]any{"temperature": 0.3, "max_tokens": 1500},
	})
	if err != nil {
		return nil, err
	}

	return []string{resp.Content}, nil
}

// synthesize picks the winning direction.
func (s *Subconscious) synthesize(ctx context.Context, provider providers.Provider, debated []string, evidence string) (string, string, error) {
	debateText := ""
	for _, d := range debated {
		debateText += d + "\n"
	}

	prompt := fmt.Sprintf(`Based on this debate, synthesize:
1. ONE winning direction (the single best improvement to make)
2. An improvement backlog (what to try next time)

Debate results:
%s

Format your response as:
WINNER: [one clear sentence]
BACKLOG: [2-3 items for next run]`, debateText)

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options:  map[string]any{"temperature": 0.2, "max_tokens": 500},
	})
	if err != nil {
		return "", "", err
	}

	// Parse winner and backlog
	winner := resp.Content
	backlog := ""
	for _, line := range splitLines(resp.Content) {
		if len(line) > 8 && line[:7] == "WINNER:" {
			winner = line[7:]
		}
		if len(line) > 9 && line[:8] == "BACKLOG:" {
			backlog = line[8:]
		}
	}

	return winner, backlog, nil
}

// persist writes learnings back into memory.
func (s *Subconscious) persist(ctx context.Context, agentID, winner, backlog string, debated []string) {
	// Save winning direction as explicit memory (high importance, decay-exempt)
	s.memStore.Save(ctx, s.tenantID, memory.Memory{
		AgentID: agentID, Type: "explicit", Content: winner,
		Source: "subconscious", Importance: 0.9, DecayExempt: true,
	})

	// Save backlog as prime memory
	if backlog != "" {
		s.memStore.Save(ctx, s.tenantID, memory.Memory{
			AgentID: "00000000-0000-0000-0000-000000000003", Type: "prime",
			Content: fmt.Sprintf("[subconscious] Agent %s backlog: %s", agentID[:8], backlog),
			Source: "subconscious", Importance: 0.7,
		})
	}

	// Save debate as inductive memory (lower certainty)
	for _, d := range debated {
		if len(d) > 200 { d = d[:200] }
		s.memStore.Save(ctx, s.tenantID, memory.Memory{
			AgentID: agentID, Type: "inductive", Content: d,
			Source: "subconscious-debate", Importance: 0.5,
		})
	}

	slog.Info("subconscious.persisted", "agent", agentID[:8], "winner_len", len(winner), "backlog_len", len(backlog))
}

func derefStr(s *string) string {
	if s == nil { return "" }
	return *s
}

func splitLines(s string) []string {
	var lines []string
	current := ""
	for _, c := range s {
		if c == '\n' {
			if current != "" {
				lines = append(lines, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func minSub(a, b int) int {
	if a < b { return a }
	return b
}
