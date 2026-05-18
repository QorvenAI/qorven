// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package teams

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/qorvenai/qorven/internal/channels"
)

const (
	botFrameworkAuth       = "https://login.microsoftonline.com/botframework.com/oauth2/v2.0/token"
	botFrameworkOpenIDConf = "https://login.botframework.com/v1/.well-known/openidconfiguration"
	botFrameworkIssuer     = "https://api.botframework.com"
	jwksRefreshInterval    = 24 * time.Hour
	maxMessageLen          = 28000 // Teams limit ~28KB per message
)

type Config struct {
	AgentID        string   `json:"agent_id"`
	AppID          string   `json:"app_id"`
	AppSecret      string   `json:"app_secret"`
	TenantID       string   `json:"tenant_id"` // for single-tenant apps
	WelcomeCard    bool     `json:"welcome_card"`
	PromptStarters []string `json:"prompt_starters"`
}

type TeamsChannel struct {
	cfg  Config
	handler   channels.InboundHandler
	running   bool
	mu        sync.Mutex
	client    *http.Client
	debouncer *channels.Debouncer
	jwks      *jwksCache
	// OAuth token cache
	tokenMu     sync.Mutex
	accessToken string
	tokenExpiry time.Time
	// Conversation references for proactive messaging
	convRefs sync.Map // userID → *ConversationRef
	streams  sync.Map // convID → *teamsStream
}

type ConversationRef struct {
	ServiceURL     string
	ConversationID string
	UserID         string
	UpdatedAt      time.Time
}

func New(cfg Config, handler channels.InboundHandler) *TeamsChannel {
	return &TeamsChannel{
		cfg:     cfg,
		handler: handler,
		client:  &http.Client{Timeout: 30 * time.Second},
		jwks:    &jwksCache{},
	}
}

func (t *TeamsChannel) Name() string    { return "teams" }
func (t *TeamsChannel) Type() string    { return "teams" }
func (t *TeamsChannel) AgentID() string { return t.cfg.AgentID }
func (t *TeamsChannel) IsRunning() bool { t.mu.Lock(); defer t.mu.Unlock(); return t.running }

func (t *TeamsChannel) Start(ctx context.Context) error {
	t.debouncer = channels.NewDebouncer(600*time.Millisecond, func(msg channels.InboundMessage) {
		if t.handler != nil {
			t.handler(ctx, msg)
		}
	})
	t.mu.Lock()
	t.running = true
	t.mu.Unlock()
	slog.Info("teams.started", "app_id", t.cfg.AppID, "agent", t.cfg.AgentID)
	return nil
}

func (t *TeamsChannel) Stop(_ context.Context) error {
	if t.debouncer != nil {
		t.debouncer.FlushAll()
	}
	t.mu.Lock()
	t.running = false
	t.mu.Unlock()
	return nil
}

// --- OAuth2 Token (Client Credentials Flow) ---

func (t *TeamsChannel) getToken() (string, error) {
	t.tokenMu.Lock()
	defer t.tokenMu.Unlock()
	if t.accessToken != "" && time.Now().Before(t.tokenExpiry) {
		return t.accessToken, nil
	}

	data := fmt.Sprintf("grant_type=client_credentials&client_id=%s&client_secret=%s&scope=https://api.botframework.com/.default",
		t.cfg.AppID, t.cfg.AppSecret)
	resp, err := t.client.Post(botFrameworkAuth, "application/x-www-form-urlencoded", strings.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("teams oauth: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	json.NewDecoder(resp.Body).Decode(&tokenResp)
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("teams oauth: empty token")
	}

	t.accessToken = tokenResp.AccessToken
	// Refresh 60 seconds before expiry to avoid using stale tokens.
	t.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)
	return t.accessToken, nil
}

// --- JWKS / JWT Validation ---

// jwksCache caches Bot Framework RSA public keys for incoming JWT validation.
// Keys are refreshed every 24 hours. On failed refresh, the stale cache is retained.
type jwksCache struct {
	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey // kid → public key
	fetchedAt time.Time
}

type botClaims struct {
	jwt.RegisteredClaims
	ServiceURL string `json:"serviceurl"`
	AppID      string `json:"appid"`
}

func (c *jwksCache) getKey(client *http.Client, kid string) (*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	stale := c.keys == nil || time.Since(c.fetchedAt) > jwksRefreshInterval
	if stale {
		if err := c.refresh(client); err != nil {
			if c.keys == nil {
				return nil, fmt.Errorf("teams jwks: fetch failed and no cached keys: %w", err)
			}
			// Use stale cache on refresh failure rather than rejecting all traffic.
			slog.Warn("teams.jwks.refresh_failed", "error", err)
		}
	}

	key, ok := c.keys[kid]
	if !ok {
		// One extra refresh in case it's a newly issued key not yet cached.
		if !stale {
			if err := c.refresh(client); err == nil {
				key, ok = c.keys[kid]
			}
		}
		if !ok {
			return nil, fmt.Errorf("teams jwks: unknown key id %q", kid)
		}
	}
	return key, nil
}

