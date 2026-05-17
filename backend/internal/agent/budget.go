// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// BudgetEnforcer tracks spending per agent and enforces limits.
// Three scopes: company, project, agent — each with monthly + lifetime windows.
type BudgetEnforcer struct {
	policies map[string]*BudgetPolicy // key: scope:id
	spending map[string]*SpendRecord  // key: scope:id
	mu       sync.Mutex
}

// BudgetPolicy defines a spending limit.
type BudgetPolicy struct {
	Scope       string // "company", "project", "agent"
	ScopeID     string
	MonthlyLimit int64 // cents (0 = unlimited)
	LifetimeLimit int64 // cents (0 = unlimited)
	WarnPercent  int   // alert threshold (default 80)
	AutoPause    bool  // auto-pause on exceed
}

// SpendRecord tracks actual spending.
type SpendRecord struct {
	MonthlySpent  int64
	LifetimeSpent int64
	MonthStart    time.Time
	LastRecorded  time.Time
}

// BudgetStatus is the result of a budget check.
type BudgetStatus string

const (
	BudgetOK       BudgetStatus = "ok"
	BudgetWarning  BudgetStatus = "warning"
	BudgetExceeded BudgetStatus = "exceeded"
)

// BudgetCheckResult holds the outcome of a budget check.
type BudgetCheckResult struct {
	Status   BudgetStatus
	Message  string
	Scope    string
	ScopeID  string
	Spent    int64
	Limit    int64
}

// NewBudgetEnforcer creates an enforcer.
func NewBudgetEnforcer() *BudgetEnforcer {
	return &BudgetEnforcer{
		policies: make(map[string]*BudgetPolicy),
		spending: make(map[string]*SpendRecord),
	}
}

// SetPolicy sets a budget policy for a scope.
func (b *BudgetEnforcer) SetPolicy(policy BudgetPolicy) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := policy.Scope + ":" + policy.ScopeID
	if policy.WarnPercent <= 0 {
		policy.WarnPercent = 80
	}
	b.policies[key] = &policy
}

// RecordSpend records spending for a scope.
func (b *BudgetEnforcer) RecordSpend(scope, scopeID string, cents int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := scope + ":" + scopeID
	rec, ok := b.spending[key]
	if !ok {
		rec = &SpendRecord{MonthStart: monthStart(time.Now())}
		b.spending[key] = rec
	}

	// Reset monthly if new month
	now := time.Now()
	if now.Month() != rec.MonthStart.Month() || now.Year() != rec.MonthStart.Year() {
		rec.MonthlySpent = 0
		rec.MonthStart = monthStart(now)
	}

	rec.MonthlySpent += cents
	rec.LifetimeSpent += cents
	rec.LastRecorded = now
}

// Check evaluates budget status for a scope. Returns the worst status found.
func (b *BudgetEnforcer) Check(scope, scopeID string) BudgetCheckResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := scope + ":" + scopeID
	policy, ok := b.policies[key]
	if !ok {
		return BudgetCheckResult{Status: BudgetOK}
	}

	rec := b.spending[key]
	if rec == nil {
		return BudgetCheckResult{Status: BudgetOK}
	}

	// Reset monthly if new month
	now := time.Now()
	if now.Month() != rec.MonthStart.Month() || now.Year() != rec.MonthStart.Year() {
		rec.MonthlySpent = 0
		rec.MonthStart = monthStart(now)
	}

	// Check monthly limit
	if policy.MonthlyLimit > 0 {
		if rec.MonthlySpent >= policy.MonthlyLimit {
			return BudgetCheckResult{
				Status:  BudgetExceeded,
				Message: fmt.Sprintf("%s %s monthly budget exceeded: $%.2f / $%.2f", scope, scopeID, float64(rec.MonthlySpent)/100, float64(policy.MonthlyLimit)/100),
				Scope:   scope, ScopeID: scopeID,
				Spent: rec.MonthlySpent, Limit: policy.MonthlyLimit,
			}
		}
		warnThreshold := policy.MonthlyLimit * int64(policy.WarnPercent) / 100
		if rec.MonthlySpent >= warnThreshold {
			return BudgetCheckResult{
				Status:  BudgetWarning,
				Message: fmt.Sprintf("%s %s approaching monthly limit: $%.2f / $%.2f", scope, scopeID, float64(rec.MonthlySpent)/100, float64(policy.MonthlyLimit)/100),
				Scope:   scope, ScopeID: scopeID,
				Spent: rec.MonthlySpent, Limit: policy.MonthlyLimit,
			}
		}
	}

	// Check lifetime limit
	if policy.LifetimeLimit > 0 && rec.LifetimeSpent >= policy.LifetimeLimit {
		return BudgetCheckResult{
			Status:  BudgetExceeded,
			Message: fmt.Sprintf("%s %s lifetime budget exceeded: $%.2f / $%.2f", scope, scopeID, float64(rec.LifetimeSpent)/100, float64(policy.LifetimeLimit)/100),
			Scope:   scope, ScopeID: scopeID,
			Spent: rec.LifetimeSpent, Limit: policy.LifetimeLimit,
		}
	}

	return BudgetCheckResult{Status: BudgetOK}
}

// CheckBeforeRun checks all applicable scopes before an agent run.
// Returns the worst status found across company, project, and agent scopes.
func (b *BudgetEnforcer) CheckBeforeRun(tenantID, agentID string) BudgetCheckResult {
	// Check agent budget
	if result := b.Check("agent", agentID); result.Status == BudgetExceeded {
		slog.Warn("budget.exceeded", "scope", "agent", "agent", agentID, "msg", result.Message)
		return result
	}
	// Check company budget
	if result := b.Check("company", tenantID); result.Status == BudgetExceeded {
		slog.Warn("budget.exceeded", "scope", "company", "tenant", tenantID, "msg", result.Message)
		return result
	}
	// Return worst warning
	if result := b.Check("agent", agentID); result.Status == BudgetWarning {
		return result
	}
	if result := b.Check("company", tenantID); result.Status == BudgetWarning {
		return result
	}
	return BudgetCheckResult{Status: BudgetOK}
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}
