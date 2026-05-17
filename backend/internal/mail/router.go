// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package mail

import (
	"context"
	"log/slog"
	"strings"
)

// RouteTarget is a resolved destination for an inbound email.
type RouteTarget struct {
	AgentID            string
	IdentityID         string
	ShouldTriggerAgent bool
	IsSharedInbox      bool
}

// Router resolves inbound emails to the correct Soul(s).
type Router struct {
	store *Store
}

func NewRouter(store *Store) *Router { return &Router{store: store} }

// Route processes an inbound email and delivers to the right mailbox(es).
func (r *Router) Route(ctx context.Context, tenantID string, from, fromName, subject, bodyText, bodyHTML, messageID, inReplyTo string, to []string) error {
	threadID := inReplyTo
	if threadID == "" {
		threadID = messageID
	}

	for _, addr := range to {
		targets := r.resolveTargets(ctx, strings.ToLower(strings.TrimSpace(addr)), tenantID)
		for _, t := range targets {
			_, err := r.store.StoreInbound(ctx, tenantID, t.AgentID, t.IdentityID, messageID, threadID, from, fromName, subject, bodyText, bodyHTML, to)
			if err != nil {
				slog.Warn("mail.router.store_failed", "to", addr, "error", err)
				continue
			}
			slog.Info("mail.routed", "to", addr, "agent", t.AgentID, "shared", t.IsSharedInbox)
		}
	}
	return nil
}

func (r *Router) resolveTargets(ctx context.Context, address, tenantID string) []RouteTarget {
	// 1. Exact match — dedicated Soul mailbox
	if identity, err := r.store.FindIdentityByAddress(ctx, address, tenantID); err == nil && identity.AgentID != nil {
		return []RouteTarget{{AgentID: *identity.AgentID, IdentityID: identity.ID, ShouldTriggerAgent: true}}
	}

	// 2. Alias match — shared inbox
	if aliases, err := r.store.FindAliasesByAddress(ctx, address, tenantID); err == nil && len(aliases) > 0 {
		targets := make([]RouteTarget, len(aliases))
		for i, a := range aliases {
			targets[i] = RouteTarget{AgentID: a.TargetAgentID, IsSharedInbox: true}
		}
		return targets
	}

	// 3. Plus-addressing — support+sara@domain → find "sara"
	if _, suffix, ok := parsePlusAddress(address); ok {
		// Search for identity matching the suffix as agent_key
		rows, err := r.store.pool.Query(ctx,
			`SELECT smi.id, smi.agent_id FROM soul_mail_identities smi
			 JOIN agents a ON smi.agent_id = a.id
			 WHERE a.agent_key = $1 AND smi.tenant_id = $2 AND smi.is_active = true`, suffix, tenantID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var identityID string
				var agentID *string
				rows.Scan(&identityID, &agentID)
				if agentID != nil {
					return []RouteTarget{{AgentID: *agentID, IdentityID: identityID, ShouldTriggerAgent: true}}
				}
			}
		}
	}

	slog.Warn("mail.router.no_target", "address", address)
	return nil
}

// parsePlusAddress splits "local+suffix@domain" into ("local", "suffix", true).
func parsePlusAddress(addr string) (string, string, bool) {
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return "", "", false
	}
	local := addr[:at]
	plus := strings.Index(local, "+")
	if plus < 0 {
		return "", "", false
	}
	return local[:plus], local[plus+1:], true
}
