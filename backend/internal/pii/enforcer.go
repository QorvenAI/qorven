// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package pii

import (
	"context"
	"fmt"
	"strings"
)

// Enforcer applies PII policy at all enforcement points in the system.
// It sits between content producers (channels, agents, tools) and
// storage (memory, KG, sessions).
type Enforcer struct {
	vault  *Vault
	policy Policy
	config Config
}

// NewEnforcer creates an enforcer with the given policy and optional vault.
func NewEnforcer(vault *Vault, policy Policy) *Enforcer {
	return &Enforcer{
		vault:  vault,
		policy: policy,
		config: Config{Kinds: policy.Kinds},
	}
}

// RedactForStorage scans content for PII, optionally vaults the originals,
// and returns the redacted content. Used on memory writes, KG entries,
// and bus messages headed to shared scopes.
func (e *Enforcer) RedactForStorage(ctx context.Context, agentID, content string) (string, error) {
	if !e.policy.Enabled || content == "" {
		return content, nil
	}

	dets := Scan(content, e.config)
	if len(dets) == 0 {
		return content, nil
	}

	if e.policy.VaultEnabled && e.vault != nil {
		return e.redactWithVault(ctx, agentID, content, dets)
	}

	return Redact(content, e.config), nil
}

// redactWithVault stores each PII detection in the vault and replaces
// with a vault-backed token placeholder.
func (e *Enforcer) redactWithVault(ctx context.Context, agentID, content string, dets []Detection) (string, error) {
	var sb strings.Builder
	sb.Grow(len(content))
	pos := 0

	for _, det := range dets {
		if det.Start < pos {
			continue
		}
		sb.WriteString(content[pos:det.Start])

		token, err := e.vault.Store(ctx, agentID, det.Kind, det.Value)
		if err != nil {
			sb.WriteString(fmt.Sprintf("{{PII:%s}}", det.Kind.String()))
		} else {
			sb.WriteString(fmt.Sprintf("{{PII:%s:vault:%s}}", det.Kind.String(), token))
		}
		pos = det.End
	}
	sb.WriteString(content[pos:])
	return sb.String(), nil
}

// HasPII returns true if the content contains detectable PII.
func (e *Enforcer) HasPII(content string) bool {
	if !e.policy.Enabled {
		return false
	}
	dets := Scan(content, e.config)
	return len(dets) > 0
}

// IsScopeExempt checks if this scope allows PII to remain unredacted.
func (e *Enforcer) IsScopeExempt(scope string) bool {
	return e.policy.IsScopeExempt(scope)
}

// IsAgentExempt checks if this agent bypasses PII redaction for reading.
func (e *Enforcer) IsAgentExempt(agentID string) bool {
	return e.policy.IsAgentExempt(agentID)
}
