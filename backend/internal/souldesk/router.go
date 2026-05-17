// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package souldesk

import (
	"context"
	"regexp"
	"strings"

	"github.com/qorvenai/qorven/internal/agent"
)

// MentionRouter parses @mentions from user messages and routes to the right Soul.
// If @soul_key is found, routes to that Soul. Otherwise routes to Prime Soul.
type MentionRouter struct {
	agentStore *agent.Store
	tenantID   string
}

func NewMentionRouter(agentStore *agent.Store, tenantID string) *MentionRouter {
	return &MentionRouter{agentStore: agentStore, tenantID: tenantID}
}

var mentionRe = regexp.MustCompile(`@([a-zA-Z0-9_.-]+)`)

// Route determines which Soul should handle a message.
// Returns the agent ID to route to, and the cleaned message (mention removed).
func (r *MentionRouter) Route(ctx context.Context, message string, defaultAgentID string) (agentID string, cleanedMessage string) {
	matches := mentionRe.FindStringSubmatch(message)
	if len(matches) < 2 {
		return defaultAgentID, message
	}

	soulKey := matches[1]

	// Find the mentioned Soul
	agents, err := r.agentStore.List(ctx, r.tenantID)
	if err != nil {
		return defaultAgentID, message
	}

	for _, a := range agents {
		if strings.EqualFold(a.AgentKey, soulKey) {
			// Found — route to this Soul, remove the @mention from message
			cleaned := strings.Replace(message, "@"+soulKey, "", 1)
			cleaned = strings.TrimSpace(cleaned)
			if cleaned == "" {
				cleaned = "What can you help with?"
			}
			return a.ID, cleaned
		}
	}

	// Soul not found — route to default
	return defaultAgentID, message
}
