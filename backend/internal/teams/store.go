// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package teams

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Team struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	LeadID    string    `json:"supervisor_id"`
	TenantID  string    `json:"tenant_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Member struct {
	AgentID string `json:"agent_id"`
	Role    string `json:"role"`
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Create(ctx context.Context, name, leadAgentID, tenantID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO crews (name, supervisor_id, tenant_id, created_at)
		 VALUES ($1, $2, $3, NOW()) RETURNING id`, name, leadAgentID, tenantID).Scan(&id)
	if err != nil { return "", fmt.Errorf("teams.create: %w", err) }

	// Auto-add lead as member
	s.pool.Exec(ctx,
		`INSERT INTO crew_members (team_id, agent_id, role) VALUES ($1, $2, 'lead') ON CONFLICT DO NOTHING`,
		id, leadAgentID)
	return id, nil
}

func (s *Store) Get(ctx context.Context, teamID string) (*Team, error) {
	var t Team
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, COALESCE(supervisor_id::text,''), tenant_id, created_at
		 FROM crews WHERE id = $1`, teamID).Scan(&t.ID, &t.Name, &t.LeadID, &t.TenantID, &t.CreatedAt)
	if err != nil { return nil, fmt.Errorf("teams.get: %w", err) }
	return &t, nil
}

func (s *Store) List(ctx context.Context, tenantID string) ([]Team, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, COALESCE(supervisor_id::text,''), tenant_id, created_at
		 FROM crews WHERE tenant_id = $1 OR $1 = '' ORDER BY name`, tenantID)
	if err != nil { return nil, err }
	defer rows.Close()
	teams := []Team{}
	for rows.Next() {
		var t Team
		rows.Scan(&t.ID, &t.Name, &t.LeadID, &t.TenantID, &t.CreatedAt)
		teams = append(teams, t)
	}
	return teams, nil
}

func (s *Store) Delete(ctx context.Context, teamID string) error {
	s.pool.Exec(ctx, `DELETE FROM crew_members WHERE team_id = $1`, teamID)
	_, err := s.pool.Exec(ctx, `DELETE FROM crews WHERE id = $1`, teamID)
	return err
}

func (s *Store) AddMember(ctx context.Context, teamID, agentID, role string) error {
	if role == "" { role = "member" }
	_, err := s.pool.Exec(ctx,
		`INSERT INTO crew_members (team_id, agent_id, role) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		teamID, agentID, role)
	return err
}

func (s *Store) RemoveMember(ctx context.Context, teamID, agentID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM crew_members WHERE team_id = $1 AND agent_id = $2`, teamID, agentID)
	return err
}

func (s *Store) ListMembers(ctx context.Context, teamID string) ([]Member, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT agent_id, role FROM crew_members WHERE team_id = $1 ORDER BY role`, teamID)
	if err != nil { return nil, err }
	defer rows.Close()
	members := []Member{}
	for rows.Next() {
		var m Member
		rows.Scan(&m.AgentID, &m.Role)
		members = append(members, m)
	}
	return members, nil
}

func (s *Store) UpdateLead(ctx context.Context, teamID, newLeadID string) error {
	// Demote old lead
	s.pool.Exec(ctx, `UPDATE crew_members SET role = 'member' WHERE team_id = $1 AND role = 'lead'`, teamID)
	// Promote new lead
	s.pool.Exec(ctx, `INSERT INTO crew_members (team_id, agent_id, role) VALUES ($1, $2, 'lead') ON CONFLICT (team_id, agent_id) DO UPDATE SET role = 'lead'`, teamID, newLeadID)
	_, err := s.pool.Exec(ctx, `UPDATE crews SET supervisor_id = $1 WHERE id = $2`, newLeadID, teamID)
	return err
}

func (s *Store) Rename(ctx context.Context, teamID, newName string) error {
	_, err := s.pool.Exec(ctx, `UPDATE crews SET name = $1 WHERE id = $2`, newName, teamID)
	return err
}

func (s *Store) AgentTeams(ctx context.Context, agentID string) ([]Team, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT c.id, c.name, COALESCE(c.supervisor_id::text,''), c.tenant_id, c.created_at
		 FROM crews c JOIN crew_members m ON c.id = m.team_id WHERE m.agent_id = $1`, agentID)
	if err != nil { return nil, err }
	defer rows.Close()
	teams := []Team{}
	for rows.Next() {
		var t Team
		rows.Scan(&t.ID, &t.Name, &t.LeadID, &t.TenantID, &t.CreatedAt)
		teams = append(teams, t)
	}
	return teams, nil
}
