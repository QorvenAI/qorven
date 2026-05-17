// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package council

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// Council orchestrates multi-model consensus across multiple LLM providers.
// Stage 1: parallel queries to N models → individual responses
// Stage 2: each model ranks the others (anonymized) → peer rankings
// Stage 3: Chairman synthesizes final answer from responses + rankings
type Council struct {
	// Fallback provider used when Resolver is nil or returns nil.
	// Historically the only way to wire council to an LLM.
	provider providers.Provider
	// Resolver picks the right provider for a given model ID. When the
	// gateway has Bedrock + Gemini + OpenAI-compat providers registered,
	// council members may live on different providers — resolving per
	// model avoids the "all members sent to provider.Default()" bug where
	// a Bedrock inference-profile like us.anthropic.claude-sonnet-4-6 got
	// routed to Gemini and 404'd.
	Resolver func(model string) providers.Provider
	config   Config
}

// providerFor returns the best provider for the given model ID.
// Resolver wins; falls back to the default single provider; nil is
// possible when neither is configured and the caller must handle it.
func (c *Council) providerFor(model string) providers.Provider {
	if c.Resolver != nil {
		if p := c.Resolver(model); p != nil { return p }
	}
	return c.provider
}

// Config defines the council setup.
type Config struct {
	Members       []string `json:"members"`        // model names for Stage 1+2
	Chairman      string   `json:"chairman"`        // model for Stage 3 synthesis
	AgreementGate float64  `json:"agreement_gate"`  // skip Stage 2 if similarity > this (0.85)
	MaxTokens     int      `json:"max_tokens"`      // per-model token limit
}

// DefaultConfig returns a sensible default using Bedrock models.
func DefaultConfig() Config {
	return Config{
		Members:       []string{"deepseek-v3.2", "qwen3-235b", "kimi-k2.5", "nemotron-super-120b"},
		Chairman:      "deepseek-v3.2",
		AgreementGate: 0.85,
		MaxTokens:     2048,
	}
}

// Result holds the complete council output.
type Result struct {
	Query       string          `json:"query"`
	Stage1      []ModelResponse `json:"stage1"`       // individual responses
	Stage2      []ModelRanking  `json:"stage2"`       // peer rankings (nil if gate triggered)
	Synthesis   string          `json:"synthesis"`     // chairman's final answer
	GateSkipped bool            `json:"gate_skipped"` // true if Stage 2 was skipped
	Duration    time.Duration   `json:"duration"`
	TokensUsed  int             `json:"tokens_used"`
}

// ModelResponse is one model's answer from Stage 1.
type ModelResponse struct {
	Model    string `json:"model"`
	Label    string `json:"label"`    // anonymized: "Response A", "Response B"
	Response string `json:"response"`
	Tokens   int    `json:"tokens"`
	Duration time.Duration `json:"duration"`
}

// ModelRanking is one model's ranking of the others from Stage 2.
type ModelRanking struct {
	Ranker  string   `json:"ranker"`
	Ranking []string `json:"ranking"` // ordered labels: ["Response C", "Response A", "Response B"]
	Reason  string   `json:"reason"`
}

// New creates a new council.
func New(provider providers.Provider, cfg Config) *Council {
	return &Council{provider: provider, config: cfg}
}

