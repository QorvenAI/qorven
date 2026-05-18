// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scenario

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Create(ctx context.Context, tenantID, name, seed string, agentCount, rounds int) (*Project, error) {
	p := &Project{ID: uuid.New().String(), TenantID: tenantID, Name: name, Seed: seed,
		AgentCount: agentCount, Rounds: rounds, Status: StatusCreated, CreatedAt: time.Now()}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO scenarios (id, tenant_id, name, seed, agent_count, rounds, status, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		p.ID, p.TenantID, p.Name, p.Seed, p.AgentCount, p.Rounds, p.Status, p.CreatedAt)
	return p, err
}

func (s *Store) Get(ctx context.Context, id string) (*Project, error) {
	var p Project
	var report, roundsJSON *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, name, seed, agent_count, rounds, status, report, rounds_data, created_at, completed_at FROM scenarios WHERE id = $1`, id).
		Scan(&p.ID, &p.TenantID, &p.Name, &p.Seed, &p.AgentCount, &p.Rounds, &p.Status, &report, &roundsJSON, &p.CreatedAt, &p.CompletedAt)
	if report != nil { p.Report = *report }
	if roundsJSON != nil { json.Unmarshal([]byte(*roundsJSON), &p.RoundsData) }
	return &p, err
}

func (s *Store) List(ctx context.Context, tenantID string) ([]*Project, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, seed, agent_count, rounds, status, created_at FROM scenarios WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 50`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []*Project{}
	for rows.Next() {
		var p Project
		rows.Scan(&p.ID, &p.Name, &p.Seed, &p.AgentCount, &p.Rounds, &p.Status, &p.CreatedAt)
		out = append(out, &p)
	}
	return out, nil
}

func (s *Store) UpdateStatus(ctx context.Context, id string, status Status) error {
	_, err := s.pool.Exec(ctx, `UPDATE scenarios SET status = $1 WHERE id = $2`, status, id)
	return err
}

func (s *Store) SaveReport(ctx context.Context, id, report string) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx, `UPDATE scenarios SET report = $1, status = $2, completed_at = $3 WHERE id = $4`,
		report, StatusCompleted, now, id)
	return err
}

func (s *Store) SaveRounds(ctx context.Context, id string, rounds []Round) error {
	data, _ := json.Marshal(rounds)
	_, err := s.pool.Exec(ctx, `UPDATE scenarios SET rounds_data = $1 WHERE id = $2`, string(data), id)
	return err
}

func (s *Store) GetRounds(ctx context.Context, id string) ([]Round, error) {
	var data *string
	err := s.pool.QueryRow(ctx, `SELECT rounds_data FROM scenarios WHERE id=$1`, id).Scan(&data)
	if err != nil || data == nil { return nil, err }
	rounds := []Round{}
	json.Unmarshal([]byte(*data), &rounds)
	return rounds, nil
}
