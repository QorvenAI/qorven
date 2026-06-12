// Copyright 2026 Qorven AI. All rights reserved.
package knowledgegraph

import (
	"strings"
	"testing"
)

func TestStore_UpsertMethodSet(t *testing.T) {
	_ = (*Store).UpsertEntity
	_ = (*Store).UpsertRelationship
	_ = (*Store).RelevantContext
}

func TestFormatForPrompt(t *testing.T) {
	ents := []Entity{{ID: "1", Name: "Acme", EntityType: "org", Confidence: 0.9}}
	rels := []Relationship{{SourceID: "1", TargetID: "2", RelType: "uses"}}
	out := FormatForPrompt(ents, rels, map[string]string{"1": "Acme", "2": "Qorven"})
	if !strings.Contains(out, "Acme") || !strings.Contains(out, "uses") || !strings.Contains(out, "Qorven") {
		t.Fatalf("formatter missing content: %q", out)
	}
	if FormatForPrompt(nil, nil, nil) != "" {
		t.Fatal("empty input must produce empty string")
	}
}