// Run executes the full 3-stage council process.
func (c *Council) Run(ctx context.Context, query string) (*Result, error) {
	start := time.Now()
	result := &Result{Query: query}

	// Stage 1: parallel queries
	slog.Info("council.stage1.start", "members", len(c.config.Members))
	stage1, err := c.stage1(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("stage1: %w", err)
	}
	result.Stage1 = stage1

	if len(stage1) == 0 {
		return nil, fmt.Errorf("no models responded")
	}

	// Quality gate: check if all responses agree
	if c.checkAgreement(stage1) {
		slog.Info("council.gate.triggered", "reason", "high agreement, skipping Stage 2")
		result.GateSkipped = true
	} else {
		// Stage 2: peer ranking
		slog.Info("council.stage2.start")
		stage2, err := c.stage2(ctx, query, stage1)
		if err != nil {
			slog.Warn("council.stage2.failed", "error", err)
			// Stage 2 failure is non-fatal — proceed to synthesis without rankings
		} else {
			result.Stage2 = stage2
		}
	}

	// Stage 3: chairman synthesis
	slog.Info("council.stage3.start", "chairman", c.config.Chairman)
	synthesis, err := c.stage3(ctx, query, stage1, result.Stage2)
	if err != nil {
		// Fallback: use the longest Stage 1 response
		longest := ""
		for _, r := range stage1 {
			if len(r.Response) > len(longest) {
				longest = r.Response
			}
		}
		result.Synthesis = longest
		slog.Warn("council.stage3.fallback", "error", err)
	} else {
		result.Synthesis = synthesis
	}

	result.Duration = time.Since(start)
	for _, r := range stage1 {
		result.TokensUsed += r.Tokens
	}

	slog.Info("council.complete",
		"duration_ms", result.Duration.Milliseconds(),
		"tokens", result.TokensUsed,
		"gate_skipped", result.GateSkipped,
		"members", len(stage1))

	return result, nil
}

// stage1 queries all council members in parallel.
func (c *Council) stage1(ctx context.Context, query string) ([]ModelResponse, error) {
	var mu sync.Mutex
	responses := []ModelResponse{}
	var wg sync.WaitGroup

	labels := []string{"A", "B", "C", "D", "E", "F", "G", "H"}

	for i, model := range c.config.Members {
		if i >= len(labels) {
			break
		}
		wg.Add(1)
		go func(m string, label string) {
			defer wg.Done()
			start := time.Now()

			prov := c.providerFor(m)
			if prov == nil {
				slog.Warn("council.stage1.no_provider", "model", m)
				return
			}
			resp, err := prov.Chat(ctx, providers.ChatRequest{
				Model: m,
				Messages: []providers.Message{
					{Role: "user", Content: query},
				},
				Options: map[string]any{"max_tokens": c.config.MaxTokens},
			})

			if err != nil {
				slog.Warn("council.stage1.model_failed", "model", m, "error", err)
				return
			}

			tokens := 0
			if resp.Usage != nil {
				tokens = resp.Usage.TotalTokens
			}

			mu.Lock()
			responses = append(responses, ModelResponse{
				Model:    m,
				Label:    "Response " + label,
				Response: resp.Content,
				Tokens:   tokens,
				Duration: time.Since(start),
			})
			mu.Unlock()
		}(model, labels[i])
	}

	wg.Wait()
	return responses, nil
}

// checkAgreement determines if Stage 1 responses are similar enough to skip Stage 2.
// Uses simple word overlap as a similarity metric.
func (c *Council) checkAgreement(responses []ModelResponse) bool {
	if len(responses) < 2 {
		return true
	}

	// Compare each pair of responses using word overlap (Jaccard similarity)
	totalSim := 0.0
	pairs := 0

	for i := 0; i < len(responses); i++ {
		for j := i + 1; j < len(responses); j++ {
			sim := jaccardSimilarity(responses[i].Response, responses[j].Response)
			totalSim += sim
			pairs++
		}
	}

	if pairs == 0 {
		return true
	}

	avgSim := totalSim / float64(pairs)
	slog.Info("council.gate.check", "avg_similarity", fmt.Sprintf("%.2f", avgSim), "threshold", c.config.AgreementGate)
	return avgSim >= c.config.AgreementGate
}

