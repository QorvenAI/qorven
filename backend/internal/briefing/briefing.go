// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package briefing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/datasource"
)

// Builder constructs and delivers the daily briefing for an agent.
type Builder struct {
	pool      *pgxpool.Pool
	agentLoop *agent.Loop
	snapshots *datasource.SnapshotStore // nil if not wired
}

// NewBuilder creates a briefing builder.
// snapshots may be nil; if so, connector snapshot sections are omitted.
func NewBuilder(pool *pgxpool.Pool, agentLoop *agent.Loop, snapshots *datasource.SnapshotStore) *Builder {
	return &Builder{pool: pool, agentLoop: agentLoop, snapshots: snapshots}
}

// Deliver builds and pushes the morning briefing for a given agent.
func (b *Builder) Deliver(ctx context.Context, agentID, tenantID string) error {
	content, err := b.buildContent(ctx, agentID, tenantID)
	if err != nil {
		slog.Warn("briefing.build_failed", "agent", agentID, "err", err)
		return err
	}

	_, err = b.agentLoop.Run(ctx, agent.RunRequest{
		AgentID:       agentID,
		TenantID:      tenantID,
		UserMessage:   "[SYSTEM BRIEFING REQUEST]\n\n" + content,
		SourceChannel: "system",
	}, nil)
	return err
}

func (b *Builder) buildContent(ctx context.Context, agentID, tenantID string) (string, error) {
	now := time.Now()
	day := now.Format("Monday 2 January 2006")
	hour := now.Format("15:04")

	var pendingDrafts int
	b.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM draft_replies WHERE agent_id=$1 AND status='pending'`,
		agentID,
	).Scan(&pendingDrafts)

	var pendingTasks int
	b.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tasks WHERE agent_id=$1 AND status NOT IN ('done','cancelled')`,
		agentID,
	).Scan(&pendingTasks)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Good morning! Here's your %s briefing — %s\n\n", hour, day)
	if pendingDrafts > 0 {
		fmt.Fprintf(&sb, "Pending draft replies: %d (review them in the Drafts panel)\n", pendingDrafts)
	} else {
		fmt.Fprintf(&sb, "No pending draft replies\n")
	}
	if pendingTasks > 0 {
		fmt.Fprintf(&sb, "Active tasks: %d\n", pendingTasks)
	}
	fmt.Fprintf(&sb, "\nHave a productive day!")

	// Connector snapshot sections — injected when SnapshotStore is wired.
	if b.snapshots != nil && tenantID != "" {
		slugs, err := b.snapshots.ListSlugs(ctx, tenantID)
		if err == nil {
			for _, slug := range slugs {
				data, latestErr := b.snapshots.Latest(ctx, tenantID, slug)
				if latestErr != nil || len(data) == 0 {
					continue
				}
				// Try to extract the text value directly; fall back to full JSON.
				var section string
				for _, v := range data {
					if s, ok := v.(string); ok {
						section = s
						break
					}
				}
				if section == "" {
					if jsonBytes, marshalErr := json.Marshal(data); marshalErr == nil {
						section = string(jsonBytes)
					}
				}
				if section != "" {
					sb.WriteString("\n## " + slug + "\n" + section + "\n")
				}
			}
		}
	}

	return sb.String(), nil
}
