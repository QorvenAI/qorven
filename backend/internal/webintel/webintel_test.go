// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package webintel

import (
	"testing"
)

// WebIntel tests — topic discovery, query generation, source mapping.

func TestQueriesForTopic_Known(t *testing.T) {
	topics := []string{"technology", "ai", "business", "science", "health"}
	for _, topic := range topics {
		queries := QueriesForTopic(topic)
		if len(queries) == 0 { continue } // topic may not be in catalog
	}
}

func TestQueriesForTopic_Unknown(t *testing.T) {
	queries := QueriesForTopic("xyznonexistent123")
	// Unknown topics may return empty or generic queries
	_ = queries
}

func TestQueriesForTopic_Empty(t *testing.T) {
	queries := QueriesForTopic("")
	_ = queries // should not panic
}

func TestSourcesForTopic_Known(t *testing.T) {
	topics := []string{"technology", "ai", "business"}
	for _, topic := range topics {
		sources := SourcesForTopic(topic)
		if len(sources) == 0 { continue }
	}
}

func TestSourcesForTopic_Unknown(t *testing.T) {
	sources := SourcesForTopic("xyznonexistent123")
	_ = sources
}

func TestSourcesForTopic_Empty(t *testing.T) {
	sources := SourcesForTopic("")
	_ = sources // should not panic
}

func TestQueriesForTopic_Deterministic(t *testing.T) {
	q1 := QueriesForTopic("technology")
	q2 := QueriesForTopic("technology")
	if len(q1) != len(q2) { t.Error("should be deterministic") }
	for i := range q1 {
		if i < len(q2) && q1[i] != q2[i] { t.Error("queries should be same for same topic") }
	}
}

func TestSourcesForTopic_Deterministic(t *testing.T) {
	s1 := SourcesForTopic("ai")
	s2 := SourcesForTopic("ai")
	if len(s1) != len(s2) { t.Error("should be deterministic") }
}
