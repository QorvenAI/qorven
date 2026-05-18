// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package llmstats wraps the LLM Stats REST API (https://llm-stats.com)
// and caches the live model catalog in Postgres. Data is merged into the
// static providers.StaticModelCatalog so each entry gets up-to-date
// pricing, context length, and modality information where available.
//
// Scope note: the LLM Stats public API currently only exposes
// GET /v1/models (verified against api.llm-stats.com on 2026-04-20).
// The endpoints mentioned in the Phase G spec — /v1/rankings,
// /v1/benchmarks, /v1/updates — all return 404. We fetch only /v1/models
// and leave the BenchmarkScores / OverallRank / CategoryRanks
// enrichment fields unpopulated rather than fake data. If LLM Stats
// ships ranking endpoints later, this package is the place to wire them.
package llmstats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Client is a thin HTTP wrapper around the LLM Stats API. Safe for
// concurrent use.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

const DefaultBaseURL = "https://api.llm-stats.com/v1"

// New returns a client configured against the public API. Pass an empty
// APIKey to skip — the enrichment loop will no-op.
func New(apiKey string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Model is the subset of LLM Stats /v1/models fields we care about.
// Other fields (fallback_providers, is_fallback, quantization_type) are
// ignored — nothing else on the server consumes them.
type Model struct {
	ID               string    `json:"id"`
	DisplayName      string    `json:"display_name"`
	ModelName        string    `json:"model_name"`
	OrganizationID   string    `json:"organization_id"`
	OrganizationName string    `json:"organization_name"`
	ProviderName     string    `json:"provider_name"`
	ContextLength    int       `json:"context_length"`
	InputPrice       float64   `json:"input_price"`
	OutputPrice      float64   `json:"output_price"`
	InputModalities  []string  `json:"input_modalities"`
	OutputModalities []string  `json:"output_modalities"`
	RoutingProviders []string  `json:"routing_providers"`
}

// ListModels hits GET /v1/models. Returns a snapshot and an error if
// the request failed — the caller decides whether to use a cached copy.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("llmstats: no API key configured")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/models", nil)
	if err != nil { return nil, err }
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("llmstats: GET /models → HTTP %d", resp.StatusCode)
	}
	out := []Model{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("llmstats: decode: %w", err)
	}
	return out, nil
}

// ─── Cache ─────────────────────────────────────────────────────────────────

// EnsureCacheTable creates the cache table if needed. Idempotent.
// Not a pgx migration — we don't want to add a migration file for one
// optional feature. The table is a simple KV + JSONB payload.
func EnsureCacheTable(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil { return nil }
	_, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS llm_stats_cache (
  key        TEXT PRIMARY KEY,
  data       JSONB NOT NULL,
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`)
	return err
}

// CachedModels reads the last-cached /models snapshot out of Postgres,
// returning nil + nil err if nothing is cached.
func CachedModels(ctx context.Context, pool *pgxpool.Pool) ([]Model, time.Time, error) {
	if pool == nil { return nil, time.Time{}, nil }
	raw := []byte{}
	var fetchedAt time.Time
	err := pool.QueryRow(ctx, `SELECT data, fetched_at FROM llm_stats_cache WHERE key = 'models'`).Scan(&raw, &fetchedAt)
	if err != nil { return nil, time.Time{}, nil }
	out := []Model{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, time.Time{}, fmt.Errorf("llmstats: cached payload malformed: %w", err)
	}
	return out, fetchedAt, nil
}

// WriteCache persists a /models snapshot under key='models'.
func WriteCache(ctx context.Context, pool *pgxpool.Pool, models []Model) error {
	if pool == nil { return nil }
	data, err := json.Marshal(models)
	if err != nil { return err }
	_, err = pool.Exec(ctx, `
INSERT INTO llm_stats_cache (key, data, fetched_at) VALUES ('models', $1, now())
ON CONFLICT (key) DO UPDATE SET data = EXCLUDED.data, fetched_at = EXCLUDED.fetched_at`, data)
	return err
}
