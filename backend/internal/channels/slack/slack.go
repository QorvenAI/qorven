// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package slack

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	slackapi "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"github.com/qorvenai/qorven/internal/channels"
)

const (
	maxMessageLen       = 4000 // Slack recommended text limit (hard limit is 40,000)
	pairingDebounceTime = 60 * time.Second
)

// PairingStore interface for pairing service integration.
type PairingStore interface {
	IsPaired(ctx context.Context, senderID, channelName string) (bool, error)
	RequestPairing(ctx context.Context, senderID, channelName, chatID, agentID string, meta map[string]string) (string, error)
}

type Config struct {
	AgentID        string   `json:"agent_id"`
	BotToken       string   `json:"bot_token"`
	AppToken       string   `json:"app_token"`
	RequireMention *bool    `json:"require_mention"`
	DMPolicy       string   `json:"dm_policy"`
	GroupPolicy    string   `json:"group_policy"`
	AllowFrom      []string `json:"allow_from"`
	HistoryLimit   int      `json:"history_limit"`
	DebounceDelay  int      `json:"debounce_delay_ms"`
	ThreadTTL      *int     `json:"thread_ttl_hours"`
}

type SlackChannel struct {
	cfg             Config
	handler         channels.InboundHandler
	api             *slackapi.Client
	sm              *socketmode.Client
	botID           string
	botName         string
	running         bool
	cancel          context.CancelFunc
	mu              sync.Mutex
	requireMention  bool
	allowList       []string
	pairingService  PairingStore
	pairingDebounce sync.Map
	approvedGroups  sync.Map
	groupHistory    *channels.PendingHistory
	historyLimit    int
	debounceDelay   time.Duration
	threadTTL       time.Duration
	dedup           sync.Map // msgKey → time.Time
	threadParticip  sync.Map // channelID:threadTS → time.Time
	placeholders    sync.Map // localKey → placeholderTS
	userCache       sync.Map // userID → displayName
}

func New(cfg Config, handler channels.InboundHandler) *SlackChannel {
	requireMention := true
	if cfg.RequireMention != nil {
		requireMention = *cfg.RequireMention
	}
	historyLimit := cfg.HistoryLimit
	if historyLimit == 0 {
		historyLimit = channels.DefaultGroupHistoryLimit
	}
	debounceDelay := time.Duration(cfg.DebounceDelay) * time.Millisecond
	if cfg.DebounceDelay == 0 {
		debounceDelay = 300 * time.Millisecond
	}
	threadTTL := 24 * time.Hour
	if cfg.ThreadTTL != nil {
		if *cfg.ThreadTTL <= 0 {
			threadTTL = 0
		} else {
			threadTTL = time.Duration(*cfg.ThreadTTL) * time.Hour
		}
	}

	return &SlackChannel{
		cfg:            cfg,
		handler:        handler,
		requireMention: requireMention,
		allowList:      cfg.AllowFrom,
		groupHistory:   channels.NewPendingHistory(),
		historyLimit:   historyLimit,
		debounceDelay:  debounceDelay,
		threadTTL:      threadTTL,
	}
}

func (s *SlackChannel) SetPairingService(ps PairingStore) { s.pairingService = ps }

func (s *SlackChannel) Name() string    { return "slack" }
func (s *SlackChannel) Type() string    { return "slack" }
func (s *SlackChannel) AgentID() string { return s.cfg.AgentID }
func (s *SlackChannel) IsRunning() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.running }

func (s *SlackChannel) Start(ctx context.Context) error {
	if !strings.HasPrefix(s.cfg.BotToken, "xoxb-") {
		return fmt.Errorf("bot_token must start with xoxb-")
	}
	if !strings.HasPrefix(s.cfg.AppToken, "xapp-") {
		return fmt.Errorf("app_token must start with xapp-")
	}

	s.api = slackapi.New(s.cfg.BotToken, slackapi.OptionAppLevelToken(s.cfg.AppToken))
	s.sm = socketmode.New(s.api)

	auth, err := s.api.AuthTest()
	if err != nil {
		return fmt.Errorf("slack auth: %w", err)
	}
	s.botID = auth.UserID
	s.botName = auth.User

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	go s.eventLoop(ctx)
	go s.sm.RunContext(ctx)

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	slog.Info("slack.started", "bot", auth.User, "agent", s.cfg.AgentID)
	return nil
}

func (s *SlackChannel) Stop(_ context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	return nil
}

