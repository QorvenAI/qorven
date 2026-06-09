// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"

	"github.com/qorvenai/qorven/internal/memory"
)

// CKOCurator builds a knowledge brief for a scope by gathering the org's
// knowledge sources and synthesizing them, then persisting the result.
// Collaborators are injected so the routine is testable and so production
// can wire the real memory store, Drive, work-item store, research, and LLM.
type CKOCurator struct {
	TenantID string

	// GatherSources returns the facts for a scope and the highest classification among them.
	GatherSources func(ctx context.Context, scope, scopeKey string) ([]string, memory.Classification)
	// Synthesize turns facts into a concise brief (LLM-backed in production).
	Synthesize func(ctx context.Context, scope, scopeKey string, facts []string) (string, error)
	// WriteBrief persists the brief (BriefStore.Upsert in production).
	WriteBrief func(ctx context.Context, b memory.Brief) error

	// ExternalResearchEnabled gates the costly external-research source (off by default).
	ExternalResearchEnabled bool
}

// Refresh curates and writes the brief for one scope. It is a no-op when there
// are no facts (avoids writing empty briefs).
func (c *CKOCurator) Refresh(ctx context.Context, scope, scopeKey string) error {
	facts, clearance := c.GatherSources(ctx, scope, scopeKey)
	if len(facts) == 0 {
		return nil
	}
	content, err := c.Synthesize(ctx, scope, scopeKey, facts)
	if err != nil {
		return err
	}
	return c.WriteBrief(ctx, memory.Brief{
		Scope:     scope,
		ScopeKey:  scopeKey,
		Clearance: clearance,
		Content:   content,
	})
}
