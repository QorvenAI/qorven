// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package facebook implements the Facebook Messenger channel for Qorven.
// Uses the Facebook Messenger Platform API (Send API + Webhooks).
// Reference: https://developers.facebook.com/docs/messenger-platform
package facebook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

const (
	graphAPIBase  = "https://graph.facebook.com/v20.0"
	maxMessageLen = 2000 // Messenger hard limit: 2,000 chars per message
)

// Config holds Messenger channel configuration.
type Config struct {
	AgentID         string   `json:"agent_id"`
	PageAccessToken string   `json:"page_access_token"` // long-lived page / system user token
	VerifyToken     string   `json:"verify_token"`      // webhook verification token
	AppSecret       string   `json:"app_secret"`        // HMAC-SHA256 signature secret
	AllowFrom       []string `json:"allow_from"`        // optional PSID allowlist
}

// MessengerChannel implements the Facebook Messenger platform.
type MessengerChannel struct {
	cfg       Config
	handler   channels.InboundHandler
	client    *http.Client
	running   bool
	mu        sync.Mutex
	dedup     sync.Map // MID → time.Time — prevents double-fire on platform retries
	nameCache sync.Map // PSID → display name
}

// New creates a new Messenger channel.
func New(cfg Config, handler channels.InboundHandler) *MessengerChannel {
	return &MessengerChannel{
		cfg:     cfg,
		handler: handler,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (m *MessengerChannel) Name() string    { return fmt.Sprintf("messenger:%s", m.cfg.AgentID) }
func (m *MessengerChannel) Type() string    { return "facebook" }
func (m *MessengerChannel) AgentID() string { return m.cfg.AgentID }

func (m *MessengerChannel) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *MessengerChannel) Start(_ context.Context) error {
	m.mu.Lock()
	m.running = true
	m.mu.Unlock()
	slog.Info("messenger.started", "agent", m.cfg.AgentID)
	return nil
}

func (m *MessengerChannel) Stop(_ context.Context) error {
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
	slog.Info("messenger.stopped", "agent", m.cfg.AgentID)
	return nil
}

// Send sends a message via the Messenger Send API.
// Long messages are split at word/newline boundaries.
func (m *MessengerChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	recipientID := msg.RecipientID
	if recipientID == "" {
		recipientID = msg.ChatID
	}
	if recipientID == "" {
		return fmt.Errorf("messenger: no recipient ID")
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	for len(content) > 0 {
		chunk := content
		if len(chunk) > maxMessageLen {
			cut := maxMessageLen
			if idx := strings.LastIndex(content[:maxMessageLen], "\n"); idx > maxMessageLen/2 {
				cut = idx + 1
			}
			chunk = content[:cut]
			content = content[cut:]
		} else {
			content = ""
		}
		if err := m.sendTextMessage(recipientID, chunk); err != nil {
			return err
		}
	}
	return nil
}

// sendTextMessage sends a plain text message with the required messaging_type field.
// messaging_type "RESPONSE" is required for replies within the 24-hour window.
func (m *MessengerChannel) sendTextMessage(recipientID, text string) error {
	payload := map[string]any{
		"recipient":      map[string]string{"id": recipientID},
		"messaging_type": "RESPONSE",
		"message":        map[string]string{"text": text},
	}
	return m.callSendAPI(payload)
}

// SendQuickReplies sends a message with quick reply buttons (max 13 per spec).
func (m *MessengerChannel) SendQuickReplies(recipientID, text string, replies []string) error {
	if len(replies) > 13 {
		replies = replies[:13]
	}
	qrs := make([]map[string]string, 0, len(replies))
	for _, r := range replies {
		title := r
		if len(title) > 20 {
			title = title[:20]
		}
		qrs = append(qrs, map[string]string{
			"content_type": "text",
			"title":        title,
			"payload":      r,
		})
	}
	payload := map[string]any{
		"recipient":      map[string]string{"id": recipientID},
		"messaging_type": "RESPONSE",
		"message": map[string]any{
			"text":          text,
			"quick_replies": qrs,
		},
	}
	return m.callSendAPI(payload)
}

// callSendAPI POSTs to the Messenger Send API using Authorization header.
func (m *MessengerChannel) callSendAPI(payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("messenger: marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/me/messages", graphAPIBase)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("messenger: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+m.cfg.PageAccessToken)

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("messenger: send API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("messenger: send API %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// HandleWebhook processes incoming Messenger webhook events.
func (m *MessengerChannel) HandleWebhook(rw http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		m.handleVerification(rw, r)
		return
	}
	m.handleEvent(rw, r)
}

func (m *MessengerChannel) handleVerification(rw http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == m.cfg.VerifyToken && isWebhookChallenge(challenge) {
		slog.Info("messenger.webhook.verified")
		rw.WriteHeader(200)
		rw.Write([]byte(challenge))
		return
	}
	slog.Warn("messenger.webhook.verify_failed", "mode", mode)
	http.Error(rw, "forbidden", http.StatusForbidden)
}

// isWebhookChallenge returns true if s is a safe webhook challenge token.
// Allows letters, digits, hyphens, and underscores.
func isWebhookChallenge(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (m *MessengerChannel) handleEvent(rw http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(rw, "read error", 400)
		return
	}

	if m.cfg.AppSecret != "" && !m.VerifySignature(r, body) {
		slog.Warn("messenger.webhook.signature_invalid")
		http.Error(rw, "invalid signature", http.StatusUnauthorized)
		return
	}

	var event webhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		rw.WriteHeader(200)
		return
	}

	if event.Object != "page" {
		rw.WriteHeader(200)
		return
	}

	for _, entry := range event.Entry {
		for _, msg := range entry.Messaging {
			m.processMessagingEvent(r.Context(), msg)
		}
	}

	rw.WriteHeader(200)
	rw.Write([]byte(`{"status":"ok"}`))
}

func (m *MessengerChannel) processMessagingEvent(ctx context.Context, msg messagingEvent) {
	senderID := msg.Sender.ID
	if senderID == "" {
		return
	}

	// Skip echo events (messages the page itself sent) — Messenger has no auto-filter.
	if msg.Message != nil && msg.Message.IsEcho {
		return
	}

	// Dedup by MID — Messenger retries with same message ID on 5xx or timeout.
	if msg.Message != nil && msg.Message.MID != "" {
		if _, already := m.dedup.LoadOrStore(msg.Message.MID, time.Now()); already {
			return
		}
		go func() { time.Sleep(2 * time.Hour); m.dedup.Delete(msg.Message.MID) }()
	}

	// Allowlist check
	if len(m.cfg.AllowFrom) > 0 {
		allowed := false
		for _, id := range m.cfg.AllowFrom {
			if id == senderID {
				allowed = true
				break
			}
		}
		if !allowed {
			slog.Debug("messenger.sender_not_allowed", "sender", senderID)
			return
		}
	}

	var content string
	switch {
	case msg.Message != nil && msg.Message.Text != "":
		content = msg.Message.Text
	case msg.Message != nil && len(msg.Message.Attachments) > 0:
		// Forward attachment type to agent as descriptive text
		parts := make([]string, 0, len(msg.Message.Attachments))
		for _, att := range msg.Message.Attachments {
			switch att.Type {
			case "location":
				parts = append(parts, fmt.Sprintf("[Location: %.6f, %.6f]", att.Payload.Coordinates.Lat, att.Payload.Coordinates.Long))
			default:
				parts = append(parts, fmt.Sprintf("[%s attachment]", att.Type))
			}
		}
		content = strings.Join(parts, "\n")
	case msg.Postback != nil:
		content = msg.Postback.Payload
		if msg.Postback.Title != "" {
			content = fmt.Sprintf("[%s] %s", msg.Postback.Title, msg.Postback.Payload)
		}
	default:
		return
	}

	if content == "" || m.handler == nil {
		return
	}

	// Name lookup — from cache or background fetch (never blocks the webhook handler).
	senderName := m.cachedName(senderID)

	slog.Info("messenger.inbound", "sender", senderID, "content_len", len(content))
	m.handler(ctx, channels.InboundMessage{
		ChannelName: m.Name(),
		ChannelType: "facebook",
		AgentID:     m.cfg.AgentID,
		SenderID:    senderID,
		SenderName:  senderName,
		ChatID:      senderID,
		Content:     content,
		Metadata:    map[string]string{"psid": senderID, "page_id": msg.Recipient.ID},
	})
}

// cachedName returns the sender's display name from cache.
// If not cached, triggers a background fetch and returns empty string for now.
func (m *MessengerChannel) cachedName(psid string) string {
	if v, ok := m.nameCache.Load(psid); ok {
		return v.(string)
	}
	go func() {
		name := m.fetchName(psid)
		if name != "" {
			m.nameCache.Store(psid, name)
		}
	}()
	return ""
}

// fetchName calls the Graph API to get a user's display name.
func (m *MessengerChannel) fetchName(psid string) string {
	url := fmt.Sprintf("%s/%s?fields=name", graphAPIBase, psid)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.PageAccessToken)
	resp, err := m.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ""
	}
	var profile struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return ""
	}
	return profile.Name
}

