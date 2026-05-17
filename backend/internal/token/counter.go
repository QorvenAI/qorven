// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package token

import "github.com/qorvenai/qorven/internal/providers"

// Counter estimates token usage for pre-flight budget checks.
type Counter struct{}

// Estimate returns approximate token count. ~4 chars per token for English.
func (c *Counter) Estimate(messages []providers.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)/4 + 4 // +4 for role/formatting overhead per message
	}
	return total
}

// WillExceedLimit checks if messages would exceed 85% of the context window.
func (c *Counter) WillExceedLimit(messages []providers.Message, limitTokens int) bool {
	return c.Estimate(messages) > int(float64(limitTokens)*0.85)
}

// WillExceedBudget checks if estimated cost exceeds remaining budget.
// Rough pricing: $0.001 per 1K input tokens (conservative estimate).
func (c *Counter) EstimateCostCents(messages []providers.Message) int64 {
	tokens := c.Estimate(messages)
	return int64(float64(tokens) / 1000.0 * 0.1) // 0.1 cents per 1K tokens
}
