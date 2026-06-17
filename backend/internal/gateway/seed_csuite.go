// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/agent"
)

// csuiteOfficer describes one C-suite seat to seed.
type csuiteOfficer struct {
	key     string  // agent_key — unique, checked for existence
	name    string  // display_name
	title   string  // job title label
	role    string  // agent role (maps to AgentSeeds key)
	orgRole string  // org_role field
	budget  float64 // monthly_budget_usd
}

// defaultCSuiteOfficers are the functional officers seeded under Prime on a
// fresh install. Prime itself is the COO (the coordinator the user talks to);
// the human user is the CEO and is not seeded as an agent. Coders are NOT seeded
// — the CTO spawns them on demand per project.
var defaultCSuiteOfficers = []csuiteOfficer{
	{key: "officer-cto", name: "CTO", title: "Chief Technology Officer", role: "cto", orgRole: "cto", budget: 50},
	{key: "officer-cmo", name: "CMO", title: "Chief Marketing Officer", role: "cmo", orgRole: "cmo", budget: 50},
	{key: "officer-cfo", name: "CFO", title: "Chief Financial Officer", role: "cfo", orgRole: "cfo", budget: 50},
	{key: "officer-chro", name: "CHRO", title: "Chief HR Officer", role: "chro", orgRole: "chro", budget: 50},
}

