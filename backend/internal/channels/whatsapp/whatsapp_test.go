// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package whatsapp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/channels"
)

func TestWhatsAppChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*WhatsAppChannel)(nil)
}

func TestWhatsAppChannel_New_CloudMode(t *testing.T) {
	ch := New(Config{
		AgentID:       "a1",
		PhoneNumberID: "phone123",
		AccessToken:   "tok",
		VerifyToken:   "verify",
	}, nil)
	if ch == nil { t.Fatal("nil") }
	if ch.Type() != "whatsapp" { t.Errorf("type=%q", ch.Type()) }
	if ch.AgentID() != "a1" { t.Errorf("agentID=%q", ch.AgentID()) }
	if ch.cfg.Mode != "cloud" { t.Errorf("mode=%q want cloud", ch.cfg.Mode) }
	if ch.IsRunning() { t.Error("should not be running") }
}

func TestWhatsAppChannel_New_BridgeMode(t *testing.T) {
	ch := New(Config{
		AgentID:   "a1",
		BridgeURL: "ws://localhost:3000",
	}, nil)
	if ch.cfg.Mode != "bridge" { t.Errorf("mode=%q want bridge when BridgeURL set", ch.cfg.Mode) }
}

func TestWhatsAppChannel_New_ExplicitMode(t *testing.T) {
	ch := New(Config{AgentID: "a1", Mode: "cloud"}, nil)
	if ch.cfg.Mode != "cloud" { t.Errorf("mode=%q want cloud", ch.cfg.Mode) }
}

func TestWhatsAppChannel_StartStop_Cloud(t *testing.T) {
	ch := New(Config{AgentID: "a1", PhoneNumberID: "p1", AccessToken: "t"}, nil)
	ctx := context.Background()

	ch.Start(ctx)
	if !ch.IsRunning() { t.Error("should be running after Start") }

	ch.Stop(ctx)
	if ch.IsRunning() { t.Error("should not be running after Stop") }
}

func TestWhatsAppChannel_HandleWebhook_Verification(t *testing.T) {
	ch := New(Config{AgentID: "a1", PhoneNumberID: "p1", AccessToken: "t", VerifyToken: "my-verify-token"}, nil)

	// Valid webhook verification (GET request from Facebook)
	req := httptest.NewRequest("GET", "/webhook?hub.mode=subscribe&hub.verify_token=my-verify-token&hub.challenge=challenge-xyz", nil)
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)
	if rr.Code != 200 { t.Errorf("verify: status=%d want 200", rr.Code) }
	if rr.Body.String() != "challenge-xyz" { t.Errorf("challenge=%q want challenge-xyz", rr.Body.String()) }

	// Invalid verify token
	req2 := httptest.NewRequest("GET", "/webhook?hub.verify_token=wrong&hub.challenge=xyz", nil)
	rr2 := httptest.NewRecorder()
	ch.HandleWebhook(rr2, req2)
	if rr2.Code != 403 { t.Errorf("wrong verify_token: status=%d want 403", rr2.Code) }
}

func TestWhatsAppChannel_HandleWebhook_InboundMessage(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{
		AgentID:       "a1",
		PhoneNumberID: "phone123",
		AccessToken:   "tok",
		AppSecret:     "", // no signature verification
	}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})

	// Construct a minimal Cloud API webhook payload
	payload := map[string]any{
		"object": "whatsapp_business_account",
		"entry": []map[string]any{{
			"id": "entry1",
			"changes": []map[string]any{{
				"value": map[string]any{
					"messaging_product": "whatsapp",
					"contacts": []map[string]any{{
						"wa_id": "+15551234567",
						"profile": map[string]any{"name": "Test User"},
					}},
					"messages": []map[string]any{{
						"id":        "msg1",
						"from":      "+15551234567", // not phone123
						"timestamp": "1234567890",
						"type":      "text",
						"text":      map[string]any{"body": "Hello WhatsApp!"},
					}},
				},
				"field": "messages",
			}},
		}},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 { t.Errorf("status=%d want 200", rr.Code) }
	// Content may be prefixed with sender name
	if !strings.Contains(received.Content, "Hello WhatsApp!") { t.Errorf("content=%q should contain message text", received.Content) }
	if received.ChannelType != "whatsapp" { t.Errorf("channelType=%q", received.ChannelType) }
}

