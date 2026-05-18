// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package whatsapp

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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/qorvenai/qorven/internal/channels"
)

const (
	cloudAPIBase        = "https://graph.facebook.com/v21.0"
	maxMessageLen       = 4096
	pairingDebounceTime = 60 * time.Second
)

// PairingStore interface for pairing service integration.
type PairingStore interface {
	IsPaired(ctx context.Context, senderID, channelName string) (bool, error)
	RequestPairing(ctx context.Context, senderID, channelName, chatID, agentID string, meta map[string]string) (string, error)
}

// Config for WhatsApp channel.
type Config struct {
	AgentID     string   `json:"agent_id"`
	Mode        string   `json:"mode"` // "cloud" or "bridge"
	DMPolicy    string   `json:"dm_policy"`
	GroupPolicy string   `json:"group_policy"`
	AllowFrom   []string `json:"allow_from"`

	// Cloud API mode
	PhoneNumberID string `json:"phone_number_id"`
	AccessToken   string `json:"access_token"`
	VerifyToken   string `json:"verify_token"`
	AppSecret     string `json:"app_secret"`

	// Bridge mode (Baileys/whatsapp-web.js sidecar)
	BridgeURL string `json:"bridge_url"`
}

// WhatsAppChannel supports Cloud API and WebSocket bridge modes.
type WhatsAppChannel struct {
	cfg             Config
	handler         channels.InboundHandler
	running         bool
	mu              sync.Mutex
	client          *http.Client
	pairingService  PairingStore
	pairingDebounce sync.Map
	approvedGroups  sync.Map
	allowList       []string
	dedup           sync.Map // msgID → time.Time — prevents double-fire on platform retries

	// bridge mode — managed sidecar
	bridgeProc *BridgeProcess

	gate           *senderGate
	sendOTPMessage func(ctx context.Context, to, otp string)

	// Bridge mode
	conn      *websocket.Conn
	connMu    sync.Mutex
	connected bool
	ctx       context.Context
	cancel    context.CancelFunc

	// Optional STT
	Transcribe func(ctx context.Context, audio []byte, format string) (string, error)
}

func New(cfg Config, handler channels.InboundHandler) *WhatsAppChannel {
	if cfg.Mode == "" {
		if cfg.BridgeURL != "" {
			cfg.Mode = "bridge"
		} else {
			cfg.Mode = "cloud"
		}
	}
	ch := &WhatsAppChannel{
		cfg:       cfg,
		handler:   handler,
		client:    &http.Client{Timeout: 30 * time.Second},
		allowList: cfg.AllowFrom,
		gate:      newSenderGate(),
	}
	ch.sendOTPMessage = ch.defaultSendOTPMessage
	return ch
}

// SetPairingService sets the pairing store for DM/group policy.
func (w *WhatsAppChannel) SetPairingService(ps PairingStore) { w.pairingService = ps }

func (w *WhatsAppChannel) Name() string    { return "whatsapp" }
func (w *WhatsAppChannel) Type() string    { return "whatsapp" }
func (w *WhatsAppChannel) AgentID() string { return w.cfg.AgentID }

func (w *WhatsAppChannel) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (w *WhatsAppChannel) Start(ctx context.Context) error {
	w.ctx, w.cancel = context.WithCancel(ctx)

	if w.cfg.Mode == "bridge" {
		if w.cfg.BridgeURL != "" {
			// Legacy: user-provided external sidecar URL
			if err := w.bridgeConnect(); err != nil {
				slog.Warn("whatsapp.bridge.initial_connect_failed", "error", err)
			}
			go w.bridgeListenLoop()
		} else {
			// Managed sidecar: Qorven spawns it
			dataDir := filepath.Join(os.TempDir(), "qorven-wa-"+w.cfg.AgentID)
			if err := os.MkdirAll(dataDir, 0700); err != nil {
				slog.Warn("whatsapp.bridge.mkdir_failed", "error", err)
			}

			if w.bridgeProc == nil {
				w.bridgeProc = NewBridgeProcess(w.cfg.AgentID, dataDir, "")
			}
			port, err := w.bridgeProc.StartServer(w.ctx)
			if err != nil {
				return fmt.Errorf("whatsapp bridge server: %w", err)
			}
			slog.Info("whatsapp.bridge.server_started", "port", port)

			w.bridgeProc.SubscribeMessages(func(m BridgeMessage) {
				w.handleBridgeProcessMessage(w.ctx, m)
			})

			if err := w.bridgeProc.SpawnSidecar(w.ctx); err != nil {
				slog.Warn("whatsapp.bridge.sidecar_spawn_failed", "error", err)
				// Not fatal — sidecar can be started manually; WS server is up
			}
		}
	}

	w.mu.Lock()
	w.running = true
	w.mu.Unlock()

	slog.Info("whatsapp.started", "mode", w.cfg.Mode, "agent", w.cfg.AgentID)
	return nil
}

