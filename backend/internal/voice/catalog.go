// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// ─── Voice-provider catalog ────────────────────────────────────────────
//
// Mirrors the LLM providers.StaticModelCatalog. Embedded JSON (one
// file, shipped in the binary) describing every supported driver.
// The Settings UI and the setup wizard render provider cards
// straight off this data.
//
// New providers: add an entry to voice_catalog.json, add a case in
// builder.go, add the Go struct. That's it — no UI code to touch.

//go:embed voice_catalog.json
var voiceCatalogJSON []byte

// CatalogField describes one form input for a driver's config.
type CatalogField struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"`        // password | url | text
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
}

// CatalogEntry is one provider row. KindSupports is an array because
// some drivers ship both TTS and STT from the same credentials
// (OpenAI, HuggingFace, Ollama).
type CatalogEntry struct {
	ID            string                     `json:"id"`
	Name          string                     `json:"name"`
	KindSupports  []string                   `json:"kind_supports"`
	Hosting       string                     `json:"hosting"` // cloud | local
	Auth          string                     `json:"auth"`    // api_key | none | oauth
	Streaming     bool                       `json:"streaming"`
	Hint          string                     `json:"hint,omitempty"`
	HardwareHint  string                     `json:"hardware_hint,omitempty"`
	Fields        []CatalogField             `json:"fields"`
	Models        map[string][]string        `json:"models,omitempty"`
	DefaultModel  map[string]string          `json:"default_model,omitempty"`
}

// Catalog is the in-memory representation.
type Catalog struct {
	Drivers []CatalogEntry `json:"drivers"`
}

var (
	catalogOnce sync.Once
	catalog     *Catalog
	catalogErr  error
)

// LoadCatalog parses the embedded JSON once and caches the result.
// Safe for concurrent use after the first call. Returns a non-nil
// error only if the JSON is malformed — callers can treat it as
// never-fail in practice.
func LoadCatalog() (*Catalog, error) {
	catalogOnce.Do(func() {
		var c Catalog
		if err := json.Unmarshal(voiceCatalogJSON, &c); err != nil {
			catalogErr = err
			return
		}
		catalog = &c
	})
	return catalog, catalogErr
}

// Filter returns catalog entries matching the supplied filters. An
// empty filter value matches everything. This is the single helper
// the REST handler uses.
func (c *Catalog) Filter(kind, hosting string) []CatalogEntry {
	if c == nil { return nil }
	out := make([]CatalogEntry, 0, len(c.Drivers))
	for _, e := range c.Drivers {
		if hosting != "" && e.Hosting != hosting { continue }
		if kind != "" {
			ok := false
			for _, k := range e.KindSupports {
				if k == kind { ok = true; break }
			}
			if !ok { continue }
		}
		out = append(out, e)
	}
	return out
}

// Lookup finds a catalog entry by id. Returns nil when unknown.
func (c *Catalog) Lookup(id string) *CatalogEntry {
	if c == nil { return nil }
	for i := range c.Drivers {
		if c.Drivers[i].ID == id {
			return &c.Drivers[i]
		}
	}
	return nil
}
