// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
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
// It is a thin wrapper around RouteAndResolve that discards the resolved targets
// for callers that only need storage (back-compat).
func (r *Router) Route(ctx context.Context, tenantID string, from, fromName, subject, bodyText, bodyHTML, messageID, inReplyTo string, to []string) error {
	_, err := r.RouteAndResolve(ctx, tenantID, from, fromName, subject, bodyText, bodyHTML, messageID, inReplyTo, to)
	return err
}

// RouteAndResolve stores the inbound email exactly like Route and additionally
// returns the deduplicated set of resolved targets where ShouldTriggerAgent is
// true, so callers can fire the agent brain for each one. Targets are deduped
// by AgentID — if the same agent appears via multiple recipient addresses only
// one RouteTarget is returned.
func (r *Router) RouteAndResolve(ctx context.Context, tenantID string, from, fromName, subject, bodyText, bodyHTML, messageID, inReplyTo string, to []string) ([]RouteTarget, error) {
	threadID := inReplyTo
	if threadID == "" {
		threadID = messageID
	}

	seen := map[string]struct{}{}
	var triggerTargets []RouteTarget

	for _, addr := range to {
		targets := r.resolveTargets(ctx, strings.ToLower(strings.TrimSpace(addr)), tenantID)
		for _, t := range targets {
			_, err := r.store.StoreInbound(ctx, tenantID, t.AgentID, t.IdentityID, messageID, threadID, from, fromName, subject, bodyText, bodyHTML, to)
			if err != nil {
				slog.Warn("mail.router.store_failed", "to", addr, "error", err)
				continue
			}
			slog.Info("mail.routed", "to", addr, "agent", t.AgentID, "shared", t.IsSharedInbox)

			if t.ShouldTriggerAgent && t.AgentID != "" {
				if _, dup := seen[t.AgentID]; !dup {
					seen[t.AgentID] = struct{}{}
					triggerTargets = append(triggerTargets, t)
				}
			}
		}
	}
	return triggerTargets, nil
}

func (r *Router) resolveTargets(ctx context.Context, address, tenantID string) []RouteTarget {
	// 1. Exact match — dedicated Soul mailbox
	if identity, err := r.store.FindIdentityByAddress(ctx, address, tenantID); err == nil && identity.AgentID != nil {
		return []RouteTarget{{AgentID: *identity.AgentID, IdentityID: identity.ID, ShouldTriggerAgent: true}}
	}

	// 2. Alias match — shared inbox: route to each mapped agent and mark for brain trigger.
	if aliases, err := r.store.FindAliasesByAddress(ctx, address, tenantID); err == nil && len(aliases) > 0 {
		targets := make([]RouteTarget, len(aliases))
		for i, a := range aliases {
			targets[i] = RouteTarget{AgentID: a.TargetAgentID, IsSharedInbox: true, ShouldTriggerAgent: true}
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
