// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package pricing fetches model pricing from qorven.ai/data/model-pricing.json
// (the canonical Qorven pricing feed) and feeds it into the gateway cost ledger.
// Falls back to direct LiteLLM fetch if the canonical URL is unreachable.
package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	gatewayllm "github.com/qorvenai/qorven/internal/gateway/llm"
	"github.com/qorvenai/qorven/internal/providers"
)

const (
	canonicalURL = "https://qorven.ai/data/model-pricing.json"
	fallbackURL  = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
	maxBodyBytes = 20 * 1024 * 1024 // 20 MiB
)

// PricingSource holds per-source pricing data for a single model.
type PricingSource struct {
	Input        float64 `json:"input"`
	Output       float64 `json:"output"`
	Intelligence float64 `json:"intelligence,omitempty"`
}

// PricingModel is one model entry from the canonical feed.
type PricingModel struct {
	ID                string                    `json:"id"`
	Name              string                    `json:"name"`
	Provider          string                    `json:"provider"`
	InputPer1M        float64                   `json:"input_per_1m"`
	OutputPer1M       float64                   `json:"output_per_1m"`
	CacheWritePer1M   float64                   `json:"cache_write_per_1m"`
	CacheReadPer1M    float64                   `json:"cache_read_per_1m"`
	ContextWindow     int                       `json:"context_window"`
	IntelligenceIndex float64                   `json:"intelligence_index"`
	CodingIndex       float64                   `json:"coding_index"`
	SpeedTPS          float64                   `json:"speed_tps"`
	Sources           map[string]PricingSource  `json:"sources"`
}

// PricingFeed is the root structure of model-pricing.json.
type PricingFeed struct {
	Version   string         `json:"version"`
	UpdatedAt string         `json:"updated_at"`
	Models    []PricingModel `json:"models"`
}

// litellmEntry is one entry from the raw LiteLLM JSON (fallback format).
type litellmEntry struct {
	InputCostPerToken  float64 `json:"input_cost_per_token"`
	OutputCostPerToken float64 `json:"output_cost_per_token"`
	MaxTokens          int     `json:"max_tokens"`
	MaxInputTokens     int     `json:"max_input_tokens"`
}

// Aggregator fetches model pricing and populates the gateway pricing table
// and the model_pricing_sources DB table.
type Aggregator struct {
	db         *pgxpool.Pool
	httpClient *http.Client
}

