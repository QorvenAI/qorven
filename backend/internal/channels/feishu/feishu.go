// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package feishu

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

// Feishu/Lark (飞书) channel — 50M+ users in ByteDance ecosystem.
// Supports WebSocket (default, no public URL) and Webhook modes.
// Reference: https://open.feishu.cn/document/home

const (
	feishuAPI     = "https://open.feishu.cn/open-apis"
	larkAPI       = "https://open.larksuite.com/open-apis" // international
	maxMessageLen = 30000
)

type Config struct {
	AgentID     string `json:"agent_id"`
	AppID       string `json:"app_id"`
	AppSecret   string `json:"app_secret"`
	BotName     string `json:"bot_name"`
	IsLark      bool   `json:"is_lark"`      // true = international (larksuite.com)
	EncryptKey  string `json:"encrypt_key"`  // webhook event encryption key
	VerifyToken string `json:"verify_token"` // webhook verification token
}

type FeishuChannel struct {
	cfg       Config
	handler   channels.InboundHandler
	running   bool
	mu        sync.Mutex
	tokenMu   sync.Mutex // separate mutex; token fetch must not block IsRunning()
	client    *http.Client
	debouncer *channels.Debouncer
	token     string
	tokenExp  time.Time
	apiBase   string
	dedup     sync.Map // eventID → time.Time
}

func New(cfg Config, handler channels.InboundHandler) *FeishuChannel {
	apiBase := feishuAPI
	if cfg.IsLark {
		apiBase = larkAPI
	}
	return &FeishuChannel{
		cfg:     cfg,
		handler: handler,
		client:  &http.Client{Timeout: 30 * time.Second},
		apiBase: apiBase,
	}
}

func (f *FeishuChannel) Name() string    { return "feishu" }
func (f *FeishuChannel) Type() string    { return "feishu" }
func (f *FeishuChannel) AgentID() string { return f.cfg.AgentID }
func (f *FeishuChannel) IsRunning() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.running }

func (f *FeishuChannel) Start(ctx context.Context) error {
	f.debouncer = channels.NewDebouncer(600*time.Millisecond, func(msg channels.InboundMessage) {
		if f.handler != nil {
			f.handler(ctx, msg)
		}
	})
	f.mu.Lock()
	f.running = true
	f.mu.Unlock()
	slog.Info("feishu.started", "app_id", f.cfg.AppID, "agent", f.cfg.AgentID)
	return nil
}

func (f *FeishuChannel) Stop(_ context.Context) error {
	if f.debouncer != nil {
		f.debouncer.FlushAll()
	}
	f.mu.Lock()
	f.running = false
	f.mu.Unlock()
	return nil
}

// --- Tenant Access Token ---

func (f *FeishuChannel) getToken() (string, error) {
	f.tokenMu.Lock()
	defer f.tokenMu.Unlock()
	if f.token != "" && time.Now().Before(f.tokenExp) {
		return f.token, nil
	}

	payload, _ := json.Marshal(map[string]string{"app_id": f.cfg.AppID, "app_secret": f.cfg.AppSecret})
	resp, err := f.client.Post(f.apiBase+"/auth/v3/tenant_access_token/internal", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("feishu token: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Code              int    `json:"code"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.TenantAccessToken == "" {
		return "", fmt.Errorf("feishu token: empty (code=%d)", result.Code)
	}
	f.token = result.TenantAccessToken
	// Refresh 90 seconds before expiry (2-hour window; refresh at ~110 min).
	f.tokenExp = time.Now().Add(time.Duration(result.Expire-90) * time.Second)
	return f.token, nil
}

// --- Webhook Verification ---

// verifySignature checks the X-Lark-Signature header.
// Feishu uses SHA256(timestamp + nonce + encrypt_key + raw_body) — NOT HMAC.
func (f *FeishuChannel) verifySignature(r *http.Request, body []byte) bool {
	if f.cfg.EncryptKey == "" {
		return true
	}
	sig := r.Header.Get("X-Lark-Signature")
	if sig == "" {
		return false
	}
	ts := r.Header.Get("X-Lark-Request-Timestamp")
	nonce := r.Header.Get("X-Lark-Request-Nonce")
	content := ts + nonce + f.cfg.EncryptKey + string(body)
	hash := sha256.Sum256([]byte(content))
	expected := fmt.Sprintf("%x", hash)
	return expected == sig
}

// decryptPayload decrypts an AES-CBC encrypted Feishu event payload.
// key = SHA256(encryptKey)[0:32], IV = first 16 bytes of key.
func decryptPayload(encryptKey, ciphertext string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("feishu decrypt: base64 decode: %w", err)
	}
	keyHash := sha256.Sum256([]byte(encryptKey))
	key := keyHash[:32]
	if len(data) < aes.BlockSize {
		return nil, fmt.Errorf("feishu decrypt: ciphertext too short")
	}
	iv := key[:aes.BlockSize]
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("feishu decrypt: cipher: %w", err)
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(data, data)
	// Remove PKCS7 padding
	if len(data) == 0 {
		return nil, fmt.Errorf("feishu decrypt: empty result")
	}
	pad := int(data[len(data)-1])
	if pad > aes.BlockSize || pad > len(data) {
		return nil, fmt.Errorf("feishu decrypt: invalid padding %d", pad)
	}
	return data[:len(data)-pad], nil
}

