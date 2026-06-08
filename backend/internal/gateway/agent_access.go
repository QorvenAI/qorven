// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// errAgentNotChattable is returned when a user tries to open a direct chat
// session with an agent that is not an executive. Matched with errors.Is at
// call sites so wrapping never silently degrades the 403 response. Mirrors the
// errChannelRequiresExecutive pattern in channel_access.go.
var errAgentNotChattable = errors.New("agent_not_chattable")

// isNotChattable reports whether err is the not-chattable denial.
func isNotChattable(err error) bool {
	return errors.Is(err, errAgentNotChattable)
}

// agentChattable reports whether a user may open a direct chat session with an
// agent at the given org_level. Only executives (L1 COO, L2 C-officers) are
// chattable. L3 workers receive delegated work from their manager and are
// observed through their read-only monitor view, never chatted directly.
func agentChattable(orgLevel string) bool {
	switch strings.ToLower(strings.TrimSpace(orgLevel)) {
	case "l1", "l2":
		return true
	default:
		return false
	}
}

// agentChatAllowed returns nil if a user may open a direct chat session with
// the agent, or errAgentNotChattable if the agent is an L3 worker. Looks up
// org_level from the DB. Mirrors channelAllowedForAgent.
func (gw *Gateway) agentChatAllowed(ctx context.Context, agentID string) error {
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
	if !agentChattable(level) {
		return errAgentNotChattable
	}
	return nil
}