func (w *WhatsAppChannel) Stop(_ context.Context) error {
	if w.cancel != nil {
		w.cancel()
	}

	w.connMu.Lock()
	if w.conn != nil {
		w.conn.Close()
		w.conn = nil
	}
	w.connected = false
	w.connMu.Unlock()

	if w.bridgeProc != nil {
		w.bridgeProc.Stop()
	}

	w.mu.Lock()
	w.running = false
	w.mu.Unlock()

	slog.Info("whatsapp.stopped")
	return nil
}

func (w *WhatsAppChannel) Send(ctx context.Context, msg channels.OutboundMessage) error {
	if w.cfg.Mode == "bridge" {
		return w.bridgeSend(msg)
	}
	return w.cloudSend(msg)
}

// IsAllowed checks if sender is in allowlist.
func (w *WhatsAppChannel) IsAllowed(senderID string) bool {
	if len(w.allowList) == 0 {
		return true
	}
	for _, allowed := range w.allowList {
		if senderID == allowed || strings.TrimPrefix(allowed, "@") == senderID {
			return true
		}
	}
	return false
}

// ============================================================
// BRIDGE MODE — WebSocket to Baileys/whatsapp-web.js sidecar
// ============================================================

func (w *WhatsAppChannel) bridgeConnect() error {
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second

	conn, _, err := dialer.Dial(w.cfg.BridgeURL, nil)
	if err != nil {
		return fmt.Errorf("dial bridge %s: %w", w.cfg.BridgeURL, err)
	}

	w.connMu.Lock()
	w.conn = conn
	w.connected = true
	w.connMu.Unlock()

	slog.Info("whatsapp.bridge.connected", "url", w.cfg.BridgeURL)
	return nil
}

func (w *WhatsAppChannel) bridgeListenLoop() {
	backoff := time.Second

	for {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		w.connMu.Lock()
		conn := w.conn
		w.connMu.Unlock()

		if conn == nil {
			slog.Info("whatsapp.bridge.reconnecting", "backoff", backoff)
			select {
			case <-w.ctx.Done():
				return
			case <-time.After(backoff):
			}

			if err := w.bridgeConnect(); err != nil {
				slog.Warn("whatsapp.bridge.reconnect_failed", "error", err)
				backoff = min(backoff*2, 30*time.Second)
				continue
			}
			backoff = time.Second
			continue
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			slog.Warn("whatsapp.bridge.read_error", "error", err)
			w.connMu.Lock()
			if w.conn != nil {
				w.conn.Close()
				w.conn = nil
			}
			w.connected = false
			w.connMu.Unlock()
			continue
		}

		var msg map[string]any
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		msgType, _ := msg["type"].(string)
		switch msgType {
		case "message":
			w.handleBridgeMessage(msg)
		case "qr":
			qr, _ := msg["qr"].(string)
			slog.Info("whatsapp.qr.received", "qr_length", len(qr))
		case "ready":
			phone, _ := msg["phone"].(string)
			slog.Info("whatsapp.bridge.ready", "phone", phone)
		case "disconnected":
			reason, _ := msg["reason"].(string)
			slog.Warn("whatsapp.bridge.disconnected", "reason", reason)
		}
	}
}

