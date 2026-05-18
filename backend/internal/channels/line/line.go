// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package line

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
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

// LINE Messaging API channel — 200M+ users across Japan, Taiwan, Thailand, Indonesia.
// Webhook-based: LINE pushes events to your server URL.
// Reference: https://developers.line.biz/en/docs/messaging-api/

const (
	lineAPIBase   = "https://api.line.me/v2/bot"
	lineDataAPI   = "https://api-data.line.me/v2/bot"
	maxMessageLen = 5000
	maxFlexAltLen = 400
	maxQuickReply = 13
)

type Config struct {
	AgentID       string `json:"agent_id"`
	ChannelSecret string `json:"channel_secret"` // for webhook signature verification
	ChannelToken  string `json:"channel_token"`  // long-lived channel access token
	ChannelUserID string `json:"channel_user_id"` // bot's own User ID (Uxxxxxxx) for destination check
	WebhookPath   string `json:"webhook_path"`    // e.g. /webhooks/line
}

type LINEChannel struct {
	cfg       Config
	handler   channels.InboundHandler
	running   bool
	mu        sync.Mutex
	client    *http.Client
	debouncer *channels.Debouncer
	nameCache sync.Map // userID → display name string
}

func New(cfg Config, handler channels.InboundHandler) *LINEChannel {
	return &LINEChannel{cfg: cfg, handler: handler, client: &http.Client{Timeout: 30 * time.Second}}
}

func (l *LINEChannel) Name() string    { return "line" }
func (l *LINEChannel) Type() string    { return "line" }
func (l *LINEChannel) AgentID() string { return l.cfg.AgentID }
func (l *LINEChannel) IsRunning() bool { l.mu.Lock(); defer l.mu.Unlock(); return l.running }

func (l *LINEChannel) Start(ctx context.Context) error {
	l.debouncer = channels.NewDebouncer(600*time.Millisecond, func(msg channels.InboundMessage) {
		if l.handler != nil {
			l.handler(ctx, msg)
		}
	})
	l.mu.Lock()
	l.running = true
	l.mu.Unlock()
	slog.Info("line.started", "agent", l.cfg.AgentID)
	return nil
}

func (l *LINEChannel) Stop(_ context.Context) error {
	if l.debouncer != nil {
		l.debouncer.FlushAll()
	}
	l.mu.Lock()
	l.running = false
	l.mu.Unlock()
	return nil
}

// --- Webhook Handler ---

func (l *LINEChannel) HandleWebhook(rw http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))

	if !l.verifySignature(r.Header.Get("X-Line-Signature"), body) {
		http.Error(rw, "invalid signature", http.StatusUnauthorized)
		return
	}

	var payload webhookPayload
	json.Unmarshal(body, &payload)

	// destination check — prevents cross-channel delivery bugs on shared webhook endpoints
	if l.cfg.ChannelUserID != "" && payload.Destination != "" && payload.Destination != l.cfg.ChannelUserID {
		slog.Warn("line.webhook.destination_mismatch", "got", payload.Destination, "want", l.cfg.ChannelUserID)
		rw.WriteHeader(200)
		return
	}

	for _, event := range payload.Events {
		switch event.Type {
		case "message":
			l.handleMessage(r.Context(), &event)
		case "follow":
			slog.Info("line.follow", "user", event.Source.UserID)
		case "unfollow":
			slog.Info("line.unfollow", "user", event.Source.UserID)
		case "join":
			slog.Info("line.join_group", "group", event.Source.GroupID)
		case "postback":
			l.handlePostback(r.Context(), &event)
		}
	}
	rw.WriteHeader(200)
}

