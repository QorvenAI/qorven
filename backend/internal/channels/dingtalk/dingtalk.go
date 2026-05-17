// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package dingtalk

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

// DingTalk (钉钉) channel — 600M+ Chinese enterprise users.
// Supports webhook bot (outgoing only) and Stream mode (bidirectional WebSocket).
// Reference: https://open.dingtalk.com/document/robots/stream

const (
	// api.dingtalk.com is the current domain; oapi.dingtalk.com is deprecated.
	dingtalkAPI      = "https://api.dingtalk.com"
	maxTextLen       = 3800 // hard split before DingTalk auto-truncates text messages
	maxMarkdownLen   = 5000
	webhookReplayTTL = 60 * time.Minute // reject requests with timestamps older than this
)

type Config struct {
	AgentID       string `json:"agent_id"`
	AppKey        string `json:"app_key"`        // AppKey / ClientID from DingTalk console
	AppSecret     string `json:"app_secret"`     // AppSecret / ClientSecret
	RobotCode     string `json:"robot_code"`     // same as AppKey for robot API calls
	BotUserID     string `json:"bot_user_id"`    // chatbotUserId — filter to prevent bot loops
	WebhookURL    string `json:"webhook_url"`    // outgoing custom robot webhook URL
	WebhookSecret string `json:"webhook_secret"` // for signing outgoing custom webhooks
}

type DingTalkChannel struct {
	cfg       Config
	handler   channels.InboundHandler
	running   bool
	mu        sync.Mutex
	tokenMu   sync.Mutex // separate mutex; token fetch must not block IsRunning()
	client    *http.Client
	debouncer *channels.Debouncer
	token     string
	tokenExp  time.Time
}

func New(cfg Config, handler channels.InboundHandler) *DingTalkChannel {
	return &DingTalkChannel{
		cfg:     cfg,
		handler: handler,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (d *DingTalkChannel) Name() string    { return "dingtalk" }
func (d *DingTalkChannel) Type() string    { return "dingtalk" }
func (d *DingTalkChannel) AgentID() string { return d.cfg.AgentID }
func (d *DingTalkChannel) IsRunning() bool { d.mu.Lock(); defer d.mu.Unlock(); return d.running }

func (d *DingTalkChannel) Start(ctx context.Context) error {
	d.debouncer = channels.NewDebouncer(600*time.Millisecond, func(msg channels.InboundMessage) {
		if d.handler != nil {
			d.handler(ctx, msg)
		}
	})
	d.mu.Lock()
	d.running = true
	d.mu.Unlock()
	slog.Info("dingtalk.started", "agent", d.cfg.AgentID)
	return nil
}

func (d *DingTalkChannel) Stop(_ context.Context) error {
	if d.debouncer != nil {
		d.debouncer.FlushAll()
	}
	d.mu.Lock()
	d.running = false
	d.mu.Unlock()
	return nil
}

// --- Access Token ---

func (d *DingTalkChannel) getToken() (string, error) {
	d.tokenMu.Lock()
	defer d.tokenMu.Unlock()
	if d.token != "" && time.Now().Before(d.tokenExp) {
		return d.token, nil
	}

	payload, _ := json.Marshal(map[string]string{"appKey": d.cfg.AppKey, "appSecret": d.cfg.AppSecret})
	resp, err := d.client.Post(dingtalkAPI+"/v1.0/oauth2/accessToken", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("dingtalk token: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int    `json:"expireIn"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.AccessToken == "" {
		return "", fmt.Errorf("dingtalk token: empty")
	}
	d.token = result.AccessToken
	// Refresh 60 seconds before expiry.
	d.tokenExp = time.Now().Add(time.Duration(result.ExpireIn-60) * time.Second)
	return d.token, nil
}

// --- Webhook Signature (HTTP mode) ---

// verifyWebhookSignature checks the DingTalk HTTP webhook sign header.
// Algorithm: HMAC-SHA256(appSecret, timestamp + "\n" + appSecret), then base64.
// The appSecret appears both as HMAC key and in the message body — DingTalk spec.
func (d *DingTalkChannel) verifyWebhookSignature(r *http.Request) bool {
	if d.cfg.AppSecret == "" {
		return true
	}
	tsStr := r.Header.Get("timestamp")
	sign := r.Header.Get("sign")
	if tsStr == "" || sign == "" {
		return false
	}
	// Reject replays older than 60 minutes.
	tsMs, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return false
	}
	age := time.Since(time.UnixMilli(tsMs))
	if age > webhookReplayTTL || age < -webhookReplayTTL {
		return false
	}
	stringToSign := tsStr + "\n" + d.cfg.AppSecret
	mac := hmac.New(sha256.New, []byte(d.cfg.AppSecret))
	mac.Write([]byte(stringToSign))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sign))
}