// VerifySignature verifies the X-Hub-Signature-256 header against the app secret.
func (m *MessengerChannel) VerifySignature(r *http.Request, body []byte) bool {
	if m.cfg.AppSecret == "" {
		return true
	}
	signature := strings.TrimPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=")
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(m.cfg.AppSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

// ─── Webhook payload types ────────────────────────────────────────────────────

type webhookEvent struct {
	Object string      `json:"object"`
	Entry  []pageEntry `json:"entry"`
}

type pageEntry struct {
	ID        string           `json:"id"`
	Time      int64            `json:"time"`
	Messaging []messagingEvent `json:"messaging"`
}

type messagingEvent struct {
	Sender    struct{ ID string `json:"id"` } `json:"sender"`
	Recipient struct{ ID string `json:"id"` } `json:"recipient"`
	Timestamp int64                           `json:"timestamp"`
	Message   *incomingMessage                `json:"message,omitempty"`
	Postback  *postbackEvent                  `json:"postback,omitempty"`
}

type incomingMessage struct {
	MID    string `json:"mid"`
	Text   string `json:"text"`
	IsEcho bool   `json:"is_echo"` // true for messages sent by the page itself — must skip
	Attachments []struct {
		Type    string `json:"type"`
		Payload struct {
			URL         string `json:"url"`
			Coordinates struct {
				Lat  float64 `json:"lat"`
				Long float64 `json:"long"`
			} `json:"coordinates"`
		} `json:"payload"`
	} `json:"attachments,omitempty"`
	QuickReply *struct {
		Payload string `json:"payload"`
	} `json:"quick_reply,omitempty"`
}

type postbackEvent struct {
	Title   string `json:"title"`
	Payload string `json:"payload"`
}