func (w *WhatsAppChannel) handleBridgeMessage(msg map[string]any) {
	ctx := w.ctx
	senderID, _ := msg["from"].(string)
	if senderID == "" {
		return
	}

	// Dedup by message ID when present (bridge may reconnect and replay).
	if msgID, ok := msg["id"].(string); ok && msgID != "" {
		if _, already := w.dedup.LoadOrStore(msgID, time.Now()); already {
			return
		}
		go func() { time.Sleep(10 * time.Minute); w.dedup.Delete(msgID) }()
	}

	chatID, _ := msg["chat"].(string)
	if chatID == "" {
		chatID = senderID
	}

	peerKind := "direct"
	if strings.HasSuffix(chatID, "@g.us") {
		peerKind = "group"
	}

	// Policy check
	if peerKind == "direct" {
		if !w.checkDMPolicy(ctx, senderID, chatID) {
			return
		}
	} else {
		if !w.checkGroupPolicy(ctx, senderID, chatID) {
			return
		}
	}

	content, _ := msg["content"].(string)
	if content == "" {
		content = "[empty message]"
	}

	metadata := make(map[string]string)
	if messageID, ok := msg["id"].(string); ok {
		metadata["message_id"] = messageID
	}
	if userName, ok := msg["from_name"].(string); ok {
		metadata["user_name"] = userName
		content = fmt.Sprintf("[From: %s]\n%s", userName, content)
	}

	slog.Debug("whatsapp.message", "from", senderID, "chat", chatID)

	if w.handler != nil {
		w.handler(ctx, channels.InboundMessage{
			ChannelName: "whatsapp",
			ChannelType: "whatsapp",
			AgentID:     w.cfg.AgentID,
			SenderID:    senderID,
			ChatID:      chatID,
			Content:     content,
			PeerKind:    peerKind,
			Metadata:    metadata,
		})
	}
}

// handleBridgeProcessMessage processes events from the managed BridgeProcess.
func (w *WhatsAppChannel) handleBridgeProcessMessage(ctx context.Context, m BridgeMessage) {
	if m.From == "" {
		return
	}

	// OTP interception: if sender has a pending OTP and message looks like a 6-digit code
	if w.gate != nil && w.gate.isPending(m.From) && isOTPSubmission(m.Body) {
		result := w.gate.verify(m.From, strings.TrimSpace(m.Body))
		switch result {
		case otpVerifyApproved:
			w.Send(ctx, channels.OutboundMessage{ChatID: m.From, Content: "✅ Access approved. Your original message will be processed."})
		case otpVerifyWrong:
			w.Send(ctx, channels.OutboundMessage{ChatID: m.From, Content: "❌ Incorrect code. Please try again."})
		case otpVerifyLockedOut:
			w.Send(ctx, channels.OutboundMessage{ChatID: m.From, Content: "🔒 Too many wrong attempts. Please wait 5 minutes."})
		}
		return
	}

	if m.ID != "" {
		if _, already := w.dedup.LoadOrStore(m.ID, time.Now()); already {
			return
		}
		go func() { time.Sleep(10 * time.Minute); w.dedup.Delete(m.ID) }()
	}

	chatID := m.Chat
	if chatID == "" {
		chatID = m.From
	}

	peerKind := "direct"
	if strings.HasSuffix(chatID, "@g.us") {
		peerKind = "group"
	}

	if peerKind == "direct" {
		if !w.checkDMPolicy(ctx, m.From, chatID) {
			return
		}
	} else {
		if !w.checkGroupPolicy(ctx, m.From, chatID) {
			return
		}
	}

	if w.handler != nil {
		w.handler(ctx, channels.InboundMessage{
			ChannelName: "whatsapp",
			ChannelType: "whatsapp",
			AgentID:     w.cfg.AgentID,
			SenderID:    m.From,
			SenderName:  m.FromName,
			ChatID:      chatID,
			Content:     m.Body,
			PeerKind:    peerKind,
			Metadata: map[string]string{
				"message_id": m.ID,
				"chat_id":    chatID,
			},
		})
	}
}

