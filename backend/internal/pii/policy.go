// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package pii

// Policy defines the tenant-wide PII handling rules.
// Applied automatically unless an agent has an explicit override.
type Policy struct {
	Enabled      bool     `json:"enabled"`
	Kinds        Kind     `json:"kinds"`
	ExemptAgents []string `json:"exempt_agents"`
	ExemptScopes []string `json:"exempt_scopes"`
	VaultEnabled bool     `json:"vault_enabled"`
}

// DefaultPolicy is the production default: everything on.
var DefaultPolicy = Policy{
	Enabled:      true,
	Kinds:        All,
	ExemptScopes: []string{"session"},
	VaultEnabled: true,
}

// IsAgentExempt checks if an agent bypasses PII redaction for reading.
func (p Policy) IsAgentExempt(agentID string) bool {
	for _, id := range p.ExemptAgents {
		if id == agentID {
			return true
		}
	}
	return false
}

// IsScopeExempt checks if a scope allows PII to remain unredacted.
func (p Policy) IsScopeExempt(scope string) bool {
	for _, s := range p.ExemptScopes {
		if s == scope {
			return true
		}
	}
	return false
}
