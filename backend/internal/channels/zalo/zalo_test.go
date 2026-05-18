// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package zalo

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

func flushDebouncer(d *channels.Debouncer) {
	d.FlushAll()
	time.Sleep(50 * time.Millisecond)
}

func makeZaloSig(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "MAC=" + hex.EncodeToString(mac.Sum(nil))
}

func TestChannelInterface(t *testing.T) {
	var _ channels.Channel = (*Channel)(nil)
}

func TestZaloChannel_New(t *testing.T) {
	ch := New(ZaloConfig{AgentID: "a1", AppID: "123"}, nil)
	if ch.Type() != "zalo" {
		t.Errorf("type=%q", ch.Type())
	}
	if ch.AgentID() != "a1" {
		t.Errorf("agentID=%q", ch.AgentID())
	}
}

func TestZaloChannel_StartStop(t *testing.T) {
	ch := New(ZaloConfig{AgentID: "a1"}, nil)
	ctx := context.Background()
	ch.Start(ctx)
	if !ch.IsRunning() {
		t.Error("should be running after Start")
	}
	ch.Stop(ctx)
	if ch.IsRunning() {
		t.Error("should not be running after Stop")
	}
}

func TestZaloChannel_OABaseURL(t *testing.T) {
	if oaBaseURL != "https://openapi.zalo.me/v2.0/oa" {
		t.Errorf("oaBaseURL=%q want v2.0 (v1.0 is deprecated, v3.0 does not exist)", oaBaseURL)
	}
}

func TestZaloChannel_MaxTextLen(t *testing.T) {
	if maxTextLen != 2000 {
		t.Errorf("maxTextLen=%d want 2000", maxTextLen)
	}
}

func TestZaloChannel_VerifySignature_Valid(t *testing.T) {
	ch := New(ZaloConfig{AppSecret: "mysecret"}, nil)
	body := []byte(`{"event_name":"user_send_text"}`)
	sig := makeZaloSig("mysecret", body)

	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("X-ZEvent-Signature", sig)

	if !ch.verifySignature(req, body) {
		t.Error("valid signature should pass")
	}
}

func TestZaloChannel_VerifySignature_BadSign(t *testing.T) {
	ch := New(ZaloConfig{AppSecret: "mysecret"}, nil)
	req := httptest.NewRequest("POST", "/webhook", nil)
	req.Header.Set("X-ZEvent-Signature", "MAC=badsig")

	if ch.verifySignature(req, []byte("body")) {
		t.Error("invalid signature should fail")
	}
}

func TestZaloChannel_VerifySignature_MissingHeader(t *testing.T) {
	ch := New(ZaloConfig{AppSecret: "mysecret"}, nil)
	req := httptest.NewRequest("POST", "/webhook", nil)

	if ch.verifySignature(req, []byte("body")) {
		t.Error("missing header should fail")
	}
}

func TestZaloChannel_VerifySignature_NoSecret(t *testing.T) {
	ch := New(ZaloConfig{}, nil)
	req := httptest.NewRequest("POST", "/webhook", nil)

	if !ch.verifySignature(req, []byte("anything")) {
		t.Error("no app_secret = always pass")
	}
}

func TestZaloChannel_HandleWebhook_SignatureRequired(t *testing.T) {
	ch := New(ZaloConfig{AgentID: "a1", AppSecret: "mysecret"}, nil)

	body, _ := json.Marshal(zaloEvent{EventName: "user_send_text"})
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	// No signature header
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 401 {
		t.Errorf("status=%d want 401 (missing signature)", rr.Code)
	}
}