func (c *jwksCache) refresh(client *http.Client) error {
	resp, err := client.Get(botFrameworkOpenIDConf)
	if err != nil {
		return fmt.Errorf("openid config fetch: %w", err)
	}
	defer resp.Body.Close()

	var config struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return fmt.Errorf("openid config parse: %w", err)
	}

	resp2, err := client.Get(config.JWKSURI)
	if err != nil {
		return fmt.Errorf("jwks fetch: %w", err)
	}
	defer resp2.Body.Close()

	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("jwks parse: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" || k.N == "" || k.E == "" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		e := int(new(big.Int).SetBytes(eBytes).Int64())
		if e == 0 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}
	}

	c.keys = keys
	c.fetchedAt = time.Now()
	return nil
}

// validateJWT verifies the Azure Bot Framework JWT on an incoming webhook request.
// Skipped when AppID is empty (dev/test mode with no credentials configured).
func (t *TeamsChannel) validateJWT(r *http.Request, activity *Activity) error {
	authHeader := r.Header.Get("Authorization")
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == "" || tokenStr == authHeader {
		return fmt.Errorf("missing bearer token")
	}

	var claims botClaims
	_, err := jwt.ParseWithClaims(tokenStr, &claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		kid, _ := token.Header["kid"].(string)
		return t.jwks.getKey(t.client, kid)
	},
		jwt.WithIssuer(botFrameworkIssuer),
		jwt.WithAudience(t.cfg.AppID),
	)
	if err != nil {
		return fmt.Errorf("jwt parse: %w", err)
	}

	// serviceurl claim must match the Activity's serviceUrl to prevent replay across tenants.
	if claims.ServiceURL != "" && activity.ServiceURL != "" && claims.ServiceURL != activity.ServiceURL {
		return fmt.Errorf("serviceurl mismatch: token=%q activity=%q", claims.ServiceURL, activity.ServiceURL)
	}

	return nil
}

// --- Webhook Handler (Bot Framework activities) ---

