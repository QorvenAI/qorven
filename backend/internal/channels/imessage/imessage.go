// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package imessage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/channels"
)

// iMessage channel via BlueBubbles bridge (requires always-on Mac).
// BlueBubbles API: https://docs.bluebubbles.app/server/developer-guides/rest-api-and-webhooks
//
// Auth: password is a URL query param on every call — not a header.
// Webhook: no HMAC; verify the optional `secret` field in the POST body.
// tempGuid: required on every send — BlueBubbles silently drops duplicate tempGuids (v1.5.0+).
// bb-url-change: fired when tunnel URL rotates (ngrok free tier) — must update ServerURL.

type Config struct {
	AgentID       string `json:"agent_id"`
	ServerURL     string `json:"server_url"`     // Cloudflare/ngrok/Tailscale tunnel URL to BB server
	Password      string `json:"password"`        // BB server password — appended as ?password= on every call
	WebhookSecret string `json:"webhook_secret"`  // optional: matched against `secret` field in webhook body
	UseWebhook    bool   `json:"use_webhook"`     // true=webhook mode (recommended); false=HTTP polling fallback
}

type IMessageChannel struct {
	cfg       Config
	cfgMu     sync.RWMutex // protects cfg.ServerURL — updated live by bb-url-change events
	handler   channels.InboundHandler
	running   bool
	cancel    context.CancelFunc
	mu        sync.Mutex
	client    *http.Client
	debouncer *channels.Debouncer
	lastCheck time.Time
}

func New(cfg Config, handler channels.InboundHandler) *IMessageChannel {
	return &IMessageChannel{
		cfg:     cfg,
		handler: handler,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (im *IMessageChannel) Name() string    { return "imessage" }
func (im *IMessageChannel) Type() string    { return "imessage" }
func (im *IMessageChannel) AgentID() string { return im.cfg.AgentID }
func (im *IMessageChannel) IsRunning() bool { im.mu.Lock(); defer im.mu.Unlock(); return im.running }

func (im *IMessageChannel) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	im.cancel = cancel
	im.lastCheck = time.Now()
	im.debouncer = channels.NewDebouncer(600*time.Millisecond, func(msg channels.InboundMessage) {
		if im.handler != nil {
			im.handler(ctx, msg)
		}
	})
	im.mu.Lock()
	im.running = true
	im.mu.Unlock()

	if !im.cfg.UseWebhook {
		go im.pollLoop(ctx)
	}
	slog.Info("imessage.started", "server", im.cfg.ServerURL, "webhook", im.cfg.UseWebhook)
	return nil
}

func (im *IMessageChannel) Stop(_ context.Context) error {
	if im.cancel != nil {
		im.cancel()
	}
	if im.debouncer != nil {
		im.debouncer.FlushAll()
	}
	im.mu.Lock()
	im.running = false
	im.mu.Unlock()
	return nil
}

// serverURL returns the current server URL (may be updated live by bb-url-change).
func (im *IMessageChannel) serverURL() string {
	im.cfgMu.RLock()
	defer im.cfgMu.RUnlock()
	return im.cfg.ServerURL
}

// --- Webhook Handler ---

func (im *IMessageChannel) HandleWebhook(rw http.ResponseWriter, r *http.Request) {
	var event bbEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		rw.WriteHeader(http.StatusOK) // BB requires 200; don't expose parse errors
		return
	}

	// Soft secret check — BB has no HMAC; this is the only verification layer
	if im.cfg.WebhookSecret != "" && event.Secret != im.cfg.WebhookSecret {
		slog.Warn("imessage.webhook.secret_mismatch")
		rw.WriteHeader(http.StatusOK) // ack anyway to prevent BB retry storms
		return
	}

	rw.WriteHeader(http.StatusOK)

	switch event.Type {
	case "new-message":
		im.processEventData(event.Data)
	case "bb-url-change":
		// Tunnel URL rotated (common with ngrok free tier) — update ServerURL immediately
		// so all subsequent outbound sends use the new address
		if event.Data.NewURL != "" {
			im.cfgMu.Lock()
			im.cfg.ServerURL = event.Data.NewURL
			im.cfgMu.Unlock()
			slog.Info("imessage.url_changed", "new_url", event.Data.NewURL)
		}
	// updated-message, typing-indicator, group-name-change, participant-*, chat-read-status-changed, bb-server-update → skip
	}
}