// SubscribeQREvents registers a callback for QR events from the managed sidecar.
// Returns an unsubscribe function. No-op in cloud mode.
func (w *WhatsAppChannel) SubscribeQREvents(fn func(string)) func() {
	if w.bridgeProc == nil {
		return func() {}
	}
	w.bridgeProc.mu.Lock()
	w.bridgeProc.qrSubs = append(w.bridgeProc.qrSubs, fn)
	idx := len(w.bridgeProc.qrSubs) - 1
	w.bridgeProc.mu.Unlock()
	return func() {
		w.bridgeProc.mu.Lock()
		if idx < len(w.bridgeProc.qrSubs) {
			w.bridgeProc.qrSubs = append(w.bridgeProc.qrSubs[:idx], w.bridgeProc.qrSubs[idx+1:]...)
		}
		w.bridgeProc.mu.Unlock()
	}
}

// RequestLatestQR asks the sidecar to re-emit its current QR.
func (w *WhatsAppChannel) RequestLatestQR() {
	if w.bridgeProc != nil {
		w.bridgeProc.RequestQR()
	}
}

// ReplayMessage injects a message as if it just arrived from the given sender.
func (w *WhatsAppChannel) ReplayMessage(ctx context.Context, senderJID, body string) {
	if w.handler != nil {
		w.handler(ctx, channels.InboundMessage{
			ChannelName: "whatsapp",
			ChannelType: "whatsapp",
			AgentID:     w.cfg.AgentID,
			SenderID:    senderJID,
			ChatID:      senderJID,
			Content:     body,
			PeerKind:    "direct",
			Metadata:    map[string]string{"replayed": "true"},
		})
	}
}

func (w *WhatsAppChannel) bridgeSend(msg channels.OutboundMessage) error {
	w.connMu.Lock()
	conn := w.conn
	w.connMu.Unlock()

	if conn == nil {
		return fmt.Errorf("bridge not connected")
	}

	chatID := msg.ChatID
	if chatID == "" {
		chatID = msg.RecipientID
	}

	payload, _ := json.Marshal(map[string]any{
		"type":    "message",
		"to":      chatID,
		"content": msg.Content,
	})

	w.connMu.Lock()
	err := w.conn.WriteMessage(websocket.TextMessage, payload)
	w.connMu.Unlock()

	return err
}

// GetQRCode fetches current QR code from bridge for pairing.
func (w *WhatsAppChannel) GetQRCode() (string, error) {
	if w.cfg.Mode != "bridge" {
		return "", fmt.Errorf("QR code only available in bridge mode")
	}

	resp, err := w.client.Get(w.cfg.BridgeURL + "/qr")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var qr struct {
		QR     string `json:"qr"`
		Status string `json:"status"`
	}
	json.NewDecoder(resp.Body).Decode(&qr)
	return qr.QR, nil
}

// GetConnectionStatus checks bridge connection status.
func (w *WhatsAppChannel) GetConnectionStatus() (string, error) {
	if w.cfg.Mode != "bridge" {
		return "cloud_api", nil
	}

	w.connMu.Lock()
	connected := w.connected
	w.connMu.Unlock()

	if !connected {
		return "disconnected", nil
	}

	resp, err := w.client.Get(w.cfg.BridgeURL + "/status")
	if err != nil {
		return "disconnected", err
	}
	defer resp.Body.Close()

	var status struct {
		Connected bool   `json:"connected"`
		Phone     string `json:"phone"`
	}
	json.NewDecoder(resp.Body).Decode(&status)

	if status.Connected {
		return "connected:" + status.Phone, nil
	}
	return "awaiting_qr", nil
}

