// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Platform is a DB-backed connector platform definition.
type Platform struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Category    string          `json:"category"`
	Description string          `json:"description"`
	Icon        string          `json:"icon"`
	AuthType    string          `json:"auth_type"`
	AuthConfig  json.RawMessage `json:"auth_config"`
	BaseURL     string          `json:"base_url"`
	DocsURL     string          `json:"docs_url"`
	Enabled     bool            `json:"enabled"`
	Connected   bool            `json:"connected,omitempty"` // populated at query time
}

// ActionDef is a DB-backed action definition.
type ActionDef struct {
	ID           string          `json:"id"`
	PlatformID   string          `json:"platform_id"`
	ActionKey    string          `json:"action_key"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	WhenToUse    string          `json:"when_to_use"`
	Method       string          `json:"method"`
	Path         string          `json:"path"`
	Headers      json.RawMessage `json:"headers"`
	Params       json.RawMessage `json:"params"`
	BodyTemplate string          `json:"body_template"`
	ResponseDesc string          `json:"response_desc"`
}

// KnowledgeStore provides CRUD for connector platforms and actions,
// plus BuildKnowledge for injecting into agent system prompts.
type KnowledgeStore struct {
	pool *pgxpool.Pool
}

func NewKnowledgeStore(pool *pgxpool.Pool) *KnowledgeStore {
	return &KnowledgeStore{pool: pool}
}

// --- Platform CRUD ---

func (s *KnowledgeStore) UpsertPlatform(ctx context.Context, p Platform) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO connector_platforms (id, name, category, description, icon, auth_type, auth_config, base_url, docs_url, enabled)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (id) DO UPDATE SET name=$2, category=$3, description=$4, icon=$5, auth_type=$6, auth_config=$7, base_url=$8, docs_url=$9, enabled=$10`,
		p.ID, p.Name, p.Category, p.Description, p.Icon, p.AuthType, p.AuthConfig, p.BaseURL, p.DocsURL, p.Enabled)
	return err
}

func (s *KnowledgeStore) ListPlatforms(ctx context.Context) ([]Platform, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, category, description, icon, auth_type, auth_config, base_url, docs_url, enabled
		 FROM connector_platforms WHERE enabled = true ORDER BY category, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Platform{}
	for rows.Next() {
		var p Platform
		rows.Scan(&p.ID, &p.Name, &p.Category, &p.Description, &p.Icon, &p.AuthType, &p.AuthConfig, &p.BaseURL, &p.DocsURL, &p.Enabled)
		out = append(out, p)
	}
	return out, nil
}

func (s *KnowledgeStore) GetPlatform(ctx context.Context, id string) (*Platform, error) {
	var p Platform
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, category, description, icon, auth_type, auth_config, base_url, docs_url, enabled
		 FROM connector_platforms WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.Category, &p.Description, &p.Icon, &p.AuthType, &p.AuthConfig, &p.BaseURL, &p.DocsURL, &p.Enabled)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// --- Action CRUD ---

func (s *KnowledgeStore) UpsertAction(ctx context.Context, a ActionDef) error {
	// Default empty JSON objects for NOT NULL JSONB fields
	if a.Headers == nil { a.Headers = json.RawMessage("{}") }
	if a.Params == nil { a.Params = json.RawMessage("{}") }
	_, err := s.pool.Exec(ctx,
		`INSERT INTO connector_actions (platform_id, action_key, name, description, when_to_use, method, path, headers, params, body_template, response_desc)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (platform_id, action_key) DO UPDATE SET name=$3, description=$4, when_to_use=$5, method=$6, path=$7, headers=$8, params=$9, body_template=$10, response_desc=$11`,
		a.PlatformID, a.ActionKey, a.Name, a.Description, a.WhenToUse, a.Method, a.Path, a.Headers, a.Params, a.BodyTemplate, a.ResponseDesc)
	return err
}

