// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package zalo

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

// zalo.go — Zalo OA (Official Account) channel adapter.
// OA mode: webhook inbound + REST API outbound with rotating OAuth tokens.
// Personal mode: QR login + WebSocket listener (legacy; no OA required).
// Reference: https://developers.zalo.me/docs/api/official-account-api

const (
	oaBaseURL   = "https://openapi.zalo.me/v2.0/oa"
	tokenURL    = "https://oauth.zaloapp.com/v4/oa/access_token"
	maxTextLen  = 2000 // Zalo OA text message character limit
)

// Channel implements channels.Channel for Zalo messaging.
type Channel struct {
	cfg         ZaloConfig
	handler     channels.InboundHandler
	running     bool
	mu          sync.Mutex
	tokenMu     sync.Mutex // separate from mu; token fetch must not block IsRunning()
	client      *http.Client
	debouncer   *channels.Debouncer
	accessToken string
	tokenExp    time.Time
	refreshToken string // rotates on every access token refresh
	dedup       sync.Map // event_id → time.Time

	// Personal mode (QR login) state
	sess     *Session
	listener *Listener
}

// New creates a new Zalo channel.
func New(cfg ZaloConfig, handler channels.InboundHandler) *Channel {
	c := &Channel{
		cfg:          cfg,
		handler:      handler,
		client:       &http.Client{Timeout: 30 * time.Second},
		refreshToken: cfg.RefreshToken,
		accessToken:  cfg.AccessToken,
	}
	if cfg.AccessToken != "" {
		// Pre-seed from config but set expiry near-zero so first request refreshes
		c.tokenExp = time.Now().Add(30 * time.Second)
	}
	return c
}

func (c *Channel) Name() string    { return "zalo" }
func (c *Channel) Type() string    { return "zalo" }
func (c *Channel) AgentID() string { return c.cfg.AgentID }
func (c *Channel) IsRunning() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.running }

// SetHandler sets the inbound message callback (legacy personal-mode path).
func (c *Channel) SetHandler(fn func(channels.InboundMessage)) {
	c.handler = func(ctx context.Context, msg channels.InboundMessage) { fn(msg) }
}

// Start authenticates and begins listening for messages.
func (c *Channel) Start(ctx context.Context) error {
	c.debouncer = channels.NewDebouncer(600*time.Millisecond, func(msg channels.InboundMessage) {
		if c.handler != nil {
			c.handler(ctx, msg)
		}
	})

	if c.cfg.PersonalMode {
		sess := NewSession()
		if err := LoginWithCredentials(ctx, sess, c.cfg); err != nil {
			return fmt.Errorf("zalo: login failed: %w", err)
		}
		c.sess = sess

		ln, err := NewListener(sess)
		if err != nil {
			return fmt.Errorf("zalo: listener: %w", err)
		}
		if err := ln.Start(ctx); err != nil {
			return fmt.Errorf("zalo: listener start: %w", err)
		}
		c.listener = ln
		c.mu.Lock()
		c.running = true
		c.mu.Unlock()
		go c.processMessages(ctx)
		slog.Info("zalo: started (personal mode)", "uid", sess.UID)
	} else {
		c.mu.Lock()
		c.running = true
		c.mu.Unlock()
		slog.Info("zalo: started (OA mode)", "app_id", c.cfg.AppID)
	}
	return nil
}

// Stop shuts down the Zalo channel.
func (c *Channel) Stop(_ context.Context) error {
	if c.debouncer != nil {
		c.debouncer.FlushAll()
	}
	if c.listener != nil {
		c.listener.Stop()
	}
	c.mu.Lock()
	c.running = false
	c.mu.Unlock()
	slog.Info("zalo: stopped")
	return nil
}

// --- Access Token (OA mode) ---

func (c *Channel) getToken() (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.tokenExp) {
		return c.accessToken, nil
	}

	if c.cfg.AppID == "" || c.cfg.AppSecret == "" || c.refreshToken == "" {
		if c.accessToken != "" {
			return c.accessToken, nil // use static token if no refresh credentials
		}
		return "", fmt.Errorf("zalo: no credentials for token refresh")
	}

	form := url.Values{
		"app_id":        {c.cfg.AppID},
		"app_secret":    {c.cfg.AppSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.refreshToken},
	}
	resp, err := c.client.PostForm(tokenURL, form)
	if err != nil {
		return "", fmt.Errorf("zalo token: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"` // Zalo rotates refresh token on every use
		ExpiresIn    int    `json:"expires_in"`
		Error        int    `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Error != 0 {
		return "", fmt.Errorf("zalo token: error %d", result.Error)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("zalo token: empty access token")
	}

	c.accessToken = result.AccessToken
	c.tokenExp = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)
	// Always save the new refresh token — Zalo invalidates the old one immediately
	if result.RefreshToken != "" {
		c.refreshToken = result.RefreshToken
	}
	return c.accessToken, nil
}

