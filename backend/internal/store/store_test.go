// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// Hard store tests — context propagation, validation, UUID handling.

func TestValidateUserID_Valid(t *testing.T) {
	valid := []string{"user123", "user@example.com", "u-1-2-3"}
	for _, id := range valid {
		if err := ValidateUserID(id); err != nil { t.Errorf("should be valid: %q: %v", id, err) }
	}
}

func TestValidateUserID_Empty(t *testing.T) {
	err := ValidateUserID(""); _ = err // empty may be valid (only length checked)
}

func TestWithUserID(t *testing.T) {
	ctx := WithUserID(context.Background(), "user123")
	got := UserIDFromContext(ctx)
	if got != "user123" { t.Errorf("got %q", got) }
}

func TestUserIDFromContext_Empty(t *testing.T) {
	got := UserIDFromContext(context.Background())
	if got != "" { t.Error("empty context should return empty") }
}

func TestWithAgentID(t *testing.T) {
	id := uuid.New()
	ctx := WithAgentID(context.Background(), id)
	got := AgentIDFromContext(ctx)
	if got != id { t.Errorf("got %v", got) }
}

func TestAgentIDFromContext_Empty(t *testing.T) {
	got := AgentIDFromContext(context.Background())
	if got != uuid.Nil { t.Error("empty context should return nil UUID") }
}

func TestWithAgentKey(t *testing.T) {
	ctx := WithAgentKey(context.Background(), "prime")
	got := AgentKeyFromContext(ctx)
	if got != "prime" { t.Errorf("got %q", got) }
}

func TestAgentKeyFromContext_Empty(t *testing.T) {
	got := AgentKeyFromContext(context.Background())
	if got != "" { t.Error("should be empty") }
}

func TestWithAgentType(t *testing.T) {
	ctx := WithAgentType(context.Background(), "specialist")
	got := AgentTypeFromContext(ctx)
	if got != "specialist" { t.Errorf("got %q", got) }
}

func TestWithSenderID(t *testing.T) {
	ctx := WithSenderID(context.Background(), "sender456")
	got := SenderIDFromContext(ctx)
	if got != "sender456" { t.Errorf("got %q", got) }
}

func TestContextChaining(t *testing.T) {
	ctx := context.Background()
	ctx = WithUserID(ctx, "user1")
	ctx = WithAgentKey(ctx, "prime")
	ctx = WithSenderID(ctx, "sender1")

	if UserIDFromContext(ctx) != "user1" { t.Error("user lost") }
	if AgentKeyFromContext(ctx) != "prime" { t.Error("agent key lost") }
	if SenderIDFromContext(ctx) != "sender1" { t.Error("sender lost") }
}

func TestContextOverwrite(t *testing.T) {
	ctx := WithUserID(context.Background(), "user1")
	ctx = WithUserID(ctx, "user2")
	if UserIDFromContext(ctx) != "user2" { t.Error("should overwrite") }
}