func TestWhatsAppChannel_VerifyWebhookSignature(t *testing.T) {
	ch := New(Config{AgentID: "a1", AppSecret: "app-secret"}, nil)
	body := []byte(`{"test": "payload"}`)

	// Compute valid HMAC-SHA256
	mac := hmac.New(sha256.New, []byte("app-secret"))
	mac.Write(body)
	validSig := hex.EncodeToString(mac.Sum(nil))

	// Valid signature
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256="+validSig)
	if !ch.VerifyWebhookSignature(req, body) { t.Error("valid signature should pass") }

	// Invalid signature
	req2 := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	req2.Header.Set("X-Hub-Signature-256", "sha256=badhash")
	if ch.VerifyWebhookSignature(req2, body) { t.Error("invalid signature should fail") }

	// No signature header → should fail when AppSecret configured
	req3 := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	if ch.VerifyWebhookSignature(req3, body) { t.Error("missing signature should fail when secret configured") }
}

func TestWhatsAppChannel_VerifyWebhookSignature_NoSecret(t *testing.T) {
	ch := New(Config{AgentID: "a1", AppSecret: ""}, nil)
	body := []byte(`{"test": "payload"}`)
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	// No AppSecret = always pass
	if !ch.VerifyWebhookSignature(req, body) { t.Error("no AppSecret should always pass verification") }
}

func TestWhatsAppChannel_AllowList(t *testing.T) {
	ch := New(Config{
		AgentID:   "a1",
		AllowFrom: []string{"+15551111111", "+15552222222"},
	}, nil)
	if len(ch.allowList) != 2 { t.Errorf("allowList=%v", ch.allowList) }
}

func TestWhatsAppChannel_Name(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)
	if ch.Name() != "whatsapp" { t.Errorf("name=%q want whatsapp", ch.Name()) }
}

func TestWhatsAppChannel_HandleWebhook_EmptyPayload(t *testing.T) {
	ch := New(Config{AgentID: "a1", AppSecret: ""}, nil)
	body := []byte(`{}`)
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)
	if rr.Code != 200 { t.Errorf("empty payload: status=%d", rr.Code) }
}

func TestWhatsAppChannel_HandleWebhook_IgnoreSentByUs(t *testing.T) {
	// Messages from our own phone number should be ignored
	called := false
	ch := New(Config{
		AgentID:       "a1",
		PhoneNumberID: "phone123",
		AccessToken:   "tok",
	}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})

	payload := map[string]any{
		"object": "whatsapp_business_account",
		"entry": []map[string]any{{
			"changes": []map[string]any{{
				"value": map[string]any{
					"messages": []map[string]any{{
						"from":      "phone123", // same as our phone number
						"type":      "text",
						"text":      map[string]any{"body": "self message"},
					}},
				},
			}},
		}},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/wh", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if called { t.Error("self-messages should be ignored") }
}

func TestWhatsAppChannel_Mode_Detection(t *testing.T) {
	tests := []struct{
		cfg  Config
		want string
	}{
		{Config{AgentID: "a", BridgeURL: "ws://x"}, "bridge"},
		{Config{AgentID: "a", PhoneNumberID: "p", AccessToken: "t"}, "cloud"},
		{Config{AgentID: "a", Mode: "cloud"}, "cloud"},
		{Config{AgentID: "a", Mode: "bridge", BridgeURL: "ws://x"}, "bridge"},
	}
	for _, tt := range tests {
		ch := New(tt.cfg, nil)
		if ch.cfg.Mode != tt.want {
			t.Errorf("cfg=%+v: mode=%q want %q", tt.cfg, ch.cfg.Mode, tt.want)
		}
	}
}

// ensure truncation constant is reasonable
func TestWhatsAppChannel_MaxMessageLen(t *testing.T) {
	if maxMessageLen < 1000 { t.Errorf("maxMessageLen=%d too small", maxMessageLen) }
	if maxMessageLen > 10000 { t.Errorf("maxMessageLen=%d too large", maxMessageLen) }
}

// helper to suppress unused import
var _ = strings.TrimSpace