func (l *LINEChannel) verifySignature(signature string, body []byte) bool {
	if l.cfg.ChannelSecret == "" {
		return true
	}
	mac := hmac.New(sha256.New, []byte(l.cfg.ChannelSecret))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

func (l *LINEChannel) handleMessage(ctx context.Context, event *webhookEvent) {
	// Determine canonical chat ID and user ID based on source type.
	// In groups, userId may be null — never fetch profile with empty ID.
	chatID := event.Source.UserID
	userID := event.Source.UserID
	meta := map[string]string{
		"reply_token": event.ReplyToken,
		"source_type": event.Source.Type,
	}

	switch event.Source.Type {
	case "group":
		chatID = event.Source.GroupID
		meta["group_id"] = event.Source.GroupID
		meta["user_id"] = event.Source.UserID
	case "room":
		chatID = event.Source.RoomID
		meta["room_id"] = event.Source.RoomID
		meta["user_id"] = event.Source.UserID
	}

	var text string
	switch event.Message.Type {
	case "text":
		text = event.Message.Text
	case "image":
		// Do NOT download content synchronously — blocks webhook response and triggers LINE retries.
		meta["message_id"] = event.Message.ID
		meta["media_type"] = "image"
		text = "[Image attachment]"
	case "video":
		meta["message_id"] = event.Message.ID
		meta["media_type"] = "video"
		text = "[Video attachment]"
	case "audio":
		meta["message_id"] = event.Message.ID
		meta["media_type"] = "audio"
		text = "[Audio attachment]"
	case "file":
		text = fmt.Sprintf("[File: %s (%d bytes)]", event.Message.FileName, event.Message.FileSize)
	case "location":
		text = fmt.Sprintf("[Location: %s (%.6f, %.6f)]", event.Message.Title, event.Message.Latitude, event.Message.Longitude)
	case "sticker":
		text = fmt.Sprintf("[Sticker: package %s, sticker %s]", event.Message.PackageID, event.Message.StickerID)
	default:
		text = fmt.Sprintf("[%s message]", event.Message.Type)
	}

	if text == "" {
		return
	}

	// Cached name lookup — never block the webhook handler on an HTTP profile fetch.
	senderName := l.cachedName(userID)

	slog.Info("line.inbound", "from", userID, "type", event.Message.Type)
	l.debouncer.Push(channels.InboundMessage{
		ChannelName: "line",
		ChannelType: "line",
		AgentID:     l.cfg.AgentID,
		SenderID:    userID,
		SenderName:  senderName,
		ChatID:      chatID,
		Content:     text,
		Metadata:    meta,
	})
}

func (l *LINEChannel) handlePostback(ctx context.Context, event *webhookEvent) {
	if event.Postback.Data == "" {
		return
	}
	chatID := event.Source.UserID
	if event.Source.GroupID != "" {
		chatID = event.Source.GroupID
	} else if event.Source.RoomID != "" {
		chatID = event.Source.RoomID
	}
	if l.handler != nil {
		l.handler(ctx, channels.InboundMessage{
			ChannelName: "line",
			ChannelType: "line",
			AgentID:     l.cfg.AgentID,
			SenderID:    event.Source.UserID,
			ChatID:      chatID,
			Content:     event.Postback.Data,
			Metadata:    map[string]string{"reply_token": event.ReplyToken, "postback": "true"},
		})
	}
}

// cachedName returns the user's display name from cache.
// If not cached, triggers a background fetch and returns empty string immediately.
// Never called with an empty userID (e.g., in groups without profile permission).
func (l *LINEChannel) cachedName(userID string) string {
	if userID == "" {
		return ""
	}
	if v, ok := l.nameCache.Load(userID); ok {
		return v.(string)
	}
	go func() {
		name := l.fetchDisplayName(userID)
		if name != "" {
			l.nameCache.Store(userID, name)
		}
	}()
	return ""
}

func (l *LINEChannel) fetchDisplayName(userID string) string {
	req, err := http.NewRequest("GET", lineAPIBase+"/profile/"+userID, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+l.cfg.ChannelToken)
	resp, err := l.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ""
	}
	var profile struct {
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return ""
	}
	return profile.DisplayName
}

// --- Send ---

func (l *LINEChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	// Use reply token if available (free, instant), otherwise push (costs quota).
	replyToken := ""
	to := msg.RecipientID
	if to == "" {
		to = msg.ChatID
	}
	if msg.Metadata != nil {
		if rt, ok := msg.Metadata["reply_token"]; ok {
			replyToken = rt
		}
	}

	for _, chunk := range splitMessage(content, maxMessageLen) {
		lineMsg := map[string]string{"type": "text", "text": chunk}
		if replyToken != "" {
			if err := l.replyMessage(replyToken, lineMsg); err != nil {
				// Reply token expired or already used — fall back to push.
				if to == "" {
					return fmt.Errorf("line: reply token failed and no push target: %w", err)
				}
				if err2 := l.pushMessage(to, lineMsg); err2 != nil {
					return err2
				}
			}
			replyToken = "" // one-time token; subsequent chunks go via push
		} else {
			if to == "" {
				return fmt.Errorf("line: no recipient ID for push message")
			}
			if err := l.pushMessage(to, lineMsg); err != nil {
				return err
			}
		}
	}
	return nil
}

