// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package artificialanalysis

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qorvenai/qorven/internal/providers"
)

// RefreshAndMerge fetches /data/llms/models (or reuses the DB cache when
// fresh) and merges rankings + benchmark scores into the static catalog.
// Safe to call on a goroutine — all errors are logged and no-op.
func RefreshAndMerge(ctx context.Context, client *Client, pool *pgxpool.Pool, maxAge time.Duration) (merged int, fromCache bool) {
	if client == nil || client.APIKey == "" {
		return 0, false
	}

	if err := EnsureCacheTable(ctx, pool); err != nil {
		slog.Warn("artificialanalysis.cache_table_failed", "error", err)
	}

	cached, fetchedAt, err := CachedModels(ctx, pool)
	if err != nil {
		slog.Warn("artificialanalysis.cache_read_failed", "error", err)
	}
	if len(cached) > 0 && time.Since(fetchedAt) < maxAge {
		if m := applyToStaticCatalog(cached, fetchedAt); m > 0 {
			slog.Info("artificialanalysis.merged_from_cache", "merged", m, "fetched_at", fetchedAt.Format(time.RFC3339))
			return m, true
		}
	}

	models, err := client.ListModels(ctx)
	if err != nil {
		if len(cached) > 0 {
			slog.Warn("artificialanalysis.live_fetch_failed_using_cache", "error", err, "cache_age", time.Since(fetchedAt))
			if m := applyToStaticCatalog(cached, fetchedAt); m > 0 {
				return m, true
			}
		} else {
			slog.Warn("artificialanalysis.live_fetch_failed_no_cache", "error", err)
		}
		return 0, false
	}
	if err := WriteCache(ctx, pool, models); err != nil {
		slog.Warn("artificialanalysis.cache_write_failed", "error", err)
	}
	m := applyToStaticCatalog(models, time.Now())
	slog.Info("artificialanalysis.merged_live", "merged", m, "models_available", len(models))
	return m, false
}

// applyToStaticCatalog writes AA benchmark scores, rank, speed, and
// pricing into the static catalog entries. The AA list is returned in
// rank order (index 0 = best), so we assign rank = index+1.
func applyToStaticCatalog(models []Model, fetchedAt time.Time) int {
	catalog, err := providers.LoadCatalog()
	if err != nil || catalog == nil {
		return 0
	}

	updatedAt := fetchedAt.UTC().Format(time.RFC3339)
	updates := map[string]providers.ModelCatalogEntry{}

	for i, m := range models {
		rank := i + 1
		entry := providers.ModelCatalogEntry{
			DisplayName: m.Name,
			OverallRank: rank,
			BenchmarkScores: map[string]float64{
				"intelligence_index": m.Evaluations.IntelligenceIndex,
				"coding_index":       m.Evaluations.CodingIndex,
				"math_index":         m.Evaluations.MathIndex,
				"mmlu_pro":           m.Evaluations.MMLUPro,
				"gpqa":               m.Evaluations.GPQA,
				"livecodebench":      m.Evaluations.LiveCodeBench,
			},
			Pricing: providers.CatalogPricing{
				InputPerM:  m.Pricing.InputPerM,
				OutputPerM: m.Pricing.OutputPerM,
			},
			LLMStatsUpdated: updatedAt,
		}

		// Index under every plausible key form so the catalog's
		// MergeEnrichment looseVariants() can match provider-prefixed IDs.
		for _, key := range []string{m.ID, m.Slug, strings.ToLower(m.ID), strings.ToLower(m.Slug)} {
			if key == "" {
				continue
			}
			updates[key] = entry
		}
	}
	return catalog.MergeEnrichment(updates)
}

// RankedModel is the wire shape returned by the /v1/routing/model-rankings
// endpoint. Populated from the Postgres cache so the handler stays fast.
type RankedModel struct {
	Rank                        int     `json:"rank"`
	ID                          string  `json:"id"`
	Name                        string  `json:"name"`
	Organization                string  `json:"organization"`
	IntelligenceIndex           float64 `json:"intelligence_index"`
	CodingIndex                 float64 `json:"coding_index"`
	MathIndex                   float64 `json:"math_index"`
	MedianOutputTokensPerSecond float64 `json:"speed_tokens_per_sec"`
	InputPricePerM              float64 `json:"input_price_per_m"`
	OutputPricePerM             float64 `json:"output_price_per_m"`
}

// GetRankedModels returns the cached model list as RankedModel slices.
// If the cache is empty it returns nil without an error — the caller
// should present a "not configured" state to the user.
func GetRankedModels(ctx context.Context, pool *pgxpool.Pool) ([]RankedModel, time.Time, error) {
	models, fetchedAt, err := CachedModels(ctx, pool)
	if err != nil || len(models) == 0 {
		return nil, time.Time{}, err
	}
	out := make([]RankedModel, 0, len(models))
	for i, m := range models {
		out = append(out, RankedModel{
			Rank:                        i + 1,
			ID:                          m.ID,
			Name:                        m.Name,
			Organization:                m.ModelCreator.Name,
			IntelligenceIndex:           m.Evaluations.IntelligenceIndex,
			CodingIndex:                 m.Evaluations.CodingIndex,
			MathIndex:                   m.Evaluations.MathIndex,
			MedianOutputTokensPerSecond: m.MedianOutputTokensPerSecond,
			InputPricePerM:              m.Pricing.InputPerM,
			OutputPricePerM:             m.Pricing.OutputPerM,
		})
	}
	return out, fetchedAt, nil
}
