// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// levelAllowsChannel reports whether an agent at the given org_level may own
// communication channels. Only executives (L1 COO, L2 C-officers) qualify —
// L3 workers receive delegated work internally and never face the outside world.
func levelAllowsChannel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "l1", "l2":
		return true
	default:
		return false
	}
}

// channelAllowedForAgent returns nil if the agent may own channels, or an
// error explaining why not. Looks up the agent's org_level from the DB.
func (gw *Gateway) channelAllowedForAgent(ctx context.Context, agentID string) error {
	if gw.db == nil {
		return fmt.Errorf("database not available")
	}
	if agentID == "" {
		return fmt.Errorf("agent_id required")
	}
	var level string
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT COALESCE(org_level, 'l3') FROM agents WHERE id = $1 AND tenant_id = $2`,
		agentID, defaultTenant,
	).Scan(&level)
	if err != nil {
		return fmt.Errorf("agent not found")
	}
	if !levelAllowsChannel(level) {
		return fmt.Errorf("channel_requires_executive")
	}
	return nil
}

// disableAgentChannels disables and stops every channel owned by an agent.
// Used when an agent is demoted to L3 or terminated — their channels must
// stop running but are preserved in the DB (re-enableable if re-promoted).
// Returns the number of channels disabled.
func (gw *Gateway) disableAgentChannels(ctx context.Context, agentID string) (int, error) {
	if gw.db == nil || agentID == "" {
		return 0, nil
	}
	rows, err := gw.db.Pool.Query(ctx,
		`SELECT id FROM channel_instances WHERE agent_id = $1 AND tenant_id = $2 AND enabled = true`,
		agentID, defaultTenant)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	if len(ids) == 0 {
		return 0, nil
	}
	_, err = gw.db.Pool.Exec(ctx,
		`UPDATE channel_instances SET enabled = false, status = 'disabled' WHERE agent_id = $1 AND tenant_id = $2`,
		agentID, defaultTenant)
	if err != nil {
		return 0, err
	}
	if gw.chanMgr != nil {
		for _, id := range ids {
			if stopErr := gw.chanMgr.Stop(ctx, id); stopErr != nil {
				slog.Warn("channel.disable.stop_failed", "instance", id, "error", stopErr)
			}
		}
	}
	slog.Info("channel.disabled_for_agent", "agent", agentID, "count", len(ids))
	return len(ids), nil
}
