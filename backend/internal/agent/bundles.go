// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Bundle is a named instruction block for an agent.
type Bundle struct {
	ID         string `json:"id"`
	AgentID    string `json:"agent_id"`
	BundleType string `json:"bundle_type"` // soul, tools, identity, custom
	Name       string `json:"name"`
	Content    string `json:"content"`
	Priority   int    `json:"priority"`
	Enabled    bool   `json:"enabled"`
}

// BundleStore manages per-agent instruction bundles.
type BundleStore struct{ pool *pgxpool.Pool }

func NewBundleStore(pool *pgxpool.Pool) *BundleStore { return &BundleStore{pool: pool} }

// Upsert creates or updates a bundle.
func (s *BundleStore) Upsert(ctx context.Context, b Bundle) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_bundles (agent_id, bundle_type, name, content, priority, enabled)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (agent_id, bundle_type, name) DO UPDATE SET content=$4, priority=$5, enabled=$6, updated_at=NOW()`,
		b.AgentID, b.BundleType, b.Name, b.Content, b.Priority, b.Enabled)
	return err
}

// List returns all bundles for an agent, ordered by priority desc.
func (s *BundleStore) List(ctx context.Context, agentID string) ([]Bundle, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, bundle_type, name, content, priority, enabled
		 FROM agent_bundles WHERE agent_id=$1 AND enabled=true ORDER BY priority DESC, name`,
		agentID)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []Bundle{}
	for rows.Next() {
		var b Bundle
		rows.Scan(&b.ID, &b.AgentID, &b.BundleType, &b.Name, &b.Content, &b.Priority, &b.Enabled)
		out = append(out, b)
	}
	return out, nil
}

// Delete removes a bundle.
func (s *BundleStore) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM agent_bundles WHERE id=$1", id)
	return err
}

// BuildPromptSection assembles all bundles into a system prompt section.
func (s *BundleStore) BuildPromptSection(ctx context.Context, agentID string) string {
	bundles, err := s.List(ctx, agentID)
	if err != nil || len(bundles) == 0 { return "" }

	var sb strings.Builder
	for _, b := range bundles {
		sb.WriteString(fmt.Sprintf("\n## %s\n%s\n", strings.Title(b.BundleType), b.Content))
	}
	return sb.String()
}

// SeedDefaults creates default bundles for an agent based on role.
func (s *BundleStore) SeedDefaults(ctx context.Context, agentID, role string) {
	switch role {
	case "chief":
		s.Upsert(ctx, Bundle{AgentID: agentID, BundleType: "identity", Name: "chief", Priority: 100, Enabled: true,
			Content: `You are the Chief of Staff — the user's primary AI assistant.
You have full access to all tools, agents, and services.
You can delegate tasks to specialist agents.
You manage budgets, review work, and coordinate the team.
When delegating, always confirm: "I'll have [Agent] handle that."
Report back when delegated tasks complete.`})
		s.Upsert(ctx, Bundle{AgentID: agentID, BundleType: "tools", Name: "sdk", Priority: 90, Enabled: true,
			Content: `You have access to ALL tools including:
- execute_action: Call any connected external service (Gmail, Slack, Sheets, etc.)
- web_search / web_fetch: Search and browse the web
- All MCP server tools
- Task management: create, assign, transition tasks
- Agent management: create, update agents
Always use tools to take action — never just describe what you would do.`})
	case "director":
		s.Upsert(ctx, Bundle{AgentID: agentID, BundleType: "identity", Name: "director", Priority: 100, Enabled: true,
			Content: `You are a Department Director. You manage specialists in your area.
You can delegate tasks within your department.
You have access to department-relevant tools and connections.
Report progress to the Chief of Staff when asked.`})
	case "specialist":
		s.Upsert(ctx, Bundle{AgentID: agentID, BundleType: "identity", Name: "specialist", Priority: 100, Enabled: true,
			Content: `You are a Specialist agent focused on your specific domain.
Execute tasks assigned to you efficiently.
Use only the tools available to you.
Report completion to your manager.`})
	}
}
