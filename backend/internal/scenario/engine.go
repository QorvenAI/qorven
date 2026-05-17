// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package scenario

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/providers"
)

type Engine struct {
	provider providers.Provider
}

func NewEngine(provider providers.Provider) *Engine {
	return &Engine{provider: provider}
}

// GeneratePersonas creates N agent personas from seed text.
func (e *Engine) GeneratePersonas(ctx context.Context, seed string, count int) ([]Agent, error) {
	prompt := fmt.Sprintf(`Generate %d diverse personas for a scenario simulation about:
"%s"

Each persona should represent a different stakeholder perspective.
Return JSON array: [{"name":"...","role":"...","bio":"one sentence","stance":"bullish/bearish/neutral/skeptical","traits":"2-3 traits"}]
Only return the JSON array, nothing else.`, count, seed)

	resp, err := e.provider.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options:  map[string]any{"temperature": 0.8, "max_tokens": 2000},
	})
	if err != nil { return nil, err }

	var agents []Agent
	content := strings.TrimSpace(resp.Content)
	// Extract JSON from response
	if idx := strings.Index(content, "["); idx >= 0 {
		if end := strings.LastIndex(content, "]"); end > idx {
			content = content[idx : end+1]
		}
	}
	if err := json.Unmarshal([]byte(content), &agents); err != nil {
		return nil, fmt.Errorf("failed to parse personas: %w", err)
	}
	for i := range agents {
		agents[i].ID = uuid.New().String()[:8]
	}
	return agents, nil
}

// RunSimulation executes N rounds of conversation between agents.
func (e *Engine) RunSimulation(ctx context.Context, seed string, agents []Agent, numRounds int, onRound func(Round)) ([]Round, error) {
	var rounds []Round
	var history []string

	slog.Info("scenario.simulation.start", "agents", len(agents), "rounds", numRounds)

	for r := 1; r <= numRounds; r++ {
		// Pick 2-3 agents to speak this round
		speakers := pickSpeakers(agents, 2+rand.Intn(2))

		for _, agent := range speakers {
			prompt := buildAgentPrompt(seed, agent, history, r)

			resp, err := e.provider.Chat(ctx, providers.ChatRequest{
				Messages: []providers.Message{{Role: "user", Content: prompt}},
				Options:  map[string]any{"temperature": 0.9, "max_tokens": 300},
			})
			if err != nil {
				slog.Warn("scenario.agent.error", "agent", agent.Name, "error", err)
				continue
			}

			round := Round{
				Number: r, AgentID: agent.ID, AgentName: agent.Name,
				Content: strings.TrimSpace(resp.Content), Timestamp: time.Now(),
			}
			rounds = append(rounds, round)
			history = append(history, fmt.Sprintf("[%s (%s)]: %s", agent.Name, agent.Role, round.Content))
			if onRound != nil { onRound(round) }
		}
	}

	slog.Info("scenario.simulation.complete", "rounds", len(rounds))
	return rounds, nil
}

// GenerateReport synthesizes simulation results into a report.
func (e *Engine) GenerateReport(ctx context.Context, seed string, agents []Agent, rounds []Round) (string, error) {
	// Build transcript
	var transcript strings.Builder
	for _, r := range rounds {
		fmt.Fprintf(&transcript, "Round %d — %s (%s): %s\n\n", r.Number, r.AgentName, r.AgentID, r.Content)
	}

	prompt := fmt.Sprintf(`You are a research analyst. Analyze this scenario simulation and write a structured report.

## Scenario Seed
%s

## Participants
%s

## Simulation Transcript
%s

Write a report with these sections:
1. **Executive Summary** — Key findings in 2-3 sentences
2. **Stakeholder Analysis** — How each group reacted
3. **Key Themes** — Major topics and sentiments
4. **Predictions** — Likely outcomes based on the simulation
5. **Risks & Concerns** — What could go wrong
6. **Recommendations** — Suggested actions

Use markdown formatting. Be specific and cite agent statements.`,
		seed, formatAgents(agents), transcript.String())

	resp, err := e.provider.Chat(ctx, providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options:  map[string]any{"temperature": 0.3, "max_tokens": 4000},
	})
	if err != nil { return "", err }
	return resp.Content, nil
}

