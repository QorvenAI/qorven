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
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"

	"github.com/qorvenai/qorven/internal/channels"
)

const (
	cloudAPIBase  = "https://graph.facebook.com/v21.0"
	maxMessageLen = 4096
)

// Config for WhatsApp channel.
type Config struct {
	AgentID     string   `json:"agent_id"`
	Mode        string   `json:"mode"` // "cloud" or "qr"
	DMPolicy    string   `json:"dm_policy"`
	GroupPolicy string   `json:"group_policy"`
	AllowFrom   []string `json:"allow_from"`

	// Cloud API mode
	PhoneNumberID string `json:"phone_number_id"`
	AccessToken   string `json:"access_token"`
	VerifyToken   string `json:"verify_token"`
	AppSecret     string `json:"app_secret"`

	// QR mode (in-process whatsmeow)
	DBDSN string `json:"-"`
}

// WhatsAppChannel supports Cloud API and in-process QR modes.
type WhatsAppChannel struct {
	cfg             Config
	handler   channels.InboundHandler
	running   bool
	mu        sync.Mutex
	client    *http.Client
	allowList []string
	dedup           sync.Map // msgID → time.Time — prevents double-fire on platform retries

	ctx    context.Context
	cancel context.CancelFunc

	// Optional STT
	Transcribe func(ctx context.Context, audio []byte, format string) (string, error)

	// QR fan-out hub (mode-independent; used by the SSE endpoint)
	qrMu     sync.Mutex
	qrSubs   map[int]func(string)
	qrNextID int
	lastQR   string

	// Connected fan-out hub — fires once when QR pairing succeeds
	connMu     sync.Mutex
	connSubs   map[int]func()
	connNextID int

	// whatsmeow (qr mode)
	wmClient    *whatsmeow.Client
	wmContainer *sqlstore.Container
}

func New(cfg Config, handler channels.InboundHandler) *WhatsAppChannel {
	if cfg.Mode == "" {
		cfg.Mode = "cloud"
	}
	ch := &WhatsAppChannel{
		cfg:       cfg,
		handler:   handler,
		client:    &http.Client{Timeout: 30 * time.Second},
		allowList: cfg.AllowFrom,
	}
	return ch
}

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

	if w.cfg.Mode == "qr" {
		if err := w.startWhatsmeow(w.ctx); err != nil {
			return fmt.Errorf("whatsapp qr start: %w", err)
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

	if w.cfg.Mode == "qr" {
		w.stopWhatsmeow()
	}

	w.mu.Lock()
	w.running = false
	w.mu.Unlock()

	slog.Info("whatsapp.stopped")
	return nil
}

func (w *WhatsAppChannel) Send(ctx context.Context, msg channels.OutboundMessage) error {
	if w.cfg.Mode == "qr" {
		recipient := msg.ChatID
		if recipient == "" {
			recipient = msg.RecipientID
		}
		return w.sendWhatsmeow(ctx, recipient, msg.Content)
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


// SubscribeQREvents registers a callback that fires on every new QR code.
// The callback is also fired immediately if a QR is already cached (replay for
// late subscribers). Returns an unsubscribe function.
func (w *WhatsAppChannel) SubscribeQREvents(fn func(string)) func() {
	w.qrMu.Lock()
	if w.qrSubs == nil {
		w.qrSubs = map[int]func(string){}
	}
	id := w.qrNextID
	w.qrNextID++
	w.qrSubs[id] = fn
	last := w.lastQR
	w.qrMu.Unlock()
	if last != "" {
		go fn(last)
	}
	return func() { w.qrMu.Lock(); delete(w.qrSubs, id); w.qrMu.Unlock() }
}

// RequestLatestQR re-broadcasts the most-recently cached QR to all subscribers.
func (w *WhatsAppChannel) RequestLatestQR() {
	w.qrMu.Lock()
	last := w.lastQR
	subs := make([]func(string), 0, len(w.qrSubs))
	for _, fn := range w.qrSubs {
		subs = append(subs, fn)
	}
	w.qrMu.Unlock()
	if last == "" {
		return
	}
	for _, fn := range subs {
		go fn(last)
	}
}

// publishQR stores a new QR code and broadcasts it to all subscribers.
func (w *WhatsAppChannel) publishQR(code string) {
	w.qrMu.Lock()
	w.lastQR = code
	subs := make([]func(string), 0, len(w.qrSubs))
	for _, fn := range w.qrSubs {
		subs = append(subs, fn)
	}
	w.qrMu.Unlock()
	for _, fn := range subs {
		go fn(code)
	}
}

// SubscribeConnectedEvents registers a callback that fires once when QR pairing
// succeeds. Returns an unsubscribe function.
func (w *WhatsAppChannel) SubscribeConnectedEvents(fn func()) func() {
	w.connMu.Lock()
	if w.connSubs == nil {
		w.connSubs = map[int]func(){}
	}
	id := w.connNextID
	w.connNextID++
	w.connSubs[id] = fn
	w.connMu.Unlock()
	return func() { w.connMu.Lock(); delete(w.connSubs, id); w.connMu.Unlock() }
}

// publishConnected fires all connected-event subscribers.
// Called when whatsmeow reports a successful QR pairing.
func (w *WhatsAppChannel) publishConnected() {
	w.connMu.Lock()
	subs := make([]func(), 0, len(w.connSubs))
	for _, fn := range w.connSubs {
		subs = append(subs, fn)
	}
	w.connMu.Unlock()
	for _, fn := range subs {
		go fn()
	}
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
// (up to 3 buttons).
func (w *WhatsAppChannel) SendInteractiveButtons(ctx context.Context, to, bodyText string, buttons []string) error {
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
func (w *WhatsAppChannel) SendInteractiveList(ctx context.Context, to, bodyText, buttonLabel string, items []string) error {
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