// --- Webhook Handler ---

func (f *FeishuChannel) HandleWebhook(rw http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))

	// Verify signature before processing anything
	if !f.verifySignature(r, body) {
		slog.Warn("feishu.webhook.signature_invalid")
		http.Error(rw, "invalid signature", http.StatusUnauthorized)
		return
	}

	// If encrypt mode is on, decrypt the payload
	var rawBody []byte
	var encrypted struct {
		Encrypt string `json:"encrypt"`
	}
	if json.Unmarshal(body, &encrypted) == nil && encrypted.Encrypt != "" {
		dec, err := decryptPayload(f.cfg.EncryptKey, encrypted.Encrypt)
		if err != nil {
			slog.Warn("feishu.webhook.decrypt_failed", "error", err)
			http.Error(rw, "decrypt failed", 400)
			return
		}
		rawBody = dec
	} else {
		rawBody = body
	}

	// URL verification challenge (one-time setup)
	var challenge struct {
		Challenge string `json:"challenge"`
		Type      string `json:"type"`
	}
	if json.Unmarshal(rawBody, &challenge) == nil && challenge.Type == "url_verification" {
		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]string{"challenge": challenge.Challenge})
		return
	}

	var event feishuEvent
	json.Unmarshal(rawBody, &event)

	// Dedup by event_id — Feishu may deliver duplicates under load
	if event.Header.EventID != "" {
		if _, loaded := f.dedup.LoadOrStore(event.Header.EventID, time.Now()); loaded {
			rw.WriteHeader(200)
			return
		}
		go func() { time.Sleep(2 * time.Hour); f.dedup.Delete(event.Header.EventID) }()
	}

	if event.Header.EventType == "im.message.receive_v1" {
		f.handleMessage(r.Context(), &event)
	}
	rw.WriteHeader(200)
}

