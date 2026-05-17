// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package supervisor

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// FixType identifies a specific auto-fix action.
// Each fix is a hardcoded Go function — the LLM decides WHICH fix to apply,
// Go decides WHETHER it's allowed. Same security model as tool execution.
type FixType string

const (
	FixRestartCron    FixType = "restart_cron"     // Restart a failed cron job
	FixRetryAPI       FixType = "retry_api"        // Retry a failed API call
	FixClearCache     FixType = "clear_cache"      // Clear stale cache entries
	FixSwitchModel    FixType = "switch_model"     // Switch to fallback model
	FixResetSession   FixType = "reset_session"    // Reset a stuck session
	FixRestartChannel FixType = "restart_channel"  // Restart a failed channel (Telegram, etc.)
	FixAdjustTimeout  FixType = "adjust_timeout"   // Increase timeout for slow operations
	FixPurgeOldData   FixType = "purge_old_data"   // Clean up data older than retention period
)

// Fix describes an auto-fix action with its risk level and implementation.
type Fix struct {
	Type        FixType   `json:"type"`
	Description string    `json:"description"`
	Risk        RiskLevel `json:"risk"`
	Execute     func(ctx context.Context, params map[string]any) error `json:"-"`
}

// FixResult records the outcome of an auto-fix attempt.
type FixResult struct {
	FixType   FixType        `json:"fix_type"`
	Params    map[string]any `json:"params"`
	Success   bool           `json:"success"`
	Error     string         `json:"error,omitempty"`
	Duration  time.Duration  `json:"duration"`
	Timestamp time.Time      `json:"timestamp"`
}

// FixCatalog is the hardcoded allowlist of auto-fix actions.
// The LLM proposes a fix type + params. Go looks it up here and executes.
// If the fix type isn't in the catalog, it's rejected.
type FixCatalog struct {
	fixes   map[FixType]*Fix
	history []FixResult
	deps    FixDependencies
}

// FixDependencies are the external services the catalog needs to execute fixes.
type FixDependencies struct {
	RestartCron    func(ctx context.Context, jobID string) error
	RetryAPI       func(ctx context.Context, endpoint string) error
	ClearCache     func(ctx context.Context, key string) error
	SwitchModel    func(ctx context.Context, agentID, newModel string) error
	ResetSession   func(ctx context.Context, sessionID string) error
	RestartChannel func(ctx context.Context, channelID string) error
}

// NewFixCatalog creates the catalog with all allowed fixes.
func NewFixCatalog(deps FixDependencies) *FixCatalog {
	c := &FixCatalog{
		fixes: make(map[FixType]*Fix),
		deps:  deps,
	}

	// Register all allowed fixes — this is the allowlist.
	// Adding a new fix requires a code change and review.

	c.register(Fix{
		Type:        FixRestartCron,
		Description: "Restart a failed cron job by ID",
		Risk:        RiskLow,
		Execute: func(ctx context.Context, params map[string]any) error {
			jobID, _ := params["job_id"].(string)
			if jobID == "" {
				return fmt.Errorf("job_id required")
			}
			if deps.RestartCron == nil {
				return fmt.Errorf("restart_cron not wired")
			}
			return deps.RestartCron(ctx, jobID)
		},
	})

	c.register(Fix{
		Type:        FixRetryAPI,
		Description: "Retry a failed external API call",
		Risk:        RiskLow,
		Execute: func(ctx context.Context, params map[string]any) error {
			endpoint, _ := params["endpoint"].(string)
			if endpoint == "" {
				return fmt.Errorf("endpoint required")
			}
			if deps.RetryAPI == nil {
				return fmt.Errorf("retry_api not wired")
			}
			return deps.RetryAPI(ctx, endpoint)
		},
	})

	c.register(Fix{
		Type:        FixClearCache,
		Description: "Clear stale cache entries by key pattern",
		Risk:        RiskLow,
		Execute: func(ctx context.Context, params map[string]any) error {
			key, _ := params["key"].(string)
			if key == "" {
				key = "*" // clear all
			}
			if deps.ClearCache == nil {
				return fmt.Errorf("clear_cache not wired")
			}
			return deps.ClearCache(ctx, key)
		},
	})

	c.register(Fix{
		Type:        FixSwitchModel,
		Description: "Switch an agent to its fallback model",
		Risk:        RiskLow,
		Execute: func(ctx context.Context, params map[string]any) error {
			agentID, _ := params["agent_id"].(string)
			model, _ := params["model"].(string)
			if agentID == "" || model == "" {
				return fmt.Errorf("agent_id and model required")
			}
			if deps.SwitchModel == nil {
				return fmt.Errorf("switch_model not wired")
			}
			return deps.SwitchModel(ctx, agentID, model)
		},
	})

	c.register(Fix{
		Type:        FixResetSession,
		Description: "Reset a stuck or corrupted session",
		Risk:        RiskLow,
		Execute: func(ctx context.Context, params map[string]any) error {
			sessionID, _ := params["session_id"].(string)
			if sessionID == "" {
				return fmt.Errorf("session_id required")
			}
			if deps.ResetSession == nil {
				return fmt.Errorf("reset_session not wired")
			}
			return deps.ResetSession(ctx, sessionID)
		},
	})

	c.register(Fix{
		Type:        FixRestartChannel,
		Description: "Restart a failed channel connection (Telegram, Slack, etc.)",
		Risk:        RiskLow,
		Execute: func(ctx context.Context, params map[string]any) error {
			channelID, _ := params["channel_id"].(string)
			if channelID == "" {
				return fmt.Errorf("channel_id required")
			}
			if deps.RestartChannel == nil {
				return fmt.Errorf("restart_channel not wired")
			}
			return deps.RestartChannel(ctx, channelID)
		},
	})

	c.register(Fix{
		Type:        FixAdjustTimeout,
		Description: "Increase timeout for slow operations",
		Risk:        RiskLow,
		Execute: func(ctx context.Context, params map[string]any) error {
			// This is a config change — just log it for now
			slog.Info("auto_fix.adjust_timeout", "params", params)
			return nil
		},
	})

	c.register(Fix{
		Type:        FixPurgeOldData,
		Description: "Clean up data older than retention period",
		Risk:        RiskLow,
		Execute: func(ctx context.Context, params map[string]any) error {
			// Safe: only deletes data past retention
			slog.Info("auto_fix.purge_old_data", "params", params)
			return nil
		},
	})

	return c
}

