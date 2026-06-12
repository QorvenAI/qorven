// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package drive

import "context"

// Scope values for drive_files.scope.
const (
	ScopePrivate    = "private"    // owner agent only (+ admin users)
	ScopeCompany    = "company"    // every agent and user in the tenant
	ScopeDepartment = "department" // agents in scope_id's department (or a descendant)
	ScopeCustom     = "custom"     // owner + explicit drive_permissions grants
)

// decideAccess is the pure access decision given already-resolved facts.
// callerAgent is "" when the caller is a human user (not an agent).
// callerDepts is the set of department ids the calling agent belongs to
// (including ancestor departments). hasGrant is whether a matching
// drive_permissions row exists for a custom-scoped file. isAdminUser grants
// human admins full read.
func decideAccess(scope, scopeID, ownerAgent, callerAgent string, callerDepts map[string]bool, hasGrant, isAdminUser bool) bool {
	if isAdminUser {
		return true
	}
	if callerAgent != "" && callerAgent == ownerAgent {
		return true
	}
	switch scope {
	case ScopeCompany:
		return true
	case ScopePrivate:
		return false
	case ScopeDepartment:
		return callerDepts[scopeID]
	case ScopeCustom:
		return hasGrant
	}
	return false
}

// CanAccess resolves the DB-backed facts (caller department set, custom grant)
// and returns whether the caller may access the file. callerAgent is "" for a
// human user; isAdminUser is true for an admin human. Called by every Drive
// handler before reading/serving/deleting a file.
func (s *Store) CanAccess(ctx context.Context, f *File, callerAgent string, isAdminUser bool) (bool, error) {
	owner := ""
	if f.AgentID != nil {
		owner = *f.AgentID
	}
	if isAdminUser || (callerAgent != "" && callerAgent == owner) || f.Scope == ScopeCompany {
		return true, nil
	}
	var depts map[string]bool
	if f.Scope == ScopeDepartment && callerAgent != "" {
		depts = map[string]bool{}
		for _, d := range s.agentDepartments(ctx, callerAgent) {
			depts[d] = true
		}
	}
	hasGrant := false
	if f.Scope == ScopeCustom && callerAgent != "" {
		hasGrant = s.hasCustomGrant(ctx, f.ID, callerAgent)
	}
	scopeID := ""
	if f.ScopeID != nil {
		scopeID = *f.ScopeID
	}
	return decideAccess(f.Scope, scopeID, owner, callerAgent, depts, hasGrant, isAdminUser), nil
}

// agentDepartments returns the agent's department id plus all ancestor
// department ids (walking parent_department_id), so a department-scoped file
// is visible to sub-department agents too.
func (s *Store) agentDepartments(ctx context.Context, agentID string) []string {
	var dept *string
	if err := s.pool.QueryRow(ctx, `SELECT department_id FROM agents WHERE id = $1`, agentID).Scan(&dept); err != nil || dept == nil {
		return nil
	}
	out := []string{}
	cur := dept
	seen := map[string]bool{}
	for cur != nil && !seen[*cur] {
		out = append(out, *cur)
		seen[*cur] = true
		var parent *string
		if err := s.pool.QueryRow(ctx, `SELECT parent_department_id FROM departments WHERE id = $1`, *cur).Scan(&parent); err != nil {
			break
		}
		cur = parent
	}
	return out
}

// hasCustomGrant reports whether a custom-scoped file has a drive_permissions
// row granting this agent (directly, or via a department it belongs to).
func (s *Store) hasCustomGrant(ctx context.Context, fileID, agentID string) bool {
	var n int
	_ = s.pool.QueryRow(ctx,
		`SELECT COUNT(1) FROM drive_permissions WHERE file_id = $1 AND grantee_type = 'agent' AND grantee_id = $2`,
		fileID, agentID).Scan(&n)
	if n > 0 {
		return true
	}
	for _, d := range s.agentDepartments(ctx, agentID) {
		_ = s.pool.QueryRow(ctx,
			`SELECT COUNT(1) FROM drive_permissions WHERE file_id = $1 AND grantee_type = 'department' AND grantee_id = $2`,
			fileID, d).Scan(&n)
		if n > 0 {
			return true
		}
	}
	return false
}
