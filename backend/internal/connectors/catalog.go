// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

type CatalogEntry struct {
	ID         string   `json:"id"`
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	ImgSrc     string   `json:"img_src"`
	Categories []string `json:"categories"`
	Installed  bool     `json:"installed"`
}

type Catalog struct {
	mu        sync.RWMutex
	entries   []CatalogEntry
	fetchedAt time.Time
	apiKey    string
	knowledge *KnowledgeStore
	client    *http.Client
}

func NewCatalog(apiKey string, ks *KnowledgeStore) *Catalog {
	return &Catalog{
		apiKey:    apiKey,
		knowledge: ks,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Refresh fetches the full app list from Pipedream if cache is stale (>1h)
func (c *Catalog) Refresh(ctx context.Context) error {
	c.mu.RLock()
	if time.Since(c.fetchedAt) < time.Hour {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	// Fetch from Pipedream
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.pipedream.com/v1/connect/apps", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("pipedream catalog fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("pipedream catalog: HTTP %d: %s", resp.StatusCode, body)
	}

	var result struct {
		Data []struct {
			ID         string   `json:"id"`
			NameSlug   string   `json:"name_slug"`
			Name       string   `json:"name"`
			ImgSrc     string   `json:"img_src"`
			Categories []string `json:"categories"`
			Published  bool     `json:"published"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("pipedream catalog decode: %w", err)
	}

	// Get already-installed platform IDs
	installed := make(map[string]bool)
	if platforms, err := c.knowledge.ListPlatforms(ctx); err == nil {
		for _, p := range platforms {
			installed[p.ID] = true
		}
	}

	entries := make([]CatalogEntry, 0, len(result.Data))
	for _, app := range result.Data {
		if !app.Published {
			continue
		}
		entries = append(entries, CatalogEntry{
			ID:         app.ID,
			Slug:       app.NameSlug,
			Name:       app.Name,
			ImgSrc:     app.ImgSrc,
			Categories: app.Categories,
			Installed:  installed[app.NameSlug],
		})
	}

	c.mu.Lock()
	c.entries = entries
	c.fetchedAt = time.Now()
	c.mu.Unlock()

	slog.Info("pipedream.catalog.refreshed", "apps", len(entries))
	return nil
}

// Search returns catalog entries matching a query string (name or category)
func (c *Catalog) Search(query string, limit int) []CatalogEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if query == "" {
		if limit > 0 && limit < len(c.entries) {
			return c.entries[:limit]
		}
		return c.entries
	}

	q := strings.ToLower(query)
	var results []CatalogEntry
	for _, e := range c.entries {
		if strings.Contains(strings.ToLower(e.Name), q) || strings.Contains(e.Slug, q) {
			results = append(results, e)
			if limit > 0 && len(results) >= limit {
				break
			}
			continue
		}
		for _, cat := range e.Categories {
			if strings.Contains(strings.ToLower(cat), q) {
				results = append(results, e)
				if limit > 0 && len(results) >= limit {
					return results
				}
				break
			}
		}
	}
	return results
}

// Activate adds a Pipedream catalog app as a connector_platform, creating a stub with generic actions
func (c *Catalog) Activate(ctx context.Context, slug, name string, categories []string) error {
	category := "integration"
	if len(categories) > 0 {
		category = categories[0]
	}

	platform := Platform{
		ID:          slug,
		Name:        name,
		Category:    category,
		Description: fmt.Sprintf("%s integration via Pipedream", name),
		Icon:        "puzzle",
		AuthType:    "oauth2",
		AuthConfig:  json.RawMessage(`{"provider":"pipedream"}`),
		BaseURL:     "https://api.pipedream.com/v1/connect",
		DocsURL:     fmt.Sprintf("https://pipedream.com/apps/%s", slug),
		Enabled:     true,
	}

	if err := c.knowledge.UpsertPlatform(ctx, platform); err != nil {
		return fmt.Errorf("activate platform %s: %w", slug, err)
	}

	// Add generic actions that work for most Pipedream apps
	genericActions := []ActionDef{
		{
			PlatformID:        slug,
			ActionKey:         "run_action",
			Name:              fmt.Sprintf("Run %s Action", name),
			Description:       fmt.Sprintf("Execute a custom action on %s via Pipedream", name),
			WhenToUse:         fmt.Sprintf("When user needs to interact with %s and no specific action is available", name),
			Method:            "POST",
			Path:              "/connect/actions/run",
			Params:            json.RawMessage(`{"action_id":{"type":"string","required":true,"desc":"Pipedream action component ID"},"props":{"type":"object","required":false,"desc":"Action parameters as key-value pairs"}}`),
			ResponseDesc:      "Action execution result",
			ExecutionBackend:  "pipedream",
			PipedreamActionID: slug + "-run-action",
		},
	}

	for _, a := range genericActions {
		_ = c.knowledge.UpsertAction(ctx, a)
	}

	// Mark as installed in cache
	c.mu.Lock()
	for i, e := range c.entries {
		if e.Slug == slug {
			c.entries[i].Installed = true
			break
		}
	}
	c.mu.Unlock()

	return nil
}

// Count returns total catalog size
func (c *Catalog) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
