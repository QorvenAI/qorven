// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/qorvenai/qorven/internal/channels"
)

// Mattermost channel — connects via WebSocket for real-time events + REST API for sending.

const (
	maxMessageLen = 16383 // Mattermost message field hard limit
	maxFileBytes  = 50 * 1024 * 1024 // default MaxFileSize per Mattermost docs
)

type Config struct {
	AgentID        string `json:"agent_id"`
	ServerURL      string `json:"server_url"`     // e.g. https://mattermost.example.com
	BotToken       string `json:"bot_token"`       // bot or personal access token
	TeamID         string `json:"team_id"`
	RequireMention bool   `json:"require_mention"` // only respond when @mentioned
}

type MattermostChannel struct {
	cfg         Config
	handler     channels.InboundHandler
	running     bool
	cancel      context.CancelFunc
	mu          sync.Mutex
	client      *http.Client
	ws          *websocket.Conn
	debouncer   *channels.Debouncer
	botUserID   string
	botName     string
	lastSeenAt  atomic.Int64 // Unix ms — for missed-message replay on reconnect
	userCache   sync.Map     // userID → displayName
}

func New(cfg Config, handler channels.InboundHandler) *MattermostChannel {
	// Strip trailing slash — all paths are prefixed with /api/v4/...
	cfg.ServerURL = strings.TrimRight(cfg.ServerURL, "/")
	return &MattermostChannel{cfg: cfg, handler: handler, client: &http.Client{Timeout: 30 * time.Second}}
}

func (m *MattermostChannel) Name() string    { return "mattermost" }
func (m *MattermostChannel) Type() string    { return "mattermost" }
func (m *MattermostChannel) AgentID() string { return m.cfg.AgentID }
func (m *MattermostChannel) IsRunning() bool { m.mu.Lock(); defer m.mu.Unlock(); return m.running }

func (m *MattermostChannel) Start(ctx context.Context) error {
	// Get bot user info
	me, err := m.apiGet("/api/v4/users/me")
	if err != nil {
		return fmt.Errorf("mattermost auth: %w", err)
	}
	var user struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	json.Unmarshal(me, &user)
	m.botUserID = user.ID
	m.botName = user.Username

	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.debouncer = channels.NewDebouncer(600*time.Millisecond, func(msg channels.InboundMessage) {
		if m.handler != nil {
			m.handler(ctx, msg)
		}
	})
	m.mu.Lock()
	m.running = true
	m.mu.Unlock()

	go m.wsLoop(ctx)
	slog.Info("mattermost.started", "user", m.botName, "id", m.botUserID, "agent", m.cfg.AgentID)
	return nil
}

func (m *MattermostChannel) Stop(_ context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	if m.debouncer != nil {
		m.debouncer.FlushAll()
	}
	m.mu.Lock()
	if m.ws != nil {
		m.ws.Close()
	}
	m.running = false
	m.mu.Unlock()
	return nil
}

// --- WebSocket Event Loop ---

func (m *MattermostChannel) wsLoop(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := m.wsConnect(ctx)
		if err != nil {
			slog.Warn("mattermost.ws.error", "error", err, "retry_in", backoff)
			time.Sleep(backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
	}
}

func (m *MattermostChannel) wsConnect(ctx context.Context) error {
	wsURL := strings.Replace(m.cfg.ServerURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/v4/websocket"

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("ws dial: %w", err)
	}

	m.mu.Lock()
	m.ws = conn
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.ws = nil
		m.mu.Unlock()
		conn.Close()
	}()

	// Authenticate
	authMsg, _ := json.Marshal(map[string]any{
		"seq":    1,
		"action": "authentication_challenge",
		"data":   map[string]string{"token": m.cfg.BotToken},
	})
	conn.WriteMessage(websocket.TextMessage, authMsg)

	// Read auth response — server sends status:OK or closes connection
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, authResp, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("ws auth read: %w", err)
	}
	var authResult struct {
		Status string `json:"status"`
	}
	json.Unmarshal(authResp, &authResult)
	if authResult.Status != "" && authResult.Status != "OK" {
		return fmt.Errorf("ws auth failed: %s", authResult.Status)
	}

	slog.Info("mattermost.ws.connected", "url", wsURL)

	// Replay missed posts since last disconnect
	if since := m.lastSeenAt.Load(); since > 0 {
		m.replayMissedPosts(ctx, since)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("ws read: %w", err)
		}

		var event wsEvent
		json.Unmarshal(msg, &event)

		switch event.Event {
		case "posted":
			m.handlePosted(ctx, event)
		case "hello":
			// Server hello on connect — contains server_version, no action needed
		}

		m.lastSeenAt.Store(time.Now().UnixMilli())
	}
}

