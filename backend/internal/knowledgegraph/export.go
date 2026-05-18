// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package knowledgegraph

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// export.go — Export knowledge graph to JSON, GraphML, Cypher (Neo4j), Obsidian.

// GraphExport holds the full graph for serialization.
type GraphExport struct {
	Entities      []Entity       `json:"entities"`
	Relationships []Relationship `json:"relationships"`
	Stats         map[string]int `json:"stats"`
}

// ExportJSON writes the graph as formatted JSON.
func ExportJSON(entities []Entity, relationships []Relationship, path string) error {
	export := GraphExport{
		Entities:      entities,
		Relationships: relationships,
		Stats:         map[string]int{"entities": len(entities), "relationships": len(relationships)},
	}
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil { return err }
	return os.WriteFile(path, data, 0644)
}

// ExportGraphML writes the graph as GraphML XML.
func ExportGraphML(entities []Entity, relationships []Relationship, path string) error {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<graphml xmlns="http://graphml.graphstruct.org/graphml">
  <key id="label" for="node" attr.name="label" attr.type="string"/>
  <key id="type" for="node" attr.name="type" attr.type="string"/>
  <key id="weight" for="edge" attr.name="weight" attr.type="double"/>
  <graph id="G" edgedefault="directed">
`)
	for _, e := range entities {
		sb.WriteString(fmt.Sprintf("    <node id=\"%s\"><data key=\"label\">%s</data><data key=\"type\">%s</data></node>\n",
			xmlEsc(e.ID), xmlEsc(e.Name), e.EntityType))
	}
	for i, r := range relationships {
		sb.WriteString(fmt.Sprintf("    <edge id=\"e%d\" source=\"%s\" target=\"%s\"><data key=\"weight\">%.2f</data></edge>\n",
			i, xmlEsc(r.SourceID), xmlEsc(r.TargetID), r.Confidence))
	}
	sb.WriteString("  </graph>\n</graphml>\n")
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// ExportCypher writes Neo4j Cypher CREATE statements.
func ExportCypher(entities []Entity, relationships []Relationship, path string) error {
	var sb strings.Builder
	for _, e := range entities {
		sb.WriteString(fmt.Sprintf("CREATE (n_%s:%s {id: '%s', name: '%s'});\n",
			cypherSafe(e.ID), e.EntityType, e.ID, cypherEsc(e.Name)))
	}
	for _, r := range relationships {
		sb.WriteString(fmt.Sprintf("MATCH (a {id: '%s'}), (b {id: '%s'}) CREATE (a)-[:%s {confidence: %f}]->(b);\n",
			r.SourceID, r.TargetID, strings.ToUpper(r.RelType), r.Confidence))
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// ExportObsidian writes each entity as an Obsidian markdown file with wiki-links.
func ExportObsidian(entities []Entity, relationships []Relationship, dir string) error {
	os.MkdirAll(dir, 0755)

	neighbors := map[string][]string{}
	for _, r := range relationships {
		neighbors[r.SourceID] = append(neighbors[r.SourceID], r.TargetID)
		neighbors[r.TargetID] = append(neighbors[r.TargetID], r.SourceID)
	}

	entityMap := map[string]Entity{}
	for _, e := range entities { entityMap[e.ID] = e }

	for _, e := range entities {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\nType: %s\nConfidence: %.2f\n\n## Connections\n\n", e.Name, e.EntityType, e.Confidence))
		for _, nid := range neighbors[e.ID] {
			if n, ok := entityMap[nid]; ok {
				sb.WriteString(fmt.Sprintf("- [[%s]]\n", sanitizeFN(n.Name)))
			}
		}
		os.WriteFile(fmt.Sprintf("%s/%s.md", dir, sanitizeFN(e.Name)), []byte(sb.String()), 0644)
	}
	return nil
}

func xmlEsc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func cypherEsc(s string) string  { return strings.ReplaceAll(s, "'", "\\'") }
func cypherSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' { return r }
		return '_'
	}, s)
}
func sanitizeFN(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == ' ' { return r }
		return '_'
	}, s)
}
