// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package sms

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

func flushDebouncer(d *channels.Debouncer) {
	d.FlushAll()
	time.Sleep(50 * time.Millisecond)
}

// makeTwilioSig computes the Twilio HMAC-SHA1 signature for a given URL + form params.
// Twilio sorts params alphabetically, concatenates key+value pairs onto URL, then HMAC-SHA1 + base64.
func makeTwilioSig(authToken, fullURL string, params url.Values) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(fullURL)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params.Get(k))
	}

	mac := hmac.New(sha1.New, []byte(authToken))
	mac.Write([]byte(sb.String()))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestSMSChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*SMSChannel)(nil)
}

func TestSMSChannel_New(t *testing.T) {
	ch := New(Config{AgentID: "a1", AccountSID: "AC123", AuthToken: "tok", FromNumber: "+15550001234"}, nil)
	if ch == nil {
		t.Fatal("nil")
	}
	if ch.Type() != "sms" {
		t.Errorf("type=%q", ch.Type())
	}
	if ch.AgentID() != "a1" {
		t.Errorf("agentID=%q", ch.AgentID())
	}
	if !strings.Contains(ch.Name(), "sms") {
		t.Errorf("name=%q should contain sms", ch.Name())
	}
	if ch.IsRunning() {
		t.Error("should not be running")
	}
}

func TestSMSChannel_StartStop(t *testing.T) {
	ch := New(Config{AgentID: "a1", AccountSID: "AC1", AuthToken: "t", FromNumber: "+1"}, nil)
	ctx := context.Background()
	ch.Start(ctx)
	if !ch.IsRunning() {
		t.Error("should be running")
	}
	ch.Stop(ctx)
	if ch.IsRunning() {
		t.Error("should not be running")
	}
}

