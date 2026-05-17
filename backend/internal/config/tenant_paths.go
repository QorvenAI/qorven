// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package config

import (
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// masterTenantID is the default tenant for single-tenant deployments.
var masterTenantID = uuid.MustParse("0193a5b0-7000-7000-8000-000000000001")

// TenantDataDir returns the data directory root for a tenant.
// Master tenant returns dataDir unchanged (backward compat).
func TenantDataDir(dataDir string, tenantID uuid.UUID, tenantSlug string) string {
	if tenantID == masterTenantID {
		return dataDir
	}
	return safeTenantPath(dataDir, "tenants", tenantID, tenantSlug)
}

// TenantWorkspace returns the workspace root for a tenant.
func TenantWorkspace(workspace string, tenantID uuid.UUID, tenantSlug string) string {
	if tenantID == masterTenantID {
		return workspace
	}
	return safeTenantPath(workspace, "tenants", tenantID, tenantSlug)
}

// TenantTeamDir returns the team workspace directory for a tenant.
func TenantTeamDir(dataDir string, tenantID uuid.UUID, tenantSlug string, teamID uuid.UUID) string {
	return filepath.Join(TenantDataDir(dataDir, tenantID, tenantSlug), "teams", teamID.String())
}

// TenantMediaDir returns the media storage directory for a tenant.
func TenantMediaDir(dataDir string, tenantID uuid.UUID, tenantSlug string) string {
	return filepath.Join(TenantDataDir(dataDir, tenantID, tenantSlug), "media")
}

// safeTenantPath prevents path traversal via malicious slug.
func safeTenantPath(base, subdir string, tenantID uuid.UUID, tenantSlug string) string {
	result := filepath.Join(base, subdir, tenantSlug)
	tenantsBase := filepath.Join(base, subdir) + string(filepath.Separator)
	if !strings.HasPrefix(result+string(filepath.Separator), tenantsBase) {
		return filepath.Join(base, subdir, tenantID.String())
	}
	return result
}
