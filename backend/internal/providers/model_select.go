// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"strings"
)

// IndexForRole maps an org role to the benchmark index that should rank its
// model. CTO → coding; CFO → math; everyone else → general intelligence.
// (One-line extension when a dedicated writing/social score exists.)
func IndexForRole(orgRole string) string {
	switch strings.ToLower(strings.TrimSpace(orgRole)) {
	case "cto":
		return "coding_index"
	case "cfo":
		return "math_index"
	default:
		return "intelligence_index"
	}
}

// SelectModelForHire returns the concrete model ID to assign to a new agent of
// the given role, in the given tier band, ranked by the role's index and
// restricted to enabled models. Climbs the tier ladder to the nearest tier
// with an enabled, ranked candidate. Returns "" when nothing qualifies (caller
// then stores "auto"). availableProviders = enabled provider names.
func SelectModelForHire(cat *StaticModelCatalog, orgRole, tier string, availableProviders []string, enabled map[string]bool) string {
	if cat == nil {
		return ""
	}
	idx := IndexForRole(orgRole)
	for _, t := range tierLadder(tier) {
		if m := cat.BestForTierByIndex(t, idx, availableProviders, enabled); m != nil {
			return m.ID
		}
	}
	return ""
}

// EnabledModelIDs returns the set of model IDs the tenant has enabled (from
// selected_models). Empty result = no explicit selection (caller treats as
// "no enabled-model filter").
func (s *KeyPoolStore) EnabledModelIDs(ctx context.Context, tenantID string) map[string]bool {
	out := map[string]bool{}
	if s == nil || s.pool == nil {
		return out
	}
	rows, err := s.pool.Query(ctx, `SELECT model_id FROM selected_models WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil && id != "" {
			out[id] = true
		}
	}
	return out
}