func (s *KnowledgeStore) ListActions(ctx context.Context, platformID string) ([]ActionDef, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, platform_id, action_key, name, description, when_to_use, method, path, headers, params, body_template, response_desc
		 FROM connector_actions WHERE platform_id = $1 ORDER BY action_key`, platformID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ActionDef{}
	for rows.Next() {
		var a ActionDef
		rows.Scan(&a.ID, &a.PlatformID, &a.ActionKey, &a.Name, &a.Description, &a.WhenToUse, &a.Method, &a.Path, &a.Headers, &a.Params, &a.BodyTemplate, &a.ResponseDesc)
		out = append(out, a)
	}
	return out, nil
}

func (s *KnowledgeStore) GetAction(ctx context.Context, platformID, actionKey string) (*ActionDef, error) {
	var a ActionDef
	err := s.pool.QueryRow(ctx,
		`SELECT id, platform_id, action_key, name, description, when_to_use, method, path, headers, params, body_template, response_desc
		 FROM connector_actions WHERE platform_id = $1 AND action_key = $2`, platformID, actionKey,
	).Scan(&a.ID, &a.PlatformID, &a.ActionKey, &a.Name, &a.Description, &a.WhenToUse, &a.Method, &a.Path, &a.Headers, &a.Params, &a.BodyTemplate, &a.ResponseDesc)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// --- Knowledge Builder ---

// ListConnected returns platforms that have credentials stored for a tenant.
func (s *KnowledgeStore) ListConnected(ctx context.Context, tenantID string) ([]Platform, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT p.id, p.name, p.category, p.description, p.icon, p.auth_type, p.auth_config, p.base_url, p.docs_url, p.enabled
		 FROM connector_platforms p
		 INNER JOIN credentials c ON c.platform_id = p.id AND c.tenant_id = $1
		 WHERE p.enabled = true ORDER BY p.name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Platform{}
	for rows.Next() {
		var p Platform
		rows.Scan(&p.ID, &p.Name, &p.Category, &p.Description, &p.Icon, &p.AuthType, &p.AuthConfig, &p.BaseURL, &p.DocsURL, &p.Enabled)
		p.Connected = true
		out = append(out, p)
	}
	return out, nil
}

// BuildKnowledge generates a compact connector manifest for agent system prompts.
// Emits one line per connected platform — no action details.
// Agents call list_connector_actions(platform) when they need the full action catalogue.
// ~50 tokens per platform vs ~286 tokens in the old full-dump format.
func (s *KnowledgeStore) BuildKnowledge(ctx context.Context, tenantID string) (string, error) {
	connected, err := s.ListConnected(ctx, tenantID)
	if err != nil || len(connected) == 0 {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("## Connected Services\n")
	sb.WriteString("Use execute_action(platform, action, params) to call these services.\n")
	sb.WriteString("Call list_connector_actions(platform) to see a service's available actions.\n")

	for _, p := range connected {
		// Count actions for the inventory line without loading full schemas.
		actions, _ := s.ListActions(ctx, p.ID)
		if len(actions) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("- %s (%s, %d actions)\n", p.Name, p.Category, len(actions)))
	}
	return sb.String(), nil
}

// BuildKnowledgeFull returns the original full action-catalogue dump.
// Only used by diagnostic/admin endpoints — never called in the agent loop.
func (s *KnowledgeStore) BuildKnowledgeFull(ctx context.Context, tenantID string) (string, error) {
	connected, err := s.ListConnected(ctx, tenantID)
	if err != nil || len(connected) == 0 {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("## Connected Services\n\nYou can interact with these services using the `execute_action` tool.\nCall: execute_action({\"platform\": \"<id>\", \"action\": \"<action_key>\", \"params\": {...}})\n\n")

	for _, p := range connected {
		actions, err := s.ListActions(ctx, p.ID)
		if err != nil || len(actions) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("### %s (%s)\n", p.Name, p.Category))
		for _, a := range actions {
			sb.WriteString(fmt.Sprintf("- **%s** — %s\n  When: %s\n  Params: %s\n", a.ActionKey, a.Description, a.WhenToUse, string(a.Params)))
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
