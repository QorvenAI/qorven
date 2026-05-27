// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/pii"
)

// AccessController enforces knowledge access policies on all reads and writes.
// Every memory/knowledge operation in the system MUST go through this.
type AccessController struct {
	store      *Store
	hierarchy  *HierarchyStore
	classifier *Classifier
	enforcer   *pii.Enforcer
	grants     *GrantStore
	pool       *pgxpool.Pool
	tenantID   string
}

// NewAccessController creates a fully-wired access controller.
func NewAccessController(store *Store, hierarchy *HierarchyStore, grants *GrantStore, enforcer *pii.Enforcer, pool *pgxpool.Pool, tenantID string) *AccessController {
	return &AccessController{
		store:      store,
		hierarchy:  hierarchy,
		classifier: NewClassifier(),
		enforcer:   enforcer,
		grants:     grants,
		pool:       pool,
		tenantID:   tenantID,
	}
}

// ReadRequest describes what an agent is trying to read.
type ReadRequest struct {
	AgentID    string
	AgentRole  string
	Query      string
	Scopes     []Scope
	TeamID     string
	MaxResults int
}

// WriteRequest describes what an agent is trying to write.
type WriteRequest struct {
	AgentID   string
	AgentRole string
	Scope     Scope
	Content   string
	Source    string
	TeamID    string
	TaskID    string
}

// Read executes a knowledge query with full access control enforcement.
func (ac *AccessController) Read(ctx context.Context, req ReadRequest) ([]SearchResult, error) {
	if err := ValidateAccess(ac.tenantID, req.AgentID); err != nil {
		ac.logDenied(ctx, req.AgentID, "read", "", err.Error())
		return nil, err
	}

	clearance := ClearanceForRole(req.AgentRole)
	// Check for override from DB
	if override, err := ac.getOverrideClearance(ctx, req.AgentID); err == nil {
		clearance = override
	}

	var allResults []SearchResult
	maxPer := req.MaxResults
	if maxPer <= 0 {
		maxPer = 8
	}

	for _, scope := range req.Scopes {
		if !ac.canReadScope(req.AgentID, req.AgentRole, scope) {
			continue
		}

		results, err := ac.searchScope(ctx, req.AgentID, scope, req.TeamID, req.Query, maxPer)
		if err != nil {
			slog.Debug("access_controller: scope search failed", "scope", scope, "err", err)
			continue
		}

		for _, r := range results {
			if Classification(r.Classification) <= clearance {
				allResults = append(allResults, r)
			}
		}
	}

	ac.logAccess(ctx, req.AgentID, "read", req.Scopes, req.Query, len(allResults), false)
	return allResults, nil
}

// Write stores knowledge with classification, PII enforcement, and scope permissions.
func (ac *AccessController) Write(ctx context.Context, req WriteRequest) (string, error) {
	if err := ValidateAccess(ac.tenantID, req.AgentID); err != nil {
		ac.logDenied(ctx, req.AgentID, "write", string(req.Scope), err.Error())
		return "", err
	}

	if !CanWriteScope(req.AgentRole, req.Scope) {
		ac.logDenied(ctx, req.AgentID, "write", string(req.Scope), "role not authorized for scope")
		return "", ErrWriteDenied
	}

	classification := ac.classifier.Classify(req.Content, req.Source, req.AgentRole)

	if !classification.CanWriteToScope(req.Scope) {
		ac.logDenied(ctx, req.AgentID, "write", string(req.Scope),
			fmt.Sprintf("%s content cannot be written to %s scope", classification, req.Scope))
		return "", fmt.Errorf("access: %s content cannot be written to %s scope", classification, req.Scope)
	}

	// PII redaction for non-exempt scopes
	if ac.enforcer != nil && !ac.enforcer.IsScopeExempt(string(req.Scope)) {
		redacted, err := ac.enforcer.RedactForStorage(ctx, req.AgentID, req.Content)
		if err != nil {
			slog.Warn("access_controller: PII redaction failed, blocking write", "err", err)
			return "", fmt.Errorf("access: PII redaction failed: %w", err)
		}
		req.Content = redacted
	}

	// Perform the write through hierarchy store
	var id string
	var err error
	switch req.Scope {
	case ScopeCompany:
		id, err = ac.hierarchy.SaveCompany(ctx, req.Content, req.Source)
	case ScopeTeam:
		id, err = ac.hierarchy.SaveTeamMemory(ctx, req.TeamID, req.Content, req.Source)
	case ScopePrime:
		id, err = ac.hierarchy.SavePrime(ctx, req.Content, req.Source)
	case ScopeTask:
		id, err = ac.hierarchy.SaveTask(ctx, req.TaskID, req.AgentID, req.Content, req.Source)
	default:
		m := Memory{AgentID: req.AgentID, Type: "agent", Content: req.Content, Source: req.Source, Importance: 0.7}
		id, err = ac.store.Save(ctx, ac.tenantID, m)
	}

	if err == nil {
		// Update classification on the stored memory
		if id != "" {
			ac.pool.Exec(ctx, `UPDATE memories SET classification = $1 WHERE id = $2`, int(classification), id)
		}
		ac.logAccess(ctx, req.AgentID, "write", []Scope{req.Scope}, "", 1, false)
	}

	return id, err
}

