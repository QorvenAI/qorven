// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package knowledgegraph

import "fmt"

// diff.go — Compare two graph snapshots and report changes.

// GraphDiff captures changes between two graph snapshots.
type GraphDiff struct {
	AddedEntities      []Entity       `json:"added_entities"`
	RemovedEntities    []Entity       `json:"removed_entities"`
	AddedRelationships []Relationship `json:"added_relationships"`
	RemovedRelationships []Relationship `json:"removed_relationships"`
	ModifiedEntities   []EntityChange `json:"modified_entities,omitempty"`
}

// EntityChange describes a modification to an entity.
type EntityChange struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Field    string `json:"field"`
	OldValue string `json:"old_value"`
	NewValue string `json:"new_value"`
}

// DiffGraphs compares old and new entity/relationship sets.
func DiffGraphs(oldEntities, newEntities []Entity, oldRels, newRels []Relationship) GraphDiff {
	diff := GraphDiff{}

	oldEntityMap := map[string]Entity{}
	for _, e := range oldEntities { oldEntityMap[e.ID] = e }
	newEntityMap := map[string]Entity{}
	for _, e := range newEntities { newEntityMap[e.ID] = e }

	// Find added/removed entities
	for _, e := range newEntities {
		if _, ok := oldEntityMap[e.ID]; !ok { diff.AddedEntities = append(diff.AddedEntities, e) }
	}
	for _, e := range oldEntities {
		if _, ok := newEntityMap[e.ID]; !ok { diff.RemovedEntities = append(diff.RemovedEntities, e) }
	}

	// Find modified entities (name or type changed)
	for _, newE := range newEntities {
		if oldE, ok := oldEntityMap[newE.ID]; ok {
			if oldE.Name != newE.Name {
				diff.ModifiedEntities = append(diff.ModifiedEntities, EntityChange{
					ID: newE.ID, Name: newE.Name, Field: "name", OldValue: oldE.Name, NewValue: newE.Name,
				})
			}
			if oldE.EntityType != newE.EntityType {
				diff.ModifiedEntities = append(diff.ModifiedEntities, EntityChange{
					ID: newE.ID, Name: newE.Name, Field: "type", OldValue: oldE.EntityType, NewValue: newE.EntityType,
				})
			}
		}
	}

	// Find added/removed relationships
	oldRelSet := map[string]Relationship{}
	for _, r := range oldRels { oldRelSet[r.SourceID+"|"+r.TargetID+"|"+r.RelType] = r }
	newRelSet := map[string]Relationship{}
	for _, r := range newRels { newRelSet[r.SourceID+"|"+r.TargetID+"|"+r.RelType] = r }

	for key, r := range newRelSet {
		if _, ok := oldRelSet[key]; !ok { diff.AddedRelationships = append(diff.AddedRelationships, r) }
	}
	for key, r := range oldRelSet {
		if _, ok := newRelSet[key]; !ok { diff.RemovedRelationships = append(diff.RemovedRelationships, r) }
	}

	return diff
}

// Summary returns a human-readable summary of the diff.
func (d GraphDiff) Summary() string {
	return fmt.Sprintf("+%d/-%d entities, +%d/-%d relationships, %d modified",
		len(d.AddedEntities), len(d.RemovedEntities),
		len(d.AddedRelationships), len(d.RemovedRelationships),
		len(d.ModifiedEntities))
}

// IsEmpty returns true if there are no changes.
func (d GraphDiff) IsEmpty() bool {
	return len(d.AddedEntities) == 0 && len(d.RemovedEntities) == 0 &&
		len(d.AddedRelationships) == 0 && len(d.RemovedRelationships) == 0 &&
		len(d.ModifiedEntities) == 0
}
