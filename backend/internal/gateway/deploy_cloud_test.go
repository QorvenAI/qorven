// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/gateway/deploy"
)

// TestCloudDeploy_MissingRepo verifies that Deploy returns an error when
// RepoOwner or RepoName is absent — no network call is made.
func TestCloudDeploy_MissingRepo(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"vercel", "netlify"} {
		provider := provider
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			tgt := newCloudTarget(nil, provider)
			_, err := tgt.Deploy(context.Background(), deploy.Spec{
				ProjectID: "proj-1",
				// RepoOwner and RepoName intentionally empty
			})
			if err == nil {
				t.Fatal("expected error for missing repo, got nil")
			}
			if !strings.Contains(err.Error(), "connected GitHub repo") {
				t.Fatalf("unexpected error text: %v", err)
			}
		})
	}
}

// TestCloudDeploy_MissingVault verifies that Deploy returns a friendly error
// when gw.vault is nil (no provider token configured).
func TestCloudDeploy_MissingVault(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"vercel", "netlify"} {
		provider := provider
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			// gw is a zero-value Gateway — vault field is nil.
			tgt := newCloudTarget(&Gateway{}, provider)
			_, err := tgt.Deploy(context.Background(), deploy.Spec{
				ProjectID: "proj-1",
				RepoOwner: "acme",
				RepoName:  "app",
			})
			if err == nil {
				t.Fatal("expected error for missing vault, got nil")
			}
			if !strings.Contains(err.Error(), "token in Settings") {
				t.Fatalf("unexpected error text: %v", err)
			}
		})
	}
}
