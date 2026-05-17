// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"errors"
	"testing"

	apicommands "github.com/qorvenai/qorven/internal/api/commands"
	"github.com/qorvenai/qorven/internal/auth"
	"github.com/qorvenai/qorven/internal/config"
	"github.com/qorvenai/qorven/internal/plans"
)

// TestAuthorize_SingleSourceOfTruth is the P3-02 gate test: it proves
// that the command-API adapter (authorizeSessionID) and the plan HTTP
// adapter (authorizeForPlan) both go through gateway.authorize. A
// security patch applied to authorize affects BOTH surfaces.
//
// We exercise a matrix of actor/resource combinations with a stubbed
// gateway (no DB, no service account store). The nil-sessions branch
// in authorize means session-not-found → permit, which we exploit to
// isolate the user/role logic from storage.
func TestAuthorize_SingleSourceOfTruth(t *testing.T) {
	gw := &Gateway{cfg: &config.Config{Auth: config.AuthConfig{Token: "set"}}}
	// sessions == nil → authorize treats session-bound scopes as
	// "no store; permit" when the actor is otherwise allowed. We use
	// this quirk to test the actor matrix without DB dependencies.

	cases := []struct {
		name    string
		actor   *auth.User
		scope   AuthScope
		wantErr string // empty == permit; non-empty == match against .Code
	}{
		{
			name:    "no actor, no session — session-bound scope without user is unauthenticated",
			actor:   nil,
			scope:   AuthScope{SessionID: "s1"},
			wantErr: "no_actor",
		},
		{
			name:    "admin on session-bound scope — permit",
			actor:   &auth.User{ID: "u1", Role: "admin"},
			scope:   AuthScope{SessionID: "s1"},
			wantErr: "",
		},
		{
			name:    "admin on session-less scope — permit",
			actor:   &auth.User{ID: "u1", Role: "admin"},
			scope:   AuthScope{},
			wantErr: "",
		},
		{
			name:    "regular user, session-less scope — deny",
			actor:   &auth.User{ID: "u1", Role: "user"},
			scope:   AuthScope{},
			wantErr: "no_session_admin_only",
		},
		{
			name:    "regular user, session-bound scope (session not found, sessions=nil) — permit",
			actor:   &auth.User{ID: "u1", Role: "user"},
			scope:   AuthScope{SessionID: "s1"},
			wantErr: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.actor != nil {
				ctx = withUser(ctx, tc.actor)
			}

			// Drive both adapters with the same scope.
			errCmd := gw.authorizeSessionID(ctx, tc.scope.SessionID)

			// Plan adapter needs a *plans.Plan.
			errPlan := gw.authorizeForPlan(ctx, &plans.Plan{SessionID: tc.scope.SessionID})

			// Invariant: both paths produce the same Code (or both nil).
			cmdCode := codeOf(errCmd)
			planCode := codeOf(errPlan)
			if cmdCode != planCode {
				t.Fatalf("drift between adapters: cmd=%q plan=%q (errCmd=%v errPlan=%v)",
					cmdCode, planCode, errCmd, errPlan)
			}

			if tc.wantErr == "" {
				if errCmd != nil {
					t.Fatalf("expected permit, got: %v", errCmd)
				}
				return
			}
			if errCmd == nil {
				t.Fatalf("expected deny with code %q, got permit", tc.wantErr)
			}
			if cmdCode != tc.wantErr {
				t.Fatalf("code mismatch: got %q want %q", cmdCode, tc.wantErr)
			}
		})
	}
}

// TestAuthorize_DevModeNoAuth confirms the dev-mode compromise: when
// the gateway has no auth token AND no user on context, authorize
// permits. Matches AuthMiddlewareV2's own dev-mode silent admit.
func TestAuthorize_DevModeNoAuth(t *testing.T) {
	gw := &Gateway{cfg: &config.Config{Auth: config.AuthConfig{Token: ""}}}
	if err := gw.authorize(context.Background(), AuthScope{SessionID: "s1"}); err != nil {
		t.Fatalf("dev mode should permit, got: %v", err)
	}
	if err := gw.authorizeSessionID(context.Background(), "s1"); err != nil {
		t.Fatalf("dev mode via cmd adapter should permit, got: %v", err)
	}
	if err := gw.authorizeForPlan(context.Background(), &plans.Plan{SessionID: "s1"}); err != nil {
		t.Fatalf("dev mode via plan adapter should permit, got: %v", err)
	}
}

// TestAuthorize_PlanNilFails proves the plan adapter rejects a nil
// plan rather than silently permitting.
func TestAuthorize_PlanNilFails(t *testing.T) {
	gw := &Gateway{cfg: &config.Config{Auth: config.AuthConfig{Token: "set"}}}
	err := gw.authorizeForPlan(context.Background(), nil)
	if err == nil {
		t.Fatalf("nil plan should not permit")
	}
	// Expect a non-AuthzError internal failure, not a 403-style result.
	if _, ok := isAuthzError(err); ok {
		t.Fatalf("nil plan should produce internal error, not AuthzError")
	}
}

// codeOf extracts the AuthzError code or "" if the error is nil /
// a non-AuthzError.
func codeOf(err error) string {
	if err == nil {
		return ""
	}
	var ae *apicommands.AuthzError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return "internal:" + err.Error()
}
