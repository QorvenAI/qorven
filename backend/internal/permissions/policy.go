// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package permissions

import (
	"slices"
	"strings"
	"sync"
)

// GatewayRole represents a user's permission level for gateway access.
type GatewayRole string

const (
	RoleOwner    GatewayRole = "owner"    // Tenant management + full access
	RoleAdmin    GatewayRole = "admin"    // Full access to all methods
	RoleOperator GatewayRole = "operator" // Read + write access
	RoleViewer   GatewayRole = "viewer"   // Read-only access
)

// Scope represents a specific permission scope for API keys.
type Scope string

const (
	ScopeAdmin     Scope = "operator.admin"
	ScopeRead      Scope = "operator.read"
	ScopeWrite     Scope = "operator.write"
	ScopeApprovals Scope = "operator.approvals"
	ScopePairing   Scope = "operator.pairing"
)

var AllScopes = map[Scope]bool{
	ScopeAdmin: true, ScopeRead: true, ScopeWrite: true,
	ScopeApprovals: true, ScopePairing: true,
}

func ValidScope(s string) bool { return AllScopes[Scope(s)] }

// PolicyEngine evaluates user permissions for gateway method access.
type PolicyEngine struct {
	ownerIDs map[string]bool
	mu       sync.RWMutex
}

func NewPolicyEngine(ownerIDs []string) *PolicyEngine {
	owners := make(map[string]bool, len(ownerIDs))
	for _, id := range ownerIDs {
		owners[id] = true
	}
	return &PolicyEngine{ownerIDs: owners}
}

func (pe *PolicyEngine) IsOwner(senderID string) bool {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.ownerIDs[senderID]
}

func (pe *PolicyEngine) CanAccess(role GatewayRole, method string) bool {
	return gatewayRoleLevel(role) >= gatewayRoleLevel(MethodRole(method))
}

func (pe *PolicyEngine) CanAccessWithScopes(scopes []Scope, method string) bool {
	required := MethodScopes(method)
	if len(required) == 0 {
		return true
	}
	scopeSet := make(map[Scope]bool, len(scopes))
	for _, s := range scopes {
		scopeSet[s] = true
	}
	for _, r := range required {
		if scopeSet[r] {
			return true
		}
	}
	return false
}

func RoleFromScopes(scopes []Scope) GatewayRole {
	if slices.Contains(scopes, ScopeAdmin) {
		return RoleAdmin
	}
	if slices.Contains(scopes, ScopeWrite) || slices.Contains(scopes, ScopeApprovals) || slices.Contains(scopes, ScopePairing) {
		return RoleOperator
	}
	return RoleViewer
}

func MethodRole(method string) GatewayRole {
	if isAdminMethod(method) {
		return RoleAdmin
	}
	if isWriteMethod(method) {
		return RoleOperator
	}
	return RoleViewer
}

func MethodScopes(method string) []Scope {
	if isAdminMethod(method) {
		return []Scope{ScopeAdmin}
	}
	if strings.HasPrefix(method, "approvals.") {
		return []Scope{ScopeApprovals, ScopeAdmin}
	}
	if strings.HasPrefix(method, "pairing.") {
		return []Scope{ScopePairing, ScopeAdmin}
	}
	if isWriteMethod(method) {
		return []Scope{ScopeWrite, ScopeAdmin}
	}
	return []Scope{ScopeRead, ScopeWrite, ScopeAdmin}
}

var adminMethods = map[string]bool{
	"config.apply": true, "config.patch": true,
	"agents.create": true, "agents.update": true, "agents.delete": true,
	"channels.toggle": true, "pairing.approve": true, "pairing.revoke": true,
	"teams.list": true, "teams.create": true, "teams.delete": true,
	"api_keys.list": true, "api_keys.create": true, "api_keys.revoke": true,
}

func isAdminMethod(method string) bool { return adminMethods[method] }

func isWriteMethod(method string) bool {
	writePrefixes := []string{
		"chat.send", "chat.abort", "sessions.delete", "sessions.reset",
		"cron.create", "cron.update", "cron.delete", "cron.toggle",
		"pairing.", "approvals.", "send",
	}
	for _, prefix := range writePrefixes {
		if strings.HasPrefix(method, prefix) {
			return true
		}
	}
	return false
}

func HasMinGatewayRole(role, required GatewayRole) bool {
	return gatewayRoleLevel(role) >= gatewayRoleLevel(required)
}

func gatewayRoleLevel(r GatewayRole) int {
	switch r {
	case RoleOwner:
		return 4
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}