func (im *IMessageChannel) processEventData(data bbEventData) {
	if data.IsFromMe {
		return // bot loop prevention — BB fires new-message for outbound too
	}

	sender := data.Handle.Address
	senderName := data.Handle.Address
	if data.Handle.DisplayName != "" {
		senderName = data.Handle.DisplayName
	} else if data.Handle.FirstName != "" {
		senderName = data.Handle.FirstName
	}

	// chatGuid comes from chats[0].guid — required for all replies
	chatGUID := ""
	if len(data.Chats) > 0 {
		chatGUID = data.Chats[0].GUID
	}
	if chatGUID == "" {
		chatGUID = sender
	}

	text, mediaFiles := buildContent(data.Text, data.Attachments)
	if text == "" && len(mediaFiles) == 0 {
		return
	}

	peerKind := "direct"
	if strings.Contains(chatGUID, ";+;") {
		peerKind = "group"
	}

	slog.Info("imessage.inbound", "from", sender, "chat", chatGUID, "type", peerKind)

	im.debouncer.Push(channels.InboundMessage{
		ChannelName: "imessage",
		ChannelType: "imessage",
		AgentID:     im.cfg.AgentID,
		SenderID:    sender,
		SenderName:  senderName,
		ChatID:      chatGUID,
		Content:     strings.TrimSpace(text),
		PeerKind:    peerKind,
		Metadata: map[string]string{
			"chat_guid":  chatGUID,
			"message_id": data.GUID,
		},
		Media: mediaFiles,
	})
}

// --- Poll Loop (fallback when webhook not configured) ---

func (im *IMessageChannel) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			im.fetchNewMessages()
		case <-ctx.Done():
			return
		}
	}
}