func TestSMSChannel_VerifySignature_Valid(t *testing.T) {
	ch := New(Config{AuthToken: "mytoken"}, nil)
	params := url.Values{
		"From": {"+15551234567"},
		"Body": {"Hello"},
		"To":   {"+15559876543"},
	}
	fullURL := "https://qorven.ai/webhooks/sms"
	sig := makeTwilioSig("mytoken", fullURL, params)

	req := httptest.NewRequest("POST", "/webhooks/sms", strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", sig)
	req.Host = "qorven.ai"
	req.Header.Set("X-Forwarded-Proto", "https")

	if !ch.verifySignature(req, fullURL) {
		t.Error("valid signature should pass")
	}
}

func TestSMSChannel_VerifySignature_BadSign(t *testing.T) {
	ch := New(Config{AuthToken: "mytoken"}, nil)
	req := httptest.NewRequest("POST", "/webhooks/sms", nil)
	req.Header.Set("X-Twilio-Signature", "badsig")

	if ch.verifySignature(req, "https://qorven.ai/webhooks/sms") {
		t.Error("invalid signature should fail")
	}
}

func TestSMSChannel_VerifySignature_MissingHeader(t *testing.T) {
	ch := New(Config{AuthToken: "mytoken"}, nil)
	req := httptest.NewRequest("POST", "/webhooks/sms", nil)

	if ch.verifySignature(req, "https://qorven.ai/webhooks/sms") {
		t.Error("missing header should fail")
	}
}

func TestSMSChannel_VerifySignature_NoToken(t *testing.T) {
	ch := New(Config{}, nil)
	req := httptest.NewRequest("POST", "/webhooks/sms", nil)

	if !ch.verifySignature(req, "https://example.com") {
		t.Error("no auth_token = always pass")
	}
}

func TestSMSChannel_HandleWebhook_Inbound(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1", FromNumber: "+15550001"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	params := url.Values{
		"From":       {"+15559998888"},
		"To":         {"+15550001"},
		"Body":       {"Hello from SMS!"},
		"MessageSid": {"SM001"},
		"NumMedia":   {"0"},
	}
	req := httptest.NewRequest("POST", "/webhook/sms", strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 {
		t.Errorf("status=%d want 200", rr.Code)
	}
	// Must return TwiML (empty ack) immediately — agent replies async via REST API
	if !strings.Contains(rr.Body.String(), "<Response>") {
		t.Errorf("expected TwiML ack, got: %s", rr.Body.String())
	}
	flushDebouncer(ch.debouncer)

	if received.Content != "Hello from SMS!" {
		t.Errorf("content=%q", received.Content)
	}
	if received.SenderID != "+15559998888" {
		t.Errorf("senderID=%q", received.SenderID)
	}
	if received.ChatID != "+15559998888" {
		t.Errorf("chatID=%q want sender phone number", received.ChatID)
	}
	if received.ChannelType != "sms" {
		t.Errorf("channelType=%q", received.ChannelType)
	}
	if received.Metadata["message_sid"] != "SM001" {
		t.Errorf("message_sid=%q", received.Metadata["message_sid"])
	}
}

func TestSMSChannel_HandleWebhook_SignatureRequired(t *testing.T) {
	ch := New(Config{AgentID: "a1", AuthToken: "mytoken", FromNumber: "+1"}, nil)

	params := url.Values{"From": {"+15551234567"}, "Body": {"hi"}, "MessageSid": {"SM999"}}
	req := httptest.NewRequest("POST", "/webhook/sms", strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// No X-Twilio-Signature header
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 403 {
		t.Errorf("status=%d want 403 (missing signature)", rr.Code)
	}
}

func TestSMSChannel_HandleWebhook_Dedup(t *testing.T) {
	count := 0
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		count++
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	params := url.Values{
		"From":       {"+15559998888"},
		"Body":       {"hello"},
		"MessageSid": {"SM-DEDUP"},
		"NumMedia":   {"0"},
	}
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/webhook", strings.NewReader(params.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ch.HandleWebhook(httptest.NewRecorder(), req)
	}
	flushDebouncer(ch.debouncer)

	if count != 1 {
		t.Errorf("count=%d want 1 (dedup by MessageSid — Twilio retries on non-200)", count)
	}
}

func TestSMSChannel_HandleWebhook_EmptyBody(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})

	params := url.Values{
		"From":     {"+15559998888"},
		"Body":     {""},
		"NumMedia": {"0"},
	}
	req := httptest.NewRequest("POST", "/sms", strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	flushDebouncer(ch.debouncer)
	if called {
		t.Error("handler should not be called with empty body and no media")
	}
	if rr.Code != 200 {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestSMSChannel_HandleWebhook_OptOut(t *testing.T) {
	called := false
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})

	for _, kw := range []string{"STOP", "UNSUBSCRIBE"} {
		params := url.Values{"From": {"+15559998888"}, "Body": {kw}, "MessageSid": {kw + "-sid"}}
		req := httptest.NewRequest("POST", "/sms", strings.NewReader(params.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		ch.HandleWebhook(httptest.NewRecorder(), req)
	}
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("opt-out keywords should not invoke handler")
	}
}

func TestSMSChannel_HandleWebhook_MMSAttachment(t *testing.T) {
	var received channels.InboundMessage
	ch := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	params := url.Values{
		"From":               {"+15559998888"},
		"Body":               {"check this out"},
		"MessageSid":         {"SM-MMS"},
		"NumMedia":           {"1"},
		"MediaUrl0":          {"https://api.twilio.com/media/ME123"},
		"MediaContentType0":  {"image/jpeg"},
	}
	req := httptest.NewRequest("POST", "/sms", strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ch.HandleWebhook(httptest.NewRecorder(), req)
	flushDebouncer(ch.debouncer)

	if !strings.Contains(received.Content, "check this out") {
		t.Errorf("content=%q should contain body text", received.Content)
	}
	if !strings.Contains(received.Content, "[Image attachment]") {
		t.Errorf("content=%q should include image label", received.Content)
	}
	if received.Metadata["has_media"] != "true" {
		t.Errorf("has_media=%q want 'true'", received.Metadata["has_media"])
	}
}

func TestSMSChannel_HandleStatusWebhook(t *testing.T) {
	ch := New(Config{AgentID: "a1"}, nil)
	params := url.Values{
		"MessageSid":    {"SM123"},
		"MessageStatus": {"delivered"},
		"To":            {"+15550000000"},
	}
	req := httptest.NewRequest("POST", "/status", strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	ch.HandleStatusWebhook(rr, req)
	if rr.Code != 200 {
		t.Errorf("status=%d", rr.Code)
	}
}

func TestSMSChannel_Send_ApiKey(t *testing.T) {
	var authHeader string
	var gotTo, gotBody, gotFrom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		r.ParseForm()
		gotTo = r.FormValue("To")
		gotBody = r.FormValue("Body")
		gotFrom = r.FormValue("From")
		w.WriteHeader(201)
		w.Write([]byte(`{"sid":"SM123","status":"queued"}`))
	}))
	defer srv.Close()

	// Inject a custom client pointing to our mock server by replacing the API URL
	ch := New(Config{
		AgentID:      "a1",
		AccountSID:   "AC123",
		ApiKeySid:    "SK-key",
		ApiKeySecret: "key-secret",
		FromNumber:   "+15550001",
	}, nil)
	// Patch sendSMS to use test server URL
	ch.client = &http.Client{
		Transport: &mockTransport{
			realURL:  "https://api.twilio.com/2010-04-01/Accounts/AC123/Messages.json",
			proxyURL: srv.URL + "/messages",
		},
	}

	err := ch.Send(context.Background(), channels.OutboundMessage{
		RecipientID: "+15559999",
		Content:     "Test message",
	})
	if err != nil {
		t.Fatal(err)
	}

	// When ApiKeySid is set, auth should use SK key (not AccountSID)
	if !strings.Contains(authHeader, "Basic") {
		t.Errorf("auth=%q should use Basic auth", authHeader)
	}
	if gotTo != "+15559999" {
		t.Errorf("To=%q", gotTo)
	}
	if gotBody != "Test message" {
		t.Errorf("Body=%q", gotBody)
	}
	if gotFrom != "+15550001" {
		t.Errorf("From=%q", gotFrom)
	}
	_ = gotFrom
}

