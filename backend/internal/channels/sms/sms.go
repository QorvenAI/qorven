// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package sms

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

// Twilio SMS/MMS channel.
// Inbound: HTTPS webhook with HMAC-SHA1 signature (unique to Twilio — not SHA256).
// Outbound: REST API with application/x-www-form-urlencoded (not JSON).
// Reference: https://www.twilio.com/docs/messaging/api

const (
	twilioAPIBase = "https://api.twilio.com/2010-04-01"
)

type Config struct {
	AgentID       string `json:"agent_id"`
	AccountSID    string `json:"account_sid"`      // AC... — always required for API URL
	AuthToken     string `json:"auth_token"`       // for signature verification + dev auth
	ApiKeySid     string `json:"api_key_sid"`      // SK... — use in production instead of AuthToken
	ApiKeySecret  string `json:"api_key_secret"`   // paired with ApiKeySid
	FromNumber    string `json:"from_number"`      // +E.164 format
	MessagingServiceSid string `json:"messaging_service_sid"` // optional; overrides FromNumber for pooled sending
}

type SMSChannel struct {
	cfg       Config
	handler   channels.InboundHandler
	running   bool
	mu        sync.Mutex
	client    *http.Client
	debouncer *channels.Debouncer
	dedup     sync.Map // MessageSid → time.Time
}

func New(cfg Config, handler channels.InboundHandler) *SMSChannel {
	ch := &SMSChannel{
		cfg:     cfg,
		handler: handler,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
	ch.debouncer = channels.NewDebouncer(600*time.Millisecond, func(msg channels.InboundMessage) {
		if handler != nil {
			handler(context.Background(), msg)
		}
	})
	return ch
}

func (s *SMSChannel) Name() string    { return fmt.Sprintf("sms:%s", s.cfg.FromNumber) }
func (s *SMSChannel) Type() string    { return "sms" }
func (s *SMSChannel) AgentID() string { return s.cfg.AgentID }
func (s *SMSChannel) IsRunning() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.running }

func (s *SMSChannel) Start(_ context.Context) error {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()
	slog.Info("sms.started", "from", s.cfg.FromNumber)
	return nil
}

func (s *SMSChannel) Stop(_ context.Context) error {
	if s.debouncer != nil {
		s.debouncer.FlushAll()
	}
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	return nil
}

// --- Signature Verification ---

// verifySignature verifies the X-Twilio-Signature header.
// Twilio uses HMAC-SHA1 (not SHA256) with a unique input construction:
//
//	input = fullURL + sorted(params[0].key + params[0].value + params[1].key + ...)
//	signature = base64(HMAC-SHA1(authToken, input))
func (s *SMSChannel) verifySignature(r *http.Request, fullURL string) bool {
	if s.cfg.AuthToken == "" {
		return true
	}
	sig := r.Header.Get("X-Twilio-Signature")
	if sig == "" {
		return false
	}

	r.ParseForm()

	// Build sorted key+value string appended directly to the URL — no separators
	keys := make([]string, 0, len(r.PostForm))
	for k := range r.PostForm {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(fullURL)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(r.PostForm.Get(k))
	}

	mac := hmac.New(sha1.New, []byte(s.cfg.AuthToken))
	mac.Write([]byte(sb.String()))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig))
}

// --- Webhook Handler ---

func (s *SMSChannel) HandleWebhook(rw http.ResponseWriter, r *http.Request) {
	// Verify signature using the full request URL
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	fullURL := scheme + "://" + r.Host + r.RequestURI

	if !s.verifySignature(r, fullURL) {
		slog.Warn("sms.webhook.signature_invalid")
		http.Error(rw, "invalid signature", http.StatusForbidden)
		return
	}

	r.ParseForm()

	from := r.FormValue("From")
	to := r.FormValue("To")
	body := r.FormValue("Body")
	msgSid := r.FormValue("MessageSid")

	if from == "" {
		rw.Header().Set("Content-Type", "text/xml")
		rw.Write([]byte("<Response></Response>"))
		return
	}

	// Dedup by MessageSid — Twilio retries with exponential backoff on non-200
	if msgSid != "" {
		if _, loaded := s.dedup.LoadOrStore(msgSid, time.Now()); loaded {
			rw.Header().Set("Content-Type", "text/xml")
			rw.Write([]byte("<Response></Response>"))
			return
		}
		go func() { time.Sleep(2 * time.Hour); s.dedup.Delete(msgSid) }()
	}

	// Check opt-out keywords — acknowledge but don't forward to agent
	if isOptOut(body) {
		slog.Info("sms.opt_out", "from", from)
		rw.Header().Set("Content-Type", "text/xml")
		rw.Write([]byte("<Response></Response>"))
		return
	}

	// Build inbound message
	content := strings.TrimSpace(body)
	numMedia := r.FormValue("NumMedia")
	if content == "" && numMedia == "0" {
		rw.Header().Set("Content-Type", "text/xml")
		rw.Write([]byte("<Response></Response>"))
		return
	}

	meta := map[string]string{
		"from":        from,
		"to":          to,
		"message_sid": msgSid,
	}

	// Handle MMS attachments (NumMedia > 0)
	if numMedia != "" && numMedia != "0" {
		mediaLabels := buildMediaLabels(r.PostForm)
		if content == "" {
			content = strings.Join(mediaLabels, " ")
		} else {
			content = content + " " + strings.Join(mediaLabels, " ")
		}
		meta["has_media"] = "true"
	}

	if content == "" {
		rw.Header().Set("Content-Type", "text/xml")
		rw.Write([]byte("<Response></Response>"))
		return
	}

	slog.Info("sms.inbound", "from", from, "sid", msgSid)

	// Return empty ack immediately — agent processes async, replies via REST API
	// (not via TwiML inline, since LLM response time > Twilio's timeout)
	rw.Header().Set("Content-Type", "text/xml")
	rw.Write([]byte("<Response></Response>"))

	s.debouncer.Push(channels.InboundMessage{
		ChannelName: s.Name(),
		ChannelType: "sms",
		AgentID:     s.cfg.AgentID,
		SenderID:    from,
		ChatID:      from, // SMS chat ID is the sender's phone number
		Content:     content,
		Metadata:    meta,
	})
}