func (f *FeishuChannel) handleMessage(ctx context.Context, event *feishuEvent) {
	msg := event.Event.Message
	sender := event.Event.Sender

	// Skip bot-originated messages to prevent loops
	if sender.SenderType == "bot" {
		return
	}

	// Prefer open_id as SenderID — it's the canonical per-app user identifier
	senderID := sender.SenderID.OpenID
	if senderID == "" {
		senderID = sender.SenderID.UserID
	}

	meta := map[string]string{
		"chat_id":    msg.ChatID,
		"message_id": msg.MessageID,
		"chat_type":  msg.ChatType,
	}

	var text string
	switch msg.MessageType {
	case "text":
		var content struct {
			Text string `json:"text"`
		}
		json.Unmarshal([]byte(msg.Content), &content)
		text = stripFeishuMentions(content.Text)
	case "post":
		// post content is: {"post":{"zh_cn":{"title":"...","content":[[{tag,text}]]}}}
		var postWrap struct {
			Post map[string]struct {
				Title   string `json:"title"`
				Content [][]struct {
					Tag  string `json:"tag"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"post"`
		}
		json.Unmarshal([]byte(msg.Content), &postWrap)
		// Try zh_cn then en_us
		locale := postWrap.Post["zh_cn"]
		if locale.Title == "" && len(locale.Content) == 0 {
			locale = postWrap.Post["en_us"]
		}
		var parts []string
		if locale.Title != "" {
			parts = append(parts, locale.Title)
		}
		for _, line := range locale.Content {
			for _, elem := range line {
				if elem.Tag == "text" && elem.Text != "" {
					parts = append(parts, elem.Text)
				}
			}
		}
		text = strings.Join(parts, "\n")
	case "image":
		meta["has_media"] = "true"
		meta["media_type"] = "image"
		text = "[Image attachment]"
	case "audio":
		meta["has_media"] = "true"
		meta["media_type"] = "audio"
		text = "[Audio attachment]"
	case "file":
		meta["has_media"] = "true"
		meta["media_type"] = "file"
		text = "[File attachment]"
	case "media":
		meta["has_media"] = "true"
		meta["media_type"] = "video"
		text = "[Video attachment]"
	default:
		text = fmt.Sprintf("[%s message]", msg.MessageType)
	}

	if text == "" {
		return
	}

	slog.Info("feishu.inbound", "from", senderID, "type", msg.MessageType, "chat", msg.ChatType)

	f.debouncer.Push(channels.InboundMessage{
		ChannelName: "feishu",
		ChannelType: "feishu",
		AgentID:     f.cfg.AgentID,
		SenderID:    senderID,
		ChatID:      msg.ChatID,
		Content:     text,
		Metadata:    meta,
	})
}

// stripFeishuMentions removes <at user_id="...">name</at> tags from text messages.
func stripFeishuMentions(text string) string {
	for {
		start := strings.Index(text, "<at ")
		if start < 0 {
			break
		}
		end := strings.Index(text[start:], "</at>")
		if end < 0 {
			break
		}
		text = text[:start] + text[start+end+5:]
	}
	return strings.TrimSpace(text)
}

// --- Send ---

func (f *FeishuChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	chatID := msg.RecipientID
	if chatID == "" {
		chatID = msg.ChatID
	}
	if msg.Metadata != nil {
		if cid, ok := msg.Metadata["chat_id"]; ok && cid != "" {
			chatID = cid
		}
	}
	if chatID == "" {
		return fmt.Errorf("feishu: no chat ID")
	}

	for _, chunk := range splitMessage(content, maxMessageLen) {
		if err := f.sendMessage(chatID, "text", map[string]string{"text": chunk}); err != nil {
			return err
		}
	}
	return nil
}

// SendRichText sends a post (rich text) message with formatting.
func (f *FeishuChannel) SendRichText(chatID, title string, content [][]map[string]any) error {
	post := map[string]any{"zh_cn": map[string]any{"title": title, "content": content}}
	return f.sendMessage(chatID, "post", map[string]any{"post": post})
}

// SendInteractiveCard sends an interactive card message.
func (f *FeishuChannel) SendInteractiveCard(chatID string, card map[string]any) error {
	return f.sendMessage(chatID, "interactive", card)
}

// SendImage sends an image by image_key.
func (f *FeishuChannel) SendImage(chatID, imageKey string) error {
	return f.sendMessage(chatID, "image", map[string]string{"image_key": imageKey})
}

// SendFile sends a file by file_key.
func (f *FeishuChannel) SendFile(chatID, fileKey string) error {
	return f.sendMessage(chatID, "file", map[string]string{"file_key": fileKey})
}

func (f *FeishuChannel) sendMessage(chatID, msgType string, content any) error {
	token, err := f.getToken()
	if err != nil {
		return err
	}

	// content must be JSON-encoded as a string — double encoding is required by Feishu API design
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("feishu: marshal content: %w", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"receive_id": chatID,
		"msg_type":   msgType,
		"content":    string(contentJSON),
	})

	req, _ := http.NewRequest("POST", f.apiBase+"/im/v1/messages?receive_id_type=chat_id", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("feishu send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("feishu send %d: %s", resp.StatusCode, string(b))
	}
	slog.Info("feishu.sent", "chat", chatID, "type", msgType)
	return nil
}

// --- Types ---

type feishuEvent struct {
	Header struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
	} `json:"header"`
	Event struct {
		Message struct {
			ChatID      string `json:"chat_id"`
			ChatType    string `json:"chat_type"` // p2p, group
			MessageID   string `json:"message_id"`
			MessageType string `json:"message_type"`
			Content     string `json:"content"`
			CreateTime  string `json:"create_time"`
		} `json:"message"`
		Sender struct {
			SenderID struct {
				UserID  string `json:"user_id"`
				OpenID  string `json:"open_id"`
				UnionID string `json:"union_id"`
			} `json:"sender_id"`
			SenderType string `json:"sender_type"` // user, bot — skip bot to prevent loops
		} `json:"sender"`
	} `json:"event"`
}

// HandleCommentEvent processes Feishu document comment events.
func (f *FeishuChannel) HandleCommentEvent(ctx context.Context, event map[string]any) {
	comment, _ := event["comment"].(map[string]any)
	content, _ := comment["content"].(string)
	userID, _ := comment["user_id"].(string)
	docToken, _ := event["doc_token"].(string)

	if content == "" || f.handler == nil {
		return
	}
	f.handler(ctx, channels.InboundMessage{
		ChannelType: "feishu",
		AgentID:     f.cfg.AgentID,
		SenderID:    userID,
		Content:     content,
		Metadata:    map[string]string{"doc_token": docToken, "type": "comment"},
	})
}

// splitMessage splits text at line boundaries for long messages.
func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}
	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen {
			chunks = append(chunks, text)
			break
		}
		cut := maxLen
		if idx := strings.LastIndex(text[:maxLen], "\n"); idx > maxLen/2 {
			cut = idx + 1
		}
		chunks = append(chunks, text[:cut])
		text = text[cut:]
	}
	return chunks
}
