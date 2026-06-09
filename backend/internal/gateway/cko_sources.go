// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/providers"
)

// ckoMaxFacts caps how many source rows feed a single brief — keeps the
// synthesis prompt (and the resulting brief) lean.
const ckoMaxFacts = 40

// gatherCKOSources collects the knowledge facts for a scope from the tenant
// memory store, and returns them with the highest classification observed.
//
// Scope semantics (LOCKED CONTRACT):
//   - "company": tenant-wide memories (type='company') — visible to all agents.
//   - "department": scopeKey is the department UUID. Team/shared memories for
//     agents in that department.
//   - "role": scopeKey is the org_role string (e.g. "code"). Shared memories
//     authored by agents holding that role.
//
// v1 sources from the memories table only. Room decisions and completed
// work-items are NOT pulled yet (no clean tenant-wide list API); that is a
// documented follow-up.
func (gw *Gateway) gatherCKOSources(ctx context.Context, scope, scopeKey string) ([]string, memory.Classification) {
	maxClass := memory.ClassInternal // default when nothing higher is observed
	if gw.db == nil || gw.db.Pool == nil {
		return nil, maxClass
	}

	var (
		sql  string
		args []any
	)
	switch scope {
	case "company":
		// Company-wide knowledge: high-importance, decay-exempt facts visible to all.
		sql = `SELECT content, COALESCE(classification, 1)
		       FROM memories
		       WHERE tenant_id = $1
		         AND (memory_type = 'company' OR source_type = 'company')
		       ORDER BY importance DESC, created_at DESC
		       LIMIT $2`
		args = []any{defaultTenant, ckoMaxFacts}
	case "department":
		if scopeKey == "" {
			return nil, maxClass
		}
		// Shared (non-private) memories authored by agents in this department.
		sql = `SELECT m.content, COALESCE(m.classification, 1)
		       FROM memories m
		       JOIN agents a ON a.id = m.agent_id
		       WHERE m.tenant_id = $1
		         AND a.department_id = $2::uuid
		         AND m.source_type <> 'private'
		       ORDER BY m.importance DESC, m.created_at DESC
		       LIMIT $3`
		args = []any{defaultTenant, scopeKey, ckoMaxFacts}
	case "role":
		if scopeKey == "" {
			return nil, maxClass
		}
		// Shared memories authored by agents holding this org_role.
		sql = `SELECT m.content, COALESCE(m.classification, 1)
		       FROM memories m
		       JOIN agents a ON a.id = m.agent_id
		       WHERE m.tenant_id = $1
		         AND a.org_role = $2
		         AND m.source_type <> 'private'
		       ORDER BY m.importance DESC, m.created_at DESC
		       LIMIT $3`
		args = []any{defaultTenant, scopeKey, ckoMaxFacts}
	default:
		return nil, maxClass
	}

	rows, err := gw.db.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, maxClass
	}
	defer rows.Close()

	var facts []string
	for rows.Next() {
		var content string
		var class int
		if err := rows.Scan(&content, &class); err != nil {
			continue
		}
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		facts = append(facts, content)
		if memory.Classification(class) > maxClass {
			maxClass = memory.Classification(class)
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("cko.gather_sources.rows_err", "scope", scope, "err", err)
	}
	return facts, maxClass
}

// synthesizeBrief turns gathered facts into a concise organizational brief.
//
// Production path: a single metered LLM call (Origin "background", charged to
// the tenant overhead bucket since AgentID is blank) summarizes the facts into
// a tight digest. If no provider is available the call falls back to a
// deterministic bulleted join so a real brief row is still written.
func (gw *Gateway) synthesizeBrief(ctx context.Context, scope, scopeKey string, facts []string) (string, error) {
	if len(facts) == 0 {
		return "", nil
	}

	scopeLabel := scope
	if scope == "company" {
		scopeLabel = "the company"
	} else if scopeKey != "" {
		scopeLabel = fmt.Sprintf("%s %q", scope, scopeKey)
	}

	// Deterministic fallback used when no LLM provider is configured.
	fallback := func() string {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("Knowledge brief for %s:\n", scopeLabel))
		for _, f := range facts {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(f)
			b.WriteString("\n")
		}
		return strings.TrimSpace(b.String())
	}

	if gw.providerReg == nil {
		return fallback(), nil
	}
	provider := gw.providerReg.Default()
	if provider == nil {
		return fallback(), nil
	}

	prompt := fmt.Sprintf(`You are the Chief Knowledge Officer. Synthesize the facts below into a concise organizational knowledge brief for %s.

Rules:
- Group related facts; remove duplicates and noise.
- Lead with the most decision-relevant knowledge.
- Use short bullets; no preamble, no closing remarks.
- Keep it under 250 words. Plain text only.

Facts:
%s`, scopeLabel, "- "+strings.Join(facts, "\n- "))

	var model string
	if gw.agentLoop != nil && gw.agentLoop.SmartRouter != nil {
		model = gw.agentLoop.SmartRouter.BestModelForTier(providers.TierStandard)
	}

	// Background origin → overhead bucket (blank AgentID).
	mctx := providers.WithMeterScope(ctx, providers.MeterScope{
		TenantID: defaultTenant,
		Origin:   providers.OriginBackground,
	})
	resp, err := provider.Chat(mctx, providers.ChatRequest{
		Model:    model,
		Messages: []providers.Message{{Role: "user", Content: prompt}},
	})
	if err != nil || resp == nil || strings.TrimSpace(resp.Content) == "" {
		// Never fail the refresh on an LLM hiccup — write a real brief anyway.
		return fallback(), nil
	}
	return strings.TrimSpace(resp.Content), nil
}
