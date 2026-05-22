// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	gatewayllm "github.com/qorvenai/qorven/internal/gateway/llm"
)

// handleGatewayPricingGaps returns models that have unpriced calls in
// gateway_spend_raw (pricing_missing=true), ordered by token volume.
//
// GET /v1/gateway/pricing/gaps
func (gw *Gateway) handleGatewayPricingGaps(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	gaps, err := gatewayllm.QueryPricingGaps(r.Context(), gw.db.Pool)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	if gaps == nil {
		gaps = []gatewayllm.PricingGap{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"gaps":  gaps,
		"total": len(gaps),
	})
}

// handleGatewayPricingBackfill triggers an immediate backfill of all
// pricing_missing=true rows using the current in-process pricing table.
// Also kicks off a full aggregator refresh so newly added prices are loaded first.
//
// POST /v1/gateway/pricing/backfill
func (gw *Gateway) handleGatewayPricingBackfill(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	// Refresh pricing table first so any newly added prices are in memory.
	if gw.pricingAgg != nil {
		_, _ = gw.pricingAgg.Refresh(r.Context())
	}

	result, err := gatewayllm.BackfillMissingPrices(r.Context(), gw.db.Pool)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleGatewayPricingSet lets an admin (or Prime) set a custom price for
// a model. Stored in model_pricing with source='manual'. Immediately triggers
// backfill so any past calls with pricing_missing=true get repriced.
//
// PUT /v1/gateway/pricing/{modelId}
// Body: { "input_per_1m": 3.0, "output_per_1m": 15.0,
//         "cache_write_per_1m": 3.75, "cache_read_per_1m": 0.30 }
func (gw *Gateway) handleGatewayPricingSet(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	modelID := chi.URLParam(r, "modelId")
	if modelID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model_id required"})
		return
	}

	var body struct {
		InputPer1M      float64 `json:"input_per_1m"`
		OutputPer1M     float64 `json:"output_per_1m"`
		CacheWritePer1M float64 `json:"cache_write_per_1m"`
		CacheReadPer1M  float64 `json:"cache_read_per_1m"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.InputPer1M == 0 && body.OutputPer1M == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one of input_per_1m or output_per_1m must be non-zero"})
		return
	}

	// Persist to model_pricing with source='manual' (overrides feed).
	_, err := gw.db.Pool.Exec(r.Context(), `
		INSERT INTO model_pricing
			(model_id, provider, input_cost_per_token, output_cost_per_token, context_window, source, updated_at)
		VALUES ($1, '', $2, $3, 0, 'manual', now())
		ON CONFLICT (model_id) DO UPDATE SET
			input_cost_per_token  = $2,
			output_cost_per_token = $3,
			source                = 'manual',
			updated_at            = now()
	`,
		modelID,
		body.InputPer1M/1_000_000,
		body.OutputPer1M/1_000_000,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	// Update the in-process pricing table immediately.
	gatewayllm.UpdatePricingTable(map[string]gatewayllm.ModelPricing{
		modelID: {
			InputPer1M:  body.InputPer1M,
			OutputPer1M: body.OutputPer1M,
			CacheWrite:  body.CacheWritePer1M,
			CacheRead:   body.CacheReadPer1M,
		},
	})

	// Backfill past calls for this model immediately.
	result, err := gatewayllm.BackfillMissingPrices(r.Context(), gw.db.Pool)
	if err != nil {
		// Pricing saved, backfill failed — not fatal.
		writeJSON(w, http.StatusOK, map[string]any{
			"model_id": modelID,
			"saved":    true,
			"backfill": map[string]any{"error": sanitizeError(err)},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"model_id": modelID,
		"saved":    true,
		"backfill": result,
	})
}

// handleGatewayPricingList returns all models in the pricing table with their
// current rate and source (litellm / qorven_feed / manual / missing).
//
// GET /v1/gateway/pricing
func (gw *Gateway) handleGatewayPricingList(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	// Optional filter: ?source=manual or ?missing=true
	filterSource  := r.URL.Query().Get("source")
	filterMissing := r.URL.Query().Get("missing") == "true"

	type pricingRow struct {
		ModelID         string  `json:"model_id"`
		Provider        string  `json:"provider"`
		InputPer1M      float64 `json:"input_per_1m"`
		OutputPer1M     float64 `json:"output_per_1m"`
		Source          string  `json:"source"`
		UpdatedAt       string  `json:"updated_at"`
	}

	var rows []pricingRow

	if filterMissing {
		// Return models that have unpriced calls but no entry in model_pricing.
		gaps, err := gatewayllm.QueryPricingGaps(r.Context(), gw.db.Pool)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
			return
		}
		for _, g := range gaps {
			rows = append(rows, pricingRow{
				ModelID:    g.ModelID,
				Provider:   g.ProviderID,
				Source:     "missing",
			})
		}
	} else {
		query := `
			SELECT model_id, COALESCE(provider,''),
			       COALESCE(input_cost_per_token,0)  * 1000000,
			       COALESCE(output_cost_per_token,0) * 1000000,
			       COALESCE(source,'unknown'),
			       updated_at::text
			FROM model_pricing
		`
		args := []any{}
		if filterSource != "" {
			query += " WHERE source = $1"
			args = append(args, filterSource)
		}
		query += " ORDER BY model_id"

		dbRows, err := gw.db.Pool.Query(r.Context(), query, args...)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
			return
		}
		defer dbRows.Close()
		for dbRows.Next() {
			var row pricingRow
			if err := dbRows.Scan(&row.ModelID, &row.Provider, &row.InputPer1M, &row.OutputPer1M, &row.Source, &row.UpdatedAt); err != nil {
				continue
			}
			// Skip if source filter doesn't match
			if filterSource != "" && !strings.EqualFold(row.Source, filterSource) {
				continue
			}
			rows = append(rows, row)
		}
	}

	if rows == nil {
		rows = []pricingRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pricing": rows,
		"total":   len(rows),
	})
}
