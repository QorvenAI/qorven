// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package calendar

import (
	"context"
	"time"
)

// TimelineItem is one entry on the unified calendar — projected from cron_jobs,
// tasks, calendar_events, or scheduled_runs. The calendar never duplicates data;
// it reads each source and maps it into this shape.
type TimelineItem struct {
	ID        string     `json:"id"`
	Source    string     `json:"source"`
	SourceID  string     `json:"source_id"`
	AgentID   *string    `json:"agent_id"`
	AgentName string     `json:"agent_name"`
	Title     string     `json:"title"`
	Detail    string     `json:"detail"`
	When      time.Time  `json:"when"`
	EndAt     *time.Time `json:"end_at,omitempty"`
	Status    string     `json:"status"`
	Recurring bool       `json:"recurring"`
	Kind      string     `json:"kind"`
	Color     string     `json:"color"`
}

// kindFor classifies an item as future or past. Anything already executing or
// finished is "past"; a not-yet-run scheduled item in the future is "future".
func kindFor(when time.Time, status string, now time.Time) string {
	switch status {
	case "running", "ok", "error", "done", "cancelled":
		return "past"
	}
	if when.After(now) {
		return "future"
	}
	return "past"
}

// snippet trims s to at most n runes, for compact detail text.
func snippet(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// Timeline aggregates every scheduled-work source into one ordered list for the
// [start,end] window, optionally filtered to a single agent. All queries are
// tenant-scoped. Agent display names resolved via LEFT JOIN so a deleted agent
// doesn't drop or crash the row.
func (s *Store) Timeline(ctx context.Context, tenantID string, agentID *string, start, end time.Time) ([]TimelineItem, error) {
	now := time.Now()
	items := []TimelineItem{}

	// 1) Future cron jobs (enabled or paused) with next_run_at in window.
	{
		q := `SELECT cj.id, cj.agent_id, COALESCE(a.display_name,''), cj.name, COALESCE(cj.payload->>'instruction',''),
		             cj.next_run_at, cj.enabled, COALESCE(cj.one_shot,false)
		      FROM cron_jobs cj LEFT JOIN agents a ON a.id = cj.agent_id
		      WHERE cj.tenant_id=$1 AND cj.next_run_at IS NOT NULL AND cj.next_run_at >= $2 AND cj.next_run_at <= $3`
		args := []any{tenantID, start, end}
		if agentID != nil {
			q += ` AND cj.agent_id = $4`
			args = append(args, *agentID)
		}
		q += ` ORDER BY cj.next_run_at LIMIT 500`
		rows, err := s.pool.Query(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, name, instr, agentName string
			var aid *string
			var when time.Time
			var enabled, oneShot bool
			if rows.Scan(&id, &aid, &agentName, &name, &instr, &when, &enabled, &oneShot) != nil {
				continue
			}
			status := "scheduled"
			if !enabled {
				status = "paused"
			}
			items = append(items, TimelineItem{
				ID: "cron:" + id, Source: "cron", SourceID: id, AgentID: aid, AgentName: agentName,
				Title: name, Detail: snippet(instr, 140), When: when, Status: status,
				Recurring: !oneShot, Kind: kindFor(when, status, now),
			})
		}
		rows.Close()
	}

	// 2) Past runs (real execution history).
	{
		q := `SELECT sr.id, sr.agent_id, COALESCE(a.display_name,''), sr.title, sr.source,
		             sr.started_at, sr.status, sr.result_snippet
		      FROM scheduled_runs sr LEFT JOIN agents a ON a.id = sr.agent_id
		      WHERE sr.tenant_id=$1 AND sr.started_at >= $2 AND sr.started_at <= $3`
		args := []any{tenantID, start, end}
		if agentID != nil {
			q += ` AND sr.agent_id = $4`
			args = append(args, *agentID)
		}
		q += ` ORDER BY sr.started_at DESC LIMIT 500`
		rows, err := s.pool.Query(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, agentName, title, src, status, res string
			var aid *string
			var when time.Time
			if rows.Scan(&id, &aid, &agentName, &title, &src, &when, &status, &res) != nil {
				continue
			}
			items = append(items, TimelineItem{
				ID: "run:" + id, Source: "run", SourceID: id, AgentID: aid, AgentName: agentName,
				Title: title, Detail: snippet(res, 140), When: when, Status: status,
				Kind: "past",
			})
		}
		rows.Close()
	}

	// 3) Tasks with a due date in window.
	{
		q := `SELECT t.id, t.assigned_agent_id, t.title, COALESCE(t.description,''),
		             t.due_at, t.status, t.completed_at
		      FROM tasks t
		      WHERE t.tenant_id=$1 AND t.due_at IS NOT NULL AND t.due_at >= $2 AND t.due_at <= $3`
		args := []any{tenantID, start, end}
		if agentID != nil {
			q += ` AND t.assigned_agent_id = $4`
			args = append(args, *agentID)
		}
		q += ` ORDER BY t.due_at LIMIT 500`
		rows, err := s.pool.Query(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, title, desc, status string
			var assignedAgent *string
			var dueAt time.Time
			var completedAt *time.Time
			if rows.Scan(&id, &assignedAgent, &title, &desc, &dueAt, &status, &completedAt) != nil {
				continue
			}
			items = append(items, TimelineItem{
				ID: "task:" + id, Source: "task", SourceID: id, AgentID: assignedAgent,
				Title: title, Detail: snippet(desc, 140), When: dueAt, Status: status,
				Kind: kindFor(dueAt, status, now),
			})
		}
		rows.Close()
	}

	// 4) Manual calendar events (event_type='event' only).
	{
		q := `SELECT ce.id, ce.agent_id, COALESCE(a.display_name,''), ce.title, COALESCE(ce.description,''),
		             ce.start_at, ce.end_at, COALESCE(ce.color,'violet')
		      FROM calendar_events ce LEFT JOIN agents a ON a.id = ce.agent_id
		      WHERE ce.tenant_id=$1 AND ce.event_type='event' AND ce.start_at >= $2 AND ce.start_at <= $3`
		args := []any{tenantID, start, end}
		if agentID != nil {
			q += ` AND ce.agent_id = $4`
			args = append(args, *agentID)
		}
		q += ` ORDER BY ce.start_at LIMIT 500`
		rows, err := s.pool.Query(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, agentName, title, desc, color string
			var aid *string
			var startAt time.Time
			var endAt *time.Time
			if rows.Scan(&id, &aid, &agentName, &title, &desc, &startAt, &endAt, &color) != nil {
				continue
			}
			items = append(items, TimelineItem{
				ID: "event:" + id, Source: "event", SourceID: id, AgentID: aid, AgentName: agentName,
				Title: title, Detail: snippet(desc, 140), When: startAt, EndAt: endAt,
				Status: "scheduled", Kind: kindFor(startAt, "scheduled", now), Color: color,
			})
		}
		rows.Close()
	}

	return items, nil
}
