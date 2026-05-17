// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"github.com/go-chi/chi/v5"
)

// buildV1Router installs the /v1 route group on the given parent
// router. It is the single source of truth for authenticated route
// wiring: gateway.New() calls it during startup; integration tests
// call it to exercise the exact same middleware chain and handler
// registrations without booting the dreamer, rtHub, LSP, or other
// New()-side-effect goroutines.
//
// Contract:
//
//  1. gw.ensureProtocolSurfaces() must be invoked before this function
//     (or callable inside). This builds events, cmdServer, plans,
//     approvals, permissionGate, orchestrator, serviceAccounts.
//  2. AuthMiddlewareV2 is installed as the FIRST middleware on the
//     sub-router. Every /v1/* endpoint is gated.
//  3. Handler registrations that depend on optional services
//     (scenarioHandlers, a2aServer, voiceMgr, etc.) are conditionally
//     mounted exactly as the production path would, so tests see the
//     same shape.
//
// The function does NOT install public-facing routes (/health,
// /metrics, /auth/*, webhooks) — those belong on the parent router
// and are written there by New(). This split keeps the
// authenticated route surface testable in isolation.
//
// Returns the root parent router for fluent chaining.
func buildV1Router(gw *Gateway, r chi.Router) chi.Router {
	gw.ensureProtocolSurfaces()
	gw.registerV1Routes(r)
	return r
}
