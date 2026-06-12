// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/calendar"
)

// runCalendarSync pushes future timeline items OUT to every enabled external
// calendar sync whose scope covers them. One-way OUT only. Panic-isolated by
// the caller.
func (gw *Gateway) runCalendarSync(ctx context.Context) {
	if gw.calSyncStore == nil || gw.calendarStore == nil || gw.connExec == nil {
		return
	}
	syncs, err := gw.calSyncStore.ListSyncs(ctx, defaultTenant)
	if err != nil || len(syncs) == 0 {
		return
	}
	start := time.Now()
	end := start.AddDate(0, 0, 30)
	for _, sc := range syncs {
		if !sc.Enabled {
			continue
		}
		var agentFilter *string
		if sc.Scope == "private" && sc.OwnerAgentID != nil {
			agentFilter = sc.OwnerAgentID
		}
		items, terr := gw.calendarStore.Timeline(ctx, defaultTenant, agentFilter, start, end)
		if terr != nil {
			_ = gw.calSyncStore.MarkSynced(ctx, sc.ID, terr.Error())
			continue
		}
		sScopeID := ""
		if sc.ScopeID != nil {
			sScopeID = *sc.ScopeID
		}
		sOwner := ""
		if sc.OwnerAgentID != nil {
			sOwner = *sc.OwnerAgentID
		}
		pushed := 0
		for _, it := range items {
			if it.Kind != "future" {
				continue
			}
			itemAgent := ""
			if it.AgentID != nil {
				itemAgent = *it.AgentID
			}
			itemDept := ""
			if sc.Scope == "department" && itemAgent != "" {
				itemDept = gw.agentDepartmentID(ctx, itemAgent)
			}
			if !calendar.SyncMatchesItem(sc.Scope, sScopeID, sOwner, itemAgent, itemDept) {
				continue
			}
			// Skip items already pushed successfully — create_event has no upsert,
			// and this ticker re-reads all future items every run, so without this
			// guard each item would be duplicated in the external calendar per tick.
			if gw.calSyncStore.AlreadyPushed(ctx, it.ID, sc.ID) {
				continue
			}
			endStr := it.When.Add(time.Hour).Format(time.RFC3339)
			if it.EndAt != nil {
				endStr = it.EndAt.Format(time.RFC3339)
			}
			params := map[string]any{
				"summary":     it.Title,
				"start":       it.When.Format(time.RFC3339),
				"end":         endStr,
				"description": it.Detail,
				"event_id":    gw.calSyncStore.RemoteEventID(ctx, it.ID, sc.ID),
			}
			out, perr := gw.connExec.Execute(ctx, sc.Provider, "create_event", params)
			if perr != nil {
				_ = gw.calSyncStore.RecordEventPush(ctx, it.ID, sc.ID, "", "error", perr.Error())
				continue
			}
			_ = gw.calSyncStore.RecordEventPush(ctx, it.ID, sc.ID, extractRemoteID(out), "ok", "")
			pushed++
		}
		_ = gw.calSyncStore.MarkSynced(ctx, sc.ID, "")
		slog.Info("calendar.sync.done", "sync", sc.ID, "provider", sc.Provider, "pushed", pushed)
	}
}

// agentDepartmentID returns an agent's direct department id ("" if none).
func (gw *Gateway) agentDepartmentID(ctx context.Context, agentID string) string {
	if gw.db == nil {
		return ""
	}
	var dept *string
	_ = gw.db.Pool.QueryRow(ctx, `SELECT department_id FROM agents WHERE id = $1`, agentID).Scan(&dept)
	if dept == nil {
		return ""
	}
	return *dept
}

// startCalendarSyncTicker runs the calendar sync every 5 minutes, panic-isolated.
func (gw *Gateway) startCalendarSyncTicker(ctx context.Context) {
	if gw.calSyncStore == nil {
		return
	}
	safeGo("calendar.sync.ticker", func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				func() {
					defer func() {
						if rec := recover(); rec != nil {
							slog.Error("calendar.sync.panic", "panic", rec)
						}
					}()
					cctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer cancel()
					gw.runCalendarSync(cctx)
				}()
			}
		}
	})
}