func TestSMSChannel_Send_MessagingService(t *testing.T) {
	var gotMsgSvc, gotFrom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotMsgSvc = r.FormValue("MessagingServiceSid")
		gotFrom = r.FormValue("From")
		w.WriteHeader(201)
		w.Write([]byte(`{"sid":"SM456","status":"queued"}`))
	}))
	defer srv.Close()

	ch := New(Config{
		AccountSID:          "AC123",
		AuthToken:           "tok",
		FromNumber:          "+15550001",
		MessagingServiceSid: "MG-pool",
	}, nil)
	ch.client = &http.Client{
		Transport: &mockTransport{
			realURL:  "https://api.twilio.com/2010-04-01/Accounts/AC123/Messages.json",
			proxyURL: srv.URL + "/messages",
		},
	}

	err := ch.sendSMS("+15559999", "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	// When MessagingServiceSid is set, From should be omitted; MessagingServiceSid used instead
	if gotMsgSvc != "MG-pool" {
		t.Errorf("MessagingServiceSid=%q want MG-pool", gotMsgSvc)
	}
	if gotFrom != "" {
		t.Errorf("From=%q should be empty when using MessagingServiceSid", gotFrom)
	}
}

func TestIsOptOut(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{"STOP", true},
		{"stop", true},
		{"UNSUBSCRIBE", true},
		{"CANCEL", true},
		{"END", true},
		{"QUIT", true},
		{"hello", false},
		{"stop please", false}, // must be exact word
		{"", false},
	}
	for _, tt := range tests {
		got := isOptOut(tt.body)
		if got != tt.want {
			t.Errorf("isOptOut(%q)=%v want %v", tt.body, got, tt.want)
		}
	}
}

func TestBuildMediaLabels(t *testing.T) {
	form := url.Values{
		"MediaContentType0": {"image/jpeg"},
		"MediaUrl0":         {"https://api.twilio.com/media/ME1"},
		"MediaContentType1": {"audio/ogg"},
		"MediaUrl1":         {"https://api.twilio.com/media/ME2"},
	}
	labels := buildMediaLabels(form)
	if len(labels) != 2 {
		t.Fatalf("labels=%d want 2", len(labels))
	}
	if labels[0] != "[Image attachment]" {
		t.Errorf("labels[0]=%q", labels[0])
	}
	if labels[1] != "[Audio attachment]" {
		t.Errorf("labels[1]=%q", labels[1])
	}
}

func TestSMSChannel_Name(t *testing.T) {
	ch := New(Config{AgentID: "a1", FromNumber: "+15550001234"}, nil)
	if !strings.Contains(ch.Name(), "+15550001234") {
		t.Errorf("name=%q should contain phone number", ch.Name())
	}
}

// mockTransport rewrites requests from realURL to proxyURL for test isolation.
type mockTransport struct {
	realURL  string
	proxyURL string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), m.realURL) || req.URL.String() == m.realURL {
		newURL := m.proxyURL
		proxyReq, _ := http.NewRequest(req.Method, newURL, req.Body)
		proxyReq.Header = req.Header
		return http.DefaultTransport.RoundTrip(proxyReq)
	}
	// For other URLs, read and discard
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte(`{}`)))}, nil
}
