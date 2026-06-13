// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"net/http"
	"net/http/httptest"
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

func TestBuildAndRegisterChannel_ClassB_RoutesRegistered(t *testing.T) {
	cases := []struct {
		name    string
		chType  string
		cfg     map[string]any
		method  string
		path    string
	}{
		{
			name:   "teams inbound POST",
			chType: "teams",
			cfg:    map[string]any{"app_id": "aid", "app_secret": "sec"},
			method: http.MethodPost,
			path:   "/v1/webhooks/teams/inst-teams",
		},
		{
			name:   "feishu inbound POST",
			chType: "feishu",
			cfg:    map[string]any{"app_id": "aid", "app_secret": "sec"},
			method: http.MethodPost,
			path:   "/v1/webhooks/feishu/inst-feishu",
		},
		{
			name:   "dingtalk inbound POST",
			chType: "dingtalk",
			cfg:    map[string]any{"app_key": "key", "app_secret": "sec"},
			method: http.MethodPost,
			path:   "/v1/webhooks/dingtalk/inst-dingtalk",
		},
		{
			name:   "wecom inbound GET (echostr verify)",
			chType: "wecom",
			cfg:    map[string]any{"corp_id": "corp", "agent_secret": "sec"},
			method: http.MethodGet,
			path:   "/v1/webhooks/wecom/inst-wecom",
		},
		{
			name:   "wecom inbound POST (events)",
			chType: "wecom",
			cfg:    map[string]any{"corp_id": "corp", "agent_secret": "sec"},
			method: http.MethodPost,
			path:   "/v1/webhooks/wecom/inst-wecom",
		},
		{
			// webhook with inbound_path only (sync/inbound-only, no outbound URL)
			name:   "webhook inbound-only POST",
			chType: "webhook",
			cfg:    map[string]any{"inbound_path": "/v1/webhooks/in/inst-webhook"},
			method: http.MethodPost,
			path:   "/v1/webhooks/in/inst-webhook",
		},
		{
			// webhook with default generated path (no inbound_path, no outbound_url — should NOT load)
			// skip: guard requires at least one of inbound_path or outbound_url
			name:   "webhook default-path inbound POST",
			chType: "webhook",
			cfg:    map[string]any{"outbound_url": "http://example.com/cb"},
			method: http.MethodPost,
			path:   "/v1/webhooks/in/inst-webhook2",
		},
		{
			// webchat WS route registered as GET
			name:   "webchat ws GET",
			chType: "webchat",
			cfg:    map[string]any{},
			method: http.MethodGet,
			path:   "/v1/channels/inst-webchat/webchat/ws",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instID := "inst-" + tc.chType
			if tc.name == "webhook default-path inbound POST" {
				instID = "inst-webhook2"
			}
			gw := newTestGatewayForBuilder(t)
			ok := gw.buildAndRegisterChannel(instID, "agent-1", tc.cfg, tc.chType)
			if !ok {
				t.Fatalf("buildAndRegisterChannel returned false for %s cfg %v", tc.name, tc.cfg)
			}
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			gw.router.ServeHTTP(rec, req)
			if rec.Code == http.StatusNotFound {
				t.Fatalf("%s: route %s %s returned 404 — not registered", tc.name, tc.method, tc.path)
			}
		})
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
