// Copyright 2026 Qorven AI. All rights reserved.
package connectors

import (
	"context"
	"testing"
)

func TestWithAgentID_RoundTrip(t *testing.T) {
	ctx := WithAgentID(context.Background(), "agent-123")
	if got := agentIDFromContext(ctx); got != "agent-123" {
		t.Fatalf("agentIDFromContext = %q, want agent-123", got)
	}
	// Empty context → empty (human-initiated, allowed by the gate default).
	if got := agentIDFromContext(context.Background()); got != "" {
		t.Fatalf("empty ctx should yield empty agent id, got %q", got)
	}
	// The OLD string-key must NOT satisfy the typed read (proves the bug is fixed).
	strCtx := context.WithValue(context.Background(), contextKeyForTest, "via-string-key")
	if got := agentIDFromContext(strCtx); got != "" {
		t.Fatalf("string-key value must not be read by the typed accessor, got %q", got)
	}
}

// contextKeyForTest mimics the OLD bare-string key to prove it no longer matches.
type stringKeyT string

const contextKeyForTest stringKeyT = "agent_id"
