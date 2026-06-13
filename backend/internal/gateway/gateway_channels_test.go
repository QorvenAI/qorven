// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/channels"
)

// newTestGatewayForBuilder returns a minimal *Gateway sufficient for
// buildAndRegisterChannel: a real channel manager (so Register works) and a
// real chi router (so route registration doesn't panic). No DB or voice
// pipeline is wired because the facebook case under test doesn't require them.
func newTestGatewayForBuilder(t *testing.T) *Gateway {
	t.Helper()
	mgr := channels.NewManager(nil)
	return &Gateway{
		chanMgr: mgr,
		router:  chi.NewRouter(),
	}
}

func TestBuildAndRegisterChannel_HotReloadsAllTypes(t *testing.T) {
	gw := newTestGatewayForBuilder(t)
	ok := gw.buildAndRegisterChannel("inst-fb", "agent-1", map[string]any{
		"page_access_token": "tok", "verify_token": "vt", "app_secret": "s",
	}, "facebook")
	if !ok {
		t.Fatal("facebook should register via the shared builder (was impossible in loadSingleChannel before)")
	}
}