func TestZaloChannel_HandleWebhook_TextMessage(t *testing.T) {
	var received channels.InboundMessage
	ch := New(ZaloConfig{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	event := zaloEvent{
		AppID:     "app123",
		EventName: "user_send_text",
	}
	event.Sender.ID = "user123"
	event.Message.MsgID = "msg001"
	event.Message.Text = "Hello Zalo!"

	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 {
		t.Errorf("status=%d", rr.Code)
	}
	flushDebouncer(ch.debouncer)

	if received.Content != "Hello Zalo!" {
		t.Errorf("content=%q", received.Content)
	}
	if received.SenderID != "user123" {
		t.Errorf("senderID=%q", received.SenderID)
	}
	if received.ChatID != "user123" {
		t.Errorf("chatID=%q want user123 (must be set directly)", received.ChatID)
	}
	if received.Metadata["message_id"] != "msg001" {
		t.Errorf("message_id=%q", received.Metadata["message_id"])
	}
}

func TestZaloChannel_HandleWebhook_Dedup(t *testing.T) {
	count := 0
	ch := New(ZaloConfig{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		count++
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	event := zaloEvent{EventName: "user_send_text"}
	event.Sender.ID = "user1"
	event.Message.MsgID = "dedup-msg"
	event.Message.Text = "hello"

	body, _ := json.Marshal(event)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
		ch.HandleWebhook(httptest.NewRecorder(), req)
	}
	flushDebouncer(ch.debouncer)

	if count != 1 {
		t.Errorf("count=%d want 1 (dedup by msg_id — Zalo retries on non-200)", count)
	}
}

func TestZaloChannel_HandleWebhook_MediaTypes(t *testing.T) {
	cases := []struct {
		eventName string
		want      string
	}{
		{"user_send_image", "[Image attachment]"},
		{"user_send_gif", "[GIF attachment]"},
		{"user_send_audio", "[Audio attachment]"},
		{"user_send_video", "[Video attachment]"},
	}

	for i, c := range cases {
		var received channels.InboundMessage
		ch := New(ZaloConfig{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
			received = msg
		})
		ch.Start(context.Background())

		event := zaloEvent{EventName: c.eventName}
		event.Sender.ID = "user1"
		event.Message.MsgID = fmt.Sprintf("media-%d", i)

		body, _ := json.Marshal(event)
		req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
		ch.HandleWebhook(httptest.NewRecorder(), req)
		flushDebouncer(ch.debouncer)
		ch.Stop(context.Background())

		if received.Content != c.want {
			t.Errorf("event=%q content=%q want %q", c.eventName, received.Content, c.want)
		}
	}
}

func TestZaloChannel_HandleWebhook_FileMessage(t *testing.T) {
	var received channels.InboundMessage
	ch := New(ZaloConfig{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	event := zaloEvent{EventName: "user_send_file"}
	event.Sender.ID = "user1"
	event.Message.MsgID = "file-msg"
	event.Message.Attachments = []zaloAttachment{{
		Type: "file",
		Payload: struct {
			URL         string `json:"url"`
			Name        string `json:"name"`
			Coordinates struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"coordinates"`
		}{
			URL:  "https://example.com/doc.pdf",
			Name: "report.pdf",
		},
	}}

	body, _ := json.Marshal(event)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)
	flushDebouncer(ch.debouncer)

	if received.Content != "[File: report.pdf]" {
		t.Errorf("content=%q want '[File: report.pdf]'", received.Content)
	}
	if received.Metadata["file_url"] != "https://example.com/doc.pdf" {
		t.Errorf("file_url=%q", received.Metadata["file_url"])
	}
}

func TestZaloChannel_HandleWebhook_FollowUnfollow_Skipped(t *testing.T) {
	called := false
	ch := New(ZaloConfig{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	for _, eventName := range []string{"follow", "unfollow"} {
		event := zaloEvent{EventName: eventName}
		event.Follower.ID = "user1"
		body, _ := json.Marshal(event)
		req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
		ch.HandleWebhook(httptest.NewRecorder(), req)
	}
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("follow/unfollow events should not invoke handler (logged only)")
	}
}

func TestZaloChannel_SplitMessage(t *testing.T) {
	chunks := splitMessage(strings.Repeat("x", 2001), 2000)
	if len(chunks) != 2 {
		t.Errorf("chunks=%d want 2", len(chunks))
	}
}

func TestZaloChannel_TokenRotation(t *testing.T) {
	// Verify that RefreshToken field exists and is stored in the channel
	ch := New(ZaloConfig{
		AppID:        "123",
		AppSecret:    "secret",
		RefreshToken: "initial-refresh-token",
	}, nil)

	ch.tokenMu.Lock()
	if ch.refreshToken != "initial-refresh-token" {
		t.Errorf("refreshToken=%q want 'initial-refresh-token'", ch.refreshToken)
	}
	ch.tokenMu.Unlock()
}

func TestZaloChannel_TokenMutex_Separate(t *testing.T) {
	ch := New(ZaloConfig{AgentID: "a1"}, nil)
	ctx := context.Background()
	ch.Start(ctx)
	for i := 0; i < 10; i++ {
		go ch.IsRunning()
		go func() { ch.tokenMu.Lock(); ch.tokenMu.Unlock() }()
	}
	time.Sleep(10 * time.Millisecond)
	ch.Stop(ctx)
}

// --- Crypto tests (unchanged) ---

func TestEncryptCBC_Hex(t *testing.T) {
	key := []byte("0123456789abcdef")
	ct, err := EncryptCBC(key, "hello world", true)
	if err != nil {
		t.Fatal(err)
	}
	if ct == "" {
		t.Error("empty ciphertext")
	}
	if _, err := hex.DecodeString(ct); err != nil {
		t.Errorf("not valid hex: %v", err)
	}
}

func TestEncryptCBC_Base64(t *testing.T) {
	key := []byte("0123456789abcdef")
	ct, err := EncryptCBC(key, "hello world", false)
	if err != nil {
		t.Fatal(err)
	}
	if ct == "" {
		t.Error("empty ciphertext")
	}
}

func TestDecryptCBC_Roundtrip(t *testing.T) {
	key := []byte("0123456789abcdef")
	original := "hello world test message"
	ct, err := EncryptCBC(key, original, false)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := DecryptCBC(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != original {
		t.Errorf("got %q, want %q", string(plain), original)
	}
}

func TestDecryptCBC_InvalidBase64(t *testing.T) {
	key := []byte("0123456789abcdef")
	_, err := DecryptCBC(key, "not-valid-base64!!!")
	if err == nil {
		t.Error("should fail on invalid base64")
	}
}

func TestDecryptCBC_EmptyInput(t *testing.T) {
	key := []byte("0123456789abcdef")
	_, err := DecryptCBC(key, "")
	if err == nil {
		t.Error("should fail on empty input")
	}
}

func TestEncryptCBC_InvalidKeySize(t *testing.T) {
	key := []byte("short")
	_, err := EncryptCBC(key, "hello", false)
	if err == nil {
		t.Error("should fail with invalid key size")
	}
}

func TestDecryptGCM(t *testing.T) {
	key := []byte("0123456789abcdef")
	iv := []byte("0123456789abcdef")
	_, _ = DecryptGCM(key, iv, nil, []byte("short"))
}

func TestPKCS7Pad(t *testing.T) {
	data := []byte("hello")
	padded, err := pkcs7Pad(data, 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(padded)%16 != 0 {
		t.Errorf("not aligned: len=%d", len(padded))
	}
	if len(padded) != 16 {
		t.Errorf("expected 16, got %d", len(padded))
	}
}

func TestPKCS7Pad_AlreadyAligned(t *testing.T) {
	data := []byte("0123456789abcdef")
	padded, err := pkcs7Pad(data, 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(padded) != 32 {
		t.Errorf("expected 32, got %d", len(padded))
	}
}

func TestPKCS7Unpad(t *testing.T) {
	data := []byte("hello")
	padded, _ := pkcs7Pad(data, 16)
	unpadded, err := pkcs7Unpad(padded, 16)
	if err != nil {
		t.Fatal(err)
	}
	if string(unpadded) != "hello" {
		t.Errorf("got %q", string(unpadded))
	}
}

func TestPKCS7Unpad_InvalidPadding(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 0}
	_, err := pkcs7Unpad(data, 16)
	if err == nil {
		t.Error("should fail on invalid padding")
	}
}

func TestPKCS7Pad_InvalidBlockSize(t *testing.T) {
	_, err := pkcs7Pad([]byte("hello"), 0)
	if err == nil {
		t.Error("should fail on zero block size")
	}
}

func TestSecretKey_Bytes(t *testing.T) {
	sk := SecretKey("aGVsbG8=") // "hello" in base64
	b := sk.Bytes()
	if string(b) != "hello" {
		t.Errorf("got %q", string(b))
	}
}

func TestNewSession(t *testing.T) {
	sess := NewSession()
	if sess == nil {
		t.Fatal("nil session")
	}
	if sess.UserAgent == "" {
		t.Error("empty user agent")
	}
	if sess.Client == nil {
		t.Error("nil client")
	}
	if sess.CookieJar == nil {
		t.Error("nil cookie jar")
	}
}

func TestThreadType(t *testing.T) {
	if ThreadUser != 0 {
		t.Error("ThreadUser should be 0")
	}
	if ThreadGroup != 1 {
		t.Error("ThreadGroup should be 1")
	}
}

func TestZaloConfig(t *testing.T) {
	cfg := ZaloConfig{
		AppID: "123", AppSecret: "sec", RefreshToken: "rt",
		AccessToken: "at", AgentID: "agent1", PersonalMode: true,
	}
	if cfg.AppID != "123" {
		t.Error("wrong app_id")
	}
	if cfg.AppSecret != "sec" {
		t.Error("wrong app_secret")
	}
	if cfg.RefreshToken != "rt" {
		t.Error("wrong refresh_token")
	}
	if !cfg.PersonalMode {
		t.Error("should be personal mode")
	}
}
