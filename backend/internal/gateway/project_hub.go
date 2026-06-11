// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"context"
	"log/slog"
)

// ensureProjectHub returns the project's coordination room id, creating it once
// (advisory-lock guarded, mirroring ensureCompanyHub) and seeding members from
// the project's agents. Idempotent: safe to call on every project-page open.
func (gw *Gateway) ensureProjectHub(ctx context.Context, briefID string) string {
	if gw.db == nil || gw.db.Pool == nil || briefID == "" {
		return ""
	}
	// Fast path: already exists.
	var id string
	if err := gw.db.Pool.QueryRow(ctx,
		`SELECT id FROM rooms WHERE tenant_id=$1 AND project_brief_id=$2 LIMIT 1`,
		defaultTenant, briefID).Scan(&id); err == nil && id != "" {
		gw.seedProjectHubMembers(ctx, id, briefID)
		return id
	}
	// Slow path: lock, re-check, create.
	tx, err := gw.db.Pool.Begin(ctx)
	if err != nil {
		return ""
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "project-hub:"+briefID); err != nil {
		return ""
	}
	// Re-check under the lock — another goroutine may have created it.
	if err := tx.QueryRow(ctx,
		`SELECT id FROM rooms WHERE tenant_id=$1 AND project_brief_id=$2 LIMIT 1`,
		defaultTenant, briefID).Scan(&id); err == nil && id != "" {
		_ = tx.Commit(ctx)
		gw.seedProjectHubMembers(ctx, id, briefID)
		return id
	}
	var title string
	_ = tx.QueryRow(ctx,
		`SELECT COALESCE(NULLIF(title,''),'Project') FROM project_briefs WHERE id=$1`,
		briefID).Scan(&title)
	if title == "" {
		title = "Project"
	}
	slug := sanitizeKey(briefID)
	if slug == "" {
		slug = briefID
	}
	if len(slug) > 8 {
		slug = slug[:8]
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO rooms (tenant_id, project_brief_id, name, display_name, description, created_by)
		 VALUES ($1,$2,$3,$4,'Project coordination and roll-ups','system') RETURNING id`,
		defaultTenant, briefID, "project-"+slug, title+" — Hub").Scan(&id); err != nil {
		slog.Warn("project_hub.create_failed", "brief_id", briefID, "err", err)
		return ""
	}
	if err := tx.Commit(ctx); err != nil {
		return ""
	}
	slog.Info("project_hub.created", "room_id", id, "brief_id", briefID)
	gw.seedProjectHubMembers(ctx, id, briefID)
	return id
}

// seedProjectHubMembers adds the project's agents as room members (idempotent).
// room_members has a PRIMARY KEY (room_id, agent_id) so ON CONFLICT DO NOTHING
// is safe and prevents duplicates.
func (gw *Gateway) seedProjectHubMembers(ctx context.Context, roomID, briefID string) {
	if _, err := gw.db.Pool.Exec(ctx,
		`INSERT INTO room_members (room_id, agent_id)
		 SELECT $1, id FROM agents
		 WHERE tenant_id=$2 AND project_brief_id=$3 AND deleted_at IS NULL
		 ON CONFLICT DO NOTHING`,
		roomID, defaultTenant, briefID); err != nil {
		slog.Warn("project_hub.member_seed_failed", "room_id", roomID, "brief_id", briefID, "err", err)
	}
}