// seedCSuite creates the default C-suite under Prime (the COO) on a fresh
// install. The human user is the CEO — no CEO agent is created. It is fully
// idempotent:
//   - If any functional officer (cto/cmo/cfo/chro) already exists for the
//     tenant, the org is considered customised and the entire function no-ops.
//   - Prime stays the COO (its boot-time default); it is the coordinator the
//     user talks to and the manager every officer reports to.
//   - Each individual officer is guarded by agent_key before creation, so a partial
//     previous seed cannot produce duplicates.
func (gw *Gateway) seedCSuite(ctx context.Context) error {
	if gw.agents == nil || gw.db == nil {
		return nil
	}

	// ── Freshness check ────────────────────────────────────────────────────────
	// If any functional officer already exists, the org is populated — no-op.
	// 'chief' (Prime, the COO) is excluded so its presence alone never blocks the
	// officer seed. 'ceo' is included so an org from the older CEO-promotion seed
	// is still recognised as populated and left untouched.
	var count int
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM agents
		 WHERE tenant_id = $1
		   AND org_role IN ('cfo','cto','cmo','chro','ceo')
		   AND agent_key != 'chief'
		   AND (terminated_at IS NULL AND (status IS NULL OR status != 'suspended'))`,
		defaultTenant).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		slog.Info("csuite.seed.skipped", "reason", "org already populated", "existing_officers", count)
		return nil
	}

	// ── Get Prime ──────────────────────────────────────────────────────────────
	prime, err := gw.agents.GetByKey(ctx, "chief")
	if err != nil || prime == nil {
		return err // Prime must exist; ensureChief is called before us
	}

	// Prime stays the COO — the coordinator the user talks to and the manager
	// every officer reports to. The human user is the CEO (not an agent), shown
	// at the top of the org chart. Ensure Prime's org row reflects COO at L1.
	if gw.orgChartStore != nil {
		if tenantUID, tErr := uuid.Parse(defaultTenant); tErr == nil {
			if agentUID, aErr := uuid.Parse(prime.ID); aErr == nil {
				_ = gw.orgChartStore.SyncFromAgent(ctx, tenantUID, agentUID, nil, "l1", "coo", 0)
			}
		}
	}

	// ── Create C-suite officers ────────────────────────────────────────────────
	primeUID := prime.ID
	for _, off := range defaultCSuiteOfficers {
		// Per-officer idempotency guard.
		existing, lookupErr := gw.agents.GetByKey(ctx, off.key)
		if lookupErr == nil && existing != nil {
			slog.Info("csuite.seed.officer.already_exists", "key", off.key)
			continue
		}

		seed, hasSeed := agent.AgentSeeds[off.role]
		soulContent := ""
		if hasSeed && seed.Soul != "" {
			soulContent = seed.Soul
		} else {
			soulContent = "You are " + off.name + ", the " + off.title + " of this organisation."
		}

		a, createErr := gw.agents.Create(ctx, defaultTenant, agent.CreateAgentInput{
			AgentKey:          off.key,
			DisplayName:       off.name,
			Role:              off.role,
			Title:             off.title,
			OrgRole:           off.orgRole,
			OrgLevel:          "l2",
			ManagerID:         primeUID,
			SystemPrompt:      soulContent,
			Model:             "auto",
			Temperature:       0.5,
			ContextWindow:     128000,
			MaxToolIterations: 20,
			ToolProfile:       "full",
		})
		if createErr != nil {
			slog.Warn("csuite.seed.officer.create_failed", "key", off.key, "err", createErr)
			continue
		}

		// Set monthly budget.
		if off.budget > 0 {
			_, _ = gw.db.Pool.Exec(ctx,
				`UPDATE agents SET monthly_budget_usd=$1, can_delegate=true WHERE id=$2`,
				off.budget, a.ID)
		}

		// Write org_hierarchy overlay.
		if gw.orgChartStore != nil {
			if tenantUID, tErr := uuid.Parse(defaultTenant); tErr == nil {
				if agentUID, aErr := uuid.Parse(a.ID); aErr == nil {
					if managerUID, mErr := uuid.Parse(primeUID); mErr == nil {
						if syncErr := gw.orgChartStore.SyncFromAgent(ctx, tenantUID, agentUID, &managerUID, "l2", off.orgRole, off.budget); syncErr != nil {
							slog.Warn("csuite.seed.org_hierarchy_sync.failed", "agent_id", a.ID, "err", syncErr)
						}
					}
				}
			}
		}

		// Seed archetype bundles (soul + identity + tools).
		if gw.bundleStore != nil {
			gw.bundleStore.Upsert(ctx, agent.Bundle{
				AgentID: a.ID, BundleType: "soul", Name: "soul",
				Content: soulContent, Priority: 200, Enabled: true,
			})
			if hasSeed && seed.Identity != "" {
				gw.bundleStore.Upsert(ctx, agent.Bundle{
					AgentID: a.ID, BundleType: "identity", Name: "identity",
					Content: seed.Identity, Priority: 100, Enabled: true,
				})
			}
			if hasSeed && seed.Tools != "" {
				gw.bundleStore.Upsert(ctx, agent.Bundle{
					AgentID: a.ID, BundleType: "tools", Name: "tools",
					Content: seed.Tools, Priority: 90, Enabled: true,
				})
			}
		}

		// Write to org_roster.
		_, _ = gw.db.Pool.Exec(ctx,
			`INSERT INTO org_roster (tenant_id, agent_id, org_level, org_role, display_name, status, hired_by)
			 VALUES ($1,$2,'l2',$3,$4,'active',$5) ON CONFLICT DO NOTHING`,
			defaultTenant, a.ID, off.orgRole, off.name, primeUID)

		// Create department headed by this officer.
		var deptID string
		_ = gw.db.Pool.QueryRow(ctx,
			`INSERT INTO departments (tenant_id, name, head_agent_id)
			 VALUES ($1, $2, $3::uuid)
			 ON CONFLICT (tenant_id, name) DO UPDATE SET head_agent_id = EXCLUDED.head_agent_id
			 RETURNING id::text`,
			defaultTenant, off.orgRole, a.ID).Scan(&deptID)
		if deptID != "" {
			_, _ = gw.db.Pool.Exec(ctx, `UPDATE agents SET department_id = $1 WHERE id = $2`, deptID, a.ID)
		}

		// Activate runtime.
		if gw.runtimeMgr != nil {
			gw.runtimeMgr.EnsureRuntime(a.ID, defaultTenant)
		}

		slog.Info("csuite.seed.officer.created",
			"key", off.key,
			"agent_id", a.ID,
			"org_role", off.orgRole,
			"manager_id", strings.TrimSpace(primeUID),
		)
	}

	slog.Info("csuite.seed.done")
	return nil
}