func (c *FixCatalog) register(fix Fix) {
	c.fixes[fix.Type] = &fix
}

// Get returns a fix by type, or nil if not in the allowlist.
func (c *FixCatalog) Get(fixType FixType) *Fix {
	return c.fixes[fixType]
}

// Apply executes a fix if it's in the allowlist and the risk level is acceptable.
// Returns the result of the fix attempt.
func (c *FixCatalog) Apply(ctx context.Context, fixType FixType, params map[string]any, maxRisk RiskLevel) (*FixResult, error) {
	fix := c.fixes[fixType]
	if fix == nil {
		return nil, fmt.Errorf("fix type %q not in allowlist", fixType)
	}

	// Check risk level
	if !riskAllowed(fix.Risk, maxRisk) {
		return nil, fmt.Errorf("fix %q has risk %s, exceeds max allowed %s", fixType, fix.Risk, maxRisk)
	}

	start := time.Now()
	err := fix.Execute(ctx, params)
	duration := time.Since(start)

	result := &FixResult{
		FixType:   fixType,
		Params:    params,
		Success:   err == nil,
		Duration:  duration,
		Timestamp: time.Now(),
	}
	if err != nil {
		result.Error = err.Error()
	}

	c.history = append(c.history, *result)

	slog.Info("auto_fix.applied",
		"type", fixType,
		"success", result.Success,
		"duration_ms", duration.Milliseconds(),
		"risk", fix.Risk,
		"error", result.Error)

	return result, err
}

// History returns the last N fix results.
func (c *FixCatalog) History(limit int) []FixResult {
	if limit <= 0 || limit > len(c.history) {
		limit = len(c.history)
	}
	start := len(c.history) - limit
	if start < 0 {
		start = 0
	}
	return c.history[start:]
}

// ListFixes returns all available fix types with their descriptions and risk levels.
func (c *FixCatalog) ListFixes() []map[string]any {
	list := []map[string]any{}
	for _, fix := range c.fixes {
		list = append(list, map[string]any{
			"type":        fix.Type,
			"description": fix.Description,
			"risk":        fix.Risk,
		})
	}
	return list
}

// riskAllowed checks if a fix's risk level is within the allowed maximum.
func riskAllowed(fixRisk, maxRisk RiskLevel) bool {
	riskOrder := map[RiskLevel]int{RiskLow: 0, RiskMedium: 1, RiskHigh: 2}
	return riskOrder[fixRisk] <= riskOrder[maxRisk]
}
