// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package deployment_test

import (
	"context"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/deployment"
	"github.com/qorvenai/qorven/internal/testutil"
)

func TestConfig_DefaultsToSingleTenant(t *testing.T) {
	pool, _ := testutil.NewIsolatedTenant(t)
	// deployment_config is a GLOBAL table — a parallel test may have
	// flipped the row to multi. Reset before asserting the default so
	// the test is order-independent. (Phase 4 cross-test hygiene.)
	if _, err := pool.Exec(context.Background(),
		`UPDATE deployment_config SET value='single_tenant' WHERE key='deployment_mode'`); err != nil {
		t.Fatalf("reset deployment_mode: %v", err)
	}
	c := deployment.NewConfig(pool)
	if !t.Run("before refresh", func(t *testing.T) {
		// Before Refresh, mode is the safe default.
		if c.Mode(context.Background()) != deployment.ModeSingleTenant {
			t.Fatalf("safe default must be single_tenant")
		}
	}) {
		return
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if c.IsMultiTenant(context.Background()) {
		t.Fatalf("migration 038 seeds single_tenant; got multi")
	}
}

func TestConfig_SetMode(t *testing.T) {
	pool, _ := testutil.NewIsolatedTenant(t)
	c := deployment.NewConfig(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Flip to multi.
	if err := c.SetMode(ctx, deployment.ModeMultiTenant); err != nil {
		t.Fatalf("SetMode multi: %v", err)
	}
	if !c.IsMultiTenant(ctx) {
		t.Fatalf("expected multi after SetMode")
	}

	// Flip back to single.
	if err := c.SetMode(ctx, deployment.ModeSingleTenant); err != nil {
		t.Fatalf("SetMode single: %v", err)
	}
	if c.IsMultiTenant(ctx) {
		t.Fatalf("expected single after SetMode back")
	}
}

func TestConfig_SetMode_Rejects_Invalid(t *testing.T) {
	pool, _ := testutil.NewIsolatedTenant(t)
	c := deployment.NewConfig(pool)
	if err := c.SetMode(context.Background(), "anarchy"); err == nil {
		t.Fatalf("invalid mode must be rejected")
	}
}

func TestConfig_NilPool_Safe(t *testing.T) {
	var c *deployment.Config
	// A nil Config still returns the safe default.
	if c.Mode(context.Background()) != deployment.ModeSingleTenant {
		t.Fatalf("nil Config must default to single_tenant")
	}
	if c.IsMultiTenant(context.Background()) {
		t.Fatalf("nil Config must not claim multi-tenant")
	}
}
