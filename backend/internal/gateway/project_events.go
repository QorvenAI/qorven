// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package gateway

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/qorvenai/qorven/internal/realtime"
)

// projectEventRow mirrors the project_events table.
type projectEventRow struct {
	ProjectBriefID string
	Type           string
	Title          string
	Payload        map[string]any
	TaskID         string // "" → NULL
	AgentID        string // "" → NULL
}

// realtimeTypeFor maps a project_event type to the WS event type the frontend
// listens on. task_progress/done/blocked map to their dedicated constants;
// task_started (and any unmapped type) falls back to the generic project_updated
// refresh — which the project timeline/dashboard already subscribe to — so a
// task-start event still triggers a live refresh.
func realtimeTypeFor(t string) string {
	switch t {
	case "task_progress":
		return realtime.EventTaskProgress
	case "done":
		return realtime.EventTaskDone
	case "blocked":
		return realtime.EventTaskBlocked
	case "budget_warning":
		return realtime.EventBudgetWarning
	default: // task_started, pr_opened, gate_decision, agent_spawned, …
		return realtime.EventProjectUpdated
	}
}

// buildProjectEvent is the pure core (no DB/hub): it builds the DB row and the WS
// event, embedding project_id in the event data so the FE can filter by project.
func buildProjectEvent(briefID, typ, title string, payload map[string]any, taskID, agentID string) (projectEventRow, realtime.Event) {
	if payload == nil {
		payload = map[string]any{}
	}
	data := map[string]any{"project_id": briefID, "type": typ, "title": title}
	for k, v := range payload {
		data[k] = v
	}
	if taskID != "" {
		data["task_id"] = taskID
	}
	if agentID != "" {
		data["agent_id"] = agentID
	}
	return projectEventRow{ProjectBriefID: briefID, Type: typ, Title: title, Payload: payload, TaskID: taskID, AgentID: agentID},
		realtime.Event{Type: realtimeTypeFor(typ), AgentID: agentID, Data: data}
}

// projectBriefForTask returns the project_brief_id for a task, or "" if the
// task has no project link or the DB is unavailable. Best-effort: any error
// is silently swallowed so the caller never stalls.
func (gw *Gateway) projectBriefForTask(ctx context.Context, taskID string) string {
	if gw.db == nil || gw.db.Pool == nil || taskID == "" {
		return ""
	}
	var bid string
	_ = gw.db.Pool.QueryRow(ctx,
		`SELECT COALESCE(project_brief_id::text,'') FROM tasks WHERE id=$1`, taskID).Scan(&bid)
	return bid
}

// emitProjectEvent durably records a project event and broadcasts it live.
// Best-effort: a failure in either half is logged, never blocks the caller
// (agents must not stall on telemetry). This is the contract 8C emits into.
func (gw *Gateway) emitProjectEvent(ctx context.Context, briefID, typ, title string, payload map[string]any, taskID, agentID string) {
	if briefID == "" {
		return
	}
	row, ev := buildProjectEvent(briefID, typ, title, payload, taskID, agentID)
	if gw.db != nil && gw.db.Pool != nil {
		pj, _ := json.Marshal(row.Payload)
		if _, err := gw.db.Pool.Exec(ctx,
			`INSERT INTO project_events (tenant_id, project_brief_id, task_id, agent_id, type, title, payload)
			 VALUES ($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5,$6,$7)`,
			defaultTenant, row.ProjectBriefID, row.TaskID, row.AgentID, row.Type, row.Title, pj); err != nil {
			slog.Warn("project_event.insert_failed", "brief", briefID, "type", typ, "err", err)
		}
	}
	if gw.rtHub != nil {
		gw.rtHub.Broadcast(ev)
	}
}