func (t *TeamsChannel) HandleWebhook(rw http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))

	var activity Activity
	if err := json.Unmarshal(body, &activity); err != nil {
		rw.WriteHeader(200)
		return
	}

	// JWT validation — skip only when AppID is unconfigured (dev mode).
	if t.cfg.AppID != "" {
		if err := t.validateJWT(r, &activity); err != nil {
			slog.Warn("teams.webhook.auth_failed", "error", err)
			http.Error(rw, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Store conversation reference for proactive messaging.
	if activity.From.ID != "" {
		t.convRefs.Store(activity.From.ID, &ConversationRef{
			ServiceURL:     activity.ServiceURL,
			ConversationID: activity.Conversation.ID,
			UserID:         activity.From.ID,
			UpdatedAt:      time.Now(),
		})
	}

	switch activity.Type {
	case "message":
		t.handleMessage(r.Context(), &activity)
	case "conversationUpdate":
		t.handleConversationUpdate(r.Context(), &activity)
	case "invoke":
		t.handleInvoke(rw, &activity)
		return // invoke needs custom response
	case "messageReaction":
		slog.Debug("teams.reaction", "from", activity.From.Name)
	}
	rw.WriteHeader(200)
}

func (t *TeamsChannel) handleMessage(ctx context.Context, activity *Activity) {
	if activity.Text == "" {
		return
	}
	if activity.From.ID == t.cfg.AppID {
		return
	}

	text := strings.TrimSpace(activity.Text)
	// Strip bot @mention tags (Teams includes <at>BotName</at> in channel messages).
	text = stripTeamsMention(text)
	if text == "" {
		return
	}

	go t.sendTyping(activity.ServiceURL, activity.Conversation.ID)

	slog.Info("teams.inbound", "from", activity.From.Name, "text", text[:min(len(text), 60)])
	t.debouncer.Push(channels.InboundMessage{
		ChannelName: "teams",
		ChannelType: "teams",
		AgentID:     t.cfg.AgentID,
		SenderID:    activity.From.ID,
		SenderName:  activity.From.Name,
		ChatID:      activity.Conversation.ID,
		Content:     text,
		Metadata: map[string]string{
			"conversation_id":   activity.Conversation.ID,
			"conversation_type": activity.Conversation.ConversationType,
			"service_url":       activity.ServiceURL,
			"message_id":        activity.ID,
			"reply_to_id":       activity.ReplyToID,
		},
	})
}

func (t *TeamsChannel) handleConversationUpdate(ctx context.Context, activity *Activity) {
	if !t.cfg.WelcomeCard {
		return
	}
	for _, member := range activity.MembersAdded {
		if member.ID == t.cfg.AppID {
			continue
		}
		card := t.buildWelcomeCard(member.Name)
		t.sendActivity(activity.ServiceURL, activity.Conversation.ID, Activity{
			Type: "message",
			Attachments: []Attachment{{
				ContentType: "application/vnd.microsoft.card.adaptive",
				Content:     card,
			}},
		})
	}
}

func (t *TeamsChannel) handleInvoke(rw http.ResponseWriter, activity *Activity) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(200)
	json.NewEncoder(rw).Encode(map[string]any{"status": 200})
}

// --- Send ---

func (t *TeamsChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	serviceURL := ""
	convID := msg.RecipientID
	if convID == "" {
		convID = msg.ChatID
	}
	replyToID := ""
	if msg.Metadata != nil {
		if su, ok := msg.Metadata["service_url"]; ok {
			serviceURL = su
		}
		if ci, ok := msg.Metadata["conversation_id"]; ok {
			convID = ci
		}
		if ri, ok := msg.Metadata["message_id"]; ok {
			replyToID = ri
		}
	}

	if serviceURL == "" {
		t.convRefs.Range(func(_, v any) bool {
			ref := v.(*ConversationRef)
			if ref.ConversationID == convID {
				serviceURL = ref.ServiceURL
				return false
			}
			return true
		})
	}
	if serviceURL == "" {
		return fmt.Errorf("teams: no service URL for conversation %s", convID)
	}

	chunks := splitMessage(content, maxMessageLen)
	for i, chunk := range chunks {
		a := Activity{
			Type:       "message",
			Text:       chunk,
			TextFormat: "markdown",
		}
		// replyToId threads the first chunk into the original message; subsequent chunks are follow-ons.
		if i == 0 && replyToID != "" {
			a.ReplyToID = replyToID
		}
		if err := t.sendActivity(serviceURL, convID, a); err != nil {
			return err
		}
	}
	return nil
}

// --- Streaming ---

func (t *TeamsChannel) StreamEnabled(isGroup bool) bool { return true }
func (t *TeamsChannel) ReasoningStreamEnabled() bool     { return false }

func (t *TeamsChannel) CreateStream(ctx context.Context, chatID string, firstStream bool) (channels.ChannelStream, error) {
	serviceURL := ""
	t.convRefs.Range(func(_, v any) bool {
		ref := v.(*ConversationRef)
		if ref.ConversationID == chatID {
			serviceURL = ref.ServiceURL
			return false
		}
		return true
	})
	if serviceURL == "" {
		return nil, fmt.Errorf("teams: no service URL for conversation %s — cannot stream", chatID)
	}

	activityID, err := t.sendActivityGetID(serviceURL, chatID, Activity{Type: "message", Text: "⏳ Thinking..."})
	if err != nil {
		return nil, err
	}

	s := &teamsStream{
		ch:         t,
		serviceURL: serviceURL,
		convID:     chatID,
		activityID: activityID,
	}
	t.streams.Store(chatID, s)
	return s, nil
}

func (t *TeamsChannel) FinalizeStream(_ context.Context, chatID string, _ channels.ChannelStream) {
	t.streams.Delete(chatID)
}

type teamsStream struct {
	ch         *TeamsChannel
	serviceURL string
	convID     string
	activityID string
	lastText   string
	lastEdit   time.Time
	mu         sync.Mutex
	stopped    bool
}

func (s *teamsStream) Update(_ context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || text == s.lastText {
		return nil
	}
	// Throttle: Teams rate-limits activity updates to ~1/s.
	if time.Since(s.lastEdit) < 1200*time.Millisecond {
		return nil
	}
	if err := s.ch.EditMessage(s.serviceURL, s.convID, s.activityID, text+" ▌"); err != nil {
		return err
	}
	s.lastText = text
	s.lastEdit = time.Now()
	return nil
}

func (s *teamsStream) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	if s.lastText != "" {
		s.ch.EditMessage(s.serviceURL, s.convID, s.activityID, s.lastText)
	}
	return nil
}

func (s *teamsStream) MessageID() string { return s.activityID }

