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

func TestBuildAndRegisterChannel_ClassA_LoadsWithCanonicalKeys(t *testing.T) {
	cases := []struct {
		name   string
		chType string
		cfg    map[string]any
	}{
		{
			name:   "sms canonical",
			chType: "sms",
			cfg:    map[string]any{"account_sid": "AC123", "auth_token": "tok", "from_number": "+15551234567"},
		},
		{
			name:   "sms legacy keys (api_key / api_secret)",
			chType: "sms",
			cfg:    map[string]any{"api_key": "AC123", "api_secret": "tok", "from_number": "+15551234567"},
		},
		{
			name:   "line canonical",
			chType: "line",
			cfg:    map[string]any{"channel_secret": "sec", "channel_token": "tok"},
		},
		{
			name:   "line legacy key (channel_access_token)",
			chType: "line",
			cfg:    map[string]any{"channel_secret": "sec", "channel_access_token": "tok"},
		},
		{
			name:   "dingtalk canonical",
			chType: "dingtalk",
			cfg:    map[string]any{"app_key": "key", "app_secret": "sec"},
		},
		{
			name:   "dingtalk legacy keys (client_id / client_secret)",
			chType: "dingtalk",
			cfg:    map[string]any{"client_id": "key", "client_secret": "sec"},
		},
		{
			name:   "wecom canonical",
			chType: "wecom",
			cfg:    map[string]any{"corp_id": "corp", "agent_secret": "sec"},
		},
		{
			name:   "wecom legacy key (app_secret)",
			chType: "wecom",
			cfg:    map[string]any{"corp_id": "corp", "app_secret": "sec"},
		},
		{
			name:   "signal canonical",
			chType: "signal",
			cfg:    map[string]any{"api_url": "http://localhost:8080", "phone_number": "+15559999999"},
		},
		{
			name:   "signal legacy key (socket_path)",
			chType: "signal",
			cfg:    map[string]any{"socket_path": "http://localhost:8080", "phone_number": "+15559999999"},
		},
		{
			name:   "github canonical",
			chType: "github",
			cfg:    map[string]any{"access_token": "ghp_tok", "owner": "org", "repo": "repo"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newTestGatewayForBuilder(t)
			ok := gw.buildAndRegisterChannel("inst-"+tc.chType, "agent-1", tc.cfg, tc.chType)
			if !ok {
				t.Fatalf("%s: buildAndRegisterChannel returned false; guard likely failed on cfg %v", tc.name, tc.cfg)
			}
		})
	}
}
