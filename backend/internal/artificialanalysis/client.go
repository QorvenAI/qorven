// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package artificialanalysis wraps the Artificial Analysis REST API
// (https://artificialanalysis.ai/api-reference) and caches the ranked
// model list in Postgres. Data is merged into the static
// providers.StaticModelCatalog so each entry gets up-to-date benchmark
// scores, speed metrics, and overall rank.
//
// Attribution: data sourced from https://artificialanalysis.ai — link
// must be shown wherever rankings are displayed (API terms requirement).
//
// Authentication: x-api-key header.
// Rate limit: 1 000 requests/day on free tier — cache aggressively.
package artificialanalysis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const DefaultBaseURL = "https://api.artificialanalysis.ai"

// Client is a thin HTTP wrapper around the Artificial Analysis API.
// Safe for concurrent use.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// New returns a client configured against the public API. Pass an empty
// APIKey to skip — the enrichment loop will no-op.
func New(apiKey string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// ModelCreator is the organisation that produced the model.
type ModelCreator struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Evaluations holds benchmark indices from the /data/llms/models response.
type Evaluations struct {
	IntelligenceIndex float64 `json:"artificial_analysis_intelligence_index"`
	CodingIndex       float64 `json:"artificial_analysis_coding_index"`
	MathIndex         float64 `json:"artificial_analysis_math_index"`
	MMLUPro           float64 `json:"mmlu_pro"`
	GPQA              float64 `json:"gpqa"`
	LiveCodeBench     float64 `json:"livecodebench"`
}

// Pricing holds per-million-token costs.
type Pricing struct {
	InputPerM  float64 `json:"input_tokens"`
	OutputPerM float64 `json:"output_tokens"`
}

// Model is the subset of /data/llms/models fields we use.
type Model struct {
	ID                          string       `json:"id"`
	Name                        string       `json:"name"`
	Slug                        string       `json:"slug"`
	ModelCreator                ModelCreator `json:"model_creator"`
	Evaluations                 Evaluations  `json:"evaluations"`
	Pricing                     Pricing      `json:"pricing"`
	MedianOutputTokensPerSecond float64      `json:"median_output_tokens_per_second"`
	MedianTimeToFirstTokenSecs  float64      `json:"median_time_to_first_token_seconds"`
}

// ListModels hits GET /data/llms/models. Returns the full ranked list
// and an error if the request failed.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	if c.APIKey == "" {
		return nil, fmt.Errorf("artificialanalysis: no API key configured")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/data/llms/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artificialanalysis: GET /data/llms/models → HTTP %d", resp.StatusCode)
	}
	out := []Model{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("artificialanalysis: decode: %w", err)
	}
	return out, nil
}

// ─── Cache ──────────────────────────────────────────────────────────────────

// EnsureCacheTable creates the cache table if needed. Idempotent.
func EnsureCacheTable(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}
	_, err := pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS artificial_analysis_cache (
  key        TEXT PRIMARY KEY,
  data       JSONB NOT NULL,
  fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`)
	return err
}

// CachedModels reads the last-cached snapshot out of Postgres.
// Returns nil, time.Time{}, nil when nothing is cached.
func CachedModels(ctx context.Context, pool *pgxpool.Pool) ([]Model, time.Time, error) {
	if pool == nil {
		return nil, time.Time{}, nil
	}
	raw := []byte{}
	var fetchedAt time.Time
	err := pool.QueryRow(ctx,
		`SELECT data, fetched_at FROM artificial_analysis_cache WHERE key = 'models'`,
	).Scan(&raw, &fetchedAt)
	if err != nil {
		return nil, time.Time{}, nil
	}
	out := []Model{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, time.Time{}, fmt.Errorf("artificialanalysis: cached payload malformed: %w", err)
	}
	return out, fetchedAt, nil
}

// WriteCache persists a /data/llms/models snapshot under key='models'.
func WriteCache(ctx context.Context, pool *pgxpool.Pool, models []Model) error {
	if pool == nil {
		return nil
	}
	data, err := json.Marshal(models)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `
INSERT INTO artificial_analysis_cache (key, data, fetched_at) VALUES ('models', $1, now())
ON CONFLICT (key) DO UPDATE SET data = EXCLUDED.data, fetched_at = EXCLUDED.fetched_at`, data)
	return err
}
