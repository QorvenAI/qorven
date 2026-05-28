// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package governance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SLADefinition struct {
	ID          string  `json:"id"`
	TenantID    string  `json:"tenant_id"`
	Name        string  `json:"name"`
	Metric      string  `json:"metric"`
	TargetValue float64 `json:"target_value"`
	TimeWindow  string  `json:"time_window"`
	Scope       string  `json:"scope"`
	ScopeID     string  `json:"scope_id"`
	Enabled     bool    `json:"enabled"`
}

type SLAMeasurement struct {
	ID        string    `json:"id"`
	SLAID     string    `json:"sla_id"`
	Value     float64   `json:"value"`
	Met       bool      `json:"met"`
	MeasuredAt time.Time `json:"measured_at"`
}

type SLAStats struct {
	Total      int     `json:"total"`
	Met        int     `json:"met"`
	Breached   int     `json:"breached"`
	ComplianceRate float64 `json:"compliance_rate"`
}

type SLAStore struct {
	db *pgxpool.Pool
}

func NewSLAStore(db *pgxpool.Pool) *SLAStore {
	return &SLAStore{db: db}
}

func (s *SLAStore) ListDefinitions(ctx context.Context, tenantID string) ([]SLADefinition, error) {
	rows, err := s.db.Query(ctx, `SELECT id, tenant_id, name, metric, target_value, time_window, scope, COALESCE(scope_id,''), enabled FROM sla_definitions WHERE tenant_id = $1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SLADefinition
	for rows.Next() {
		var d SLADefinition
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Name, &d.Metric, &d.TargetValue, &d.TimeWindow, &d.Scope, &d.ScopeID, &d.Enabled); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *SLAStore) RecordMeasurement(ctx context.Context, tenantID, slaID string, value float64, met bool) error {
	_, err := s.db.Exec(ctx, `INSERT INTO sla_measurements (tenant_id, sla_id, measured_value, met) VALUES ($1,$2,$3,$4)`, tenantID, slaID, value, met)
	return err
}

func (s *SLAStore) Stats(ctx context.Context, tenantID string) (*SLAStats, error) {
	var stats SLAStats
	s.db.QueryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(CASE WHEN met THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN NOT met THEN 1 ELSE 0 END),0) FROM sla_measurements WHERE tenant_id = $1 AND measured_at > NOW() - INTERVAL '30 days'`, tenantID).Scan(&stats.Total, &stats.Met, &stats.Breached)
	if stats.Total > 0 {
		stats.ComplianceRate = float64(stats.Met) / float64(stats.Total) * 100
	}
	return &stats, nil
}

func (s *SLAStore) RecentMeasurements(ctx context.Context, tenantID string, limit int) ([]SLAMeasurement, error) {
	rows, err := s.db.Query(ctx, `SELECT m.id, m.sla_id, m.measured_value, m.met, m.measured_at FROM sla_measurements m WHERE m.tenant_id = $1 ORDER BY m.measured_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SLAMeasurement
	for rows.Next() {
		var m SLAMeasurement
		if err := rows.Scan(&m.ID, &m.SLAID, &m.Value, &m.Met, &m.MeasuredAt); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}
