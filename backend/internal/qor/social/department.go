// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import "context"

// ResolveMarketingDepartment finds the Marketing department for a tenant.
// It prefers the department whose head agent has agent_key='cmo'; falls back
// to any department named 'marketing' (case-insensitive). Returns "" if none
// exists — callers treat "" as unset and continue without department gating.
func (s *Store) ResolveMarketingDepartment(ctx context.Context, tenantID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT d.id::text FROM departments d
		 LEFT JOIN agents a ON a.id = d.head_agent_id
		 WHERE d.tenant_id = $1 AND (a.agent_key = 'cmo' OR d.name ILIKE 'marketing')
		 ORDER BY (a.agent_key = 'cmo') DESC LIMIT 1`, tenantID).Scan(&id)
	if err != nil {
		// No marketing department yet — not an error, caller treats "" as unset.
		return "", nil
	}
	return id, nil
}
