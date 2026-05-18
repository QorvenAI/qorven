// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentMessage is a message between two agents.
type AgentMessage struct {
	ID        string    `json:"id"`
	FromAgent string    `json:"from_agent"`
	ToAgent   string    `json:"to_agent"`
	TaskID    *string   `json:"task_id,omitempty"`
	Content   string    `json:"content"`
	Type      string    `json:"message_type"` // message, delegation, report, escalation, review
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageStore handles inter-agent communication.
type MessageStore struct {
	pool *pgxpool.Pool
}

func NewMessageStore(pool *pgxpool.Pool) *MessageStore {
	return &MessageStore{pool: pool}
}

// Send sends a message from one agent to another.
func (s *MessageStore) Send(ctx context.Context, tenantID string, msg AgentMessage) (string, error) {
	if msg.Type == "" {
		msg.Type = "message"
	}
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO agent_messages (tenant_id, from_agent, to_agent, task_id, content, message_type)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		tenantID, msg.FromAgent, msg.ToAgent, msg.TaskID, msg.Content, msg.Type,
	).Scan(&id)
	return id, err
}

// GetUnread returns unread messages for an agent.
func (s *MessageStore) GetUnread(ctx context.Context, agentID string) ([]AgentMessage, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, from_agent, to_agent, task_id, content, message_type, read, created_at
		 FROM agent_messages WHERE to_agent = $1 AND read = false
		 ORDER BY created_at`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	msgs := []AgentMessage{}
	for rows.Next() {
		var m AgentMessage
		rows.Scan(&m.ID, &m.FromAgent, &m.ToAgent, &m.TaskID, &m.Content, &m.Type, &m.Read, &m.CreatedAt)
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// MarkRead marks messages as read.
func (s *MessageStore) MarkRead(ctx context.Context, agentID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE agent_messages SET read = true WHERE to_agent = $1 AND read = false`, agentID)
	return err
}

// GetSubordinates returns agents that report to the given manager.
func GetSubordinates(ctx context.Context, pool *pgxpool.Pool, managerID string) ([]*Agent, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, agent_key, display_name, role, title, model, status
		 FROM agents WHERE manager_id = $1 AND deleted_at IS NULL ORDER BY display_name`, managerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agents := []*Agent{}
	for rows.Next() {
		a := &Agent{}
		rows.Scan(&a.ID, &a.AgentKey, &a.DisplayName, &a.Role, &a.Title, &a.Model, &a.Status)
		agents = append(agents, a)
	}
	return agents, nil
}

// GetOrgChart returns the full agent hierarchy for a tenant.
func GetOrgChart(ctx context.Context, pool *pgxpool.Pool, tenantID string) ([]map[string]any, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, agent_key, display_name, role, title, manager_id, model, status
		 FROM agents WHERE tenant_id = $1 AND deleted_at IS NULL ORDER BY display_name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	chart := []map[string]any{}
	for rows.Next() {
		var id, key, name, model, status string
		var role, title *string
		var managerID *string
		rows.Scan(&id, &key, &name, &role, &title, &managerID, &model, &status)
		entry := map[string]any{
			"id": id, "agent_key": key, "display_name": name,
			"model": model, "status": status,
		}
		if role != nil {
			entry["role"] = *role
		}
		if title != nil {
			entry["title"] = *title
		}
		if managerID != nil {
			entry["manager_id"] = *managerID
		}
		chart = append(chart, entry)
	}
	return chart, nil
}

// FormatInboxForContext formats unread messages for injection into agent's context.
func FormatInboxForContext(msgs []AgentMessage) string {
	if len(msgs) == 0 {
		return ""
	}
	var result string
	result = "## Inbox (unread messages from other agents)\n\n"
	for _, m := range msgs {
		result += fmt.Sprintf("- [%s] from %s: %s\n", m.Type, m.FromAgent, m.Content)
		if m.TaskID != nil {
			result += fmt.Sprintf("  (regarding task: %s)\n", *m.TaskID)
		}
	}
	return result
}
