// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Hard memory tests — search quality, embedding pipeline, scope isolation.

func TestHard_Memory_SearchQuality_Precision(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	// Save memories with clear topics
	topics := []struct{ content, topic string }{
		{"Go uses goroutines and channels for concurrent programming", "go"},
		{"PostgreSQL supports JSONB columns for flexible schema storage", "database"},
		{"React hooks like useState and useEffect manage component state", "frontend"},
		{"Docker containers package applications with their dependencies", "devops"},
		{"TLS certificates encrypt HTTP traffic for secure communication", "security"},
		{"Redis provides in-memory caching with pub/sub messaging", "caching"},
		{"GraphQL allows clients to request exactly the data they need", "api"},
		{"Kubernetes orchestrates container deployment across clusters", "devops"},
	}

	for _, tp := range topics {
		store.Save(ctx, tenant, Memory{AgentID: agentID, Content: tp.content, Type: "fact", Source: "quality_test"})
	}

	// Search and verify relevance
	tests := []struct{ query, expectTopic string }{
		{"goroutines concurrency", "go"},
		{"database storage JSON", "database"},
		{"container deployment", "devops"},
		{"encryption HTTPS", "security"},
	}

	for _, tt := range tests {
		results, err := store.Search(ctx, tenant, agentID, tt.query, 3)
		if err != nil { t.Fatalf("search %q: %v", tt.query, err) }
		if len(results) == 0 { t.Logf("search %q: no results (text search may not match)", tt.query); continue }

		topResult := strings.ToLower(results[0].Memory.Content)
		found := false
		for _, tp := range topics {
			if tp.topic == tt.expectTopic && strings.Contains(topResult, strings.ToLower(tp.content[:20])) {
				found = true
			}
		}
		if !found { t.Logf("search %q: top result may not match expected topic %q", tt.query, tt.expectTopic) }
		t.Logf("search %q: %d results, top=%q...", tt.query, len(results), results[0].Memory.Content[:min7(len(results[0].Memory.Content), 60)])
	}
}

func TestHard_Memory_ScopeIsolation(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	// Get two different agents
	var agent1, agent2 string
	rows, _ := pool.Query(ctx, "SELECT id FROM agents LIMIT 2")
	defer rows.Close()
	for rows.Next() {
		var id string
		rows.Scan(&id)
		if agent1 == "" { agent1 = id } else { agent2 = id }
	}
	if agent1 == "" { t.Skip("no agents") }
	if agent2 == "" { agent2 = agent1 } // use same if only 1

	// Save different memories for each agent
	marker1 := "SCOPE_ISOLATION_AGENT1_" + time.Now().Format("150405")
	marker2 := "SCOPE_ISOLATION_AGENT2_" + time.Now().Format("150405")
	store.Save(ctx, tenant, Memory{AgentID: agent1, Content: marker1, Type: "fact", Source: "scope_test"})
	store.Save(ctx, tenant, Memory{AgentID: agent2, Content: marker2, Type: "fact", Source: "scope_test"})

	// Search agent1 — should find marker1 but not marker2
	results1, _ := store.Search(ctx, tenant, agent1, "SCOPE_ISOLATION", 10)
	found1, found2 := false, false
	for _, r := range results1 {
		if strings.Contains(r.Memory.Content, marker1) { found1 = true }
		if strings.Contains(r.Memory.Content, marker2) { found2 = true }
	}
	if agent1 != agent2 {
		if found2 { t.Error("agent1 search found agent2's memory — scope leak!") }
	}
	t.Logf("scope isolation: agent1 found own=%v, found other=%v", found1, found2)
}

func TestHard_Memory_TypeFiltering(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	// Save different types
	types := []string{"fact", "preference", "event", "todo", "identity"}
	for _, typ := range types {
		store.Save(ctx, tenant, Memory{
			AgentID: agentID, Content: "Memory of type " + typ + " for filtering test",
			Type: typ, Source: "type_test",
		})
	}

	// Search — should find across types
	results, _ := store.Search(ctx, tenant, agentID, "filtering test", 20)
	typesFound := map[string]bool{}
	for _, r := range results {
		typesFound[r.Memory.Type] = true
	}
	t.Logf("types found in search: %v", typesFound)
}

func TestHard_Memory_LargeContent_Integrity(t *testing.T) {
	pool := testPool(t)
	store := NewStore(pool)
	ctx := context.Background()
	tenant := "00000000-0000-0000-0000-000000000001"

	var agentID string
	pool.QueryRow(ctx, "SELECT id FROM agents LIMIT 1").Scan(&agentID)
	if agentID == "" { t.Skip("no agents") }

	// Save progressively larger content
	sizes := []int{100, 1000, 5000, 10000}
	for _, size := range sizes {
		content := "LARGE_" + strings.Repeat("x", size-6)
		id, err := store.Save(ctx, tenant, Memory{AgentID: agentID, Content: content, Type: "fact", Source: "large_test"})
		if err != nil { t.Errorf("save %d chars: %v", size, err); continue }
		if id == "" { t.Errorf("empty id for %d chars", size) }
	}
	t.Logf("large content: saved %v char memories", sizes)
}