// replayMissedPosts fetches posts created since last seen timestamp and re-delivers them.
func (m *MattermostChannel) replayMissedPosts(ctx context.Context, sinceMs int64) {
	data, err := m.apiGet(fmt.Sprintf("/api/v4/posts?since=%d", sinceMs))
	if err != nil {
		slog.Warn("mattermost.replay.error", "error", err)
		return
	}
	var result struct {
		Order []string                    `json:"order"`
		Posts map[string]mattermostPost   `json:"posts"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return
	}
	for _, id := range result.Order {
		post, ok := result.Posts[id]
		if !ok || post.UserID == m.botUserID || post.Type != "" {
			continue
		}
		fakeEvent := wsEvent{
			Event:     "posted",
			Broadcast: map[string]any{"channel_id": post.ChannelID},
		}
		postJSON, _ := json.Marshal(post)
		fakeEvent.Data = map[string]any{"post": string(postJSON)}
		m.handlePosted(ctx, fakeEvent)
	}
}

func (m *MattermostChannel) handlePosted(ctx context.Context, event wsEvent) {
	var post mattermostPost
	if raw, ok := event.Data["post"].(string); ok {
		json.Unmarshal([]byte(raw), &post)
	}

	if post.UserID == m.botUserID {
		return
	}
	// Skip system messages (type is empty for regular user messages)
	if post.Type != "" {
		return
	}

	text := post.Message
	if text == "" {
		return
	}

	// Mention gating
	if m.cfg.RequireMention {
		if !strings.Contains(text, "@"+m.botName) && !strings.Contains(text, m.botUserID) {
			return
		}
		text = strings.ReplaceAll(text, "@"+m.botName, "")
		text = strings.TrimSpace(text)
	}

	channelID, _ := event.Broadcast["channel_id"].(string)
	if channelID == "" {
		channelID = post.ChannelID
	}

	senderName := m.resolveUserName(post.UserID)

	meta := map[string]string{
		"channel_id": channelID,
		"post_id":    post.ID,
	}
	if post.RootID != "" {
		meta["thread_id"] = post.RootID
	}

	slog.Info("mattermost.inbound", "from", post.UserID, "channel", channelID)
	m.debouncer.Push(channels.InboundMessage{
		ChannelName: "mattermost",
		ChannelType: "mattermost",
		AgentID:     m.cfg.AgentID,
		SenderID:    post.UserID,
		SenderName:  senderName,
		ChatID:      channelID,
		Content:     text,
		Metadata:    meta,
	})
}

func (m *MattermostChannel) resolveUserName(userID string) string {
	if cached, ok := m.userCache.Load(userID); ok {
		return cached.(string)
	}
	data, err := m.apiGet("/api/v4/users/" + userID)
	if err != nil {
		return userID
	}
	var user struct {
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}
	json.Unmarshal(data, &user)
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name == "" {
		name = user.Username
	}
	if name == "" {
		name = userID
	}
	m.userCache.Store(userID, name)
	return name
}

// --- Send ---

func (m *MattermostChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	channelID := msg.RecipientID
	if msg.Metadata != nil {
		if cid, ok := msg.Metadata["channel_id"]; ok {
			channelID = cid
		}
	}

	var rootID string
	if msg.Metadata != nil {
		if tid, ok := msg.Metadata["thread_id"]; ok && tid != "" {
			rootID = tid
		} else if pid, ok := msg.Metadata["post_id"]; ok && pid != "" {
			rootID = pid
		}
	}

	// Attach files if present
	var fileIDs []string
	if len(msg.Media) > 0 {
		for _, media := range msg.Media {
			if fid, err := m.uploadFile(channelID, media.URL); err == nil {
				fileIDs = append(fileIDs, fid)
			}
		}
	}

	// Split and send
	chunks := splitMessage(content, maxMessageLen)
	for i, chunk := range chunks {
		post := map[string]any{
			"channel_id": channelID,
			"message":    chunk,
		}
		if rootID != "" {
			post["root_id"] = rootID
		}
		if i == len(chunks)-1 && len(fileIDs) > 0 {
			post["file_ids"] = fileIDs
		}
		if err := m.apiPost("/api/v4/posts", post); err != nil {
			return err
		}
	}
	return nil
}

func (m *MattermostChannel) StreamEnabled(isGroup bool) bool { return true }
func (m *MattermostChannel) ReasoningStreamEnabled() bool    { return false }

func (m *MattermostChannel) CreateStream(ctx context.Context, chatID string, firstStream bool) (channels.ChannelStream, error) {
	placeholder := "⏳ Thinking..."
	if !firstStream {
		placeholder = "..."
	}
	post := map[string]any{"channel_id": chatID, "message": placeholder}
	data, err := m.apiPostWithResponse("/api/v4/posts", post)
	if err != nil {
		return nil, err
	}
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(data, &created)
	if created.ID == "" {
		return nil, fmt.Errorf("mattermost: no post ID in stream response")
	}
	return &mmStream{ch: m, channelID: chatID, postID: created.ID}, nil
}

func (m *MattermostChannel) FinalizeStream(_ context.Context, _ string, _ channels.ChannelStream) {}

type mmStream struct {
	ch        *MattermostChannel
	channelID string
	postID    string
	lastEdit  time.Time
	lastText  string
	mu        sync.Mutex
	stopped   bool
}

func (s *mmStream) Update(_ context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || text == s.lastText {
		return nil
	}
	if time.Since(s.lastEdit) < time.Second {
		return nil
	}
	display := text
	if len(display) > maxMessageLen {
		display = display[len(display)-maxMessageLen:]
	}
	s.ch.apiPut("/api/v4/posts/"+s.postID, map[string]any{"id": s.postID, "message": display + " ▌"})
	s.lastEdit = time.Now()
	s.lastText = text
	return nil
}

func (s *mmStream) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	if s.lastText != "" {
		s.ch.apiPut("/api/v4/posts/"+s.postID, map[string]any{"id": s.postID, "message": s.lastText})
	}
	return nil
}

func (s *mmStream) MessageID() string { return s.postID }

// SendTyping sends a typing indicator to a channel.
func (m *MattermostChannel) SendTyping(channelID string) {
	m.mu.Lock()
	ws := m.ws
	m.mu.Unlock()
	if ws == nil {
		return
	}

	msg, _ := json.Marshal(map[string]any{
		"action": "user_typing",
		"data":   map[string]string{"channel_id": channelID, "parent_id": ""},
	})
	ws.WriteMessage(websocket.TextMessage, msg)
}

// AddReaction adds an emoji reaction to a post.
func (m *MattermostChannel) AddReaction(postID, emoji string) error {
	return m.apiPost("/api/v4/reactions", map[string]any{
		"user_id": m.botUserID, "post_id": postID, "emoji_name": emoji,
	})
}

// --- File Upload ---

func (m *MattermostChannel) uploadFile(channelID, filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, _ := f.Stat()
	if info != nil && info.Size() > maxFileBytes {
		return "", fmt.Errorf("file too large: %d bytes (limit %d)", info.Size(), maxFileBytes)
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("channel_id", channelID)
	part, _ := w.CreateFormFile("files", filepath.Base(filePath))
	io.Copy(part, f)
	w.Close()

	req, _ := http.NewRequest("POST", m.cfg.ServerURL+"/api/v4/files", &buf)
	req.Header.Set("Authorization", "Bearer "+m.cfg.BotToken)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("mattermost file upload %d: %s", resp.StatusCode, b)
	}

	var result struct {
		FileInfos []struct {
			ID string `json:"id"`
		} `json:"file_infos"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.FileInfos) > 0 {
		return result.FileInfos[0].ID, nil
	}
	return "", fmt.Errorf("no file info returned")
}

