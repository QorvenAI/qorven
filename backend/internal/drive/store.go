// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package drive

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type File struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	AgentID   *string   `json:"agent_id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	IsFolder  bool      `json:"is_folder"`
	ParentID  *string   `json:"parent_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) ListFiles(ctx context.Context, agentID string, parentID *string) ([]File, error) {
	var rows interface{ Next() bool; Scan(...any) error; Close() }
	var err error
	base := `SELECT id, tenant_id, agent_id, name, path, COALESCE(mime_type,''), size_bytes, is_folder, parent_id, created_at, updated_at
		 FROM drive_files WHERE `
	if parentID != nil {
		if agentID != "" {
			rows, err = s.pool.Query(ctx, base+`agent_id = $1 AND parent_id = $2 ORDER BY is_folder DESC, name`, agentID, *parentID)
		} else {
			rows, err = s.pool.Query(ctx, base+`parent_id = $1 ORDER BY is_folder DESC, name`, *parentID)
		}
	} else {
		if agentID != "" {
			rows, err = s.pool.Query(ctx, base+`agent_id = $1 AND parent_id IS NULL ORDER BY is_folder DESC, name`, agentID)
		} else {
			rows, err = s.pool.Query(ctx, base+`parent_id IS NULL ORDER BY is_folder DESC, name`)
		}
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []File{}
	for rows.Next() {
		var f File
		rows.Scan(&f.ID, &f.TenantID, &f.AgentID, &f.Name, &f.Path, &f.MimeType, &f.SizeBytes, &f.IsFolder, &f.ParentID, &f.CreatedAt, &f.UpdatedAt)
		files = append(files, f)
	}
	return files, nil
}

// SearchFiles returns files whose name matches the query string, across all folders.
func (s *Store) SearchFiles(ctx context.Context, agentID, q string) ([]File, error) {
	var rows interface{ Next() bool; Scan(...any) error; Close() }
	var err error
	base := `SELECT id, tenant_id, agent_id, name, path, COALESCE(mime_type,''), size_bytes, is_folder, parent_id, created_at, updated_at
		 FROM drive_files WHERE name ILIKE '%' || $1 || '%'`
	if agentID != "" {
		rows, err = s.pool.Query(ctx, base+` AND agent_id = $2 ORDER BY is_folder DESC, name LIMIT 20`, q, agentID)
	} else {
		rows, err = s.pool.Query(ctx, base+` ORDER BY is_folder DESC, name LIMIT 20`, q)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []File{}
	for rows.Next() {
		var f File
		rows.Scan(&f.ID, &f.TenantID, &f.AgentID, &f.Name, &f.Path, &f.MimeType, &f.SizeBytes, &f.IsFolder, &f.ParentID, &f.CreatedAt, &f.UpdatedAt)
		files = append(files, f)
	}
	return files, nil
}

func (s *Store) CreateFile(ctx context.Context, tenantID, agentID, name, path, mimeType string, size int64, isFolder bool, parentID *string) (*File, error) {
	f := &File{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO drive_files (tenant_id, agent_id, name, path, mime_type, size_bytes, is_folder, parent_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, tenant_id, agent_id, name, path, mime_type, size_bytes, is_folder, parent_id, created_at, updated_at`,
		tenantID, agentID, name, path, mimeType, size, isFolder, parentID,
	).Scan(&f.ID, &f.TenantID, &f.AgentID, &f.Name, &f.Path, &f.MimeType, &f.SizeBytes, &f.IsFolder, &f.ParentID, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

func (s *Store) DeleteFile(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM drive_files WHERE id = $1`, id)
	return err
}

func (s *Store) ShareFile(ctx context.Context, fileID, granteeType, granteeID, permission string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO drive_permissions (file_id, grantee_type, grantee_id, permission) VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`, fileID, granteeType, granteeID, permission)
	return err
}

func (s *Store) GetQuota(ctx context.Context, agentID string) (used int64, total int64, err error) {
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(size_bytes),0) FROM drive_files WHERE agent_id = $1 AND is_folder = false`, agentID).Scan(&used)
	if err != nil {
		return
	}
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(drive_quota_bytes, 104857600) FROM agents WHERE id = $1`, agentID).Scan(&total)
	return
}
