// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package drive

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a file is not found for the given tenant.
var ErrNotFound = errors.New("file not found")

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type File struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	AgentID   *string   `json:"agent_id"`
	Name      string    `json:"name"`
	Path      string    `json:"-"` // server FS path — never exposed to clients
	MimeType  string    `json:"mime_type"`
	SizeBytes int64     `json:"size_bytes"`
	IsFolder  bool      `json:"is_folder"`
	ParentID  *string   `json:"parent_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Scope    string  `json:"scope"`
	ScopeID  *string `json:"scope_id"`
}

func (s *Store) ListFiles(ctx context.Context, agentID string, parentID *string) ([]File, error) {
	var rows interface{ Next() bool; Scan(...any) error; Close() }
	var err error
	base := `SELECT id, tenant_id, agent_id, name, path, COALESCE(mime_type,''), size_bytes, is_folder, parent_id, created_at, updated_at, scope, scope_id
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
		rows.Scan(&f.ID, &f.TenantID, &f.AgentID, &f.Name, &f.Path, &f.MimeType, &f.SizeBytes, &f.IsFolder, &f.ParentID, &f.CreatedAt, &f.UpdatedAt, &f.Scope, &f.ScopeID)
		files = append(files, f)
	}
	return files, nil
}

// SearchFiles returns files whose name matches the query string, across all
// folders, tenant-scoped. NOTE: results are NOT yet scope/ACL-filtered — that
// is owned by Subsystem 2 (the search UI). It leaks names/metadata (not content)
// within a tenant; content download is still ACL-gated. The tenant predicate
// here prevents cross-tenant name leakage.
func (s *Store) SearchFiles(ctx context.Context, tenantID, agentID, q string) ([]File, error) {
	var rows interface{ Next() bool; Scan(...any) error; Close() }
	var err error
	base := `SELECT id, tenant_id, agent_id, name, path, COALESCE(mime_type,''), size_bytes, is_folder, parent_id, created_at, updated_at, scope, scope_id
		 FROM drive_files WHERE tenant_id = $1 AND name ILIKE '%' || $2 || '%'`
	if agentID != "" {
		rows, err = s.pool.Query(ctx, base+` AND agent_id = $3 ORDER BY is_folder DESC, name LIMIT 20`, tenantID, q, agentID)
	} else {
		rows, err = s.pool.Query(ctx, base+` ORDER BY is_folder DESC, name LIMIT 20`, tenantID, q)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []File{}
	for rows.Next() {
		var f File
		rows.Scan(&f.ID, &f.TenantID, &f.AgentID, &f.Name, &f.Path, &f.MimeType, &f.SizeBytes, &f.IsFolder, &f.ParentID, &f.CreatedAt, &f.UpdatedAt, &f.Scope, &f.ScopeID)
		files = append(files, f)
	}
	return files, nil
}

func (s *Store) CreateFile(ctx context.Context, tenantID, agentID, name, path, mimeType string, size int64, isFolder bool, parentID *string) (*File, error) {
	f := &File{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO drive_files (tenant_id, agent_id, name, path, mime_type, size_bytes, is_folder, parent_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, tenant_id, agent_id, name, path, mime_type, size_bytes, is_folder, parent_id, created_at, updated_at, scope, scope_id`,
		tenantID, agentID, name, path, mimeType, size, isFolder, parentID,
	).Scan(&f.ID, &f.TenantID, &f.AgentID, &f.Name, &f.Path, &f.MimeType, &f.SizeBytes, &f.IsFolder, &f.ParentID, &f.CreatedAt, &f.UpdatedAt, &f.Scope, &f.ScopeID)
	return f, err
}

func (s *Store) DeleteFile(ctx context.Context, tenantID, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM drive_files WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
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

// GetFile returns one file by id, tenant-scoped, with its scope fields.
func (s *Store) GetFile(ctx context.Context, tenantID, id string) (*File, error) {
	var f File
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, agent_id, name, path, COALESCE(mime_type,''), size_bytes, is_folder, parent_id, created_at, updated_at, scope, scope_id
		 FROM drive_files WHERE tenant_id = $1 AND id = $2`, tenantID, id,
	).Scan(&f.ID, &f.TenantID, &f.AgentID, &f.Name, &f.Path, &f.MimeType, &f.SizeBytes, &f.IsFolder, &f.ParentID, &f.CreatedAt, &f.UpdatedAt, &f.Scope, &f.ScopeID)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// ListVisible returns files in a parent folder that the caller may see, by
// scope. callerAgent is "" for a human user; isAdminUser grants full read.
// This replaces the old "return everything" list path — the audit's missing
// read-side ACL. It post-filters every row through the same access decision
// so the predicate stays the single source of truth.
func (s *Store) ListVisible(ctx context.Context, tenantID, callerAgent string, isAdminUser bool, parentID *string) ([]File, error) {
	deptList := []string{}
	if callerAgent != "" {
		deptList = s.agentDepartments(ctx, callerAgent)
	}
	q := `SELECT id, tenant_id, agent_id, name, path, COALESCE(mime_type,''), size_bytes, is_folder, parent_id, created_at, updated_at, scope, scope_id
	      FROM drive_files WHERE tenant_id = $1 AND `
	if parentID != nil {
		q += `parent_id = $2`
	} else {
		q += `parent_id IS NULL`
	}
	q += ` ORDER BY is_folder DESC, name`
	var rows interface{ Next() bool; Scan(...any) error; Close() }
	var err error
	if parentID != nil {
		rows, err = s.pool.Query(ctx, q, tenantID, *parentID)
	} else {
		rows, err = s.pool.Query(ctx, q, tenantID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deptSet := map[string]bool{}
	for _, d := range deptList {
		deptSet[d] = true
	}
	out := []File{}
	for rows.Next() {
		var f File
		if rows.Scan(&f.ID, &f.TenantID, &f.AgentID, &f.Name, &f.Path, &f.MimeType, &f.SizeBytes, &f.IsFolder, &f.ParentID, &f.CreatedAt, &f.UpdatedAt, &f.Scope, &f.ScopeID) != nil {
			continue
		}
		owner := ""
		if f.AgentID != nil {
			owner = *f.AgentID
		}
		scopeID := ""
		if f.ScopeID != nil {
			scopeID = *f.ScopeID
		}
		hasGrant := false
		if f.Scope == ScopeCustom && callerAgent != "" {
			hasGrant = s.hasCustomGrant(ctx, f.ID, callerAgent)
		}
		if decideAccess(f.Scope, scopeID, owner, callerAgent, deptSet, hasGrant, isAdminUser) {
			out = append(out, f)
		}
	}
	return out, nil
}

// SetScope changes a file's scope (and optional department scope_id), tenant-scoped.
func (s *Store) SetScope(ctx context.Context, tenantID, id, scope string, scopeID *string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE drive_files SET scope = $1, scope_id = $2, updated_at = now() WHERE tenant_id = $3 AND id = $4`,
		scope, scopeID, tenantID, id)
	return err
}

// CreateFileScoped is CreateFile with an explicit scope.
func (s *Store) CreateFileScoped(ctx context.Context, tenantID, agentID, name, path, mimeType string, size int64, isFolder bool, parentID *string, scope string, scopeID *string) (*File, error) {
	f := &File{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO drive_files (tenant_id, agent_id, name, path, mime_type, size_bytes, is_folder, parent_id, scope, scope_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 RETURNING id, tenant_id, agent_id, name, path, mime_type, size_bytes, is_folder, parent_id, created_at, updated_at, scope, scope_id`,
		tenantID, agentID, name, path, mimeType, size, isFolder, parentID, scope, scopeID,
	).Scan(&f.ID, &f.TenantID, &f.AgentID, &f.Name, &f.Path, &f.MimeType, &f.SizeBytes, &f.IsFolder, &f.ParentID, &f.CreatedAt, &f.UpdatedAt, &f.Scope, &f.ScopeID)
	return f, err
}
