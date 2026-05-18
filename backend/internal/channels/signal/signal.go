// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package signal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/qorvenai/qorven/internal/channels"
)

// Signal channel — connects via signal-cli-rest-api sidecar.
// Sidecar: https://github.com/bbernhard/signal-cli-rest-api
// Signal has no official bot API; signal-cli implements the Signal protocol locally.

type Config struct {
	AgentID      string `json:"agent_id"`
	APIURL       string `json:"api_url"`       // signal-cli REST API (e.g. http://localhost:8080)
	PhoneNumber  string `json:"phone_number"`  // bot's registered Signal number (+E.164)
	UseWebSocket bool   `json:"use_websocket"` // real-time WebSocket vs HTTP polling
}

type SignalChannel struct {
	cfg       Config
	handler   channels.InboundHandler
	running   bool
	cancel    context.CancelFunc
	mu        sync.Mutex
	client    *http.Client
	debouncer *channels.Debouncer
}

func New(cfg Config, handler channels.InboundHandler) *SignalChannel {
	return &SignalChannel{
		cfg:     cfg,
		handler: handler,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *SignalChannel) Name() string    { return "signal" }
func (s *SignalChannel) Type() string    { return "signal" }
func (s *SignalChannel) AgentID() string { return s.cfg.AgentID }
func (s *SignalChannel) IsRunning() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.running }

func (s *SignalChannel) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.debouncer = channels.NewDebouncer(600*time.Millisecond, func(msg channels.InboundMessage) {
		if s.handler != nil {
			s.handler(ctx, msg)
		}
	})
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	if s.cfg.UseWebSocket {
		go s.wsLoop(ctx)
	} else {
		go s.receiveLoop(ctx)
	}
	slog.Info("signal.started", "phone", s.cfg.PhoneNumber, "ws", s.cfg.UseWebSocket)
	return nil
}

func (s *SignalChannel) Stop(_ context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.debouncer != nil {
		s.debouncer.FlushAll()
	}
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	return nil
}

// --- WebSocket Mode (real-time, recommended) ---

func (s *SignalChannel) wsLoop(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := s.wsConnect(ctx); err != nil {
			slog.Warn("signal.ws.error", "error", err, "retry_in", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		} else {
			backoff = time.Second
		}
	}
}

func (s *SignalChannel) wsConnect(ctx context.Context) error {
	// '+' in phone number must be explicitly encoded as '%2B' for signal-cli-rest-api.
	// url.PathEscape leaves '+' unencoded (valid per RFC 3986 path rules), but the
	// signal-cli-rest-api server requires '%2B'.
	encodedPhone := strings.ReplaceAll(s.cfg.PhoneNumber, "+", "%2B")
	wsBase := strings.Replace(s.cfg.APIURL, "http://", "ws://", 1)
	wsBase = strings.Replace(wsBase, "https://", "wss://", 1)
	wsURL := wsBase + "/v1/receive/" + encodedPhone

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}
	defer conn.Close()

	slog.Info("signal.ws.connected", "url", wsURL)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("ws read: %w", err)
		}
		var msg signalMessage
		json.Unmarshal(raw, &msg)
		s.processMessage(msg)
	}
}

// --- Polling Mode (fallback) ---

func (s *SignalChannel) receiveLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		encodedPhone := url.PathEscape(s.cfg.PhoneNumber)
		resp, err := s.client.Get(fmt.Sprintf("%s/v1/receive/%s", s.cfg.APIURL, encodedPhone))
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		var messages []signalMessage
		json.NewDecoder(resp.Body).Decode(&messages)
		resp.Body.Close()

		for _, msg := range messages {
			s.processMessage(msg)
		}
		time.Sleep(1 * time.Second)
	}
}

// --- Message Processing ---

func (s *SignalChannel) processMessage(msg signalMessage) {
	env := &msg.Envelope

	// Skip non-data envelopes: receipts, sync messages, typing indicators, calls
	if env.ReceiptMessage != nil || env.SyncMessage != nil ||
		env.TypingMessage != nil || env.CallMessage != nil {
		return
	}
	if env.DataMessage == nil {
		return
	}

	dm := env.DataMessage
	sender := env.Source
	if sender == "" {
		sender = env.SourceNumber
	}
	text := dm.Message

	// Build attachment labels and collect media files
	var mediaFiles []channels.MediaFile
	for _, att := range dm.Attachments {
		label := "[Attachment]"
		switch {
		case strings.HasPrefix(att.ContentType, "image/"):
			label = "[Image attachment]"
		case strings.HasPrefix(att.ContentType, "audio/"):
			label = "[Audio attachment]"
		case strings.HasPrefix(att.ContentType, "video/"):
			label = "[Video attachment]"
		case att.Filename != "":
			label = fmt.Sprintf("[File: %s]", att.Filename)
		}
		if text != "" {
			text += " " + label
		} else {
			text = label
		}
		if att.ID != "" {
			if path, err := s.downloadAttachment(att.ID); err == nil {
				mediaFiles = append(mediaFiles, channels.MediaFile{Path: path, MimeType: att.ContentType})
			}
		}
	}

	if text == "" && len(mediaFiles) == 0 {
		return
	}

	chatID := sender
	peerKind := "direct"
	meta := map[string]string{
		"sender":    sender,
		"timestamp": fmt.Sprintf("%d", dm.Timestamp),
	}

	if dm.GroupInfo != nil && dm.GroupInfo.GroupID != "" {
		chatID = dm.GroupInfo.GroupID
		peerKind = "group"
		meta["group_id"] = dm.GroupInfo.GroupID
	}

	slog.Info("signal.inbound", "from", sender, "type", peerKind, "len", len(text))

	s.debouncer.Push(channels.InboundMessage{
		ChannelName: "signal",
		ChannelType: "signal",
		AgentID:     s.cfg.AgentID,
		SenderID:    sender,
		SenderName:  env.SourceName,
		ChatID:      chatID,
		Content:     strings.TrimSpace(text),
		PeerKind:    peerKind,
		Metadata:    meta,
		Media:       mediaFiles,
	})

	go s.sendReceipt(sender, dm.Timestamp)
}

