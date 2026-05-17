// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
)

// MixtureOfAgents sends a query to multiple models in parallel, then aggregates.
type MixtureOfAgents struct {
	registry *providers.Registry
}

func NewMixtureOfAgents(registry *providers.Registry) *MixtureOfAgents {
	return &MixtureOfAgents{registry: registry}
}

// Consult sends the query to multiple models and returns aggregated response.
func (m *MixtureOfAgents) Consult(ctx context.Context, systemPrompt, query string, models []string) (string, error) {
	if len(models) == 0 { return "", fmt.Errorf("no models specified") }

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Get diverse responses in parallel
	type modelResp struct {
		Model   string
		Content string
		Err     error
	}
	results := make([]modelResp, len(models))
	var wg sync.WaitGroup

	for i, model := range models {
		wg.Add(1)
		go func(idx int, mdl string) {
			defer wg.Done()
			provider := m.registry.Default() // Use default provider for all
			resp, err := provider.Chat(ctx, providers.ChatRequest{
				Model: mdl,
				Messages: []providers.Message{
					{Role: "system", Content: systemPrompt},
					{Role: "user", Content: query},
				},
			})
			if err != nil {
				results[idx] = modelResp{Model: mdl, Err: err}
			} else {
				results[idx] = modelResp{Model: mdl, Content: resp.Content}
			}
		}(i, model)
	}
	wg.Wait()

	// Collect successful responses
	var responses []string
	for _, r := range results {
		if r.Err == nil && r.Content != "" {
			responses = append(responses, fmt.Sprintf("### Response from %s:\n%s", r.Model, r.Content))
		}
	}
	if len(responses) == 0 { return "", fmt.Errorf("all models failed") }
	if len(responses) == 1 { return results[0].Content, nil }

	// Aggregate with the strongest available model
	aggregatePrompt := fmt.Sprintf(`You are an expert aggregator. Multiple AI models were asked the same question.
Synthesize their responses into a single, comprehensive, best-possible answer.
Keep the best insights from each. Remove redundancy. Cite which model contributed what if relevant.

Question: %s

%s

Provide the synthesized best answer:`, query, strings.Join(responses, "\n\n"))

	provider := m.registry.Default()
	aggResp, err := provider.Chat(ctx, providers.ChatRequest{
		Model:    models[0], // Use first model as aggregator
		Messages: []providers.Message{{Role: "user", Content: aggregatePrompt}},
	})
	if err != nil { return responses[0], nil } // Fallback to first response

	slog.Info("mixture_of_agents.complete", "models", len(models), "responses", len(responses))
	return aggResp.Content, nil
}
