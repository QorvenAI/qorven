// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package email

import (
	"fmt"
	"strings"
)

// AliasRouter routes incoming emails to agents based on recipient aliases.
// Supports: individual mailbox (agent@domain) and shared mailbox (team@domain with aliases).
type AliasRouter struct {
	// SharedMailbox is the team email (e.g. team@qorven.ai)
	SharedMailbox string
	// Aliases maps alias names to agent IDs (e.g. "sara" → "agent-uuid-1")
	Aliases map[string]string
	// DefaultAgent receives emails that don't match any alias
	DefaultAgent string
}

// Route determines which agent should handle an incoming email.
// Checks: To address, CC, and alias patterns.
func (r *AliasRouter) Route(to, cc, subject string) (agentID string, matched bool) {
	// Check direct To address
	if id := r.matchAddress(to); id != "" {
		return id, true
	}
	// Check CC
	if id := r.matchAddress(cc); id != "" {
		return id, true
	}
	// Check subject line for @mentions (e.g. "@sara please review")
	if id := r.matchSubjectMention(subject); id != "" {
		return id, true
	}
	// Default agent
	if r.DefaultAgent != "" {
		return r.DefaultAgent, true
	}
	return "", false
}

func (r *AliasRouter) matchAddress(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	// Check each alias against the address
	for alias, agentID := range r.Aliases {
		// Match: alias@domain or alias+tag@domain
		if strings.HasPrefix(addr, strings.ToLower(alias)+"@") ||
			strings.Contains(addr, "+"+strings.ToLower(alias)+"@") {
			return agentID
		}
	}
	return ""
}

func (r *AliasRouter) matchSubjectMention(subject string) string {
	lower := strings.ToLower(subject)
	for alias, agentID := range r.Aliases {
		if strings.Contains(lower, "@"+strings.ToLower(alias)) {
			return agentID
		}
	}
	return ""
}

// FormatReplyFrom returns the From address for an agent's reply.
// Uses alias+agentname@domain format for shared mailbox.
func (r *AliasRouter) FormatReplyFrom(agentAlias string) string {
	if r.SharedMailbox == "" {
		return ""
	}
	parts := strings.SplitN(r.SharedMailbox, "@", 2)
	if len(parts) != 2 {
		return r.SharedMailbox
	}
	return fmt.Sprintf("%s+%s@%s", parts[0], agentAlias, parts[1])
}