// RequestQRPairing initiates QR pairing flow.
func (w *WhatsAppChannel) RequestQRPairing() error {
	if w.cfg.Mode != "bridge" {
		return fmt.Errorf("QR pairing only available in bridge mode")
	}

	resp, err := w.client.Post(w.cfg.BridgeURL+"/pair", "application/json", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ============================================================
// POLICY CHECKS
// ============================================================

func (w *WhatsAppChannel) checkDMPolicy(ctx context.Context, senderID, chatID string) bool {
	policy := w.cfg.DMPolicy
	if policy == "" {
		policy = "open"
	}

	switch policy {
	case "disabled":
		return false
	case "open":
		return true
	case "allowlist":
		if w.IsAllowed(senderID) {
			return true
		}
		// If sender is pending OTP, let the message through (it may be an OTP submission)
		if w.gate != nil && w.gate.isPending(senderID) {
			return true
		}
		// Unknown sender — issue OTP challenge
		if w.gate != nil {
			otp := w.gate.challenge(senderID, "", chatID)
			if otp != "" {
				go w.sendOTPMessage(ctx, chatID, otp)
			}
		}
		return false
	default: // "pairing"
		if w.IsAllowed(senderID) {
			return true
		}
		if w.pairingService != nil {
			paired, err := w.pairingService.IsPaired(ctx, senderID, w.Name())
			if err != nil {
				slog.Warn("whatsapp.pairing_check_failed", "error", err)
				return true // fail-open
			}
			if paired {
				return true
			}
		}
		w.sendPairingReply(ctx, senderID, chatID)
		return false
	}
}

func (w *WhatsAppChannel) checkGroupPolicy(ctx context.Context, senderID, chatID string) bool {
	policy := w.cfg.GroupPolicy
	if policy == "" {
		policy = "open"
	}

	switch policy {
	case "disabled":
		return false
	case "allowlist":
		return w.IsAllowed(senderID)
	case "pairing":
		if w.IsAllowed(senderID) {
			return true
		}
		if _, cached := w.approvedGroups.Load(chatID); cached {
			return true
		}
		groupSenderID := "group:" + chatID
		if w.pairingService != nil {
			paired, err := w.pairingService.IsPaired(ctx, groupSenderID, w.Name())
			if err != nil {
				return true // fail-open
			}
			if paired {
				w.approvedGroups.Store(chatID, true)
				return true
			}
		}
		w.sendPairingReply(ctx, groupSenderID, chatID)
		return false
	default: // "open"
		return true
	}
}

func (w *WhatsAppChannel) sendPairingReply(ctx context.Context, senderID, chatID string) {
	if w.pairingService == nil {
		return
	}

	if lastSent, ok := w.pairingDebounce.Load(senderID); ok {
		if time.Since(lastSent.(time.Time)) < pairingDebounceTime {
			return
		}
	}

	code, err := w.pairingService.RequestPairing(ctx, senderID, w.Name(), chatID, w.cfg.AgentID, nil)
	if err != nil {
		slog.Debug("whatsapp.pairing_request_failed", "error", err)
		return
	}

	replyText := fmt.Sprintf(
		"Access not configured.\n\nYour WhatsApp ID: %s\n\nPairing code: %s\n\nAsk the bot owner to approve with:\n  qorven pairing approve %s",
		senderID, code, code,
	)

	w.Send(ctx, channels.OutboundMessage{ChatID: chatID, Content: replyText})
	w.pairingDebounce.Store(senderID, time.Now())
	slog.Info("whatsapp.pairing_sent", "sender", senderID, "code", code)
}

func (w *WhatsAppChannel) defaultSendOTPMessage(ctx context.Context, to, otp string) {
	msg := "To use this assistant, ask the owner for the access code.\n\n" +
		"When you have the 6-digit code, send it here."
	_ = w.Send(ctx, channels.OutboundMessage{ChatID: to, Content: msg})
}

// ============================================================
// CLOUD API MODE
// ============================================================

func (w *WhatsAppChannel) HandleWebhook(rw http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		challenge := r.URL.Query().Get("hub.challenge")
		if r.URL.Query().Get("hub.verify_token") == w.cfg.VerifyToken && isWebhookChallenge(challenge) {
			rw.Write([]byte(challenge))
			return
		}
		http.Error(rw, "forbidden", http.StatusForbidden)
		return
	}

	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if !w.VerifyWebhookSignature(r, body) {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		rw.WriteHeader(200)
		return
	}

	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			for _, msg := range change.Value.Messages {
				w.handleCloudMessage(r.Context(), msg, change.Value.Contacts)
			}
		}
	}
	rw.WriteHeader(200)
}

