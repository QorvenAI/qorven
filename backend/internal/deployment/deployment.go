// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

// Package deployment exposes the deployment_config table as a strongly
// typed, cached read-mostly accessor. Application code consults this
// at request time to decide whether strict multi-tenant rules apply.
//
// Phase 3 foundations ONLY — the mode defaults to single_tenant and
// callers that check IsMultiTenant() MUST treat a false return as a
// safety-net "apply the relaxed single-tenant rules". Never flip the
// behavior of an existing single-tenant install silently.
package deployment

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Mode enumerates known deployment modes.
type Mode string

const (
	ModeSingleTenant Mode = "single_tenant"
	ModeMultiTenant  Mode = "multi_tenant"
)

// Config caches the deployment_config table. Safe for concurrent
// reads; refreshed every RefreshInterval (default 60s).
type Config struct {
	pool            *pgxpool.Pool
	RefreshInterval time.Duration

	// cachedMode is the last-read mode. Atomic so concurrent readers
	// never see a torn value.
	cachedMode     atomic.Value // stores Mode
	lastRefresh    atomic.Int64 // UnixNano of last successful refresh
	mu             sync.Mutex   // serializes refreshes
}

// NewConfig constructs a Config. Callers must call Refresh once at
// startup; subsequent reads are served from cache.
func NewConfig(pool *pgxpool.Pool) *Config {
	c := &Config{pool: pool, RefreshInterval: 60 * time.Second}
	c.cachedMode.Store(ModeSingleTenant) // safe default until first Refresh
	return c
}

// Refresh reloads the deployment_mode value from the DB. Returns any
// DB error; on success the cache is updated atomically.
func (c *Config) Refresh(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pool == nil {
		return errors.New("deployment: nil pool")
	}
	var raw string
	err := c.pool.QueryRow(ctx,
		`SELECT value FROM deployment_config WHERE key = 'deployment_mode'`,
	).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Migration 038 seeds this; if missing, we default to single_tenant.
			c.cachedMode.Store(ModeSingleTenant)
			c.lastRefresh.Store(time.Now().UnixNano())
			return nil
		}
		return fmt.Errorf("deployment: read mode: %w", err)
	}
	mode := Mode(raw)
	if mode != ModeSingleTenant && mode != ModeMultiTenant {
		return fmt.Errorf("deployment: unknown mode %q in DB", raw)
	}
	c.cachedMode.Store(mode)
	c.lastRefresh.Store(time.Now().UnixNano())
	return nil
}

// Mode returns the current mode. Triggers a refresh if the cache is
// older than RefreshInterval; falls back to the last known good value
// when the refresh fails. Never returns an empty Mode.
func (c *Config) Mode(ctx context.Context) Mode {
	if c == nil || c.pool == nil {
		return ModeSingleTenant
	}
	last := time.Unix(0, c.lastRefresh.Load())
	if time.Since(last) > c.RefreshInterval {
		// Best-effort refresh; ignore errors to preserve availability.
		_ = c.Refresh(ctx)
	}
	m, _ := c.cachedMode.Load().(Mode)
	if m == "" {
		return ModeSingleTenant
	}
	return m
}

// IsMultiTenant reports whether the instance is in multi-tenant mode.
// Call sites that branch on this MUST default their fallback to the
// single-tenant path — never flip an existing install's behavior by
// mistake.
func (c *Config) IsMultiTenant(ctx context.Context) bool {
	return c.Mode(ctx) == ModeMultiTenant
}

// SetMode updates the deployment_mode row. Intended for admin tooling
// only; application code should NOT call this. Returns an error if
// the new mode is invalid.
func (c *Config) SetMode(ctx context.Context, mode Mode) error {
	if mode != ModeSingleTenant && mode != ModeMultiTenant {
		return fmt.Errorf("deployment: invalid mode %q", mode)
	}
	if c.pool == nil {
		return errors.New("deployment: nil pool")
	}
	_, err := c.pool.Exec(ctx,
		`INSERT INTO deployment_config (key, value, updated_at)
		 VALUES ('deployment_mode', $1, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = NOW()`,
		string(mode),
	)
	if err != nil {
		return fmt.Errorf("deployment: write mode: %w", err)
	}
	// Force immediate refresh so callers see the new value without
	// waiting for the TTL.
	return c.Refresh(ctx)
}