// --- Webhook Handler (inbound from DingTalk HTTP mode) ---

func (d *DingTalkChannel) HandleWebhook(rw http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))

	if !d.verifyWebhookSignature(r) {
		slog.Warn("dingtalk.webhook.signature_invalid")
		http.Error(rw, "invalid signature", http.StatusUnauthorized)
		return
	}

	var msg dingtalkMessage
	json.Unmarshal(body, &msg)

	// Skip messages sent by the bot itself to prevent loops.
	if d.cfg.BotUserID != "" && msg.ChatbotUserID == d.cfg.BotUserID {
		rw.WriteHeader(200)
		return
	}

	// In group chats, only process messages where bot is @mentioned.
	if msg.ConversationType == "2" && !msg.IsInAtList {
		rw.WriteHeader(200)
		return
	}

	var text string
	switch msg.MsgType {
	case "text", "":
		text = strings.TrimSpace(msg.Text.Content)
	case "richText":
		// richText has mixed content; extract text parts
		text = extractRichText(msg.RichText)
	case "picture":
		text = "[Image attachment]"
	case "audio":
		text = "[Audio attachment]"
	case "video":
		text = "[Video attachment]"
	case "file":
		text = "[File attachment]"
	default:
		text = fmt.Sprintf("[%s message]", msg.MsgType)
	}

	if text == "" {
		rw.WriteHeader(200)
		return
	}

	slog.Info("dingtalk.inbound", "from", msg.SenderNick, "type", msg.MsgType)

	d.debouncer.Push(channels.InboundMessage{
		ChannelName: "dingtalk",
		ChannelType: "dingtalk",
		AgentID:     d.cfg.AgentID,
		SenderID:    msg.SenderID,
		SenderName:  msg.SenderNick,
		ChatID:      msg.ConversationID,
		Content:     text,
		Metadata: map[string]string{
			"chat_id":           msg.ConversationID,
			"message_id":        msg.MsgID,
			"conversation_type": msg.ConversationType,
			"webhook_url":       msg.SessionWebhook,
			"sender_id":         msg.SenderID,
		},
	})

	rw.WriteHeader(200)
}

// extractRichText extracts plain text parts from a richText message.
func extractRichText(rt []dingtalkRichElem) string {
	var parts []string
	for _, elem := range rt {
		if elem.Type == "text" && elem.Text != "" {
			parts = append(parts, elem.Text)
		}
	}
	return strings.Join(parts, " ")
}

// --- Send ---

func (d *DingTalkChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	// Prefer sessionWebhook from metadata (quick reply path for HTTP/Stream mode).
	if msg.Metadata != nil {
		if webhook, ok := msg.Metadata["webhook_url"]; ok && webhook != "" {
			for _, chunk := range splitMessage(content, maxTextLen) {
				if err := d.sendViaSessionWebhook(webhook, chunk, msg.Metadata["sender_id"]); err != nil {
					return err
				}
			}
			return nil
		}
	}

	// Fall back to configured outgoing webhook (custom robot).
	if d.cfg.WebhookURL != "" {
		for _, chunk := range splitMessage(content, maxTextLen) {
			if err := d.sendViaWebhook(chunk); err != nil {
				return err
			}
		}
		return nil
	}

	return fmt.Errorf("dingtalk: no webhook URL configured")
}

