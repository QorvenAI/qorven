// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"testing"
)

func TestMeterScopeRoundTrip(t *testing.T) {
	ctx := WithMeterScope(context.Background(), MeterScope{
		TenantID: "t1", AgentID: "a1", SessionID: "s1", Origin: OriginAgent,
	})
	got := MeterScopeFromCtx(ctx)
	if got.TenantID != "t1" || got.AgentID != "a1" || got.SessionID != "s1" || got.Origin != OriginAgent {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestMeterScopeZeroValueIsOverhead(t *testing.T) {
	got := MeterScopeFromCtx(context.Background())
	if got.AgentID != "" {
		t.Errorf("expected blank AgentID (overhead) from empty ctx, got %q", got.AgentID)
	}
	if !got.IsOverhead() {
		t.Errorf("zero-value scope should be overhead")
	}
}

func TestMeterScopeAgentIsNotOverhead(t *testing.T) {
	s := MeterScope{AgentID: "a1"}
	if s.IsOverhead() {
		t.Errorf("scope with AgentID must not be overhead")
	}
}

func TestMeterScopeOverwrite(t *testing.T) {
	ctx := WithMeterScope(context.Background(), MeterScope{AgentID: "a1"})
	ctx = WithMeterScope(ctx, MeterScope{AgentID: "a2"})
	if MeterScopeFromCtx(ctx).AgentID != "a2" {
		t.Errorf("expected overwrite to a2")
	}
}

func TestMeterScopeCarriesHierarchyIDs(t *testing.T) {
	ctx := WithMeterScope(context.Background(), MeterScope{
		TenantID: "t", AgentID: "a", DepartmentID: "d1", ProjectID: "p1", TaskID: "k1",
	})
	got := MeterScopeFromCtx(ctx)
	if got.DepartmentID != "d1" || got.ProjectID != "p1" || got.TaskID != "k1" {
		t.Fatalf("hierarchy ids not carried: %+v", got)
	}
}