// SendFlexMessage sends a rich Flex Message (LINE's equivalent of Adaptive Cards).
func (l *LINEChannel) SendFlexMessage(to, altText string, contents map[string]any) error {
	if len(altText) > maxFlexAltLen {
		altText = altText[:maxFlexAltLen]
	}
	msg := map[string]any{"type": "flex", "altText": altText, "contents": contents}
	return l.pushMessage(to, msg)
}

// SendQuickReply sends a message with quick reply buttons (max 13 per spec).
func (l *LINEChannel) SendQuickReply(to, text string, items []QuickReplyItem) error {
	if len(items) > maxQuickReply {
		items = items[:maxQuickReply]
	}
	quickReply := make([]map[string]any, len(items))
	for i, item := range items {
		label := item.Label
		if len(label) > 20 {
			label = label[:20]
		}
		quickReply[i] = map[string]any{
			"type": "action",
			"action": map[string]string{
				"type":  "message",
				"label": label,
				"text":  item.Text,
			},
		}
	}
	msg := map[string]any{
		"type":       "text",
		"text":       text,
		"quickReply": map[string]any{"items": quickReply},
	}
	return l.pushMessage(to, msg)
}

// SendImage sends an image message.
func (l *LINEChannel) SendImage(to, originalURL, previewURL string) error {
	if previewURL == "" {
		previewURL = originalURL
	}
	msg := map[string]string{
		"type":               "image",
		"originalContentUrl": originalURL,
		"previewImageUrl":    previewURL,
	}
	return l.pushMessage(to, msg)
}

type QuickReplyItem struct {
	Label string
	Text  string
}

// --- LINE API Calls ---

func (l *LINEChannel) replyMessage(replyToken string, message any) error {
	payload, _ := json.Marshal(map[string]any{"replyToken": replyToken, "messages": []any{message}})
	return l.apiCall("POST", lineAPIBase+"/message/reply", payload)
}

func (l *LINEChannel) pushMessage(to string, message any) error {
	payload, _ := json.Marshal(map[string]any{"to": to, "messages": []any{message}})
	return l.apiCall("POST", lineAPIBase+"/message/push", payload)
}

// DownloadContent fetches media binary for a given message ID.
// Callers should invoke this outside the webhook handler (e.g., in the agent's tool call),
// since the temporary URL from LINE expires shortly after the webhook event.
func (l *LINEChannel) DownloadContent(messageID string) ([]byte, string, error) {
	req, err := http.NewRequest("GET", lineDataAPI+"/message/"+messageID+"/content", nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+l.cfg.ChannelToken)
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("line content API %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	return data, resp.Header.Get("Content-Type"), err
}

func (l *LINEChannel) apiCall(method, url string, body []byte) error {
	req, _ := http.NewRequest(method, url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+l.cfg.ChannelToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("line api %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// --- Types ---

type webhookPayload struct {
	Destination string         `json:"destination"` // bot's own User ID — validate against ChannelUserID
	Events      []webhookEvent `json:"events"`
}

type webhookEvent struct {
	Type       string `json:"type"`
	ReplyToken string `json:"replyToken"`
	Source     struct {
		Type    string `json:"type"` // user, group, room
		UserID  string `json:"userId"`
		GroupID string `json:"groupId"`
		RoomID  string `json:"roomId"`
	} `json:"source"`
	Message struct {
		ID        string  `json:"id"`
		Type      string  `json:"type"`
		Text      string  `json:"text"`
		Title     string  `json:"title"`
		FileName  string  `json:"fileName"`
		FileSize  int     `json:"fileSize"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		PackageID string  `json:"packageId"`
		StickerID string  `json:"stickerId"`
	} `json:"message"`
	Postback struct {
		Data string `json:"data"`
	} `json:"postback"`
}

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