// NewAggregator creates an Aggregator wired to the given DB pool.
func NewAggregator(db *pgxpool.Pool) *Aggregator {
	return &Aggregator{
		db: db,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SeedFromBuiltin immediately loads pricing from providers.ModelRegistry into
// UpdatePricingTable. Called at startup before the first network fetch so
// cost accounting works immediately without any external calls.
func (a *Aggregator) SeedFromBuiltin() {
	entries := make(map[string]gatewayllm.ModelPricing, len(providers.ModelRegistry))
	for id, spec := range providers.ModelRegistry {
		if spec.InputCostPer1M == 0 && spec.OutputCostPer1M == 0 {
			continue
		}
		entries[id] = gatewayllm.ModelPricing{
			InputPer1M:  spec.InputCostPer1M,
			OutputPer1M: spec.OutputCostPer1M,
		}
	}
	gatewayllm.UpdatePricingTable(entries)
	slog.Info("pricing.seed_builtin", "models", len(entries))
}

// Refresh fetches fresh pricing from the canonical feed (with LiteLLM fallback),
// updates model_pricing_sources and model_pricing in DB, and refreshes the
// in-process pricing table. Returns the number of models updated.
// Safe to call from a goroutine.
func (a *Aggregator) Refresh(ctx context.Context) (int, error) {
	feed, err := a.fetchCanonical(ctx)
	if err != nil {
		slog.Warn("pricing.canonical_failed", "error", err, "trying_fallback", fallbackURL)
		feed, err = a.fetchFromLiteLLM(ctx)
		if err != nil {
			return 0, err
		}
	}

	entries := make(map[string]gatewayllm.ModelPricing, len(feed.Models))
	for _, m := range feed.Models {
		entries[m.ID] = gatewayllm.ModelPricing{
			InputPer1M:  m.InputPer1M,
			OutputPer1M: m.OutputPer1M,
			CacheWrite:  m.CacheWritePer1M,
			CacheRead:   m.CacheReadPer1M,
		}
	}

	// Update in-process pricing table first (no DB dependency).
	gatewayllm.UpdatePricingTable(entries)

	// Persist to DB if available.
	if a.db != nil {
		a.persistToDB(ctx, feed.Models)
	}

	return len(feed.Models), nil
}

// fetchCanonical downloads and parses the canonical feed from qorven.ai.
func (a *Aggregator) fetchCanonical(ctx context.Context) (*PricingFeed, error) {
	body, err := a.get(ctx, canonicalURL)
	if err != nil {
		return nil, err
	}
	var feed PricingFeed
	if err := json.Unmarshal(body, &feed); err != nil {
		return nil, err
	}
	if len(feed.Models) == 0 {
		return nil, errorf("canonical feed has 0 models")
	}
	slog.Info("pricing.canonical_ok", "models", len(feed.Models), "updated_at", feed.UpdatedAt)
	return &feed, nil
}

// fetchFromLiteLLM falls back to the raw LiteLLM JSON and converts it to a
// PricingFeed. No benchmark data is available in this format.
func (a *Aggregator) fetchFromLiteLLM(ctx context.Context) (*PricingFeed, error) {
	body, err := a.get(ctx, fallbackURL)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	feed := &PricingFeed{Version: "litellm-fallback", UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	for id, data := range raw {
		var entry litellmEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}
		if entry.InputCostPerToken == 0 && entry.OutputCostPerToken == 0 {
			continue
		}
		ctx := entry.MaxInputTokens
		if ctx == 0 {
			ctx = entry.MaxTokens
		}
		m := PricingModel{
			ID:            id,
			Name:          id,
			InputPer1M:    roundPrice(entry.InputCostPerToken * 1_000_000),
			OutputPer1M:   roundPrice(entry.OutputCostPerToken * 1_000_000),
			ContextWindow: ctx,
			Sources:       map[string]PricingSource{"litellm": {Input: roundPrice(entry.InputCostPerToken * 1_000_000), Output: roundPrice(entry.OutputCostPerToken * 1_000_000)}},
		}
		feed.Models = append(feed.Models, m)
	}
	slog.Info("pricing.litellm_fallback_ok", "models", len(feed.Models))
	return feed, nil
}

// persistToDB upserts all models into model_pricing_sources (per source) and
// into model_pricing (backward compat with PricingStore.GetPrice).
func (a *Aggregator) persistToDB(ctx context.Context, models []PricingModel) {
	now := time.Now().UTC()
	count := 0

	for _, m := range models {
		// Upsert into model_pricing (backward compat).
		if _, err := a.db.Exec(ctx,
			`INSERT INTO model_pricing (model_id, provider, input_cost_per_token, output_cost_per_token, context_window, source, updated_at)
			 VALUES ($1, $2, $3, $4, $5, 'qorven_feed', now())
			 ON CONFLICT (model_id) DO UPDATE SET
			   provider              = $2,
			   input_cost_per_token  = $3,
			   output_cost_per_token = $4,
			   context_window        = $5,
			   source                = 'qorven_feed',
			   updated_at            = now()`,
			m.ID, m.Provider,
			m.InputPer1M/1_000_000,
			m.OutputPer1M/1_000_000,
			m.ContextWindow,
		); err != nil {
			slog.Error("pricing.db_upsert_model_pricing", "model", m.ID, "err", err)
		}

		// Upsert each source row into model_pricing_sources.
		for srcName, srcData := range m.Sources {
			if _, err := a.db.Exec(ctx,
				`INSERT INTO model_pricing_sources
				   (model_id, source, provider, input_per_1m, output_per_1m,
				    cache_write_per_1m, cache_read_per_1m,
				    intelligence_index, coding_index, speed_tps,
				    context_window, fetched_at)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
				 ON CONFLICT (model_id, source) DO UPDATE SET
				   provider           = $3,
				   input_per_1m       = $4,
				   output_per_1m      = $5,
				   cache_write_per_1m = $6,
				   cache_read_per_1m  = $7,
				   intelligence_index = $8,
				   coding_index       = $9,
				   speed_tps          = $10,
				   context_window     = $11,
				   fetched_at         = $12`,
				m.ID, srcName, m.Provider,
				srcData.Input, srcData.Output,
				m.CacheWritePer1M, m.CacheReadPer1M,
				m.IntelligenceIndex, m.CodingIndex, m.SpeedTPS,
				m.ContextWindow, now,
			); err != nil {
				slog.Error("pricing.db_upsert_sources", "model", m.ID, "source", srcName, "err", err)
				continue
			}
			count++
		}
	}
	slog.Info("pricing.db_persisted", "source_rows", count, "models", len(models))
}

// get performs a GET request and returns the body bytes.
func (a *Aggregator) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
}

func roundPrice(v float64) float64 {
	// Round to 4 decimal places to match the JS script behaviour.
	r := v * 10000
	if r < 0 {
		r -= 0.5
	} else {
		r += 0.5
	}
	return float64(int64(r)) / 10000
}

func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
