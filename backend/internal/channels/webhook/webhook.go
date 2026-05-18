// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package webhook

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

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/channels"
)

// Generic webhook channel — universal escape hatch for any HTTP-capable system.
//
// Inbound payload keys: text (required), userId, conversationId, callbackUrl, metadata
// Auth patterns (checked in order, all optional per config):
//
//	Pattern A — X-Qorven-Secret: <secret>       (shared secret header)
//	Pattern B — X-Qorven-Signature: sha256=<hex> (HMAC-SHA256 of raw body)
//	Pattern C — Authorization: Bearer <token>    (Bearer token)
//
// Response modes:
//
//	Sync  — no callbackUrl in payload → handler called, response returned in HTTP body (30s timeout)
//	Async — callbackUrl present → 202 returned immediately, response POSTed to callbackUrl
//
// Outbound signing: POST to callbackUrl/OutboundURL includes X-Qorven-Signature header.

type Config struct {
	AgentID     string `json:"agent_id"`
	InboundPath string `json:"inbound_path"` // e.g. /webhooks/my-tool
	OutboundURL string `json:"outbound_url"` // default callbackUrl; overridden per-request
	Secret      string `json:"secret"`       // used for all three auth patterns
}

// pendingReq holds the response channel for synchronous (no callbackUrl) requests.
type pendingReq struct {
	respCh chan string
}

type WebhookChannel struct {
	cfg     Config
	handler channels.InboundHandler
	running bool
	mu      sync.Mutex
	pending sync.Map // conversationID → *pendingReq  (sync mode only)
	dedup   sync.Map // idempotencyKey → time.Time     (24h dedup window)
	client  *http.Client
}

func New(cfg Config, handler channels.InboundHandler) *WebhookChannel {
	return &WebhookChannel{
		cfg:     cfg,
		handler: handler,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *WebhookChannel) Name() string    { return fmt.Sprintf("webhook:%s", w.cfg.InboundPath) }
func (w *WebhookChannel) Type() string    { return "webhook" }
func (w *WebhookChannel) AgentID() string { return w.cfg.AgentID }
func (w *WebhookChannel) IsRunning() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.running }
func (w *WebhookChannel) Start(_ context.Context) error {
	w.mu.Lock()
	w.running = true
	w.mu.Unlock()
	return nil
}
func (w *WebhookChannel) Stop(_ context.Context) error {
	w.mu.Lock()
	w.running = false
	w.mu.Unlock()
	return nil
}

func (w *WebhookChannel) HandleWebhook(rw http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)

	// Auth verification
	if w.cfg.Secret != "" && !w.verifyAuth(r, body) {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	var payload struct {
		// Primary field names (spec)
		Text           string            `json:"text"`
		UserID         string            `json:"userId"`
		ConversationID string            `json:"conversationId"`
		CallbackURL    string            `json:"callbackUrl"`
		IdempotencyKey string            `json:"idempotencyKey"`
		Metadata       map[string]string `json:"metadata"`
		// Backward-compat aliases
		Content  string `json:"content"`
		SenderID string `json:"sender_id"`
		Name     string `json:"name"`
	}
	json.Unmarshal(body, &payload)

	// Field aliasing: prefer spec names, fall back to legacy names
	text := payload.Text
	if text == "" {
		text = payload.Content
	}
	userID := payload.UserID
	if userID == "" {
		userID = payload.SenderID
	}
	convID := payload.ConversationID
	if convID == "" {
		convID = userID
	}

	if text == "" {
		writeJSON(rw, http.StatusBadRequest, map[string]string{
			"error":   "field_missing",
			"message": "Required field 'text' not found in payload",
			"code":    "FIELD_MISSING",
		})
		return
	}

	// Idempotency dedup — 24h window
	if payload.IdempotencyKey != "" {
		if _, loaded := w.dedup.LoadOrStore(payload.IdempotencyKey, time.Now()); loaded {
			writeJSON(rw, http.StatusOK, map[string]string{"status": "duplicate", "idempotencyKey": payload.IdempotencyKey})
			return
		}
		go func() { time.Sleep(24 * time.Hour); w.dedup.Delete(payload.IdempotencyKey) }()
	}

	// Build metadata map for agent context
	meta := map[string]string{
		"conversation_id": convID,
		"user_id":         userID,
	}
	if payload.CallbackURL != "" {
		meta["callback_url"] = payload.CallbackURL
	}
	for k, v := range payload.Metadata {
		meta[k] = v
	}

	msgID := uuid.New().String()
	inbound := channels.InboundMessage{
		ChannelName: w.Name(),
		ChannelType: "webhook",
		AgentID:     w.cfg.AgentID,
		SenderID:    userID,
		SenderName:  payload.Name,
		ChatID:      convID,
		Content:     text,
		PeerKind:    "direct",
		Metadata:    meta,
	}
	meta["message_id"] = msgID

	// callbackUrl from payload is caller-controlled → require HTTPS.
	// OutboundURL from config is admin-controlled → trusted, no HTTPS requirement.
	if payload.CallbackURL != "" && !strings.HasPrefix(payload.CallbackURL, "https://") {
		writeJSON(rw, http.StatusBadRequest, map[string]string{
			"error":   "invalid_callback_url",
			"message": "callbackUrl must use HTTPS",
		})
		return
	}

	callbackURL := payload.CallbackURL
	if callbackURL == "" {
		callbackURL = w.cfg.OutboundURL
	}

	if callbackURL == "" {
		// Sync mode: wait for agent response (up to 30s), return in HTTP body
		req := &pendingReq{respCh: make(chan string, 1)}
		w.pending.Store(convID, req)
		defer w.pending.Delete(convID)

		if w.handler != nil {
			w.handler(r.Context(), inbound)
		}

		select {
		case response := <-req.respCh:
			writeJSON(rw, http.StatusOK, map[string]any{
				"response":       response,
				"conversationId": convID,
				"messageId":      msgID,
				"metadata":       map[string]any{"model": ""},
			})
		case <-time.After(30 * time.Second):
			writeJSON(rw, http.StatusOK, map[string]any{
				"conversationId": convID,
				"messageId":      msgID,
				"status":         "timeout",
			})
		}
		return
	}

	// Async mode: return 202 immediately, POST response to callbackUrl
	writeJSON(rw, http.StatusAccepted, map[string]string{
		"messageId": msgID,
		"status":    "queued",
	})

	go func() {
		if w.handler != nil {
			w.handler(context.Background(), inbound)
		}
	}()

	slog.Info("webhook.inbound.async", "convID", convID, "callback", callbackURL)
}

