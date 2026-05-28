// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package governance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CapacityForecast struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	PeriodStart  time.Time `json:"period_start"`
	PeriodEnd    time.Time `json:"period_end"`
	Department   string    `json:"department"`
	MetricName   string    `json:"metric_name"`
	CurrentValue float64   `json:"current_value"`
	Forecast     float64   `json:"forecast_value"`
	Confidence   float64   `json:"confidence"`
	CreatedAt    time.Time `json:"created_at"`
}

type ForecastStore struct {
	db *pgxpool.Pool
}

func NewForecastStore(db *pgxpool.Pool) *ForecastStore {
	return &ForecastStore{db: db}
}

func (s *ForecastStore) List(ctx context.Context, tenantID string, limit int) ([]CapacityForecast, error) {
	rows, err := s.db.Query(ctx, `SELECT id, tenant_id, period_start, period_end, department, metric_name, current_value, forecast_value, confidence, created_at FROM capacity_forecasts WHERE tenant_id = $1 ORDER BY period_start DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CapacityForecast
	for rows.Next() {
		var f CapacityForecast
		if err := rows.Scan(&f.ID, &f.TenantID, &f.PeriodStart, &f.PeriodEnd, &f.Department, &f.MetricName, &f.CurrentValue, &f.Forecast, &f.Confidence, &f.CreatedAt); err != nil {
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

func (s *ForecastStore) Record(ctx context.Context, f CapacityForecast) error {
	_, err := s.db.Exec(ctx, `INSERT INTO capacity_forecasts (tenant_id, period_start, period_end, department, metric_name, current_value, forecast_value, confidence) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		f.TenantID, f.PeriodStart, f.PeriodEnd, f.Department, f.MetricName, f.CurrentValue, f.Forecast, f.Confidence)
	return err
}