func (im *IMessageChannel) fetchNewMessages() {
	since := im.lastCheck.UnixMilli()
	im.lastCheck = time.Now() // advance before request — prevents re-processing on slow responses

	url := fmt.Sprintf("%s/api/v1/message?after=%d&limit=50&sort=ASC&password=%s",
		im.serverURL(), since, im.cfg.Password)

	resp, err := im.client.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var result struct {
		Data []bbMessage `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	for _, msg := range result.Data {
		if msg.IsFromMe {
			continue
		}

		chatGUID := ""
		if len(msg.Chats) > 0 {
			chatGUID = msg.Chats[0].GUID
		}
		if chatGUID == "" {
			chatGUID = msg.Handle.Address
		}

		text, _ := buildContent(msg.Text, msg.Attachments)
		if text == "" {
			continue
		}

		senderName := msg.Handle.Address
		if msg.Handle.FirstName != "" {
			senderName = msg.Handle.FirstName
		}

		peerKind := "direct"
		if strings.Contains(chatGUID, ";+;") {
			peerKind = "group"
		}

		slog.Info("imessage.inbound.poll", "from", msg.Handle.Address, "chat", chatGUID)
		im.debouncer.Push(channels.InboundMessage{
			ChannelName: "imessage",
			ChannelType: "imessage",
			AgentID:     im.cfg.AgentID,
			SenderID:    msg.Handle.Address,
			SenderName:  senderName,
			ChatID:      chatGUID,
			Content:     strings.TrimSpace(text),
			PeerKind:    peerKind,
			Metadata: map[string]string{
				"chat_guid":  chatGUID,
				"message_id": fmt.Sprintf("%d", msg.ROWID),
			},
		})
	}
}

// buildContent assembles display text and media list from raw text + attachments.
func buildContent(text string, attachments []bbAttachment) (string, []channels.MediaFile) {
	var mediaFiles []channels.MediaFile
	for _, att := range attachments {
		label := "[Attachment]"
		switch {
		case strings.HasPrefix(att.MimeType, "image/"):
			label = "[Image attachment]"
		case strings.HasPrefix(att.MimeType, "audio/"):
			label = "[Audio attachment]"
		case strings.HasPrefix(att.MimeType, "video/"):
			label = "[Video attachment]"
		case att.TransferName != "":
			label = fmt.Sprintf("[File: %s]", att.TransferName)
		}
		if text != "" {
			text += " " + label
		} else {
			text = label
		}
		if att.GUID != "" {
			mediaFiles = append(mediaFiles, channels.MediaFile{MimeType: att.MimeType})
		}
	}
	return text, mediaFiles
}

// --- Send ---

func (im *IMessageChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	chatGUID := msg.RecipientID
	if msg.ChatID != "" {
		chatGUID = msg.ChatID
	}
	if msg.Metadata != nil {
		if cg := msg.Metadata["chat_guid"]; cg != "" {
			chatGUID = cg
		}
	}

	// tempGuid is required — BB silently drops messages with a duplicate tempGuid (v1.5.0+)
	tempGUID := "temp-" + uuid.New().String()

	payload, _ := json.Marshal(map[string]any{
		"chatGuid": chatGUID,
		"tempGuid": tempGUID,
		"message":  content,
		"method":   "private-api",
	})
	url := fmt.Sprintf("%s/api/v1/message/text?password=%s", im.serverURL(), im.cfg.Password)
	resp, err := im.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("imessage send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("imessage send %d: %s", resp.StatusCode, b)
	}
	return nil
}

// SendTyping sends a typing indicator. Endpoint: POST /api/v1/chat/typing
func (im *IMessageChannel) SendTyping(chatGUID string) {
	payload, _ := json.Marshal(map[string]any{"chatGuid": chatGUID})
	url := fmt.Sprintf("%s/api/v1/chat/typing?password=%s", im.serverURL(), im.cfg.Password)
	resp, err := im.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return
	}
	resp.Body.Close()
}

// MarkRead marks a chat as read. Endpoint: POST /api/v1/chat/read
func (im *IMessageChannel) MarkRead(chatGUID string) {
	payload, _ := json.Marshal(map[string]any{"chatGuid": chatGUID})
	url := fmt.Sprintf("%s/api/v1/chat/read?password=%s", im.serverURL(), im.cfg.Password)
	resp, err := im.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return
	}
	resp.Body.Close()
}

// SendReaction sends a tapback reaction (requires Private API mode on the Mac).
// reaction: love, like, dislike, laugh, emphasize, question
func (im *IMessageChannel) SendReaction(chatGUID, messageGUID, reaction string) error {
	payload, _ := json.Marshal(map[string]any{
		"chatGuid":            chatGUID,
		"selectedMessageGuid": messageGUID,
		"reaction":            reaction,
	})
	url := fmt.Sprintf("%s/api/v1/message/react?password=%s", im.serverURL(), im.cfg.Password)
	resp, err := im.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("imessage react: %w", err)
	}
	resp.Body.Close()
	return nil
}

// RegisterWebhook registers a webhook URL with the BlueBubbles server via REST API.
// Must be called once after BB server setup — not wired to lifecycle automatically.
func (im *IMessageChannel) RegisterWebhook(webhookURL string, events []string) error {
	if len(events) == 0 {
		events = []string{
			"new-message", "updated-message", "bb-url-change",
			"typing-indicator", "chat-read-status-changed",
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"url":    webhookURL,
		"events": events,
	})
	url := fmt.Sprintf("%s/api/v1/webhook?password=%s", im.serverURL(), im.cfg.Password)
	resp, err := im.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("imessage register webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("imessage register webhook %d: %s", resp.StatusCode, b)
	}
	return nil
}

// --- Types ---

type bbEvent struct {
	Type   string      `json:"type"`
	Data   bbEventData `json:"data"`
	Secret string      `json:"secret"`
}

type bbEventData struct {
	// new-message fields
	GUID           string         `json:"guid"`
	Text           string         `json:"text"`
	IsFromMe       bool           `json:"isFromMe"`
	DateCreated    int64          `json:"dateCreated"`
	HasAttachments bool           `json:"hasAttachments"`
	Handle         bbHandle       `json:"handle"`
	Chats          []bbChat       `json:"chats"`
	Attachments    []bbAttachment `json:"attachments"`
	// bb-url-change field
	NewURL string `json:"newUrl"`
}

type bbHandle struct {
	Address     string `json:"address"`
	FirstName   string `json:"firstName"`
	DisplayName string `json:"displayName"`
	Country     string `json:"country"`
}

type bbChat struct {
	GUID        string  `json:"guid"`
	DisplayName *string `json:"displayName"`
	IsIMessage  bool    `json:"isIMessage"`
}

type bbAttachment struct {
	GUID         string `json:"guid"`
	TransferName string `json:"transferName"`
	MimeType     string `json:"mimeType"`
}

// bbMessage is used by the polling path only.
type bbMessage struct {
	ROWID       int            `json:"ROWID"`
	Text        string         `json:"text"`
	IsFromMe    bool           `json:"isFromMe"`
	Handle      bbHandle       `json:"handle"`
	Chats       []bbChat       `json:"chats"`
	Attachments []bbAttachment `json:"attachments"`
}
