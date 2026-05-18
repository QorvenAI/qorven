// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	apicommands "github.com/qorvenai/qorven/internal/api/commands"
	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/api/sessioncancel"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/auth"
	"github.com/qorvenai/qorven/internal/config"
	"github.com/qorvenai/qorven/internal/deployment"
	orchestratorpkg "github.com/qorvenai/qorven/internal/orchestrator"
	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/serviceaccounts"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/store"
	"github.com/qorvenai/qorven/internal/testutil"
)

// testMinimalGatewayJWTSecret is the stable JWT secret used by every
// minimal gateway test. Kept constant so IssueToken/ValidateToken
// roundtrip across test invocations.
const testMinimalGatewayJWTSecret = "qorven-minimal-gateway-test-secret"

// MinimalGatewayOpts lets individual tests tweak the minimal gateway
// before it finishes bootstrapping. Today's only knob is DeploymentMode
// — a test that wants to exercise multi-tenant branches sets this to
// deployment.ModeMultiTenant and the helper flips the deployment_config
// row before returning.
type MinimalGatewayOpts struct {
	// DeploymentMode forces the instance into the given mode. Empty
	// means "do not touch" — the seeded migration value (single_tenant)
	// wins, matching production's default.
	DeploymentMode deployment.Mode

	// AuthTokenOverride sets cfg.Auth.Token. Empty means "no token"
	// (the dev-mode path); non-empty activates the strict-auth path.
	// Most tests leave this empty.
	AuthTokenOverride string
}

// newMinimalGateway constructs a Gateway with every dependency the
// Phase 2/3 /v1 surface needs, but none of the side-effect-heavy
// New()-only pieces (dreamer, rtHub.Run(), LSP, provider registry).
//
// Ruling coverage (from Step 2 closeout):
//   - Item #3: the auth service wiring is explicitly exercised here —
//     authSvc is constructed, JWT secret is stable (env var set + cleanup
//     registered), and token round-trips are validated in the unit test
//     for this helper.
//   - Item #6: callers no longer duplicate field setup. phase2_auth_test
//     and router_seam_test both call this.
//
// Returns the Gateway, the isolated tenant id, and a wired test server
// backed by the production buildV1Router output. The caller is free to
// mutate additional fields on Gateway before using it.
func newMinimalGateway(t *testing.T, opts MinimalGatewayOpts) (*Gateway, *pgxpool.Pool, string) {
	t.Helper()

	// Phase 3 (FU-030): every test owns its tenant.
	pool, tenantID := testutil.NewIsolatedTenant(t)

	// Stable JWT secret so auth tokens survive across Issue/Validate.
	// Cleanup unsets so a later test run doesn't inherit the override.
	os.Setenv("JWT_SECRET", testMinimalGatewayJWTSecret)
	t.Cleanup(func() { os.Unsetenv("JWT_SECRET") })

	authSvc := auth.NewAuthService(pool)
	db := &store.DB{Pool: pool}

	gw := &Gateway{
		cfg: &config.Config{
			Auth: config.AuthConfig{
				Token:         opts.AuthTokenOverride,
				EncryptionKey: "minimal-gw-enc-key-32-bytes-xxxx",
			},
			Server: config.ServerConfig{Listen: ":0"},
		},
		db:        db,
		authSvc:   authSvc,
		sessions:  session.NewStore(pool),
		rtHub:     realtime.NewHub(), // NOT started — we don't want the goroutine
		startTime: time.Now(),
	}

	// Phase 1-3 stores that the /v1 surface requires.
	gw.sessionCancels = sessioncancel.NewRegistry()
	gw.serviceAccounts = serviceaccounts.NewStore(pool)
	_ = gw.serviceAccounts.Refresh(context.Background())
	gw.plans = plans.NewStore(pool)
	gw.approvals = approvals.NewStore(pool)
	gw.events = apievents.NewEmitter(apievents.WithHub(gw.rtHub))
	gw.permissionGate = permissions.NewGate(pool, gw.events)
	gw.cmdServer = &apicommands.Server{
		Emitter:    gw.events,
		Drafts:     apicommands.NewDraftStore(time.Minute),
		Submit:     gw.protocolSubmit,
		Run:        gw.protocolRunCommand,
		Resolve:    gw.protocolResolveSession,
		OwnerCheck: gw.protocolOwnerCheck,
	}

	// P3D-03: deployment config is live in tests.
	//
	// deployment_config is a GLOBAL table (not tenant-scoped). Any
	// prior test that flipped to multi_tenant would leak that flip
	// into the next test. We explicitly SetMode on every setup so the
	// helper is order-independent.
	gw.deploymentConfig = deployment.NewConfig(pool)
	desiredMode := opts.DeploymentMode
	if desiredMode == "" {
		desiredMode = deployment.ModeSingleTenant
	}
	if err := gw.deploymentConfig.SetMode(context.Background(), desiredMode); err != nil {
		t.Fatalf("deployment.SetMode(%s): %v", desiredMode, err)
	}
	// Register cleanup so a test that flipped to multi doesn't leak
	// that into a subsequent test that uses Pool(t) + TestTenantID
	// directly without going through the helper.
	t.Cleanup(func() {
		_ = gw.deploymentConfig.SetMode(context.Background(), deployment.ModeSingleTenant)
	})

	// Orchestrator is constructed nil-safe — agentLoop is absent in
	// minimal tests, so NewService returns nil and ExecutePlan becomes
	// a no-op. That matches the Phase 2 test pattern.
	gw.orchestrator = orchestratorpkg.NewService(gw.plans, gw.approvals, nil, gw.events, nil)

	return gw, pool, tenantID
}