// buildMediaLabels returns human-readable labels for each MMS attachment.
func buildMediaLabels(form url.Values) []string {
	var labels []string
	for i := 0; ; i++ {
		contentType := form.Get(fmt.Sprintf("MediaContentType%d", i))
		if contentType == "" {
			break
		}
		mediaURL := form.Get(fmt.Sprintf("MediaUrl%d", i))
		switch {
		case strings.HasPrefix(contentType, "image/"):
			labels = append(labels, "[Image attachment]")
		case strings.HasPrefix(contentType, "audio/"):
			labels = append(labels, "[Audio attachment]")
		case strings.HasPrefix(contentType, "video/"):
			labels = append(labels, "[Video attachment]")
		default:
			labels = append(labels, fmt.Sprintf("[File attachment: %s]", contentType))
		}
		_ = mediaURL // URL available for download; not auto-fetched here
	}
	return labels
}

// --- Delivery Status Webhook ---

func (s *SMSChannel) HandleStatusWebhook(rw http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	sid := r.FormValue("MessageSid")
	status := r.FormValue("MessageStatus") // queued, sent, delivered, undelivered, failed
	to := r.FormValue("To")
	slog.Info("sms.status", "sid", sid, "status", status, "to", to)
	rw.Header().Set("Content-Type", "text/xml")
	rw.Write([]byte("<Response></Response>"))
}

// --- Outbound Send ---

func (s *SMSChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	to := msg.RecipientID
	if to == "" {
		to = msg.ChatID
	}
	if to == "" && msg.Metadata != nil {
		to = msg.Metadata["from"]
	}
	if to == "" {
		return fmt.Errorf("sms: no recipient")
	}

	return s.sendSMS(to, content, "")
}

func (s *SMSChannel) sendSMS(to, body, mediaURL string) error {
	apiURL := fmt.Sprintf("%s/Accounts/%s/Messages.json", twilioAPIBase, s.cfg.AccountSID)

	data := url.Values{
		"To":   {to},
		"Body": {body},
	}
	if s.cfg.MessagingServiceSid != "" {
		data.Set("MessagingServiceSid", s.cfg.MessagingServiceSid)
	} else {
		data.Set("From", s.cfg.FromNumber)
	}
	if mediaURL != "" {
		data.Set("MediaUrl", mediaURL)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("sms: request: %w", err)
	}
	// Use API Key + Secret in production; fall back to AccountSID + AuthToken
	if s.cfg.ApiKeySid != "" && s.cfg.ApiKeySecret != "" {
		req.SetBasicAuth(s.cfg.ApiKeySid, s.cfg.ApiKeySecret)
	} else {
		req.SetBasicAuth(s.cfg.AccountSID, s.cfg.AuthToken)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("sms: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twilio: HTTP %d: %s", resp.StatusCode, string(b))
	}
	slog.Info("sms.sent", "to", to)
	return nil
}

// SendMMS sends an SMS message with a media attachment.
func (s *SMSChannel) SendMMS(to, body, mediaURL string) error {
	return s.sendSMS(to, body, mediaURL)
}

// --- Opt-Out Handling ---

var optOutKeywords = []string{"STOP", "UNSUBSCRIBE", "CANCEL", "END", "QUIT"}

func isOptOut(body string) bool {
	upper := strings.ToUpper(strings.TrimSpace(body))
	for _, kw := range optOutKeywords {
		if upper == kw {
			return true
		}
	}
	return false
}