func (t *TeamsChannel) sendActivityGetID(serviceURL, convID string, activity Activity) (string, error) {
	token, err := t.getToken()
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/v3/conversations/%s/activities", strings.TrimRight(serviceURL, "/"), convID)
	body, _ := json.Marshal(activity)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("teams stream create: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("teams stream create %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.ID, nil
}

// SendAdaptiveCard sends a rich Adaptive Card.
func (t *TeamsChannel) SendAdaptiveCard(serviceURL, convID string, card map[string]any) error {
	return t.sendActivity(serviceURL, convID, Activity{
		Type: "message",
		Attachments: []Attachment{{
			ContentType: "application/vnd.microsoft.card.adaptive",
			Content:     card,
		}},
	})
}

// SendProactive sends a message to a user using a stored conversation reference.
func (t *TeamsChannel) SendProactive(userID, text string) error {
	ref, ok := t.convRefs.Load(userID)
	if !ok {
		return fmt.Errorf("teams: no conversation reference for user %s", userID)
	}
	convRef := ref.(*ConversationRef)
	return t.sendActivity(convRef.ServiceURL, convRef.ConversationID, Activity{
		Type:       "message",
		Text:       text,
		TextFormat: "markdown",
	})
}

// EditMessage updates a previously sent message (used for streaming simulation).
func (t *TeamsChannel) EditMessage(serviceURL, convID, activityID, newText string) error {
	token, err := t.getToken()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/v3/conversations/%s/activities/%s", strings.TrimRight(serviceURL, "/"), convID, activityID)
	body, _ := json.Marshal(Activity{Type: "message", Text: newText, TextFormat: "markdown"})
	req, _ := http.NewRequest("PUT", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// DeleteMessage deletes a previously sent message.
func (t *TeamsChannel) DeleteMessage(serviceURL, convID, activityID string) error {
	token, err := t.getToken()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/v3/conversations/%s/activities/%s", strings.TrimRight(serviceURL, "/"), convID, activityID)
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// --- Typing Indicator ---

func (t *TeamsChannel) sendTyping(serviceURL, convID string) {
	t.sendActivity(serviceURL, convID, Activity{Type: "typing"})
}

// --- Bot Framework API ---

func (t *TeamsChannel) sendActivity(serviceURL, convID string, activity Activity) error {
	token, err := t.getToken()
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/v3/conversations/%s/activities", strings.TrimRight(serviceURL, "/"), convID)
	body, _ := json.Marshal(activity)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("teams send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("teams send %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// --- Welcome Card ---

func (t *TeamsChannel) buildWelcomeCard(userName string) map[string]any {
	body := []map[string]any{
		{"type": "TextBlock", "size": "Medium", "weight": "Bolder", "text": fmt.Sprintf("👋 Welcome, %s!", userName)},
		{"type": "TextBlock", "text": "I'm your Qorven AI assistant. Ask me anything!", "wrap": true},
	}
	if len(t.cfg.PromptStarters) > 0 {
		body = append(body, map[string]any{"type": "TextBlock", "text": "Try asking:", "weight": "Bolder"})
		for _, ps := range t.cfg.PromptStarters {
			body = append(body, map[string]any{"type": "TextBlock", "text": "• " + ps, "wrap": true})
		}
	}
	return map[string]any{
		"type":    "AdaptiveCard",
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"version": "1.5",
		"body":    body,
	}
}

// --- Types ---

type Activity struct {
	Type         string       `json:"type"`
	ID           string       `json:"id,omitempty"`
	Text         string       `json:"text,omitempty"`
	TextFormat   string       `json:"textFormat,omitempty"` // "markdown" or "plain"
	From         Actor        `json:"from,omitempty"`
	Recipient    Actor        `json:"recipient,omitempty"`
	Conversation Conversation `json:"conversation,omitempty"`
	ServiceURL   string       `json:"serviceUrl,omitempty"`
	ReplyToID    string       `json:"replyToId,omitempty"`
	Attachments  []Attachment `json:"attachments,omitempty"`
	Entities     []Entity     `json:"entities,omitempty"`
	MembersAdded []Actor      `json:"membersAdded,omitempty"`
	ChannelData  any          `json:"channelData,omitempty"`
}

type Actor struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type Conversation struct {
	ID               string `json:"id"`
	ConversationType string `json:"conversationType,omitempty"` // personal, groupChat, channel
}

type Attachment struct {
	ContentType string `json:"contentType"`
	Content     any    `json:"content"`
}

// Entity represents a Teams entity such as a @mention.
type Entity struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Mentioned *Actor `json:"mentioned,omitempty"`
}

// --- Helpers ---

func stripTeamsMention(text string) string {
	for {
		start := strings.Index(text, "<at>")
		if start < 0 {
			break
		}
		end := strings.Index(text, "</at>")
		if end < 0 {
			break
		}
		text = text[:start] + text[end+5:]
	}
	return strings.TrimSpace(text)
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