// Send delivers the agent response either to a pending sync request or via HTTP POST.
func (w *WebhookChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	convID := msg.ChatID
	if convID == "" {
		convID = msg.RecipientID
	}
	if convID == "" && msg.Metadata != nil {
		convID = msg.Metadata["conversation_id"]
	}

	// Sync mode: route to pending request channel
	if convID != "" {
		if v, ok := w.pending.Load(convID); ok {
			req := v.(*pendingReq)
			select {
			case req.respCh <- msg.Content:
				return nil
			default:
			}
		}
	}

	// Async mode: POST to callbackUrl (from metadata) or OutboundURL
	targetURL := ""
	if msg.Metadata != nil {
		targetURL = msg.Metadata["callback_url"]
	}
	if targetURL == "" {
		targetURL = w.cfg.OutboundURL
	}
	if targetURL == "" {
		return nil
	}

	msgID := uuid.New().String()
	responseBody, _ := json.Marshal(map[string]any{
		"messageId":      msgID,
		"conversationId": convID,
		"response":       msg.Content,
		"status":         "complete",
	})

	req, _ := http.NewRequest("POST", targetURL, bytes.NewReader(responseBody))
	req.Header.Set("Content-Type", "application/json")
	if w.cfg.Secret != "" {
		req.Header.Set("X-Qorven-Signature", signBody(responseBody, w.cfg.Secret))
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook send: %w", err)
	}
	resp.Body.Close()
	slog.Info("webhook.sent", "url", targetURL, "conv", convID)
	return nil
}

// verifyAuth checks inbound request auth against configured secret.
// Checks in order: X-Qorven-Signature (HMAC), X-Qorven-Secret (shared secret), Authorization Bearer.
func (w *WebhookChannel) verifyAuth(r *http.Request, body []byte) bool {
	// Pattern B: HMAC-SHA256
	if sig := r.Header.Get("X-Qorven-Signature"); sig != "" {
		return VerifySignature(body, sig, w.cfg.Secret)
	}
	// Pattern A: shared secret header
	if sec := r.Header.Get("X-Qorven-Secret"); sec != "" {
		return hmac.Equal([]byte(sec), []byte(w.cfg.Secret))
	}
	// Pattern C: Bearer token
	if auth := r.Header.Get("Authorization"); auth != "" {
		return hmac.Equal([]byte(auth), []byte("Bearer "+w.cfg.Secret))
	}
	return false
}

// VerifySignature checks HMAC-SHA256 signature. Signature format: "sha256=<hex>"
func VerifySignature(payload []byte, signature, secret string) bool {
	if secret == "" || signature == "" {
		return true
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// signBody returns the HMAC-SHA256 signature header value for an outbound payload.
func signBody(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
