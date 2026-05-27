// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

// DefaultClearance maps agent org_role to its maximum accessible classification level.
var DefaultClearance = map[string]Classification{
	"chief":   ClassConfidential,
	"cko":     ClassRestricted,
	"coo":     ClassConfidential,
	"cfo":     ClassConfidential,
	"chro":    ClassConfidential,
	"caio":    ClassConfidential,
	"cto":     ClassInternal,
	"cmo":     ClassInternal,
	"cso":     ClassConfidential,
	"cco":     ClassConfidential,
	"ciso":    ClassConfidential,
	"code":    ClassInternal,
	"architect": ClassInternal,
	"reviewer":  ClassInternal,
	"devops":    ClassInternal,
	"researcher": ClassInternal,
	"analyst":   ClassConfidential,
	"designer":  ClassInternal,
	"product":   ClassConfidential,
	"qa":        ClassInternal,
	"writer":    ClassInternal,
	"marketer":  ClassInternal,
	"sales":     ClassConfidential,
	"support":   ClassConfidential,
	"legal":     ClassConfidential,
	"social":    ClassInternal,
	"general":   ClassInternal,
	"worker":    ClassInternal,
}

// ClearanceForRole returns the default classification clearance for an org role.
func ClearanceForRole(role string) Classification {
	if c, ok := DefaultClearance[role]; ok {
		return c
	}
	return ClassInternal
}

// ScopeWritePermissions defines which roles can write to each scope.
var ScopeWritePermissions = map[Scope][]string{
	ScopeCompany: {"chief", "cko", "coo"},
	ScopeTeam:    {"chief", "cko", "coo", "cto", "cmo", "cso", "cco", "cfo", "chro", "caio", "ciso"},
	ScopePrime:   {"chief"},
}

// CanWriteScope checks if a given role can write to the specified scope.
// Agent/session/task/discussion scopes are always writable by the owning agent.
func CanWriteScope(role string, scope Scope) bool {
	switch scope {
	case ScopeAgent, ScopeSession, ScopeTask, ScopeDiscussion:
		return true
	}
	allowed, ok := ScopeWritePermissions[scope]
	if !ok {
		return false
	}
	for _, r := range allowed {
		if r == role {
			return true
		}
	}
	return false
}