// --- Send ---

func (s *SignalChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	body := map[string]any{
		"message": content,
		"number":  s.cfg.PhoneNumber,
	}

	// Group messages use group_id; DMs use recipients array
	groupID := ""
	if msg.Metadata != nil {
		groupID = msg.Metadata["group_id"]
	}
	if groupID != "" {
		body["recipients"] = []string{}
		body["group_id"] = groupID
	} else {
		to := msg.RecipientID
		if to == "" {
			to = msg.ChatID
		}
		body["recipients"] = []string{to}
	}

	if len(msg.Media) > 0 {
		var attachments []string
		for _, m := range msg.Media {
			if m.URL != "" {
				attachments = append(attachments, m.URL)
			}
		}
		if len(attachments) > 0 {
			body["base64_attachments"] = attachments
		}
	}

	payload, _ := json.Marshal(body)
	resp, err := s.client.Post(s.cfg.APIURL+"/v2/send", "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("signal send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("signal send %d: %s", resp.StatusCode, b)
	}
	return nil
}

// SendTyping sends a typing indicator via PUT /v1/typing-indicator/{number}.
func (s *SignalChannel) SendTyping(recipient string) {
	payload, _ := json.Marshal(map[string]any{"recipient": recipient})
	encodedPhone := url.PathEscape(s.cfg.PhoneNumber)
	req, _ := http.NewRequest("PUT", s.cfg.APIURL+"/v1/typing-indicator/"+encodedPhone, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// sendReceipt sends a read receipt for a received message.
// Endpoint: POST /v1/receipts/{number}/{receipt_type}
func (s *SignalChannel) sendReceipt(sender string, timestamp int64) {
	if timestamp == 0 {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"recipient":  sender,
		"timestamps": []int64{timestamp},
	})
	encodedPhone := url.PathEscape(s.cfg.PhoneNumber)
	resp, err := s.client.Post(
		s.cfg.APIURL+"/v1/receipts/"+encodedPhone+"/read",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// downloadAttachment downloads a signal-cli attachment by ID to a temp file.
func (s *SignalChannel) downloadAttachment(attachmentID string) (string, error) {
	resp, err := s.client.Get(fmt.Sprintf("%s/v1/attachments/%s", s.cfg.APIURL, attachmentID))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	tmpFile := filepath.Join(os.TempDir(), "signal-att-"+attachmentID)
	f, err := os.Create(tmpFile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	io.Copy(f, resp.Body)
	return tmpFile, nil
}

// --- Types ---

type signalMessage struct {
	Envelope signalEnvelope `json:"envelope"`
	Account  string         `json:"account"`
}

type signalEnvelope struct {
	Source         string           `json:"source"`
	SourceNumber   string           `json:"sourceNumber"`
	SourceUuid     string           `json:"sourceUuid"`
	SourceName     string           `json:"sourceName"`
	SourceDevice   int              `json:"sourceDevice"`
	Timestamp      int64            `json:"timestamp"`
	DataMessage    *dataMessage     `json:"dataMessage"`
	SyncMessage    *json.RawMessage `json:"syncMessage"`
	ReceiptMessage *json.RawMessage `json:"receiptMessage"`
	TypingMessage  *json.RawMessage `json:"typingMessage"`
	CallMessage    *json.RawMessage `json:"callMessage"`
}

type dataMessage struct {
	Message     string       `json:"message"`
	Timestamp   int64        `json:"timestamp"`
	GroupInfo   *groupInfo   `json:"groupInfo"`
	Attachments []attachment `json:"attachments"`
}

type groupInfo struct {
	GroupID string `json:"groupId"`
	Type    string `json:"type"`
}

type attachment struct {
	ID          string `json:"id"`
	ContentType string `json:"contentType"`
	Filename    string `json:"filename"`
	Size        int    `json:"size"`
}
