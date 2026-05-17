// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package providers

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DiscoveryScanner runs daily, calls FetchModelsLive for each provider key,
// and records any newly-seen model IDs in model_discoveries.
type DiscoveryScanner struct {
	pool          *pgxpool.Pool
	encryptionKey string
	tenantID      string
	// OnNew is called when a new model is found; use it to emit notifications.
	OnNew func(tenantID, providerID, modelID string)
}

// NewDiscoveryScanner creates a scanner for a single tenant.
func NewDiscoveryScanner(pool *pgxpool.Pool, encKey, tenantID string) *DiscoveryScanner {
	return &DiscoveryScanner{pool: pool, encryptionKey: encKey, tenantID: tenantID}
}

// Start runs the scanner once immediately, then once every 24 hours.
func (d *DiscoveryScanner) Start(ctx context.Context) {
	go func() {
		d.run(ctx)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				d.run(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// run executes one full discovery pass.
func (d *DiscoveryScanner) run(ctx context.Context) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, provider_type, COALESCE(api_base,''), COALESCE(api_key, '')
		 FROM providers
		 WHERE tenant_id = $1 AND enabled = true AND api_key IS NOT NULL AND length(api_key) > 0`,
		d.tenantID)
	if err != nil {
		slog.Warn("discovery.list_providers_failed", "error", err)
		return
	}
	type prow struct {
		id, ptype, base, encKey string
	}
	provs := []prow{}
	for rows.Next() {
		var p prow
		rows.Scan(&p.id, &p.ptype, &p.base, &p.encKey)
		provs = append(provs, p)
	}
	rows.Close()

	if len(provs) == 0 {
		return
	}

	keyStore := NewKeyPoolStore(d.pool, d.encryptionKey)

	for _, p := range provs {
		// Pick first verified key for this provider
		keys, err := keyStore.ListKeys(ctx, d.tenantID, p.id)
		if err != nil || len(keys) == 0 {
			continue
		}
		var rawKey string
		for _, k := range keys {
			if k.Status == "verified" {
				dk, err := DecryptKeyBytes(k.EncryptedKey(), d.encryptionKey)
				if err == nil {
					rawKey = string(dk)
					break
				}
			}
		}
		if rawKey == "" {
			continue
		}

		models, err := FetchModelsLive(ctx, p.ptype, p.base, rawKey)
		if err != nil {
			slog.Debug("discovery.fetch_failed", "provider", p.id, "error", err)
			// Update last_run so we don't hammer a broken key
			d.pool.Exec(ctx, `UPDATE provider_keys SET discovery_last_run_at = now()
				WHERE provider_id = $1 AND tenant_id = $2 AND status = 'verified'`,
				p.id, d.tenantID)
			continue
		}

		newCount := 0
		for _, m := range models {
			tag, err := d.pool.Exec(ctx,
				`INSERT INTO model_discoveries (tenant_id, provider_id, model_id)
				 VALUES ($1, $2, $3)
				 ON CONFLICT (tenant_id, provider_id, model_id) DO NOTHING`,
				d.tenantID, p.id, m.ID)
			if err != nil {
				slog.Warn("discovery.insert_failed", "provider", p.id, "model", m.ID, "error", err)
				continue
			}
			if tag.RowsAffected() > 0 {
				newCount++
				if d.OnNew != nil {
					d.OnNew(d.tenantID, p.id, m.ID)
				}
			}
		}

		// Stamp last run on all verified keys for this provider
		d.pool.Exec(ctx, `UPDATE provider_keys SET discovery_last_run_at = now()
			WHERE provider_id = $1 AND tenant_id = $2 AND status = 'verified'`,
			p.id, d.tenantID)

		if newCount > 0 {
			slog.Info("discovery.new_models", "provider", p.id, "new", newCount, "total", len(models))
		}
	}
}