func (d *DingTalkChannel) sendViaSessionWebhook(webhookURL, text, atUserID string) error {
	token, err := d.getToken()
	if err != nil {
		return err
	}

	at := map[string]any{"isAtAll": false}
	if atUserID != "" {
		at["atUserIds"] = []string{atUserID}
	}

	payload, _ := json.Marshal(map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": text},
		"at":      at,
	})
	req, _ := http.NewRequest("POST", webhookURL, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("dingtalk session webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dingtalk session webhook %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (d *DingTalkChannel) sendViaWebhook(text string) error {
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	sign := d.signWebhook(timestamp)

	url := fmt.Sprintf("%s&timestamp=%s&sign=%s", d.cfg.WebhookURL, timestamp, sign)
	payload, _ := json.Marshal(map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": text},
	})
	resp, err := d.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("dingtalk webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dingtalk webhook %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// SendMarkdown sends a markdown-formatted message via sessionWebhook or configured webhook.
func (d *DingTalkChannel) SendMarkdown(webhookURL, title, text string) error {
	payload, _ := json.Marshal(map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]string{"title": title, "text": text},
	})
	req, _ := http.NewRequest("POST", webhookURL, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if token, err := d.getToken(); err == nil {
		req.Header.Set("x-acs-dingtalk-access-token", token)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("dingtalk markdown: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// SendActionCard sends an interactive card with buttons.
func (d *DingTalkChannel) SendActionCard(webhookURL, title, text string, buttons []ActionButton) error {
	btns := make([]map[string]string, len(buttons))
	for i, b := range buttons {
		btns[i] = map[string]string{"title": b.Title, "actionURL": b.URL}
	}
	payload, _ := json.Marshal(map[string]any{
		"msgtype":    "actionCard",
		"actionCard": map[string]any{"title": title, "text": text, "btns": btns, "btnOrientation": "0"},
	})
	req, _ := http.NewRequest("POST", webhookURL, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if token, err := d.getToken(); err == nil {
		req.Header.Set("x-acs-dingtalk-access-token", token)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("dingtalk action card: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

type ActionButton struct {
	Title string
	URL   string
}

func (d *DingTalkChannel) signWebhook(timestamp string) string {
	if d.cfg.WebhookSecret == "" {
		return ""
	}
	stringToSign := timestamp + "\n" + d.cfg.WebhookSecret
	mac := hmac.New(sha256.New, []byte(d.cfg.WebhookSecret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// --- Types ---

type dingtalkMessage struct {
	MsgID            string             `json:"msgId"`
	MsgType          string             `json:"msgtype"` // text, richText, picture, audio, video, file
	SenderID         string             `json:"senderId"`
	SenderNick       string             `json:"senderNick"`
	ChatbotUserID    string             `json:"chatbotUserId"` // bot's own user ID — filter to prevent loops
	ConversationID   string             `json:"conversationId"`
	ConversationType string             `json:"conversationType"` // "1"=DM, "2"=group
	IsInAtList       bool               `json:"isInAtList"`       // true if bot was @mentioned in group
	SessionWebhook   string             `json:"sessionWebhook"`
	Text             struct {
		Content string `json:"content"`
	} `json:"text"`
	RichText []dingtalkRichElem `json:"richText"`
}

type dingtalkRichElem struct {
	Type     string `json:"type"`            // "text" or "picture"
	Text     string `json:"text,omitempty"`
	DownloadCode string `json:"downloadCode,omitempty"` // for picture/file elements
}

// splitMessage splits text at line boundaries before the limit.
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
