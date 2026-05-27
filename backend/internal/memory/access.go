// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import "errors"

var (
	ErrNoAgentID      = errors.New("access: agent_id is required for all knowledge queries")
	ErrNoTenantID     = errors.New("access: tenant_id is required")
	ErrScopeViolation = errors.New("access: agent not authorized for this scope")
	ErrClassViolation = errors.New("access: agent clearance insufficient for this classification")
	ErrPIIViolation   = errors.New("access: PII content blocked for this agent")
	ErrWriteDenied    = errors.New("access: agent not authorized to write to this scope")
	ErrQuotaExceeded  = errors.New("access: knowledge access quota exceeded")
)

// ValidateAccess is called at the top of every memory/knowledge function.
func ValidateAccess(tenantID, agentID string) error {
	if tenantID == "" {
		return ErrNoTenantID
	}
	if agentID == "" {
		return ErrNoAgentID
	}
	return nil
}
