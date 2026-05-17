// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store

import "context"

type Entity struct {
	ID          string            `json:"id"`
	AgentID     string            `json:"agent_id"`
	UserID      string            `json:"user_id,omitempty"`
	ExternalID  string            `json:"external_id"`
	Name        string            `json:"name"`
	EntityType  string            `json:"entity_type"`
	Description string            `json:"description,omitempty"`
	Properties  map[string]string `json:"properties,omitempty"`
	SourceID    string            `json:"source_id,omitempty"`
	Confidence  float64           `json:"confidence"`
	CreatedAt   int64             `json:"created_at"`
	UpdatedAt   int64             `json:"updated_at"`
}

type Relation struct {
	ID             string            `json:"id"`
	AgentID        string            `json:"agent_id"`
	UserID         string            `json:"user_id,omitempty"`
	SourceEntityID string            `json:"source_entity_id"`
	RelationType   string            `json:"relation_type"`
	TargetEntityID string            `json:"target_entity_id"`
	Confidence     float64           `json:"confidence"`
	Properties     map[string]string `json:"properties,omitempty"`
	CreatedAt      int64             `json:"created_at"`
}

type TraversalResult struct {
	Entity Entity   `json:"entity"`
	Depth  int      `json:"depth"`
	Path   []string `json:"path"`
	Via    string   `json:"via"`
}

type EntityListOptions struct {
	EntityType string
	Limit      int
	Offset     int
}

type GraphStats struct {
	EntityCount   int            `json:"entity_count"`
	RelationCount int            `json:"relation_count"`
	EntityTypes   map[string]int `json:"entity_types"`
}

type DedupCandidate struct {
	ID         string  `json:"id"`
	EntityA    Entity  `json:"entity_a"`
	EntityB    Entity  `json:"entity_b"`
	Similarity float64 `json:"similarity"`
	Status     string  `json:"status"`
	CreatedAt  int64   `json:"created_at"`
}

type KnowledgeGraphStore interface {
	UpsertEntity(ctx context.Context, entity *Entity) error
	GetEntity(ctx context.Context, agentID, userID, entityID string) (*Entity, error)
	DeleteEntity(ctx context.Context, agentID, userID, entityID string) error
	ListEntities(ctx context.Context, agentID, userID string, opts EntityListOptions) ([]Entity, error)
	SearchEntities(ctx context.Context, agentID, userID, query string, limit int) ([]Entity, error)
	UpsertRelation(ctx context.Context, relation *Relation) error
	DeleteRelation(ctx context.Context, agentID, userID, relationID string) error
	ListRelations(ctx context.Context, agentID, userID, entityID string) ([]Relation, error)
	ListAllRelations(ctx context.Context, agentID, userID string, limit int) ([]Relation, error)
	Traverse(ctx context.Context, agentID, userID, startEntityID string, maxDepth int) ([]TraversalResult, error)
	IngestExtraction(ctx context.Context, agentID, userID string, entities []Entity, relations []Relation) ([]string, error)
	PruneByConfidence(ctx context.Context, agentID, userID string, minConfidence float64) (int, error)
	DedupAfterExtraction(ctx context.Context, agentID, userID string, newEntityIDs []string) (merged int, flagged int, err error)
	ScanDuplicates(ctx context.Context, agentID, userID string, threshold float64, limit int) (int, error)
	ListDedupCandidates(ctx context.Context, agentID, userID string, limit int) ([]DedupCandidate, error)
	MergeEntities(ctx context.Context, agentID, userID, targetID, sourceID string) error
	DismissCandidate(ctx context.Context, agentID, candidateID string) error
	Stats(ctx context.Context, agentID, userID string) (*GraphStats, error)
	SetEmbeddingProvider(provider EmbeddingProvider)
	Close() error
}