// --- Webhook Signature Verification ---

// verifySignature checks the X-ZEvent-Signature header.
// Algorithm: HMAC-SHA256(app_secret, raw_body), hex-encoded, prefixed with "MAC=".
func (c *Channel) verifySignature(r *http.Request, body []byte) bool {
	if c.cfg.AppSecret == "" {
		return true
	}
	sig := r.Header.Get("X-ZEvent-Signature")
	if sig == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(c.cfg.AppSecret))
	mac.Write(body)
	expected := "MAC=" + fmt.Sprintf("%x", mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig))
}

// --- Webhook Handler (OA mode inbound) ---

func (c *Channel) HandleWebhook(rw http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))

	if !c.verifySignature(r, body) {
		slog.Warn("zalo.webhook.signature_invalid")
		http.Error(rw, "invalid signature", http.StatusUnauthorized)
		return
	}

	var event zaloEvent
	if err := json.Unmarshal(body, &event); err != nil || event.EventName == "" {
		rw.WriteHeader(http.StatusOK)
		return
	}

	// Dedup by msg_id — Zalo retries on non-200
	if event.Message.MsgID != "" {
		if _, loaded := c.dedup.LoadOrStore(event.Message.MsgID, time.Now()); loaded {
			rw.WriteHeader(http.StatusOK)
			return
		}
		go func() { time.Sleep(2 * time.Hour); c.dedup.Delete(event.Message.MsgID) }()
	}

	senderID := event.Sender.ID
	if senderID == "" {
		senderID = event.Follower.ID
	}

	switch event.EventName {
	case "user_send_text":
		if event.Message.Text == "" || senderID == "" {
			break
		}
		slog.Info("zalo.inbound", "event", event.EventName, "from", senderID)
		c.debouncer.Push(channels.InboundMessage{
			ChannelName: "zalo",
			ChannelType: "zalo",
			AgentID:     c.cfg.AgentID,
			SenderID:    senderID,
			ChatID:      senderID,
			Content:     event.Message.Text,
			Metadata: map[string]string{
				"event_name": event.EventName,
				"message_id": event.Message.MsgID,
				"app_id":     event.AppID,
			},
		})

	case "user_send_image", "user_send_gif", "user_send_audio", "user_send_video":
		var mediaURL string
		if len(event.Message.Attachments) > 0 {
			mediaURL = event.Message.Attachments[0].Payload.URL
		}
		label := map[string]string{
			"user_send_image": "[Image attachment]",
			"user_send_gif":   "[GIF attachment]",
			"user_send_audio": "[Audio attachment]",
			"user_send_video": "[Video attachment]",
		}[event.EventName]
		if senderID == "" {
			break
		}
		slog.Info("zalo.inbound", "event", event.EventName, "from", senderID)
		meta := map[string]string{
			"event_name": event.EventName,
			"message_id": event.Message.MsgID,
		}
		if mediaURL != "" {
			meta["media_url"] = mediaURL
		}
		c.debouncer.Push(channels.InboundMessage{
			ChannelName: "zalo",
			ChannelType: "zalo",
			AgentID:     c.cfg.AgentID,
			SenderID:    senderID,
			ChatID:      senderID,
			Content:     label,
			Metadata:    meta,
		})

	case "user_send_file":
		var fileURL, fileName string
		if len(event.Message.Attachments) > 0 {
			fileURL = event.Message.Attachments[0].Payload.URL
			fileName = event.Message.Attachments[0].Payload.Name
		}
		text := "[File attachment]"
		if fileName != "" {
			text = fmt.Sprintf("[File: %s]", fileName)
		}
		if senderID == "" {
			break
		}
		meta := map[string]string{"event_name": event.EventName, "message_id": event.Message.MsgID}
		if fileURL != "" {
			meta["file_url"] = fileURL
		}
		c.debouncer.Push(channels.InboundMessage{
			ChannelName: "zalo",
			ChannelType: "zalo",
			AgentID:     c.cfg.AgentID,
			SenderID:    senderID,
			ChatID:      senderID,
			Content:     text,
			Metadata:    meta,
		})

	case "user_send_location":
		var coords string
		if len(event.Message.Attachments) > 0 {
			p := event.Message.Attachments[0].Payload
			coords = fmt.Sprintf("%.6f,%.6f", p.Coordinates.Latitude, p.Coordinates.Longitude)
		}
		text := "[Location shared]"
		if coords != "" {
			text = fmt.Sprintf("[Location: %s]", coords)
		}
		if senderID == "" {
			break
		}
		c.debouncer.Push(channels.InboundMessage{
			ChannelName: "zalo",
			ChannelType: "zalo",
			AgentID:     c.cfg.AgentID,
			SenderID:    senderID,
			ChatID:      senderID,
			Content:     text,
			Metadata:    map[string]string{"event_name": event.EventName},
		})

	case "follow":
		slog.Info("zalo.follow", "follower", event.Follower.ID)
	case "unfollow":
		slog.Info("zalo.unfollow", "follower", event.Follower.ID)
	case "oa_receive_reaction":
		slog.Info("zalo.reaction", "icon", event.Message.ReactIcon, "from", senderID)
	default:
		slog.Debug("zalo.webhook.unknown_event", "event", event.EventName)
	}

	rw.WriteHeader(http.StatusOK)
}