// jaccardSimilarity computes word-level Jaccard similarity between two texts.
func jaccardSimilarity(a, b string) float64 {
	wordsA := wordSet(strings.ToLower(a))
	wordsB := wordSet(strings.ToLower(b))

	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}

	union := len(wordsA) + len(wordsB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

func wordSet(text string) map[string]bool {
	words := strings.Fields(text)
	set := make(map[string]bool, len(words))
	for _, w := range words {
		// Strip punctuation
		w = strings.Trim(w, ".,!?;:\"'()[]{}—-")
		if len(w) > 2 { // skip short words
			set[w] = true
		}
	}
	return set
}

// stage2 has each model rank the others' responses (anonymized).
func (c *Council) stage2(ctx context.Context, query string, stage1 []ModelResponse) ([]ModelRanking, error) {
	// Build anonymized response text
	var responsesText strings.Builder
	for _, r := range stage1 {
		responsesText.WriteString(fmt.Sprintf("%s:\n%s\n\n", r.Label, r.Response))
	}

	rankingPrompt := fmt.Sprintf(`You are evaluating different responses to this question:

Question: %s

Here are the responses (anonymized):

%s
Evaluate each response for accuracy, completeness, and insight.
Then provide a FINAL RANKING from best to worst.

Format your ranking EXACTLY as:
FINAL RANKING:
1. Response X
2. Response Y
3. Response Z`, query, responsesText.String())

	var mu sync.Mutex
	rankings := []ModelRanking{}
	var wg sync.WaitGroup

	for _, model := range c.config.Members {
		wg.Add(1)
		go func(m string) {
			defer wg.Done()

			prov := c.providerFor(m)
			if prov == nil { return }
			resp, err := prov.Chat(ctx, providers.ChatRequest{
				Model:    m,
				Messages: []providers.Message{{Role: "user", Content: rankingPrompt}},
				Options:  map[string]any{"max_tokens": 1024},
			})
			if err != nil {
				return
			}

			ranking := parseRanking(resp.Content, stage1)
			mu.Lock()
			rankings = append(rankings, ModelRanking{
				Ranker:  m,
				Ranking: ranking,
				Reason:  resp.Content,
			})
			mu.Unlock()
		}(model)
	}

	wg.Wait()
	return rankings, nil
}

// parseRanking extracts the ordered labels from a ranking response.
func parseRanking(text string, responses []ModelResponse) []string {
	ranking := []string{}
	lines := strings.Split(text, "\n")
	inRanking := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(strings.ToUpper(line), "FINAL RANKING") {
			inRanking = true
			continue
		}
		if inRanking {
			for _, r := range responses {
				if strings.Contains(line, r.Label) {
					ranking = append(ranking, r.Label)
					break
				}
			}
		}
	}
	return ranking
}

// stage3 has the Chairman synthesize the final answer.
func (c *Council) stage3(ctx context.Context, query string, stage1 []ModelResponse, stage2 []ModelRanking) (string, error) {
	var stage1Text strings.Builder
	for _, r := range stage1 {
		stage1Text.WriteString(fmt.Sprintf("Model %s (%s):\n%s\n\n", r.Label, r.Model, r.Response))
	}

	var stage2Text string
	if len(stage2) > 0 {
		var sb strings.Builder
		for _, r := range stage2 {
			sb.WriteString(fmt.Sprintf("Ranker %s: %s\n", r.Ranker, strings.Join(r.Ranking, " > ")))
		}
		stage2Text = sb.String()
	}

	prompt := fmt.Sprintf(`You are the Chairman of an LLM Council. Multiple AI models answered a question, and then ranked each other's responses.

Original Question: %s

INDIVIDUAL RESPONSES:
%s`, query, stage1Text.String())

	if stage2Text != "" {
		prompt += fmt.Sprintf("\nPEER RANKINGS:\n%s", stage2Text)
	}

	prompt += `

Synthesize all responses into a single, comprehensive, accurate answer.
Consider the individual insights, areas of agreement, and the peer rankings.
Provide a clear, well-reasoned final answer that represents the council's collective wisdom.`

	prov := c.providerFor(c.config.Chairman)
	if prov == nil { return "", fmt.Errorf("council: no provider for chairman model %q", c.config.Chairman) }
	resp, err := prov.Chat(ctx, providers.ChatRequest{
		Model:    c.config.Chairman,
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options:  map[string]any{"max_tokens": c.config.MaxTokens * 2},
	})
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}
