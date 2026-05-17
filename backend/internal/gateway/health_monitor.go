// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// health_monitor.go — watches backend dependencies and broadcasts state
// changes to all connected clients via the realtime hub.
//
// Why: when the DB goes down the WebSocket connections stay alive (they're
// TCP sockets to this process), so the frontend's existing "Reconnecting…"
// banner never fires. This monitor catches that gap: it probes the DB on
// every tick and broadcasts a service_health event whenever the state
// transitions. Clients render a specific "database unavailable" message
// instead of a generic spinner, and recover automatically when the DB
// comes back.

package gateway

import (
	"context"
	"log/slog"
	"time"

	"github.com/qorvenai/qorven/internal/realtime"
)

const (
	healthMonitorInterval = 15 * time.Second
	healthProbeTimeout    = 3 * time.Second
)

// startHealthMonitor launches a background goroutine that probes DB health
// and broadcasts service_health events on state transitions. Called once
// from Start() after the realtime hub is running.
func (gw *Gateway) startHealthMonitor(ctx context.Context) {
	go func() {
		var lastDBOK *bool // nil = unknown (first tick)
		ticker := time.NewTicker(healthMonitorInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				dbOK := gw.probeDB(ctx)
				if lastDBOK == nil || *lastDBOK != dbOK {
					lastDBOK = &dbOK
					gw.broadcastServiceHealth(dbOK)
				}
			}
		}
	}()
}

func (gw *Gateway) probeDB(ctx context.Context) bool {
	if gw.db == nil || gw.db.Pool == nil {
		return false
	}
	pctx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
	defer cancel()
	return gw.db.Pool.Ping(pctx) == nil
}

func (gw *Gateway) broadcastServiceHealth(dbOK bool) {
	if gw.rtHub == nil {
		return
	}
	dbStatus := "ok"
	overallStatus := "healthy"
	if !dbOK {
		dbStatus = "unavailable"
		overallStatus = "degraded"
		slog.Warn("service_health: database unavailable — broadcasting to clients")
	} else {
		slog.Info("service_health: database recovered — broadcasting to clients")
	}
	gw.rtHub.Broadcast(realtime.Event{
		Type: realtime.EventServiceHealth,
		Data: map[string]string{
			"database": dbStatus,
			"status":   overallStatus,
		},
	})
}
