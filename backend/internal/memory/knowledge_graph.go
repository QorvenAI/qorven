// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import "time"

// Entity is a node in the knowledge graph. Used by the entity extractor to
// represent raw extracted entities before they are persisted via
// knowledgegraph.Store.
type Entity struct {
	ID         string            `json:"id"`
	AgentID    string            `json:"agent_id"`
	Name       string            `json:"name"`
	EntityType string            `json:"entity_type"` // person, project, concept, tool, location, org
	Properties map[string]string `json:"properties,omitempty"`
	Source     string            `json:"source,omitempty"`
	Confidence float64           `json:"confidence"`
	CreatedAt  time.Time         `json:"created_at"`
}

// Relationship is an edge in the knowledge graph. SourceID and TargetID hold
// the raw LLM-supplied entity NAME strings at extraction time; callers must
// remap them to UUIDs before persisting via knowledgegraph.Store.
type Relationship struct {
	ID         string            `json:"id"`
	SourceID   string            `json:"source_id"`
	TargetID   string            `json:"target_id"`
	RelType    string            `json:"rel_type"` // works_on, knows, uses, part_of, manages, created
	Properties map[string]string `json:"properties,omitempty"`
	Confidence float64           `json:"confidence"`
	CreatedAt  time.Time         `json:"created_at"`
}
