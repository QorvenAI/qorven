// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package rooms holds the coordination-hub behaviors layered on the /rooms
// surface: resolving @mentions to agents (by org role or key) and capping
// per-room agent activity so a room can't loop or overspend.
package rooms

import (
	"strings"

	"github.com/qorvenai/qorven/internal/agent"
)

// ResolveMention maps a mention name (the text after '@', without the '@') to an
// agent. It prefers an org-role match (so "@COO"/"@cto" reach the officer holding
// that role), then falls back to an agent-key match (existing behavior). Returns
// nil when nothing matches. Pure: operates over the supplied agent slice.
func ResolveMention(name string, agents []*agent.Agent) *agent.Agent {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return nil
	}
	// 1. org_role exact match wins.
	for _, ag := range agents {
		if ag.OrgRole != "" && strings.ToLower(ag.OrgRole) == want {
			return ag
		}
	}
	// 2. agent_key fallback (case-insensitive).
	for _, ag := range agents {
		if strings.EqualFold(ag.AgentKey, name) {
			return ag
		}
	}
	return nil
}
