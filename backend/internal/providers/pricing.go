// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PricingStore manages model pricing and usage tracking.
type PricingStore struct {
	pool *pgxpool.Pool
}

func NewPricingStore(pool *pgxpool.Pool) *PricingStore { return &PricingStore{pool: pool} }

// FetchAndCacheModelPricing downloads pricing from LiteLLM's GitHub and caches in DB.
func (s *PricingStore) FetchAndCacheModelPricing(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}

	count := 0
	for modelID, data := range raw {
		var p struct {
			InputCostPerToken  float64 `json:"input_cost_per_token"`
			OutputCostPerToken float64 `json:"output_cost_per_token"`
			MaxTokens          int     `json:"max_tokens"`
			MaxInputTokens     int     `json:"max_input_tokens"`
		}
		json.Unmarshal(data, &p) // ignore errors — some fields may be strings
		if p.InputCostPerToken == 0 && p.OutputCostPerToken == 0 {
			continue
		}
		ctx := p.MaxInputTokens
		if ctx == 0 {
			ctx = p.MaxTokens
		}
		if _, err := s.pool.Exec(context.Background(),
			`INSERT INTO model_pricing (model_id, input_cost_per_token, output_cost_per_token, context_window, source)
			 VALUES ($1, $2, $3, $4, 'litellm')
			 ON CONFLICT (model_id) DO UPDATE SET input_cost_per_token = $2, output_cost_per_token = $3, context_window = $4, updated_at = now()`,
			modelID, p.InputCostPerToken, p.OutputCostPerToken, ctx,
		); err != nil {
			slog.Error("pricing.cache.upsert", "model", modelID, "err", err)
			continue
		}
		count++
	}
	slog.Info("pricing.cached", "models", count)
	return nil
}

// GetPrice returns the per-token pricing for a model.
func (s *PricingStore) GetPrice(ctx context.Context, modelID string) (inputPerToken, outputPerToken float64, found bool) {
	err := s.pool.QueryRow(ctx,
		`SELECT input_cost_per_token, output_cost_per_token FROM model_pricing WHERE model_id = $1`, modelID,
	).Scan(&inputPerToken, &outputPerToken)
	return inputPerToken, outputPerToken, err == nil
}

// CalculateCost computes the USD cost for a call. Returns nil if price unknown.
func (s *PricingStore) CalculateCost(ctx context.Context, modelID string, inputTokens, outputTokens int) *float64 {
	inPrice, outPrice, found := s.GetPrice(ctx, modelID)
	if !found {
		return nil
	}
	// Mentor's rule: use top-level completion_tokens only (not reasoning_tokens)
	cost := float64(inputTokens)*inPrice + float64(outputTokens)*outPrice
	return &cost
}

// LogUsage records a call with cost.
func (s *PricingStore) LogUsage(ctx context.Context, tenantID, soulID, keyID, modelID string, inputTokens, outputTokens int, costUSD *float64) {
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO soul_usage (tenant_id, soul_id, provider_key_id, model_id, input_tokens, output_tokens, cost_usd)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenantID, soulID, nilStr(keyID), modelID, inputTokens, outputTokens, costUSD,
	); err != nil {
		slog.Info("pricing.log_usage.err", "soul", soulID, "model", modelID, "err", err)
	}
}

// GetSoulSpend returns total spend for a soul in a time range.
func (s *PricingStore) GetSoulSpend(ctx context.Context, soulID string, since time.Time) (totalCost float64, totalCalls int, totalTokens int) {
	s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0), COUNT(*), COALESCE(SUM(input_tokens + output_tokens), 0)
		 FROM soul_usage WHERE soul_id = $1 AND called_at >= $2`, soulID, since,
	).Scan(&totalCost, &totalCalls, &totalTokens)
	return
}

// GetAccountSpend returns total spend for the tenant this month.
func (s *PricingStore) GetAccountSpend(ctx context.Context, tenantID string) (totalCost float64) {
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0) FROM soul_usage WHERE tenant_id = $1 AND called_at >= $2`,
		tenantID, monthStart).Scan(&totalCost)
	return
}

// CheckBudget returns whether a soul can make a call based on budget limits.
func (s *PricingStore) CheckBudget(ctx context.Context, tenantID, soulID string) (allowed bool, reason string) {
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Check soul monthly budget
	var monthlyBudget, dailyBudget float64
	var onLimit string
	s.pool.QueryRow(ctx,
		`SELECT COALESCE(monthly_budget_usd, 0), COALESCE(daily_budget_usd, 0), COALESCE(on_limit_action, 'pause') FROM agents WHERE id = $1`,
		soulID).Scan(&monthlyBudget, &dailyBudget, &onLimit)

	if monthlyBudget > 0 {
		monthSpend, _, _ := s.GetSoulSpend(ctx, soulID, monthStart)
		if monthSpend >= monthlyBudget {
			return false, fmt.Sprintf("monthly budget exceeded ($%.2f/$%.2f)", monthSpend, monthlyBudget)
		}
	}

	if dailyBudget > 0 {
		daySpend, _, _ := s.GetSoulSpend(ctx, soulID, dayStart)
		if daySpend >= dailyBudget {
			return false, fmt.Sprintf("daily budget exceeded ($%.2f/$%.2f)", daySpend, dailyBudget)
		}
	}

	return true, ""
}

func nilStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
