// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/channels"
)

// Matrix channel — /sync long-poll for inbound, PUT /send for outbound.

const (
	maxEventBytes = 60000 // leave headroom under Synapse's 65536-byte event limit
)

type Config struct {
	HomeserverURL string `json:"homeserver_url"`
	AccessToken   string `json:"access_token"`
	UserID        string `json:"user_id"` // e.g. @qorven-bot:server.com
	AgentID       string `json:"agent_id"`
	Since         string `json:"since"` // pre-populated from DB on startup; persisted by caller via CurrentSince()
}

type MatrixChannel struct {
	cfg     Config
	handler channels.InboundHandler
	client  *http.Client
	mu      sync.Mutex
	since   string
	running bool
	cancel  context.CancelFunc
	txnSeq  int64
}

func New(cfg Config, handler channels.InboundHandler) *MatrixChannel {
	cfg.HomeserverURL = strings.TrimRight(cfg.HomeserverURL, "/")
	return &MatrixChannel{
		cfg:    cfg,
		since:  cfg.Since,
		handler: handler,
		client: &http.Client{Timeout: 45 * time.Second},
	}
}

func (m *MatrixChannel) Name() string    { return fmt.Sprintf("matrix:%s", m.cfg.UserID) }
func (m *MatrixChannel) Type() string    { return "matrix" }
func (m *MatrixChannel) AgentID() string { return m.cfg.AgentID }
func (m *MatrixChannel) IsRunning() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.running }

// CurrentSince returns the latest next_batch token. Callers should persist this to DB
// and pass it back in Config.Since on next startup to avoid replaying historical events.
func (m *MatrixChannel) CurrentSince() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.since
}

func (m *MatrixChannel) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Lock()
	m.running = true
	m.mu.Unlock()
	go m.syncLoop(ctx)
	slog.Info("matrix.started", "homeserver", m.cfg.HomeserverURL, "user", m.cfg.UserID)
	return nil
}

func (m *MatrixChannel) Stop(_ context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
	return nil
}

// --- Sync Loop ---

func (m *MatrixChannel) syncLoop(ctx context.Context) {
	backoff := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		m.mu.Lock()
		since := m.since
		m.mu.Unlock()

		url := fmt.Sprintf("%s/_matrix/client/v3/sync?timeout=30000", m.cfg.HomeserverURL)
		if since != "" {
			url += "&since=" + since
		}

		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+m.cfg.AccessToken)

		resp, err := m.client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("matrix.sync.error", "error", err, "retry_in", backoff)
			time.Sleep(backoff)
			if backoff < 60*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = 2 * time.Second

		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 401 {
			slog.Error("matrix.sync.unauthorized", "homeserver", m.cfg.HomeserverURL)
			time.Sleep(30 * time.Second)
			continue
		}
		if resp.StatusCode >= 400 {
			slog.Warn("matrix.sync.http_error", "status", resp.StatusCode)
			time.Sleep(backoff)
			continue
		}

		var syncResp syncResponse
		if err := json.Unmarshal(data, &syncResp); err != nil {
			continue
		}

		// Auto-join pending invites
		for roomID := range syncResp.Rooms.Invite {
			m.joinRoom(ctx, roomID)
		}

		// Process joined-room messages
		for roomID, room := range syncResp.Rooms.Join {
			for _, evt := range room.Timeline.Events {
				m.handleEvent(ctx, roomID, evt)
			}
		}

		m.mu.Lock()
		m.since = syncResp.NextBatch
		m.mu.Unlock()
	}
}

func (m *MatrixChannel) handleEvent(ctx context.Context, roomID string, evt syncEvent) {
	if evt.Type != "m.room.message" {
		return
	}
	// Skip own messages
	if evt.Sender == m.cfg.UserID {
		return
	}
	// Skip notices (bot-originated messages — avoid loops)
	if evt.Content.MsgType == "m.notice" {
		return
	}
	if evt.Content.Body == "" {
		return
	}

	slog.Debug("matrix.inbound", "from", evt.Sender, "room", roomID)
	if m.handler != nil {
		m.handler(ctx, channels.InboundMessage{
			ChannelName: m.Name(),
			ChannelType: "matrix",
			AgentID:     m.cfg.AgentID,
			SenderID:    evt.Sender,
			SenderName:  evt.Sender,
			ChatID:      roomID,
			Content:     evt.Content.Body,
			Metadata: map[string]string{
				"room_id":  roomID,
				"event_id": evt.EventID,
			},
		})
	}
}

