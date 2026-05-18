// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentBinding maps an agent to its channel instances.
// Each agent can have multiple channels (own Telegram bot, own Slack app, own email, etc.)
type AgentBinding struct {
	ID          string          `json:"id"`
	AgentID     string          `json:"agent_id"`
	TenantID    string          `json:"tenant_id"`
	ChannelType string          `json:"channel_type"` // telegram, slack, discord, whatsapp, email, webchat, webhook
	InstanceID  string          `json:"instance_id"`  // unique instance identifier
	DisplayName string          `json:"display_name"` // e.g. "Sara's Telegram Bot"
	Credentials json.RawMessage `json:"credentials"`  // encrypted channel credentials (bot token, API keys, etc.)
	Config      json.RawMessage `json:"config"`       // channel-specific config (allowed users, webhook URL, etc.)
	Enabled     bool            `json:"enabled"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// EmailRouting configures shared mailbox routing for email channels.
type EmailRouting struct {
	SharedMailbox string            `json:"shared_mailbox"` // e.g. team@qorven.ai
	Aliases       map[string]string `json:"aliases"`        // alias → agent_id mapping (e.g. "sara" → "agent-uuid")
}

// BindingStore manages agent-channel bindings in PostgreSQL.
type BindingStore struct {
	pool *pgxpool.Pool
}

// NewBindingStore creates a binding store.
func NewBindingStore(pool *pgxpool.Pool) *BindingStore {
	return &BindingStore{pool: pool}
}

// Create adds a new agent-channel binding.
func (s *BindingStore) Create(ctx context.Context, b AgentBinding) (string, error) {
	b.ID = uuid.New().String()
	b.InstanceID = fmt.Sprintf("%s-%s-%s", b.ChannelType, b.AgentID[:8], b.ID[:8])
	now := time.Now()
	b.CreatedAt = now
	b.UpdatedAt = now

	_, err := s.pool.Exec(ctx, `
		INSERT INTO agent_channel_bindings (id, agent_id, tenant_id, channel_type, instance_id, display_name, credentials, config, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		b.ID, b.AgentID, b.TenantID, b.ChannelType, b.InstanceID, b.DisplayName,
		b.Credentials, b.Config, b.Enabled, b.CreatedAt, b.UpdatedAt)
	if err != nil {
		return "", fmt.Errorf("create binding: %w", err)
	}
	return b.ID, nil
}

// ListForAgent returns all channel bindings for an agent.
func (s *BindingStore) ListForAgent(ctx context.Context, agentID string) ([]AgentBinding, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, agent_id, tenant_id, channel_type, instance_id, display_name, credentials, config, enabled, created_at, updated_at
		FROM agent_channel_bindings WHERE agent_id = $1 ORDER BY channel_type`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBindings(rows)
}

// ListForTenant returns all channel bindings for a tenant.
func (s *BindingStore) ListForTenant(ctx context.Context, tenantID string) ([]AgentBinding, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, agent_id, tenant_id, channel_type, instance_id, display_name, credentials, config, enabled, created_at, updated_at
		FROM agent_channel_bindings WHERE tenant_id = $1 ORDER BY agent_id, channel_type`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBindings(rows)
}

// Get returns a specific binding by ID.
func (s *BindingStore) Get(ctx context.Context, id string) (*AgentBinding, error) {
	var b AgentBinding
	err := s.pool.QueryRow(ctx, `
		SELECT id, agent_id, tenant_id, channel_type, instance_id, display_name, credentials, config, enabled, created_at, updated_at
		FROM agent_channel_bindings WHERE id = $1`, id).Scan(
		&b.ID, &b.AgentID, &b.TenantID, &b.ChannelType, &b.InstanceID, &b.DisplayName,
		&b.Credentials, &b.Config, &b.Enabled, &b.CreatedAt, &b.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// Update modifies a binding.
func (s *BindingStore) Update(ctx context.Context, id string, updates map[string]any) error {
	updates["updated_at"] = time.Now()
	// Whitelist of allowed columns to prevent SQL injection
	allowed := map[string]bool{
		"display_name": true, "credentials": true, "config": true, "enabled": true,
		"status": true, "channel_type": true, "updated_at": true,
	}
	// Build SET clause dynamically
	setClauses := make([]string, 0, len(updates))
	args := make([]any, 0, len(updates)+1)
	i := 1
	for col, val := range updates {
		if !allowed[col] { continue }
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, i))
		args = append(args, val)
		i++
	}
	args = append(args, id)
	query := fmt.Sprintf("UPDATE agent_channel_bindings SET %s WHERE id = $%d",
		joinStrings(setClauses, ", "), i)
	_, err := s.pool.Exec(ctx, query, args...)
	return err
}

// Delete removes a binding.
func (s *BindingStore) Delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM agent_channel_bindings WHERE id = $1", id)
	return err
}

// LoadAndBind loads all enabled bindings for a tenant and registers them with the channel manager.
func (s *BindingStore) LoadAndBind(ctx context.Context, tenantID string, mgr *Manager, factory ChannelFactory) error {
	bindings, err := s.ListForTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	for _, b := range bindings {
		if !b.Enabled {
			continue
		}
		ch, err := factory.Create(b)
		if err != nil {
			slog.Warn("channel binding create failed", "instance", b.InstanceID, "type", b.ChannelType, "error", err)
			continue
		}
		if bc, ok := ch.(interface{ SetAgentID(string) }); ok {
			bc.SetAgentID(b.AgentID)
		}
		mgr.Register(b.InstanceID, ch)
		slog.Info("agent channel bound", "agent", b.AgentID, "type", b.ChannelType, "instance", b.InstanceID)
	}
	return nil
}

// ChannelFactory creates Channel instances from bindings.
type ChannelFactory interface {
	Create(binding AgentBinding) (Channel, error)
}

func scanBindings(rows pgx.Rows) ([]AgentBinding, error) {
	bindings := []AgentBinding{}
	for rows.Next() {
		var b AgentBinding
		if err := rows.Scan(&b.ID, &b.AgentID, &b.TenantID, &b.ChannelType, &b.InstanceID,
			&b.DisplayName, &b.Credentials, &b.Config, &b.Enabled, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		bindings = append(bindings, b)
	}
	return bindings, nil
}

func joinStrings(s []string, sep string) string {
	result := ""
	for i, v := range s {
		if i > 0 {
			result += sep
		}
		result += v
	}
	return result
}
