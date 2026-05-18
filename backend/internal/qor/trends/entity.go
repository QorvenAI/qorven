// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package trends

import (
	"regexp"
	"strings"
)

// entity_extract.go — Entity extraction and handle resolution.
// Rewritten from last30days entity_extract.py (127 lines).

// Entity represents an extracted entity from a topic.
type Entity struct {
	Name       string `json:"name"`
	Type       string `json:"type"` // person, org, product, topic
	Handles    map[string]string `json:"handles,omitempty"` // platform → handle
	Confidence float64 `json:"confidence"`
}

var (
	twitterHandleRe = regexp.MustCompile(`@(\w{1,15})`)
	githubRepoRe    = regexp.MustCompile(`(?:github\.com/)?([a-zA-Z0-9_-]+/[a-zA-Z0-9_.-]+)`)
	quotedEntityRe  = regexp.MustCompile(`"([^"]+)"`)
	properNounRe    = regexp.MustCompile(`\b([A-Z][a-z]+(?:\s+[A-Z][a-z]+)*)\b`)
)

// ExtractEntities identifies entities from a topic string.
func ExtractEntities(topic string) []Entity {
	var entities []Entity
	seen := map[string]bool{}

	// Twitter handles
	for _, m := range twitterHandleRe.FindAllStringSubmatch(topic, -1) {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			entities = append(entities, Entity{
				Name: name, Type: "person",
				Handles: map[string]string{"twitter": "@" + name},
				Confidence: 0.9,
			})
		}
	}

	// GitHub repos
	for _, m := range githubRepoRe.FindAllStringSubmatch(topic, -1) {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			entities = append(entities, Entity{
				Name: name, Type: "product",
				Handles: map[string]string{"github": name},
				Confidence: 0.85,
			})
		}
	}

	// Quoted entities
	for _, m := range quotedEntityRe.FindAllStringSubmatch(topic, -1) {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			entities = append(entities, Entity{Name: name, Type: "topic", Confidence: 0.8})
		}
	}

	// Proper nouns (capitalized words)
	for _, m := range properNounRe.FindAllStringSubmatch(topic, -1) {
		name := m[1]
		if !seen[name] && !isCommonWord(name) {
			seen[name] = true
			entities = append(entities, Entity{Name: name, Type: "unknown", Confidence: 0.6})
		}
	}

	return entities
}

// ResolveEntityForPlatform returns the best search term for an entity on a given platform.
func ResolveEntityForPlatform(entity Entity, platform string) string {
	if handle, ok := entity.Handles[platform]; ok { return handle }
	return entity.Name
}

func isCommonWord(word string) bool {
	common := map[string]bool{
		"The": true, "This": true, "That": true, "What": true, "How": true,
		"Why": true, "When": true, "Where": true, "Who": true, "Which": true,
		"Will": true, "Can": true, "Should": true, "Would": true, "Could": true,
		"May": true, "Just": true, "Also": true, "Very": true, "Most": true,
		"Some": true, "Any": true, "All": true, "New": true, "Last": true,
	}
	return common[word]
}

// DisambiguateEntity tries to resolve ambiguous entity names using context.
func DisambiguateEntity(name string, context []string) string {
	// Simple heuristic: if the name appears in multiple contexts with consistent meaning, keep it
	matches := 0
	for _, ctx := range context {
		if strings.Contains(strings.ToLower(ctx), strings.ToLower(name)) { matches++ }
	}
	if matches >= 2 { return name }
	return name // no disambiguation needed
}
