// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGBackend implements Backend using PostgreSQL with pgvector.
// This is the default backend — always available, no external dependencies.
type PGBackend struct {
	store *Store
}

// NewPGBackend creates a PostgreSQL memory backend.
func NewPGBackend(pool *pgxpool.Pool) *PGBackend {
	return &PGBackend{store: NewStore(pool)}
}

func (b *PGBackend) Name() string { return "postgresql" }

func (b *PGBackend) Store(ctx context.Context, opts StoreOpts) (string, error) {
	m := Memory{
		ID:          uuid.New().String(),
		AgentID:     opts.AgentID,
		Type:        scopeType(opts),
		Content:     opts.Content,
		Source:      opts.Source,
		Importance:  opts.Importance,
		DecayExempt: opts.DecayExempt,
	}
	return b.store.Save(ctx, opts.TenantID, m)
}

func (b *PGBackend) Search(ctx context.Context, opts SearchOpts) ([]SearchResult, error) {
	switch opts.Scope {
	case ScopeCompany:
		// Company memories: type='company', visible to all agents
		return b.store.SearchByTypeQuery(ctx, opts.TenantID, "company", opts.Query, opts.MaxResults)

	case ScopeTeam:
		if opts.TeamID == "" {
			return nil, fmt.Errorf("team_id required for team scope search")
		}
		// Get all agent IDs in this team, then search their memories + team-scoped ones
		teamAgents, err := b.getTeamAgentIDs(ctx, opts.TenantID, opts.TeamID)
		if err != nil || len(teamAgents) == 0 {
			return b.store.SearchByTypeQuery(ctx, opts.TenantID, "team:"+opts.TeamID, opts.Query, opts.MaxResults)
		}
		return b.store.SearchTeam(ctx, opts.TenantID, teamAgents, opts.Query, opts.MaxResults)

	case ScopeTask:
		if opts.TaskID == "" {
			return nil, fmt.Errorf("task_id required for task scope search")
		}
		return b.store.SearchByTypeQuery(ctx, opts.TenantID, "task:"+opts.TaskID, opts.Query, opts.MaxResults)

	case ScopePrime:
		return b.store.SearchByTypeQuery(ctx, opts.TenantID, "prime", opts.Query, opts.MaxResults)

	case ScopeSession:
		return b.store.SearchByTypeQuery(ctx, opts.TenantID, "session:"+opts.AgentID, opts.Query, opts.MaxResults)

	default:
		// Agent scope (default): search this agent's memories
		if opts.AgentID == "" {
			return b.store.Search(ctx, opts.TenantID, "", opts.Query, opts.MaxResults)
		}
		return b.store.Search(ctx, opts.TenantID, opts.AgentID, opts.Query, opts.MaxResults)
	}
}

// getTeamAgentIDs returns all agent IDs in a team.
func (b *PGBackend) getTeamAgentIDs(ctx context.Context, tenantID, teamID string) ([]string, error) {
	rows, err := b.store.pool.Query(ctx,
		`SELECT agent_id FROM team_members WHERE team_id = $1`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (b *PGBackend) Delete(ctx context.Context, tenantID, id string) error {
	_, err := b.store.pool.Exec(ctx,
		"DELETE FROM memories WHERE tenant_id = $1 AND id = $2", tenantID, id)
	return err
}

func (b *PGBackend) List(ctx context.Context, opts ListOpts) ([]Memory, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	typeFilter := scopeTypeFromList(opts)
	if typeFilter != "" {
		return b.store.SearchByType(ctx, opts.AgentID, typeFilter, limit)
	}
	return b.store.SearchByType(ctx, opts.AgentID, "", limit)
}

func (b *PGBackend) Close() error { return nil }

// InternalStore exposes the raw Store for legacy code that needs direct access.
func (b *PGBackend) InternalStore() *Store { return b.store }

// scopeType builds the memory type string from store options.
func scopeType(opts StoreOpts) string {
	switch opts.Scope {
	case ScopeCompany:
		return "company"
	case ScopeTeam:
		return "team:" + opts.TeamID
	case ScopeTask:
		return "task:" + opts.TaskID
	case ScopeSession:
		return "session:" + opts.SessionID
	case ScopePrime:
		return "prime"
	default:
		if opts.Type != "" {
			return opts.Type
		}
		return "agent"
	}
}

func scopeTypeFromList(opts ListOpts) string {
	switch opts.Scope {
	case ScopeCompany:
		return "company"
	case ScopeTeam:
		return "team:" + opts.TeamID
	case ScopeTask:
		return "task:" + opts.TaskID
	default:
		return opts.Type
	}
}
