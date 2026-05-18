// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package channels

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qorvenai/qorven/internal/config"
)

// QuotaResult is returned by QuotaChecker.Check.
type QuotaResult struct {
	Allowed bool   `json:"allowed"`
	Window  string `json:"window,omitempty"`
	Used    int    `json:"used,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

// quotaCounts holds cached request counts for a user.
type quotaCounts struct {
	hour, day, week int
	fetchedAt       time.Time
}

// QuotaChecker enforces per-user/group request quotas by counting top-level
// traces in the database. Results are cached in-memory for 60 seconds.
type QuotaChecker struct {
	pool     *pgxpool.Pool
	config   config.QuotaConfig
	cache    map[string]*quotaCounts
	cacheTTL time.Duration
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// NewQuotaChecker creates a quota checker backed by the traces table.
func NewQuotaChecker(pool *pgxpool.Pool, cfg config.QuotaConfig) *QuotaChecker {
	qc := &QuotaChecker{
		pool:     pool,
		config:   cfg,
		cache:    make(map[string]*quotaCounts),
		cacheTTL: 60 * time.Second,
		stopCh:   make(chan struct{}),
	}
	go qc.cleanupLoop()
	return qc
}

// Stop shuts down the background cleanup goroutine.
func (qc *QuotaChecker) Stop() { close(qc.stopCh) }

// UpdateConfig replaces the quota configuration (e.g., after config reload).
func (qc *QuotaChecker) UpdateConfig(cfg config.QuotaConfig) {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	qc.config = cfg
}

// Check verifies if a user is within their quota limits.
func (qc *QuotaChecker) Check(ctx context.Context, userID, channel, provider string) QuotaResult {
	window := qc.resolveWindow(userID, channel, provider)
	if window.IsZero() {
		return QuotaResult{Allowed: true}
	}

	counts := qc.getCounts(ctx, userID)

	if window.Hour > 0 && counts.hour >= window.Hour {
		return QuotaResult{Allowed: false, Window: "hour", Used: counts.hour, Limit: window.Hour}
	}
	if window.Day > 0 && counts.day >= window.Day {
		return QuotaResult{Allowed: false, Window: "day", Used: counts.day, Limit: window.Day}
	}
	if window.Week > 0 && counts.week >= window.Week {
		return QuotaResult{Allowed: false, Window: "week", Used: counts.week, Limit: window.Week}
	}
	return QuotaResult{Allowed: true}
}

// Increment optimistically bumps cached counts after a request is accepted.
func (qc *QuotaChecker) Increment(userID string) {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	if c, ok := qc.cache[userID]; ok {
		c.hour++
		c.day++
		c.week++
	}
}

// resolveWindow returns the effective quota window for a user.
// Priority: Groups > Channels > Providers > Default.
func (qc *QuotaChecker) resolveWindow(userID, channel, provider string) config.QuotaWindow {
	qc.mu.RLock()
	cfg := qc.config
	qc.mu.RUnlock()

	if w, ok := cfg.Groups[userID]; ok && !w.IsZero() {
		return w
	}
	if channel != "" {
		if w, ok := cfg.Channels[channel]; ok && !w.IsZero() {
			return w
		}
	}
	if provider != "" {
		if w, ok := cfg.Providers[provider]; ok && !w.IsZero() {
			return w
		}
	}
	return cfg.Default
}

// getCounts returns cached or fresh counts for a user.
func (qc *QuotaChecker) getCounts(ctx context.Context, userID string) quotaCounts {
	qc.mu.RLock()
	if c, ok := qc.cache[userID]; ok && time.Since(c.fetchedAt) < qc.cacheTTL {
		counts := *c
		qc.mu.RUnlock()
		return counts
	}
	qc.mu.RUnlock()

	counts := qc.queryDB(ctx, userID)
	qc.mu.Lock()
	qc.cache[userID] = &counts
	qc.mu.Unlock()
	return counts
}

// queryDB counts top-level traces for a user across time windows.
func (qc *QuotaChecker) queryDB(ctx context.Context, userID string) quotaCounts {
	now := time.Now().UTC()
	var counts quotaCounts
	counts.fetchedAt = now

	err := qc.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE created_at >= $2) AS hour_count,
			COUNT(*) FILTER (WHERE created_at >= $3) AS day_count,
			COUNT(*) FILTER (WHERE created_at >= $4) AS week_count
		FROM traces
		WHERE user_id = $1 AND parent_trace_id IS NULL AND created_at >= $4`,
		userID, now.Add(-time.Hour), now.Add(-24*time.Hour), now.Add(-7*24*time.Hour),
	).Scan(&counts.hour, &counts.day, &counts.week)
	if err != nil {
		slog.Warn("quota.query_failed", "user_id", userID, "error", err)
	}
	return counts
}

// QuotaUsage represents usage vs limit for a single time window.
type QuotaUsage struct {
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

// QuotaUsageEntry represents a single user's quota consumption.
type QuotaUsageEntry struct {
	UserID string     `json:"userId"`
	Hour   QuotaUsage `json:"hour"`
	Day    QuotaUsage `json:"day"`
	Week   QuotaUsage `json:"week"`
}

// Usage returns quota consumption for all users with recent activity.
func (qc *QuotaChecker) Usage(ctx context.Context) []QuotaUsageEntry {
	now := time.Now().UTC()
	rows, err := qc.pool.Query(ctx, `
		SELECT user_id,
			COUNT(*) FILTER (WHERE created_at >= $1) AS hour_count,
			COUNT(*) FILTER (WHERE created_at >= $2) AS day_count,
			COUNT(*) AS week_count
		FROM traces
		WHERE parent_trace_id IS NULL AND created_at >= $3
		GROUP BY user_id ORDER BY week_count DESC LIMIT 50`,
		now.Add(-time.Hour), now.Add(-24*time.Hour), now.Add(-7*24*time.Hour))
	if err != nil {
		slog.Warn("quota.usage_query_failed", "error", err)
		return nil
	}
	defer rows.Close()

	entries := []QuotaUsageEntry{}
	for rows.Next() {
		var userID string
		var hour, day, week int
		rows.Scan(&userID, &hour, &day, &week)
		window := qc.resolveWindow(userID, "", "")
		entries = append(entries, QuotaUsageEntry{
			UserID: userID,
			Hour:   QuotaUsage{Used: hour, Limit: window.Hour},
			Day:    QuotaUsage{Used: day, Limit: window.Day},
			Week:   QuotaUsage{Used: week, Limit: window.Week},
		})
	}
	return entries
}

// cleanupLoop periodically evicts stale cache entries.
func (qc *QuotaChecker) cleanupLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-qc.stopCh:
			return
		case <-ticker.C:
			qc.mu.Lock()
			stale := time.Now().Add(-5 * time.Minute)
			for k, v := range qc.cache {
				if v.fetchedAt.Before(stale) {
					delete(qc.cache, k)
				}
			}
			qc.mu.Unlock()
		}
	}
}

// FormatQuotaExceeded returns a user-friendly message when quota is exceeded.
func FormatQuotaExceeded(r QuotaResult) string {
	return fmt.Sprintf("Rate limit exceeded: %d/%d requests this %s. Please try again later.", r.Used, r.Limit, r.Window)
}
