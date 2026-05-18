// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/qorvenai/qorven/internal/providers"
)

// EntityExtractor uses an LLM to extract entities and relationships from conversation text.
const entityExtractionPrompt = `Extract entities and relationships from this conversation.

Entities: people, projects, tools, concepts, organizations, locations.
Relationships: works_on, knows, uses, part_of, manages, created.

Conversation:
User: %s
Assistant: %s

Return JSON only:
{"entities":[{"name":"...","type":"person|project|tool|concept|org|location"}],"relationships":[{"source":"...","target":"...","type":"works_on|knows|uses|part_of|manages|created"}]}`

// ExtractEntities extracts entities and relationships from a conversation turn.
func ExtractEntities(ctx context.Context, provider providers.Provider, model, userMsg, assistantMsg string) ([]Entity, []Relationship) {
	if userMsg == "" && assistantMsg == "" {
		return nil, nil
	}

	// Truncate for extraction
	if len(userMsg) > 500 { userMsg = userMsg[:500] }
	if len(assistantMsg) > 500 { assistantMsg = assistantMsg[:500] }

	prompt := strings.Replace(strings.Replace(entityExtractionPrompt, "%s", userMsg, 1), "%s", assistantMsg, 1)

	resp, err := provider.Chat(ctx, providers.ChatRequest{
		Model:    model,
		Messages: []providers.Message{{Role: "user", Content: prompt}},
		Options:  map[string]any{"max_tokens": 500, "temperature": 0.1},
	})
	if err != nil {
		slog.Debug("kg.extract.failed", "error", err)
		return nil, nil
	}

	return parseEntityResponse(resp.Content)
}

func parseEntityResponse(raw string) ([]Entity, []Relationship) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, nil
	}

	var parsed struct {
		Entities []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"entities"`
		Relationships []struct {
			Source string `json:"source"`
			Target string `json:"target"`
			Type   string `json:"type"`
		} `json:"relationships"`
	}

	if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err != nil {
		return nil, nil
	}

	var entities []Entity
	for _, e := range parsed.Entities {
		if e.Name == "" { continue }
		entities = append(entities, Entity{
			Name:       e.Name,
			EntityType: e.Type,
			Confidence: 0.7,
		})
	}

	var rels []Relationship
	for _, r := range parsed.Relationships {
		if r.Source == "" || r.Target == "" { continue }
		rels = append(rels, Relationship{
			SourceID: r.Source, // will be resolved to entity IDs when saved
			TargetID: r.Target,
			RelType:  r.Type,
			Confidence: 0.6,
		})
	}

	return entities, rels
}
