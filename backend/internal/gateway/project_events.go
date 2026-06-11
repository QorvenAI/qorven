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
// listens on. task_started surfaces as its own signal; task_progress maps to
// the EventTaskProgress constant; unmapped types fall back to the generic
// project_updated refresh.
func realtimeTypeFor(t string) string {
	switch t {
	case "task_progress":
		return realtime.EventTaskProgress
	case "task_started":
		return "task_started"
	case "done":
		return realtime.EventTaskDone
	case "blocked":
		return realtime.EventTaskBlocked
	case "budget_warning":
		return realtime.EventBudgetWarning
	default:
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