// canReadScope checks if agent can read from a given scope.
func (ac *AccessController) canReadScope(agentID, agentRole string, scope Scope) bool {
	switch scope {
	case ScopeAgent, ScopeSession, ScopeTask, ScopeDiscussion:
		return true
	case ScopeCompany, ScopeTeam:
		return ClearanceForRole(agentRole) >= ClassInternal
	case ScopePrime:
		return agentRole == "chief" || agentRole == "cko"
	default:
		return false
	}
}

// searchScope routes to the correct search function per scope.
func (ac *AccessController) searchScope(ctx context.Context, agentID string, scope Scope, teamID, query string, maxResults int) ([]SearchResult, error) {
	switch scope {
	case ScopeCompany:
		return ac.store.Search(ctx, ac.tenantID, SentinelCompany, query, maxResults)
	case ScopeTeam:
		return ac.store.Search(ctx, ac.tenantID, SentinelTeam, query, maxResults)
	case ScopePrime:
		return ac.store.Search(ctx, ac.tenantID, SentinelPrime, query, maxResults)
	case ScopeAgent:
		return ac.store.Search(ctx, ac.tenantID, agentID, query, maxResults)
	default:
		return ac.store.Search(ctx, ac.tenantID, agentID, query, maxResults)
	}
}

// getOverrideClearance checks agent_clearances table for a DB override.
func (ac *AccessController) getOverrideClearance(ctx context.Context, agentID string) (Classification, error) {
	var level int
	err := ac.pool.QueryRow(ctx,
		`SELECT max_classification FROM agent_clearances WHERE agent_id = $1`,
		agentID).Scan(&level)
	if err != nil {
		return 0, err
	}
	return Classification(level), nil
}

func (ac *AccessController) logAccess(ctx context.Context, agentID, operation string, scopes []Scope, query string, resultCount int, denied bool) {
	if ac.pool == nil {
		return
	}
	scopeStr := ""
	if len(scopes) > 0 {
		scopeStr = string(scopes[0])
	}
	queryHash := ""
	if query != "" {
		h := sha256.Sum256([]byte(query))
		queryHash = hex.EncodeToString(h[:8])
	}
	ac.pool.Exec(ctx, `
		INSERT INTO knowledge_access_log (tenant_id, agent_id, operation, scope, result_count, query_hash, denied)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		ac.tenantID, agentID, operation, scopeStr, resultCount, queryHash, denied)
}

func (ac *AccessController) logDenied(ctx context.Context, agentID, operation, scope, reason string) {
	if ac.pool == nil {
		return
	}
	ac.pool.Exec(ctx, `
		INSERT INTO knowledge_access_log (tenant_id, agent_id, operation, scope, denied, deny_reason)
		VALUES ($1, $2, $3, $4, true, $5)`,
		ac.tenantID, agentID, operation, scope, reason)
}

// Sentinel agent IDs for shared scopes.
const (
	SentinelCompany = "00000000-0000-0000-0000-000000000001"
	SentinelTeam    = "00000000-0000-0000-0000-000000000002"
	SentinelPrime   = "00000000-0000-0000-0000-000000000003"
)