// --- Send ---

func (c *Channel) Send(ctx context.Context, msg channels.OutboundMessage) error {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	if c.cfg.PersonalMode && c.sess != nil {
		return c.sendPersonal(ctx, msg.ChatID, content)
	}
	return c.sendOA(ctx, msg, content)
}

func (c *Channel) sendOA(ctx context.Context, msg channels.OutboundMessage, content string) error {
	toUser := msg.RecipientID
	if toUser == "" {
		toUser = msg.ChatID
	}
	if toUser == "" && msg.Metadata != nil {
		toUser = msg.Metadata["sender_id"]
	}
	if toUser == "" {
		return fmt.Errorf("zalo: no recipient")
	}

	for _, chunk := range splitMessage(content, maxTextLen) {
		if err := c.sendOAText(ctx, toUser, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (c *Channel) sendOAText(ctx context.Context, userID, text string) error {
	token, err := c.getToken()
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{
		"recipient": map[string]string{"user_id": userID},
		"message":   map[string]string{"text": text},
	})

	req, _ := http.NewRequestWithContext(ctx, "POST", oaBaseURL+"/message/cs", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("access_token", token) // Zalo uses custom lowercase header, not Authorization
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("zalo.oa: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Error != 0 {
		return fmt.Errorf("zalo.oa: error %d: %s", result.Error, result.Message)
	}
	return nil
}

func (c *Channel) sendPersonal(ctx context.Context, chatID, content string) error {
	threadType := ThreadUser
	SendTyping(ctx, c.sess, chatID, threadType)
	time.Sleep(500 * time.Millisecond)

	for _, chunk := range splitMessage(content, maxTextLen) {
		if _, err := SendText(ctx, c.sess, chatID, threadType, chunk); err != nil {
			return fmt.Errorf("zalo.send: %w", err)
		}
	}
	return nil
}

// SendPromotion sends via the promotional endpoint (outside 48h CS window).
func (c *Channel) SendPromotion(ctx context.Context, userID, text string) error {
	token, err := c.getToken()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"recipient": map[string]string{"user_id": userID},
		"message":   map[string]string{"text": text},
	})
	req, _ := http.NewRequestWithContext(ctx, "POST", oaBaseURL+"/message/promotion", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("access_token", token)
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("zalo.promotion: %w", err)
	}
	defer resp.Body.Close()
	var result struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Error != 0 {
		return fmt.Errorf("zalo.promotion: error %d: %s", result.Error, result.Message)
	}
	return nil
}

func (c *Channel) processMessages(ctx context.Context) {
	if c.listener == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.listener.Messages():
			if !ok {
				return
			}
			c.debouncer.Push(channels.InboundMessage{
				ChannelType: "zalo",
				ChannelName: "zalo",
				AgentID:     c.cfg.AgentID,
				SenderID:    msg.SenderID,
				ChatID:      msg.ChatID,
				Content:     msg.Text,
			})
		case info := <-c.listener.Disconnected():
			slog.Warn("zalo: disconnected, reconnecting...", "code", info.Code)
			time.Sleep(5 * time.Second)
			if err := c.listener.Start(ctx); err != nil {
				slog.Error("zalo: reconnect failed", "err", err)
			}
		}
	}
}

// doPost is a helper for authenticated POST requests.
func doPost(ctx context.Context, sess *Session, url string, body []byte) (map[string]any, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	setDefaultHeaders(req, sess)

	resp, err := sess.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]any
	readJSON(resp, &result)
	return result, nil
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
