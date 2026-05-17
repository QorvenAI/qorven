// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package wecom

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http/httptest"
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

func makeSign(token, timestamp, nonce, fourth string) string {
	strs := []string{token, timestamp, nonce, fourth}
	sort.Strings(strs)
	hash := sha1.Sum([]byte(strings.Join(strs, "")))
	return fmt.Sprintf("%x", hash)
}

// encryptPayload builds a WeChat Work AES-CBC encrypted string for testing.
// Format: 16 bytes random | 4 bytes big-endian msg length | msg bytes | corpID bytes — then PKCS7+AES-CBC.
func encryptPayload(encodingKey, corpID, message string) string {
	key, _ := base64.StdEncoding.DecodeString(encodingKey + "=")
	msgBytes := []byte(message)
	buf := make([]byte, 20+len(msgBytes)+len(corpID))
	binary.BigEndian.PutUint32(buf[16:20], uint32(len(msgBytes)))
	copy(buf[20:], msgBytes)
	copy(buf[20+len(msgBytes):], []byte(corpID))
	pad := aes.BlockSize - (len(buf) % aes.BlockSize)
	padded := append(buf, bytes.Repeat([]byte{byte(pad)}, pad)...)
	block, _ := aes.NewCipher(key)
	iv := key[:aes.BlockSize]
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(padded, padded)
	return base64.StdEncoding.EncodeToString(padded)
}

// buildPostBody wraps a string in the WeChat Work outer XML envelope using CDATA.
func buildPostBody(toUser, encrypted string) []byte {
	return []byte(fmt.Sprintf(
		"<xml><ToUserName><![CDATA[%s]]></ToUserName><Encrypt><![CDATA[%s]]></Encrypt></xml>",
		toUser, encrypted,
	))
}

func TestWeComChannel_Interface(t *testing.T) {
	var _ channels.Channel = (*WeComChannel)(nil)
}

