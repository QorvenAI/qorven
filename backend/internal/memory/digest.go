// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"strings"
)

// DigestStore manages compiled memory digests for faster, more relevant retrieval.
// Instead of returning raw memories, digests provide structured summaries
// organized by topic, recency, and importance.
type DigestStore struct {
	store *Store
}

func NewDigestStore(store *Store) *DigestStore {
	return &DigestStore{store: store}
}

// GetDigest returns the compiled digest for an agent, or builds one on-demand.
func (ds *DigestStore) GetDigest(ctx context.Context, agentID string) (string, error) {
	// Try cached bulletin first
	bulletin, err := ds.store.GetLatestBulletin(ctx, agentID)
	if err == nil && bulletin != "" {
		return bulletin, nil
	}

	// Build on-demand
	return ds.BuildDigest(ctx, agentID)
}

// BuildDigest compiles raw memories into a structured digest.
func (ds *DigestStore) BuildDigest(ctx context.Context, agentID string) (string, error) {
	// Get top memories by type
	types := []string{"identity", "preference", "fact", "skill", "relationship", "goal"}
	var sections []string

	for _, memType := range types {
		mems, err := ds.store.SearchByType(ctx, agentID, memType, 5)
		if err != nil || len(mems) == 0 { continue }

		var items []string
		for _, m := range mems {
			preview := m.Content
			if len(preview) > 150 { preview = preview[:150] + "..." }
			items = append(items, fmt.Sprintf("- %s", preview))
		}
		sections = append(sections, fmt.Sprintf("**%s:**\n%s", strings.ToUpper(memType[:1]) + memType[1:], strings.Join(items, "\n")))
	}

	if len(sections) == 0 {
		return "", nil
	}

	return strings.Join(sections, "\n\n"), nil
}

// SearchWithDigest performs a hybrid search: first checks the digest for context,
// then falls back to raw memory search for specific queries.
func (ds *DigestStore) SearchWithDigest(ctx context.Context, tenantID, agentID, query string, maxResults int) ([]SearchResult, string, error) {
	// Get digest for broad context
	digest, _ := ds.GetDigest(ctx, agentID)

	// Also do targeted search for the specific query
	results, err := ds.store.Search(ctx, tenantID, agentID, query, maxResults)

	return results, digest, err
}