func (s *SlackChannel) IsAllowed(senderID string) bool {
	if len(s.allowList) == 0 {
		return true
	}
	for _, allowed := range s.allowList {
		if senderID == allowed {
			return true
		}
	}
	return false
}

// --- Event Loop ---

func (s *SlackChannel) eventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-s.sm.Events:
			if !ok {
				return
			}
			switch evt.Type {
			case socketmode.EventTypeEventsAPI:
				s.sm.Ack(*evt.Request)
				inner, ok := evt.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				switch ev := inner.InnerEvent.Data.(type) {
				case *slackevents.MessageEvent:
					s.handleMessage(ctx, ev)
				case *slackevents.AppMentionEvent:
					s.handleAppMention(ctx, ev)
				}
			}
		}
	}
}

func (s *SlackChannel) handleMessage(ctx context.Context, ev *slackevents.MessageEvent) {
	if ev.User == s.botID || ev.User == "" {
		return
	}
	if ev.SubType != "" && ev.SubType != "file_share" {
		return
	}

	// Dedup
	dedupKey := ev.Channel + ":" + ev.TimeStamp
	if _, loaded := s.dedup.LoadOrStore(dedupKey, time.Now()); loaded {
		return
	}

	senderID := ev.User
	channelID := ev.Channel
	content := ev.Text

	isDM := ev.ChannelType == "im"
	peerKind := "group"
	if isDM {
		peerKind = "direct"
	}

	displayName := s.resolveDisplayName(senderID)

	// Policy check
	if isDM {
		if !s.checkDMPolicy(ctx, senderID, channelID) {
			return
		}
	} else {
		if !s.checkGroupPolicy(ctx, senderID, channelID) {
			return
		}
	}

	if isDM && !s.IsAllowed(senderID) {
		return
	}

	if content == "" {
		return
	}

	// Thread handling
	threadTS := ev.ThreadTimeStamp
	localKey := channelID
	if threadTS != "" {
		localKey = channelID + ":thread:" + threadTS
	}

	// Mention gating in groups
	if !isDM && s.requireMention {
		mentioned := s.isBotMentioned(content)

		// Thread participation cache
		if !mentioned && threadTS != "" && s.threadTTL > 0 {
			participKey := channelID + ":particip:" + threadTS
			if lastReply, ok := s.threadParticip.Load(participKey); ok {
				if time.Since(lastReply.(time.Time)) < s.threadTTL {
					mentioned = true
				}
			}
		}

		if !mentioned {
			s.groupHistory.Record(localKey, channels.HistoryEntry{
				Sender:    displayName,
				SenderID:  senderID,
				Body:      content,
				Timestamp: time.Now(),
				MessageID: ev.TimeStamp,
			}, s.historyLimit)
			return
		}
	}

	content = s.stripBotMention(content)
	content = strings.TrimSpace(content)

	slog.Debug("slack.message", "from", displayName, "channel", channelID, "dm", isDM)

	// Send placeholder
	replyThreadTS := threadTS
	if !isDM && replyThreadTS == "" {
		replyThreadTS = ev.TimeStamp
	}

	opts := []slackapi.MsgOption{slackapi.MsgOptionText("Thinking...", false)}
	if replyThreadTS != "" {
		opts = append(opts, slackapi.MsgOptionTS(replyThreadTS))
	}
	_, placeholderTS, _ := s.api.PostMessage(channelID, opts...)
	if placeholderTS != "" {
		s.placeholders.Store(localKey, placeholderTS)
	}

	// Build final content
	finalContent := content
	if peerKind == "group" {
		annotated := fmt.Sprintf("[From: %s]\n%s", displayName, content)
		if s.historyLimit > 0 {
			finalContent = s.groupHistory.BuildContext(localKey, annotated, s.historyLimit)
		} else {
			finalContent = annotated
		}
	}

	metadata := map[string]string{
		"message_id":      ev.TimeStamp,
		"channel_id":      channelID,
		"local_key":       localKey,
		"placeholder_key": localKey,
	}
	if replyThreadTS != "" {
		metadata["message_thread_id"] = replyThreadTS
	}

	if s.handler != nil {
		s.handler(ctx, channels.InboundMessage{
			ChannelName: "slack",
			ChannelType: "slack",
			AgentID:     s.cfg.AgentID,
			SenderID:    senderID,
			SenderName:  displayName,
			ChatID:      channelID,
			Content:     finalContent,
			PeerKind:    peerKind,
			Metadata:    metadata,
		})
	}

	// Record thread participation
	if peerKind == "group" && replyThreadTS != "" {
		participKey := channelID + ":particip:" + replyThreadTS
		s.threadParticip.Store(participKey, time.Now())
	}

	// Clear pending history
	if peerKind == "group" {
		s.groupHistory.Clear(localKey)
	}
}

