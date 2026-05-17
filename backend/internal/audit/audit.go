// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorAgent  ActorType = "agent"
	ActorSystem ActorType = "system"
)

type Entry struct {
	ID         int64           `json:"id"`
	TenantID   string          `json:"tenant_id"`
	ActorType  ActorType       `json:"actor_type"`
	ActorID    string          `json:"actor_id"`
	ActorName  string          `json:"actor_name"`
	Action     string          `json:"action"`
	Resource   string          `json:"resource"`
	ResourceID string          `json:"resource_id"`
	Details    json.RawMessage `json:"details"`
	IPAddress  string          `json:"ip_address"`
	CreatedAt  time.Time       `json:"created_at"`
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Log records an audit event.
func (s *Store) Log(ctx context.Context, tenantID string, actorType ActorType, actorID, actorName, action, resource, resourceID string, details any, ip string) {
	detailsJSON, _ := json.Marshal(details)
	s.pool.Exec(ctx,
		`INSERT INTO audit_log (tenant_id, actor_type, actor_id, actor_name, action, resource, resource_id, details, ip_address)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		tenantID, actorType, actorID, actorName, action, resource, resourceID, detailsJSON, ip)
}

// Query returns audit entries with filters.
func (s *Store) Query(ctx context.Context, tenantID string, opts QueryOpts) ([]Entry, int, error) {
	where := "tenant_id = $1"
	args := []any{tenantID}
	n := 2

	if opts.ActorID != "" {
		where += " AND actor_id = $" + itoa(n)
		args = append(args, opts.ActorID)
		n++
	}
	if opts.Resource != "" {
		where += " AND resource = $" + itoa(n)
		args = append(args, opts.Resource)
		n++
	}
	if opts.Action != "" {
		where += " AND action = $" + itoa(n)
		args = append(args, opts.Action)
		n++
	}
	if !opts.Since.IsZero() {
		where += " AND created_at >= $" + itoa(n)
		args = append(args, opts.Since)
		n++
	}

	var total int
	s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_log WHERE "+where, args...).Scan(&total)

	limit := opts.Limit
	if limit <= 0 { limit = 50 }
	offset := opts.Offset

	rows, err := s.pool.Query(ctx,
		"SELECT id, tenant_id, actor_type, actor_id, actor_name, action, resource, resource_id, details, ip_address, created_at FROM audit_log WHERE "+where+" ORDER BY created_at DESC LIMIT "+itoa(limit)+" OFFSET "+itoa(offset),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries := []Entry{}
	for rows.Next() {
		var e Entry
		rows.Scan(&e.ID, &e.TenantID, &e.ActorType, &e.ActorID, &e.ActorName, &e.Action, &e.Resource, &e.ResourceID, &e.Details, &e.IPAddress, &e.CreatedAt)
		entries = append(entries, e)
	}
	return entries, total, nil
}

type QueryOpts struct {
	ActorID  string
	Resource string
	Action   string
	Since    time.Time
	Limit    int
	Offset   int
}

func itoa(n int) string {
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if s == "" { return "0" }
	return s
}
