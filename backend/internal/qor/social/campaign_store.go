// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// campaign_store.go — CRUD for social_campaigns.

// CreateCampaign inserts a new campaign and returns its generated ID.
func (s *Store) CreateCampaign(ctx context.Context, c Campaign) (string, error) {
	if c.Status == "" {
		c.Status = "draft"
	}
	now := time.Now()
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO social_campaigns
		   (tenant_id, department_id, created_by_agent_id, title, brief,
		    target_platforms, window_start, window_end, status, created_at)
		 VALUES
		   ($1::uuid, NULLIF($2,'')::uuid, NULLIF($3,'')::uuid, $4, $5,
		    $6, $7, $8, $9, $10)
		 RETURNING id::text`,
		c.TenantID, c.DepartmentID, c.CreatedByAgentID, c.Title, c.Brief,
		c.TargetPlatforms, c.WindowStart, c.WindowEnd, c.Status, now,
	).Scan(&id)
	return id, err
}

// ListCampaigns returns campaigns for a tenant, optionally filtered by department.
// If departmentID is empty, all campaigns for the tenant are returned.
func (s *Store) ListCampaigns(ctx context.Context, tenantID, departmentID string) ([]Campaign, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if departmentID == "" {
		rows, err = s.pool.Query(ctx,
			`SELECT id::text, tenant_id::text,
			        COALESCE(department_id::text,''), COALESCE(created_by_agent_id::text,''),
			        title, brief, target_platforms, window_start, window_end, status, created_at
			 FROM social_campaigns
			 WHERE tenant_id = $1::uuid
			 ORDER BY created_at DESC`, tenantID)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT id::text, tenant_id::text,
			        COALESCE(department_id::text,''), COALESCE(created_by_agent_id::text,''),
			        title, brief, target_platforms, window_start, window_end, status, created_at
			 FROM social_campaigns
			 WHERE tenant_id = $1::uuid AND department_id = $2::uuid
			 ORDER BY created_at DESC`, tenantID, departmentID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	campaigns := []Campaign{}
	for rows.Next() {
		var c Campaign
		scanErr := rows.Scan(
			&c.ID, &c.TenantID, &c.DepartmentID, &c.CreatedByAgentID,
			&c.Title, &c.Brief, &c.TargetPlatforms,
			&c.WindowStart, &c.WindowEnd, &c.Status, &c.CreatedAt,
		)
		if scanErr != nil {
			return nil, scanErr
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, rows.Err()
}

// GetCampaign returns a single campaign by ID, scoped to the tenant.
func (s *Store) GetCampaign(ctx context.Context, tenantID, id string) (*Campaign, error) {
	var c Campaign
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, tenant_id::text,
		        COALESCE(department_id::text,''), COALESCE(created_by_agent_id::text,''),
		        title, brief, target_platforms, window_start, window_end, status, created_at
		 FROM social_campaigns
		 WHERE tenant_id = $1::uuid AND id = $2::uuid`, tenantID, id).Scan(
		&c.ID, &c.TenantID, &c.DepartmentID, &c.CreatedByAgentID,
		&c.Title, &c.Brief, &c.TargetPlatforms,
		&c.WindowStart, &c.WindowEnd, &c.Status, &c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// SetCampaignStatus updates the status of a campaign, scoped to the tenant.
func (s *Store) SetCampaignStatus(ctx context.Context, tenantID, id, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE social_campaigns SET status = $1 WHERE tenant_id = $2::uuid AND id = $3::uuid`,
		status, tenantID, id)
	return err
}