func (w *WhatsAppChannel) handleCloudMessage(ctx context.Context, msg cloudMessage, contacts []cloudContact) {
	if msg.From == w.cfg.PhoneNumberID {
		return
	}

	// Dedup: WhatsApp Cloud API retries delivery on timeout; skip already-seen message IDs.
	if _, already := w.dedup.LoadOrStore(msg.ID, time.Now()); already {
		return
	}
	go func() { time.Sleep(10 * time.Minute); w.dedup.Delete(msg.ID) }()

	senderName := ""
	for _, c := range contacts {
		if c.WAID == msg.From {
			senderName = c.Profile.Name
			break
		}
	}
	if senderName == "" && len(contacts) > 0 {
		senderName = contacts[0].Profile.Name
	}

	var content string
	metadata := map[string]string{
		"chat_id":    msg.From,
		"message_id": msg.ID,
		"msg_type":   msg.Type,
	}

	switch msg.Type {
	case "text":
		content = msg.Text.Body
	case "image", "document", "audio", "video":
		content = fmt.Sprintf("[%s attachment]", msg.Type)
	case "location":
		content = fmt.Sprintf("[Location: %.6f, %.6f]", msg.Location.Latitude, msg.Location.Longitude)
	default:
		content = fmt.Sprintf("[%s message]", msg.Type)
	}

	if content == "" {
		return
	}

	w.markAsRead(msg.ID)

	if senderName != "" {
		content = fmt.Sprintf("[From: %s]\n%s", senderName, content)
	}

	if w.handler != nil {
		w.handler(ctx, channels.InboundMessage{
			ChannelName: "whatsapp",
			ChannelType: "whatsapp",
			AgentID:     w.cfg.AgentID,
			SenderID:    msg.From,
			SenderName:  senderName,
			ChatID:      msg.From,
			Content:     content,
			PeerKind:    "direct",
			Metadata:    metadata,
		})
	}
}

