// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
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

// ListAll returns every bundle for an agent regardless of enabled state (used by the editor UI).
func (s *BundleStore) ListAll(ctx context.Context, agentID string) ([]Bundle, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, bundle_type, name, content, priority, enabled
		 FROM agent_bundles WHERE agent_id=$1 ORDER BY priority DESC, name`,
		agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Bundle{}
	for rows.Next() {
		var b Bundle
		rows.Scan(&b.ID, &b.AgentID, &b.BundleType, &b.Name, &b.Content, &b.Priority, &b.Enabled)
		out = append(out, b)
	}
	return out, nil
}

// Delete removes a bundle by row ID.
func (s *BundleStore) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM agent_bundles WHERE id=$1", id)
	return err
}

// DeleteByType removes all bundles of a given type for an agent.
func (s *BundleStore) DeleteByType(ctx context.Context, agentID, bundleType string) error {
	_, err := s.pool.Exec(ctx,
		"DELETE FROM agent_bundles WHERE agent_id=$1 AND bundle_type=$2",
		agentID, bundleType)
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
// It is idempotent: existing bundles are updated in-place via ON CONFLICT DO UPDATE,
// so calling this on an already-seeded agent overwrites with the latest archetype content.
func (s *BundleStore) SeedDefaults(ctx context.Context, agentID, role string) {
	seed, ok := AgentSeeds[role]
	if !ok {
		seed = AgentSeeds["general"]
	}
	if seed.Identity != "" {
		s.Upsert(ctx, Bundle{AgentID: agentID, BundleType: "identity", Name: "identity", Priority: 100, Enabled: true, Content: seed.Identity})
	}
	if seed.Tools != "" {
		s.Upsert(ctx, Bundle{AgentID: agentID, BundleType: "tools", Name: "tools", Priority: 90, Enabled: true, Content: seed.Tools})
	}
}