// --- API Helpers ---

func (m *MattermostChannel) apiGet(path string) ([]byte, error) {
	req, _ := http.NewRequest("GET", m.cfg.ServerURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+m.cfg.BotToken)
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mattermost GET %s %d: %s", path, resp.StatusCode, b)
	}
	return io.ReadAll(resp.Body)
}

func (m *MattermostChannel) apiPost(path string, body any) error {
	_, err := m.apiPostWithResponse(path, body)
	return err
}

func (m *MattermostChannel) apiPostWithResponse(path string, body any) ([]byte, error) {
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", m.cfg.ServerURL+path, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+m.cfg.BotToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mattermost %d: %s", resp.StatusCode, b)
	}
	return b, nil
}

func (m *MattermostChannel) apiPut(path string, body any) error {
	payload, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", m.cfg.ServerURL+path, bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+m.cfg.BotToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mattermost PUT %s %d: %s", path, resp.StatusCode, b)
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

// --- Types ---

type wsEvent struct {
	Event     string         `json:"event"`
	Data      map[string]any `json:"data"`
	Broadcast map[string]any `json:"broadcast"`
}

type mattermostPost struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	ChannelID string `json:"channel_id"`
	Message   string `json:"message"`
	Type      string `json:"type"`
	RootID    string `json:"root_id"`
}