func (s *SlackChannel) handleAppMention(ctx context.Context, ev *slackevents.AppMentionEvent) {
	if ev.User == s.botID {
		return
	}

	content := s.stripBotMention(ev.Text)
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	displayName := s.resolveDisplayName(ev.User)
	channelID := ev.Channel
	threadTS := ev.ThreadTimeStamp

	localKey := channelID
	if threadTS != "" {
		localKey = channelID + ":thread:" + threadTS
	}

	// Send placeholder
	replyThreadTS := threadTS
	if replyThreadTS == "" {
		replyThreadTS = ev.TimeStamp
	}

	opts := []slackapi.MsgOption{slackapi.MsgOptionText("Thinking...", false)}
	if replyThreadTS != "" {
		opts = append(opts, slackapi.MsgOptionTS(replyThreadTS))
	}
	_, placeholderTS, _ := s.api.PostMessage(channelID, opts...)
	if placeholderTS != "" {
		s.placeholders.Store(localKey, placeholderTS)
	}

	annotated := fmt.Sprintf("[From: %s]\n%s", displayName, content)
	finalContent := s.groupHistory.BuildContext(localKey, annotated, s.historyLimit)

	metadata := map[string]string{
		"message_id":        ev.TimeStamp,
		"channel_id":        channelID,
		"local_key":         localKey,
		"placeholder_key":   localKey,
		"message_thread_id": replyThreadTS,
	}

	if s.handler != nil {
		s.handler(ctx, channels.InboundMessage{
			ChannelName: "slack",
			ChannelType: "slack",
			AgentID:     s.cfg.AgentID,
			SenderID:    ev.User,
			SenderName:  displayName,
			ChatID:      channelID,
			Content:     finalContent,
			PeerKind:    "group",
			Metadata:    metadata,
		})
	}

	// Record thread participation
	participKey := channelID + ":particip:" + replyThreadTS
	s.threadParticip.Store(participKey, time.Now())
	s.groupHistory.Clear(localKey)
}

func (s *SlackChannel) isBotMentioned(text string) bool {
	return strings.Contains(text, "<@"+s.botID+">")
}

func (s *SlackChannel) stripBotMention(text string) string {
	return strings.ReplaceAll(text, "<@"+s.botID+">", "")
}

func (s *SlackChannel) resolveDisplayName(userID string) string {
	if cached, ok := s.userCache.Load(userID); ok {
		return cached.(string)
	}
	user, err := s.api.GetUserInfo(userID)
	if err != nil {
		return userID
	}
	name := user.RealName
	if name == "" {
		name = user.Name
	}
	s.userCache.Store(userID, name)
	return name
}

// --- Policy Checks ---

func (s *SlackChannel) checkDMPolicy(ctx context.Context, senderID, channelID string) bool {
	policy := s.cfg.DMPolicy
	if policy == "" {
		policy = "open"
	}

	switch policy {
	case "disabled":
		return false
	case "open":
		return true
	case "allowlist":
		return s.IsAllowed(senderID)
	default:
		if s.IsAllowed(senderID) {
			return true
		}
		if s.pairingService != nil {
			paired, err := s.pairingService.IsPaired(ctx, senderID, s.Name())
			if err != nil {
				return true
			}
			if paired {
				return true
			}
		}
		s.sendPairingReply(ctx, senderID, channelID)
		return false
	}
}

func (s *SlackChannel) checkGroupPolicy(ctx context.Context, senderID, channelID string) bool {
	policy := s.cfg.GroupPolicy
	if policy == "" {
		policy = "open"
	}

	switch policy {
	case "disabled":
		return false
	case "allowlist":
		return s.IsAllowed(senderID)
	case "pairing":
		if s.IsAllowed(senderID) {
			return true
		}
		if _, cached := s.approvedGroups.Load(channelID); cached {
			return true
		}
		groupSenderID := "group:" + channelID
		if s.pairingService != nil {
			paired, err := s.pairingService.IsPaired(ctx, groupSenderID, s.Name())
			if err != nil {
				return true
			}
			if paired {
				s.approvedGroups.Store(channelID, true)
				return true
			}
		}
		s.sendPairingReply(ctx, groupSenderID, channelID)
		return false
	default:
		return true
	}
}

