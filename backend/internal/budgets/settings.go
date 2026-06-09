// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package budgets

import "context"

// FinanceSettings holds per-tenant CFO authority configuration.
type FinanceSettings struct {
	Authority    string  `json:"cfo_authority"`     // ask | threshold | full
	ThresholdUSD float64 `json:"cfo_threshold_usd"`
}

// GetFinanceSettings returns the tenant's CFO authority settings, defaulting to
// threshold/$25 when no row exists.
func (s *Store) GetFinanceSettings(ctx context.Context, tenantID string) FinanceSettings {
	fs := FinanceSettings{Authority: "threshold", ThresholdUSD: 25}
	_ = s.db.QueryRow(ctx,
		`SELECT cfo_authority, cfo_threshold_usd FROM tenant_finance_settings WHERE tenant_id = $1`,
		tenantID).Scan(&fs.Authority, &fs.ThresholdUSD)
	return fs
}

// SetFinanceSettings upserts the tenant's CFO authority settings.
func (s *Store) SetFinanceSettings(ctx context.Context, tenantID string, fs FinanceSettings) error {
	if fs.Authority == "" {
		fs.Authority = "threshold"
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO tenant_finance_settings (tenant_id, cfo_authority, cfo_threshold_usd, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (tenant_id) DO UPDATE
		SET cfo_authority = EXCLUDED.cfo_authority, cfo_threshold_usd = EXCLUDED.cfo_threshold_usd, updated_at = now()
	`, tenantID, fs.Authority, fs.ThresholdUSD)
	return err
}