func pickSpeakers(agents []Agent, n int) []Agent {
	if n >= len(agents) { return agents }
	perm := rand.Perm(len(agents))
	speakers := make([]Agent, n)
	for i := 0; i < n; i++ { speakers[i] = agents[perm[i]] }
	return speakers
}

func buildAgentPrompt(seed string, agent Agent, history []string, round int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "You are %s, a %s. %s\n", agent.Name, agent.Role, agent.Bio)
	fmt.Fprintf(&sb, "Your stance: %s. Your traits: %s.\n\n", agent.Stance, agent.Traits)
	fmt.Fprintf(&sb, "Scenario: %s\n\n", seed)
	if len(history) > 0 {
		recent := history
		if len(recent) > 6 { recent = recent[len(recent)-6:] }
		sb.WriteString("Recent discussion:\n")
		for _, h := range recent { fmt.Fprintf(&sb, "%s\n", h) }
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "Round %d: Share your perspective in 2-3 sentences. React to others if relevant. Stay in character.", round)
	return sb.String()
}

func formatAgents(agents []Agent) string {
	var sb strings.Builder
	for _, a := range agents {
		fmt.Fprintf(&sb, "- %s (%s) — %s, stance: %s\n", a.Name, a.Role, a.Bio, a.Stance)
	}
	return sb.String()
}

// InjectEvent adds a mid-simulation event (God's-eye control).
func (e *Engine) InjectEvent(ctx context.Context, seed string, agents []Agent, rounds []Round, injection string) ([]Round, error) {
	// Add the injection as a "system event" that all agents see
	injectionRound := Round{
		Number: rounds[len(rounds)-1].Number + 1,
		AgentID: "system", AgentName: "Breaking News",
		Content: injection, Timestamp: time.Now(),
	}
	rounds = append(rounds, injectionRound)

	// Run 2 more rounds of reactions
	var history []string
	for _, r := range rounds {
		history = append(history, fmt.Sprintf("[%s]: %s", r.AgentName, r.Content))
	}

	for r := 0; r < 2; r++ {
		speakers := pickSpeakers(agents, 2+rand.Intn(2))
		for _, agent := range speakers {
			prompt := buildAgentPrompt(seed, agent, history, injectionRound.Number+r+1)
			prompt += fmt.Sprintf("\n\n⚡ BREAKING: %s\n\nReact to this new development.", injection)

			resp, err := e.provider.Chat(ctx, providers.ChatRequest{
				Messages: []providers.Message{{Role: "user", Content: prompt}},
				Options:  map[string]any{"temperature": 0.9, "max_tokens": 300},
			})
			if err != nil { continue }

			round := Round{
				Number: injectionRound.Number + r + 1, AgentID: agent.ID,
				AgentName: agent.Name, Content: strings.TrimSpace(resp.Content),
				Timestamp: time.Now(),
			}
			rounds = append(rounds, round)
			history = append(history, fmt.Sprintf("[%s]: %s", agent.Name, round.Content))
		}
	}
	return rounds, nil
}

// PlatformMode controls simulation style.
type PlatformMode string
const (
	ModeDiscussion PlatformMode = "discussion" // default: open conversation
	ModeTwitter    PlatformMode = "twitter"    // short posts, retweets, likes
	ModeReddit     PlatformMode = "reddit"     // threads, upvotes, nested replies
)

func buildPlatformPrompt(mode PlatformMode, agent Agent, round int) string {
	switch mode {
	case ModeTwitter:
		return fmt.Sprintf("You are @%s on Twitter. Post a tweet (max 280 chars) about this topic. You can quote-tweet or reply to others. Stay in character as %s (%s).", 
			strings.ReplaceAll(agent.Name, " ", ""), agent.Role, agent.Stance)
	case ModeReddit:
		if round == 1 {
			return fmt.Sprintf("You are u/%s on Reddit. Write a post title and body about this topic. Stay in character as %s (%s).",
				strings.ReplaceAll(agent.Name, " ", "_"), agent.Role, agent.Stance)
		}
		return fmt.Sprintf("You are u/%s on Reddit. Write a comment replying to the discussion. Stay in character as %s (%s).",
			strings.ReplaceAll(agent.Name, " ", "_"), agent.Role, agent.Stance)
	default:
		return ""
	}
}