func (w *WhatsAppChannel) cloudSend(msg channels.OutboundMessage) error {
	chatID := msg.ChatID
	if chatID == "" {
		chatID = msg.RecipientID
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	// Chunk long messages
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

		if err := w.cloudSendText(chatID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (w *WhatsAppChannel) cloudSendText(to, text string) error {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                to,
		"type":              "text",
		"text": map[string]any{
			"preview_url": false,
			"body":        markdownToWhatsApp(text),
		},
	}
	return w.cloudAPICall("messages", payload)
}

func (w *WhatsAppChannel) markAsRead(messageID string) {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        messageID,
	}
	go w.cloudAPICall("messages", payload)
}

func (w *WhatsAppChannel) cloudAPICall(endpoint string, payload map[string]any) error {
	url := fmt.Sprintf("%s/%s/%s", cloudAPIBase, w.cfg.PhoneNumberID, endpoint)
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+w.cfg.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("whatsapp api %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// SendInteractiveButtons sends a WhatsApp Cloud API interactive button message
// (up to 3 buttons). Falls back to plain text on bridge mode.
func (w *WhatsAppChannel) SendInteractiveButtons(ctx context.Context, to, bodyText string, buttons []string) error {
	if w.cfg.Mode == "bridge" {
		// Bridge doesn't support interactive messages — send as plain text with numbered options.
		var opts strings.Builder
		opts.WriteString(bodyText + "\n\n")
		for i, b := range buttons {
			opts.WriteString(fmt.Sprintf("%d. %s\n", i+1, b))
		}
		return w.bridgeSend(channels.OutboundMessage{ChatID: to, Content: opts.String()})
	}

	if len(buttons) > 3 {
		buttons = buttons[:3] // WhatsApp limits to 3 reply buttons
	}
	btns := make([]map[string]any, 0, len(buttons))
	for i, label := range buttons {
		btns = append(btns, map[string]any{
			"type": "reply",
			"reply": map[string]any{
				"id":    fmt.Sprintf("btn_%d", i),
				"title": label,
			},
		})
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "interactive",
		"interactive": map[string]any{
			"type": "button",
			"body": map[string]string{"text": bodyText},
			"action": map[string]any{
				"buttons": btns,
			},
		},
	}
	return w.cloudAPICall("messages", payload)
}

// SendInteractiveList sends a WhatsApp Cloud API list picker (up to 10 items).
// Falls back to plain text on bridge mode.
func (w *WhatsAppChannel) SendInteractiveList(ctx context.Context, to, bodyText, buttonLabel string, items []string) error {
	if w.cfg.Mode == "bridge" {
		var opts strings.Builder
		opts.WriteString(bodyText + "\n\n")
		for i, item := range items {
			opts.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
		}
		return w.bridgeSend(channels.OutboundMessage{ChatID: to, Content: opts.String()})
	}

	if len(items) > 10 {
		items = items[:10]
	}
	rows := make([]map[string]any, 0, len(items))
	for i, item := range items {
		rows = append(rows, map[string]any{
			"id":    fmt.Sprintf("item_%d", i),
			"title": item,
		})
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "interactive",
		"interactive": map[string]any{
			"type": "list",
			"body": map[string]string{"text": bodyText},
			"action": map[string]any{
				"button":   buttonLabel,
				"sections": []map[string]any{{"title": "Options", "rows": rows}},
			},
		},
	}
	return w.cloudAPICall("messages", payload)
}

func (w *WhatsAppChannel) VerifyWebhookSignature(r *http.Request, body []byte) bool {
	if w.cfg.AppSecret == "" {
		return true
	}
	signature := strings.TrimPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=")
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(w.cfg.AppSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

// isWebhookChallenge returns true if s is a safe webhook challenge token.
// Allows letters, digits, hyphens, and underscores — Meta's hub.challenge
// values may contain hyphens (e.g. "challenge-abc-123").
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

func isOTPSubmission(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) != 6 {
		return false
	}
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// ============================================================
// TYPES
// ============================================================

type webhookPayload struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Messages []cloudMessage `json:"messages"`
				Contacts []cloudContact `json:"contacts"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

type cloudMessage struct {
	From     string `json:"from"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Text     struct{ Body string `json:"body"` } `json:"text"`
	Location struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"location"`
}

type cloudContact struct {
	WAID    string `json:"wa_id"`
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
}

// markdownToWhatsApp converts standard markdown to WhatsApp's limited format:
//   **bold** → *bold*, ~~strike~~ → ~strike~, `code` → ```code```
// Italic (_text_) and links are left as-is since WhatsApp supports _italic_ natively.
func markdownToWhatsApp(text string) string {
	// Bold: **text** → *text*
	out := strings.NewReplacer("**", "*").Replace(text)
	// Strikethrough: ~~text~~ → ~text~
	out = strings.ReplaceAll(out, "~~", "~")
	// Inline code: `code` → ```code```  (WhatsApp monospace uses triple backtick)
	// Only convert single backticks that aren't already triple
	result := ""
	i := 0
	for i < len(out) {
		if i+2 < len(out) && out[i:i+3] == "```" {
			// already triple backtick block — pass through
			end := strings.Index(out[i+3:], "```")
			if end >= 0 {
				result += out[i : i+3+end+3]
				i = i + 3 + end + 3
			} else {
				result += out[i:]
				break
			}
		} else if out[i] == '`' {
			end := strings.Index(out[i+1:], "`")
			if end >= 0 {
				result += "```" + out[i+1:i+1+end] + "```"
				i = i + 1 + end + 1
			} else {
				result += string(out[i])
				i++
			}
		} else {
			result += string(out[i])
			i++
		}
	}
	return result
}