func (m *MatrixChannel) joinRoom(ctx context.Context, roomID string) {
	url := fmt.Sprintf("%s/_matrix/client/v3/join/%s", m.cfg.HomeserverURL, roomID)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte("{}")))
	req.Header.Set("Authorization", "Bearer "+m.cfg.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		slog.Warn("matrix.join.error", "room", roomID, "error", err)
		return
	}
	resp.Body.Close()
	slog.Info("matrix.join", "room", roomID, "status", resp.StatusCode)
}

// --- Send ---

func (m *MatrixChannel) Send(ctx context.Context, msg channels.OutboundMessage) error {
	roomID := msg.ChatID
	if roomID == "" {
		roomID = msg.RecipientID
	}
	if roomID == "" {
		return fmt.Errorf("matrix send: no room ID in ChatID or RecipientID")
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	// Send in chunks if content exceeds event size limit
	for _, chunk := range splitMessage(content, maxEventBytes/3) { // /3 for JSON overhead + both body and formatted_body
		if err := m.sendMessage(ctx, roomID, chunk, msg.Metadata); err != nil {
			return err
		}
	}
	return nil
}

func (m *MatrixChannel) sendMessage(ctx context.Context, roomID, text string, meta map[string]string) error {
	m.mu.Lock()
	m.txnSeq++
	seq := m.txnSeq
	m.mu.Unlock()

	txnID := fmt.Sprintf("%d-%d", time.Now().UnixMilli(), seq)

	// Send as m.notice (bot convention) with both plain and HTML bodies
	payload := map[string]string{
		"msgtype":        "m.notice",
		"body":           text,
		"format":         "org.matrix.custom.html",
		"formatted_body": markdownToMatrixHTML(text),
	}

	// Thread reply if event_id present in metadata
	if meta != nil {
		if eventID, ok := meta["event_id"]; ok && eventID != "" {
			payloadWithRelation := map[string]any{
				"msgtype":        "m.notice",
				"body":           fmt.Sprintf("> <reply>\n\n%s", text),
				"format":         "org.matrix.custom.html",
				"formatted_body": markdownToMatrixHTML(text),
				"m.relates_to": map[string]any{
					"m.in_reply_to": map[string]string{"event_id": eventID},
				},
			}
			return m.putEvent(ctx, roomID, txnID, payloadWithRelation)
		}
	}

	return m.putEvent(ctx, roomID, txnID, payload)
}

func (m *MatrixChannel) putEvent(ctx context.Context, roomID, txnID string, payload any) error {
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		m.cfg.HomeserverURL, roomID, txnID)

	req, _ := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+m.cfg.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("matrix PUT %d: %s", resp.StatusCode, b)
	}
	return nil
}

// --- Helpers ---

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

// markdownToMatrixHTML converts basic markdown to the HTML subset Matrix clients render.
var (
	mxBoldRe      = regexp.MustCompile(`\*\*(.+?)\*\*`)
	mxItalicRe    = regexp.MustCompile(`(?:^|[^*])\*([^*\n]+)\*`)
	mxStrikeRe    = regexp.MustCompile(`~~(.+?)~~`)
	mxCodeRe      = regexp.MustCompile("`([^`]+)`")
	mxCodeBlockRe = regexp.MustCompile("(?s)```(?:\\w*)\\n?(.*?)```")
	mxLinkRe      = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	mxHeadingRe   = regexp.MustCompile(`(?m)^#{1,3}\s+(.+)$`)
)

func markdownToMatrixHTML(md string) string {
	// Code blocks first (protect from other transforms)
	out := mxCodeBlockRe.ReplaceAllString(md, "<pre><code>$1</code></pre>")
	out = mxCodeRe.ReplaceAllString(out, "<code>$1</code>")
	out = mxBoldRe.ReplaceAllString(out, "<strong>$1</strong>")
	out = mxStrikeRe.ReplaceAllString(out, "<del>$1</del>")
	out = mxLinkRe.ReplaceAllString(out, `<a href="$2">$1</a>`)
	out = mxHeadingRe.ReplaceAllString(out, "<strong>$1</strong>")
	// Newlines to <br> (outside pre blocks)
	out = strings.ReplaceAll(out, "\n", "<br>")
	return out
}

// --- Types ---

type syncResponse struct {
	NextBatch string `json:"next_batch"`
	Rooms     struct {
		Join   map[string]joinedRoom `json:"join"`
		Invite map[string]any        `json:"invite"`
	} `json:"rooms"`
}

type joinedRoom struct {
	Timeline struct {
		Events []syncEvent `json:"events"`
	} `json:"timeline"`
}

type syncEvent struct {
	Type    string `json:"type"`
	EventID string `json:"event_id"`
	Sender  string `json:"sender"`
	Content struct {
		Body    string `json:"body"`
		MsgType string `json:"msgtype"`
	} `json:"content"`
}
