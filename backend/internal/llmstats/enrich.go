// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package llmstats

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qorvenai/qorven/internal/providers"
)

// RefreshAndMerge fetches /v1/models (or reuses the DB cache when fresh)
// and merges the result into the static catalog. Safe to call on a
// goroutine with no error handling — all errors are logged and no-op.
//
// maxAge is the cache staleness threshold; pass 24h to get the
// "refresh if older than a day" behaviour described in the Phase G
// spec. If the cache is missing the function always fetches.
func RefreshAndMerge(ctx context.Context, client *Client, pool *pgxpool.Pool, maxAge time.Duration) (merged int, fromCache bool) {
	if client == nil || client.APIKey == "" {
		return 0, false
	}

	if err := EnsureCacheTable(ctx, pool); err != nil {
		slog.Warn("llmstats.cache_table_failed", "error", err)
	}

	// If cache is fresh, skip the network call.
	cached, fetchedAt, err := CachedModels(ctx, pool)
	if err != nil {
		slog.Warn("llmstats.cache_read_failed", "error", err)
	}
	if len(cached) > 0 && time.Since(fetchedAt) < maxAge {
		if m := applyToStaticCatalog(cached, fetchedAt); m > 0 {
			slog.Info("llmstats.merged_from_cache", "merged", m, "fetched_at", fetchedAt.Format(time.RFC3339))
			return m, true
		}
	}

	// Fetch live, write cache.
	models, err := client.ListModels(ctx)
	if err != nil {
		// Fall back to whatever's in the cache, even if stale.
		if len(cached) > 0 {
			slog.Warn("llmstats.live_fetch_failed_using_cache", "error", err, "cache_age", time.Since(fetchedAt))
			if m := applyToStaticCatalog(cached, fetchedAt); m > 0 {
				return m, true
			}
		} else {
			slog.Warn("llmstats.live_fetch_failed_no_cache", "error", err)
		}
		return 0, false
	}
	if err := WriteCache(ctx, pool, models); err != nil {
		slog.Warn("llmstats.cache_write_failed", "error", err)
	}
	m := applyToStaticCatalog(models, time.Now())
	slog.Info("llmstats.merged_live", "merged", m, "models_available", len(models))
	return m, false
}

// applyToStaticCatalog fans each live model into an enrichment entry
// keyed by ID + common aliases (Bedrock inference-profile prefix, bare
// model_name) so the static catalog matches a majority of common IDs.
//
// We intentionally do NOT populate BenchmarkScores / OverallRank /
// CategoryRanks: the LLM Stats public API doesn't expose rankings or
// benchmarks today, only the /models catalog. If they add those
// endpoints later, extend this function rather than faking data.
func applyToStaticCatalog(models []Model, fetchedAt time.Time) int {
	catalog, err := providers.LoadCatalog()
	if err != nil || catalog == nil { return 0 }

	updates := map[string]providers.ModelCatalogEntry{}
	for _, m := range models {
		entry := providers.ModelCatalogEntry{
			DisplayName:     m.DisplayName,
			ContextWindow:   m.ContextLength,
			Pricing:         providers.CatalogPricing{InputPerM: m.InputPrice, OutputPerM: m.OutputPrice},
			LLMStatsUpdated: fetchedAt.UTC().Format(time.RFC3339),
		}
		// Populate capabilities from modalities — at minimum a vision
		// flag lets the UI mark models correctly.
		caps := []string{}
		caps = append(caps, "text", "streaming")
		for _, mod := range m.InputModalities {
			if mod == "image" || mod == "video" { caps = append(caps, "vision"); break }
		}
		entry.Capabilities = caps

		// Index under every plausible ID form. The static catalog's
		// MergeEnrichment looseVariants() handles us./global./eu. prefix
		// stripping; here we forward the bare model_name so "claude-
		// sonnet-4-5" matches entries under "claude-sonnet-4-5-20250929"
		// via a startsWith lookup.
		for _, key := range []string{m.ID, m.ModelName, strings.ToLower(m.ID)} {
			if key == "" { continue }
			updates[key] = entry
		}
	}
	return catalog.MergeEnrichment(updates)
}