func TestWeComChannel_New(t *testing.T) {
	ch, err := New(Config{AgentID: "a1", CorpID: "wx123"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ch.Type() != "wecom" {
		t.Errorf("type=%q", ch.Type())
	}
	if ch.AgentID() != "a1" {
		t.Errorf("agentID=%q", ch.AgentID())
	}
}

func TestWeComChannel_New_InvalidEncodingKey(t *testing.T) {
	_, err := New(Config{EncodingKey: "not-valid-base64!!!"}, nil)
	if err == nil {
		t.Error("expected error for invalid encoding key")
	}
}

func TestWeComChannel_StartStop(t *testing.T) {
	ch, _ := New(Config{AgentID: "a1"}, nil)
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

func TestWeComChannel_VerifySignature_Valid(t *testing.T) {
	ch, _ := New(Config{Token: "mytoken"}, nil)
	ts := "1700000000"
	nonce := "abc123"
	fourth := "echoval"
	sig := makeSign("mytoken", ts, nonce, fourth)
	if !ch.verifySignature(sig, ts, nonce, fourth) {
		t.Error("valid signature should pass")
	}
}

func TestWeComChannel_VerifySignature_BadSign(t *testing.T) {
	ch, _ := New(Config{Token: "mytoken"}, nil)
	if ch.verifySignature("badsig", "1700000000", "abc", "val") {
		t.Error("invalid signature should fail")
	}
}

func TestWeComChannel_VerifySignature_NoToken(t *testing.T) {
	ch, _ := New(Config{}, nil)
	if !ch.verifySignature("anything", "ts", "nonce", "val") {
		t.Error("no token = always pass")
	}
}

func TestWeComChannel_HandleWebhook_GET_Verification(t *testing.T) {
	// Without EncodingKey, decrypt() returns the echostr as-is.
	ch, _ := New(Config{AgentID: "a1"}, nil)
	req := httptest.NewRequest("GET", "/webhook?echostr=helloworld&timestamp=ts&nonce=n&msg_signature=sig", nil)
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 200 {
		t.Errorf("status=%d", rr.Code)
	}
	if rr.Body.String() != "helloworld" {
		t.Errorf("body=%q want helloworld", rr.Body.String())
	}
}

func TestWeComChannel_HandleWebhook_TextMessage(t *testing.T) {
	var received channels.InboundMessage
	ch, _ := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
		received = msg
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	// Without EncodingKey: decrypt() returns Encrypt value as-is — put inner XML directly.
	innerXML := `<xml><FromUserName>user1</FromUserName><ToUserName>corp1</ToUserName><MsgType>text</MsgType><Content>Hello WeChat!</Content><MsgId>1001</MsgId></xml>`
	body := buildPostBody("corp1", innerXML)
	req := httptest.NewRequest("POST", "/webhook?msg_signature=sig&timestamp=ts&nonce=n", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Body.String() != "success" {
		t.Errorf("response body=%q want 'success' (empty body triggers WeChat Work retries)", rr.Body.String())
	}
	flushDebouncer(ch.debouncer)

	if received.Content != "Hello WeChat!" {
		t.Errorf("content=%q", received.Content)
	}
	if received.SenderID != "user1" {
		t.Errorf("senderID=%q", received.SenderID)
	}
	if received.ChatID != "user1" {
		t.Errorf("chatID=%q want user1 (must be set directly, not just in Metadata)", received.ChatID)
	}
}

func TestWeComChannel_HandleWebhook_SignatureRequired(t *testing.T) {
	ch, _ := New(Config{AgentID: "a1", Token: "mytoken"}, nil)

	innerXML := `<xml><FromUserName>user1</FromUserName><MsgType>text</MsgType><Content>hi</Content><MsgId>1002</MsgId></xml>`
	body := buildPostBody("corp1", innerXML)

	// Bad signature — the Encrypt value is innerXML, signature of that would be different from "badsig"
	req := httptest.NewRequest("POST", "/webhook?msg_signature=badsig&timestamp=ts&nonce=n", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	if rr.Code != 401 {
		t.Errorf("status=%d want 401 (wrong signature)", rr.Code)
	}
}

func TestWeComChannel_HandleWebhook_Dedup(t *testing.T) {
	count := 0
	ch, _ := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		count++
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	innerXML := `<xml><FromUserName>user1</FromUserName><MsgType>text</MsgType><Content>hello</Content><MsgId>9999</MsgId></xml>`
	for i := 0; i < 3; i++ {
		body := buildPostBody("corp1", innerXML)
		req := httptest.NewRequest("POST", "/webhook?msg_signature=sig&timestamp=ts&nonce=n", bytes.NewReader(body))
		ch.HandleWebhook(httptest.NewRecorder(), req)
	}
	flushDebouncer(ch.debouncer)

	if count != 1 {
		t.Errorf("count=%d want 1 (dedup by MsgId — WeChat Work retries 3x on non-success)", count)
	}
}

func TestWeComChannel_HandleWebhook_MediaTypes(t *testing.T) {
	cases := []struct {
		msgType string
		want    string
	}{
		{"image", "[Image attachment]"},
		{"voice", "[Voice attachment]"},
		{"video", "[Video attachment]"},
		{"file", "[File attachment]"},
	}
	for i, c := range cases {
		var received channels.InboundMessage
		ch, _ := New(Config{AgentID: "a1"}, func(_ context.Context, msg channels.InboundMessage) {
			received = msg
		})
		ch.Start(context.Background())

		innerXML := fmt.Sprintf(
			`<xml><FromUserName>user1</FromUserName><MsgType>%s</MsgType><MsgId>%d</MsgId></xml>`,
			c.msgType, 2000+i,
		)
		body := buildPostBody("corp1", innerXML)
		req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
		ch.HandleWebhook(httptest.NewRecorder(), req)
		flushDebouncer(ch.debouncer)
		ch.Stop(context.Background())

		if received.Content != c.want {
			t.Errorf("msgType=%q content=%q want %q", c.msgType, received.Content, c.want)
		}
	}
}

func TestWeComChannel_HandleWebhook_Event_Skipped(t *testing.T) {
	called := false
	ch, _ := New(Config{AgentID: "a1"}, func(_ context.Context, _ channels.InboundMessage) {
		called = true
	})
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	innerXML := `<xml><FromUserName>user1</FromUserName><MsgType>event</MsgType><Event>subscribe</Event><MsgId>3001</MsgId></xml>`
	body := buildPostBody("corp1", innerXML)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	ch.HandleWebhook(httptest.NewRecorder(), req)
	flushDebouncer(ch.debouncer)

	if called {
		t.Error("event messages should not invoke handler (events are logged, not forwarded)")
	}
}

func TestWeComChannel_HandleWebhook_SuccessResponse(t *testing.T) {
	ch, _ := New(Config{AgentID: "a1"}, nil)
	ch.Start(context.Background())
	defer ch.Stop(context.Background())

	innerXML := `<xml><FromUserName>user1</FromUserName><MsgType>text</MsgType><Content>hi</Content><MsgId>4001</MsgId></xml>`
	body := buildPostBody("corp1", innerXML)
	req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	ch.HandleWebhook(rr, req)

	// WeChat Work requires "success" in body — an empty 200 triggers 3 retries
	if rr.Body.String() != "success" {
		t.Errorf("body=%q want 'success'", rr.Body.String())
	}
}

func TestWeComChannel_SplitMessage(t *testing.T) {
	if maxMessageLen != 4096 {
		t.Errorf("maxMessageLen=%d want 4096", maxMessageLen)
	}
	chunks := splitMessage(strings.Repeat("x", 4097), 4096)
	if len(chunks) != 2 {
		t.Errorf("chunks=%d want 2", len(chunks))
	}
}

func TestWeComChannel_Decrypt(t *testing.T) {
	// 32 zero bytes → 44 base64 chars with padding → 43 without trailing "="
	encodingKey := strings.TrimRight(base64.StdEncoding.EncodeToString(make([]byte, 32)), "=")

	ch, err := New(Config{EncodingKey: encodingKey}, nil)
	if err != nil {
		t.Fatal(err)
	}

	message := "<xml><MsgType>text</MsgType><Content>test decrypt</Content></xml>"
	encrypted := encryptPayload(encodingKey, "corpid", message)
	decrypted := ch.decrypt(encrypted)
	if decrypted != message {
		t.Errorf("decrypt=%q want %q", decrypted, message)
	}
}

func TestWeComChannel_Decrypt_BadInput(t *testing.T) {
	encodingKey := strings.TrimRight(base64.StdEncoding.EncodeToString(make([]byte, 32)), "=")
	ch, _ := New(Config{EncodingKey: encodingKey}, nil)

	// Invalid base64 — should return empty string, not panic
	result := ch.decrypt("not-valid-base64!!!")
	if result != "" {
		t.Errorf("bad input should return empty, got %q", result)
	}
}

func TestWeComChannel_TokenMutex_Separate(t *testing.T) {
	// tokenMu and mu must be separate; verify no deadlock
	ch, _ := New(Config{AgentID: "a1"}, nil)
	ctx := context.Background()
	ch.Start(ctx)
	for i := 0; i < 10; i++ {
		go ch.IsRunning()
		go func() { ch.tokenMu.Lock(); ch.tokenMu.Unlock() }()
	}
	time.Sleep(10 * time.Millisecond)
	ch.Stop(ctx)
}
