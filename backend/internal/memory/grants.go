// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Grant is an explicit permission for one agent to access another's knowledge.
type Grant struct {
	ID              string         `json:"id"`
	GrantorAgentID  string         `json:"grantor_agent_id"`
	GranteeAgentID  string         `json:"grantee_agent_id"`
	Scope           Scope          `json:"scope"`
	MaxClass        Classification `json:"max_classification"`
	ReadOnly        bool           `json:"read_only"`
	Purpose         string         `json:"purpose"`
	GrantedBy       string         `json:"granted_by"`
	CreatedAt       time.Time      `json:"created_at"`
	ExpiresAt       *time.Time     `json:"expires_at,omitempty"`
	Revoked         bool           `json:"revoked"`
}

// GrantStore manages cross-agent knowledge sharing permissions.
type GrantStore struct {
	pool     *pgxpool.Pool
	tenantID string
}

func NewGrantStore(pool *pgxpool.Pool, tenantID string) *GrantStore {
	return &GrantStore{pool: pool, tenantID: tenantID}
}

// Create adds a new knowledge grant.
func (gs *GrantStore) Create(ctx context.Context, grant Grant) (string, error) {
	var id string
	err := gs.pool.QueryRow(ctx, `
		INSERT INTO knowledge_grants
		(tenant_id, grantor_agent_id, grantee_agent_id, scope, max_classification,
		 read_only, purpose, granted_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id`,
		gs.tenantID, grant.GrantorAgentID, grant.GranteeAgentID,
		string(grant.Scope), int(grant.MaxClass), grant.ReadOnly,
		grant.Purpose, grant.GrantedBy, grant.ExpiresAt,
	).Scan(&id)
	return id, err
}

// Revoke deactivates a grant.
func (gs *GrantStore) Revoke(ctx context.Context, grantID, revokedBy string) error {
	_, err := gs.pool.Exec(ctx, `
		UPDATE knowledge_grants
		SET revoked = true, revoked_by = $2, revoked_at = now()
		WHERE id = $1 AND tenant_id = $3`,
		grantID, revokedBy, gs.tenantID)
	return err
}

// RevokeAllForAgent revokes all grants where this agent is the grantee.
func (gs *GrantStore) RevokeAllForAgent(ctx context.Context, agentID, revokedBy string) error {
	_, err := gs.pool.Exec(ctx, `
		UPDATE knowledge_grants
		SET revoked = true, revoked_by = $2, revoked_at = now()
		WHERE grantee_agent_id = $1 AND tenant_id = $3 AND NOT revoked`,
		agentID, revokedBy, gs.tenantID)
	return err
}

// ActiveGrantsFor returns all active grants for a grantee agent.
func (gs *GrantStore) ActiveGrantsFor(ctx context.Context, granteeAgentID string) ([]Grant, error) {
	rows, err := gs.pool.Query(ctx, `
		SELECT id, grantor_agent_id, grantee_agent_id, scope, max_classification,
		       read_only, purpose, granted_by, created_at, expires_at
		FROM knowledge_grants
		WHERE tenant_id = $1 AND grantee_agent_id = $2
		  AND revoked = false
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC`,
		gs.tenantID, granteeAgentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var grants []Grant
	for rows.Next() {
		var g Grant
		var scope string
		var maxClass int
		err := rows.Scan(&g.ID, &g.GrantorAgentID, &g.GranteeAgentID, &scope, &maxClass,
			&g.ReadOnly, &g.Purpose, &g.GrantedBy, &g.CreatedAt, &g.ExpiresAt)
		if err != nil {
			continue
		}
		g.Scope = Scope(scope)
		g.MaxClass = Classification(maxClass)
		grants = append(grants, g)
	}
	return grants, nil
}

// CanAccess checks if grantee has an active grant to access grantor's knowledge.
func (gs *GrantStore) CanAccess(ctx context.Context, granteeID, grantorID string, scope Scope, class Classification) bool {
	var count int
	gs.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM knowledge_grants
		WHERE tenant_id = $1 AND grantee_agent_id = $2 AND grantor_agent_id = $3
		  AND scope = $4 AND max_classification >= $5
		  AND revoked = false AND (expires_at IS NULL OR expires_at > now())`,
		gs.tenantID, granteeID, grantorID, string(scope), int(class)).Scan(&count)
	return count > 0
}

// ListAll returns all active grants in the tenant (for CKO audit).
func (gs *GrantStore) ListAll(ctx context.Context) ([]Grant, error) {
	rows, err := gs.pool.Query(ctx, `
		SELECT id, grantor_agent_id, grantee_agent_id, scope, max_classification,
		       read_only, purpose, granted_by, created_at, expires_at
		FROM knowledge_grants
		WHERE tenant_id = $1 AND revoked = false
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY created_at DESC`,
		gs.tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var grants []Grant
	for rows.Next() {
		var g Grant
		var scope string
		var maxClass int
		err := rows.Scan(&g.ID, &g.GrantorAgentID, &g.GranteeAgentID, &scope, &maxClass,
			&g.ReadOnly, &g.Purpose, &g.GrantedBy, &g.CreatedAt, &g.ExpiresAt)
		if err != nil {
			continue
		}
		g.Scope = Scope(scope)
		g.MaxClass = Classification(maxClass)
		grants = append(grants, g)
	}
	return grants, nil
}