func (s *SlackChannel) sendPairingReply(ctx context.Context, senderID, channelID string) {
	if s.pairingService == nil {
		return
	}

	if lastSent, ok := s.pairingDebounce.Load(senderID); ok {
		if time.Since(lastSent.(time.Time)) < pairingDebounceTime {
			return
		}
	}

	code, err := s.pairingService.RequestPairing(ctx, senderID, s.Name(), channelID, s.cfg.AgentID, nil)
	if err != nil {
		return
	}

	replyText := fmt.Sprintf(
		"Access not configured.\n\nYour Slack ID: %s\n\nPairing code: %s\n\nAsk the bot owner to approve with:\n  qorven pairing approve %s",
		senderID, code, code,
	)

	s.api.PostMessage(channelID, slackapi.MsgOptionText(replyText, false))
	s.pairingDebounce.Store(senderID, time.Now())
	slog.Info("slack.pairing_sent", "sender", senderID, "code", code)
}

// --- Send ---

func (s *SlackChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	if s.api == nil {
		return fmt.Errorf("not connected")
	}

	channelID := msg.ChatID
	if channelID == "" {
		channelID = msg.RecipientID
	}

	content := markdownToSlackMrkdwn(strings.TrimSpace(msg.Content))
	localKey := msg.Metadata["local_key"]
	threadTS := msg.Metadata["message_thread_id"]

	// Handle empty content (NO_REPLY)
	if content == "" {
		if localKey != "" {
			if pTS, ok := s.placeholders.LoadAndDelete(localKey); ok {
				s.api.DeleteMessage(channelID, pTS.(string))
			}
		}
		return nil
	}

	// Try to edit placeholder
	if localKey != "" {
		if pTS, ok := s.placeholders.LoadAndDelete(localKey); ok {
			placeholderTS := pTS.(string)
			if len(content) <= maxMessageLen {
				_, _, _, err := s.api.UpdateMessage(channelID, placeholderTS, slackapi.MsgOptionText(content, false))
				if err == nil {
					return nil
				}
			}
			s.api.DeleteMessage(channelID, placeholderTS)
		}
	}

	// Send as new message(s)
	opts := []slackapi.MsgOption{}
	if threadTS != "" {
		opts = append(opts, slackapi.MsgOptionTS(threadTS))
	}

	for _, chunk := range splitMessage(content, maxMessageLen) {
		msgOpts := append(opts, slackapi.MsgOptionText(chunk, false))
		if _, _, err := s.api.PostMessage(channelID, msgOpts...); err != nil {
			return err
		}
	}
	return nil
}

// --- Streaming ---

func (s *SlackChannel) StreamEnabled(isGroup bool) bool { return true }
func (s *SlackChannel) ReasoningStreamEnabled() bool    { return false } // Slack doesn't support reasoning lane well

func (s *SlackChannel) CreateStream(ctx context.Context, chatID string, firstStream bool) (channels.ChannelStream, error) {
	_, ts, err := s.api.PostMessage(chatID, slackapi.MsgOptionText("...", false))
	if err != nil {
		return nil, err
	}
	return &slackStream{api: s.api, channelID: chatID, ts: ts}, nil
}

func (s *SlackChannel) FinalizeStream(ctx context.Context, chatID string, stream channels.ChannelStream) {}

type slackStream struct {
	api       *slackapi.Client
	channelID string
	ts        string
	lastEdit  time.Time
	lastText  string
	mu        sync.Mutex
	stopped   bool
}

func (ss *slackStream) Update(_ context.Context, text string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.stopped || text == ss.lastText {
		return nil
	}
	if time.Since(ss.lastEdit) < time.Second {
		return nil
	}
	display := markdownToSlackMrkdwn(text)
	if len(display) > maxMessageLen {
		display = display[len(display)-maxMessageLen:]
	}
	ss.api.UpdateMessage(ss.channelID, ss.ts, slackapi.MsgOptionText(display, false))
	ss.lastEdit = time.Now()
	ss.lastText = text
	return nil
}

func (ss *slackStream) Stop(_ context.Context) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.stopped = true
	if ss.lastText != "" {
		ss.api.UpdateMessage(ss.channelID, ss.ts, slackapi.MsgOptionText(markdownToSlackMrkdwn(ss.lastText), false))
	}
	return nil
}

func (ss *slackStream) MessageID() string { return ss.ts }

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
