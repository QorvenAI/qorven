// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"context"
	"time"
)

// ProjectArtifact is a typed, versioned, gated pipeline document.
type ProjectArtifact struct {
	ID            string     `json:"id"`
	BriefID       string     `json:"brief_id"`
	Type          string     `json:"type"`
	Version       int        `json:"version"`
	ContentMD     string     `json:"content_md"`
	Status        string     `json:"status"`
	RepoCommitted bool       `json:"repo_committed"`
	CreatedBy     string     `json:"created_by"`
	ApprovedBy    *string    `json:"approved_by,omitempty"`
	ApprovedAt    *time.Time `json:"approved_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

func nextArtifactVersion(prev *int) int {
	if prev == nil { return 1 }
	return *prev + 1
}

func artifactRepoPath(typ string) string { return "docs/" + typ + ".md" }

// listArtifacts returns all active (non-superseded) artifacts for a brief.
func (gw *Gateway) listArtifacts(ctx context.Context, briefID string) ([]ProjectArtifact, error) {
	rows, err := gw.db.Pool.Query(ctx,
		`SELECT id, brief_id, type, version, content_md, status, repo_committed,
		        created_by, approved_by, approved_at, created_at
		 FROM project_artifacts
		 WHERE brief_id = $1 AND status <> 'superseded'
		 ORDER BY type`, briefID)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []ProjectArtifact{}
	for rows.Next() {
		var a ProjectArtifact
		if err := rows.Scan(&a.ID, &a.BriefID, &a.Type, &a.Version, &a.ContentMD, &a.Status,
			&a.RepoCommitted, &a.CreatedBy, &a.ApprovedBy, &a.ApprovedAt, &a.CreatedAt); err != nil {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// getActiveArtifact returns the active artifact of a type, or (nil,nil) if none.
func (gw *Gateway) getActiveArtifact(ctx context.Context, briefID, typ string) (*ProjectArtifact, error) {
	var a ProjectArtifact
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT id, brief_id, type, version, content_md, status, repo_committed,
		        created_by, approved_by, approved_at, created_at
		 FROM project_artifacts
		 WHERE brief_id = $1 AND type = $2 AND status <> 'superseded'`, briefID, typ).
		Scan(&a.ID, &a.BriefID, &a.Type, &a.Version, &a.ContentMD, &a.Status,
			&a.RepoCommitted, &a.CreatedBy, &a.ApprovedBy, &a.ApprovedAt, &a.CreatedAt)
	if err != nil { return nil, nil }
	return &a, nil
}

// upsertArtifactRevision supersedes any active artifact of this type and inserts
// a fresh draft (status in_review) as the next version.
func (gw *Gateway) upsertArtifactRevision(ctx context.Context, briefID, typ, contentMD string) (*ProjectArtifact, error) {
	prev, _ := gw.getActiveArtifact(ctx, briefID, typ)
	var prevV *int
	if prev != nil { prevV = &prev.Version }
	if prev != nil {
		if _, err := gw.db.Pool.Exec(ctx,
			`UPDATE project_artifacts SET status='superseded' WHERE id=$1`, prev.ID); err != nil {
			return nil, err
		}
	}
	var a ProjectArtifact
	err := gw.db.Pool.QueryRow(ctx,
		`INSERT INTO project_artifacts (tenant_id, brief_id, type, version, content_md, status, created_by)
		 VALUES ($1,$2,$3,$4,$5,'in_review','cto')
		 RETURNING id, brief_id, type, version, content_md, status, repo_committed,
		           created_by, approved_by, approved_at, created_at`,
		defaultTenant, briefID, typ, nextArtifactVersion(prevV), contentMD).
		Scan(&a.ID, &a.BriefID, &a.Type, &a.Version, &a.ContentMD, &a.Status,
			&a.RepoCommitted, &a.CreatedBy, &a.ApprovedBy, &a.ApprovedAt, &a.CreatedAt)
	if err != nil { return nil, err }
	return &a, nil
}
