// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/qorvenai/qorven/internal/channels"
)

const (
	maxMessageLen       = 2000
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
	GuildID        string   `json:"guild_id"`
	RequireMention *bool    `json:"require_mention"`
	DMPolicy       string   `json:"dm_policy"`
	GroupPolicy    string   `json:"group_policy"`
	AllowFrom      []string `json:"allow_from"`
	HistoryLimit   int      `json:"history_limit"`
}

type DiscordChannel struct {
	cfg             Config
	handler         channels.InboundHandler
	session         *discordgo.Session
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
	placeholders    sync.Map // messageID → placeholderMsgID
	typingCtrls     sync.Map // channelID → cancel func
	dedup           sync.Map // "channelID:messageID" → time.Time — prevents double-fire on retries
	approvals       sync.Map // requestID → *discordPendingApproval
}

func New(cfg Config, handler channels.InboundHandler) *DiscordChannel {
	requireMention := true
	if cfg.RequireMention != nil {
		requireMention = *cfg.RequireMention
	}
	historyLimit := cfg.HistoryLimit
	if historyLimit == 0 {
		historyLimit = channels.DefaultGroupHistoryLimit
	}
	return &DiscordChannel{
		cfg:            cfg,
		handler:        handler,
		requireMention: requireMention,
		allowList:      cfg.AllowFrom,
		groupHistory:   channels.NewPendingHistory(),
		historyLimit:   historyLimit,
	}
}

func (d *DiscordChannel) SetPairingService(ps PairingStore) { d.pairingService = ps }

func (d *DiscordChannel) Name() string    { return "discord" }
func (d *DiscordChannel) Type() string    { return "discord" }
func (d *DiscordChannel) AgentID() string { return d.cfg.AgentID }
func (d *DiscordChannel) IsRunning() bool { d.mu.Lock(); defer d.mu.Unlock(); return d.running }

func (d *DiscordChannel) Start(ctx context.Context) error {
	s, err := discordgo.New("Bot " + d.cfg.BotToken)
	if err != nil {
		return err
	}
	s.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent
	d.session = s

	_, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	s.AddHandler(d.onMessage)

	if err := s.Open(); err != nil {
		return err
	}
	d.botID = s.State.User.ID
	d.botName = s.State.User.Username

	d.mu.Lock()
	d.running = true
	d.mu.Unlock()

	slog.Info("discord.started", "bot", d.botName, "agent", d.cfg.AgentID)
	return nil
}

func (d *DiscordChannel) Stop(_ context.Context) error {
	if d.session != nil {
		d.session.Close()
	}
	d.mu.Lock()
	d.running = false
	d.mu.Unlock()
	return nil
}

func (d *DiscordChannel) IsAllowed(senderID string) bool {
	if len(d.allowList) == 0 {
		return true
	}
	for _, allowed := range d.allowList {
		if senderID == allowed {
			return true
		}
	}
	return false
}

// --- Message Handler ---

func (d *DiscordChannel) onMessage(_ *discordgo.Session, m *discordgo.MessageCreate) {
	ctx := context.Background()
	if m.Author == nil || m.Author.Bot || m.Author.ID == d.botID {
		return
	}

	senderID := m.Author.ID
	senderName := d.resolveDisplayName(m)
	channelID := m.ChannelID
	isDM := m.GuildID == ""

	peerKind := "group"
	if isDM {
		peerKind = "direct"
	}

	// Policy check
	if isDM {
		if !d.checkDMPolicy(ctx, senderID, channelID) {
			return
		}
	} else {
		if !d.checkGroupPolicy(ctx, senderID, channelID) {
			return
		}
	}

	// Allowlist check
	if !d.IsAllowed(senderID) {
		return
	}

	// Dedup: Discord may deliver the same message twice on reconnect.
	dedupKey := channelID + ":" + m.ID
	if _, already := d.dedup.LoadOrStore(dedupKey, time.Now()); already {
		return
	}
	go func() { time.Sleep(10 * time.Minute); d.dedup.Delete(dedupKey) }()

	content := m.Content

	// Reply context
	if m.ReferencedMessage != nil {
		author := "unknown"
		if m.ReferencedMessage.Author != nil {
			author = m.ReferencedMessage.Author.Username
		}
		body := truncate(m.ReferencedMessage.Content, 500)
		replyCtx := fmt.Sprintf("[Replying to %s]\n%s\n[/Replying]", author, body)
		content = replyCtx + "\n\n" + content
	}

	// Process attachments
	if len(m.Attachments) > 0 {
		_, mediaContent := resolveMedia(m.Attachments)
		content += mediaContent
	}

	if content == "" {
		content = "[empty message]"
	}

	// Mention gating in groups
	if peerKind == "group" && d.requireMention {
		mentioned := d.isMentioned(m.Message)
		if !mentioned {
			d.groupHistory.Record(channelID, channels.HistoryEntry{
				Sender:    senderName,
				SenderID:  senderID,
				Body:      content,
				Timestamp: m.Timestamp,
				MessageID: m.ID,
			}, d.historyLimit)
			return
		}
	}

	// Strip bot mention
	content = strings.ReplaceAll(content, "<@"+d.botID+">", "")
	content = strings.ReplaceAll(content, "<@!"+d.botID+">", "")
	content = strings.TrimSpace(content)

	slog.Debug("discord.message", "from", senderName, "channel", channelID, "dm", isDM)

	// Start typing
	d.startTyping(channelID)

	// Send placeholder
	placeholder, err := d.session.ChannelMessageSend(channelID, "Thinking...")
	if err == nil {
		d.placeholders.Store(m.ID, placeholder.ID)
	}

	// Build final content with group context
	finalContent := content
	if peerKind == "group" {
		annotated := fmt.Sprintf("[From: %s (<@%s>)]\n%s", senderName, senderID, content)
		if d.historyLimit > 0 {
			finalContent = d.groupHistory.BuildContext(channelID, annotated, d.historyLimit)
		} else {
			finalContent = annotated
		}
	}

	metadata := map[string]string{
		"message_id":      m.ID,
		"channel_id":      channelID,
		"guild_id":        m.GuildID,
		"placeholder_key": m.ID,
	}

	if d.handler != nil {
		d.handler(ctx, channels.InboundMessage{
			ChannelName: "discord",
			ChannelType: "discord",
			AgentID:     d.cfg.AgentID,
			SenderID:    senderID,
			SenderName:  senderName,
			ChatID:      channelID,
			Content:     finalContent,
			PeerKind:    peerKind,
			Metadata:    metadata,
		})
	}

	// Clear pending history
	if peerKind == "group" {
		d.groupHistory.Clear(channelID)
	}
}

func (d *DiscordChannel) isMentioned(m *discordgo.Message) bool {
	for _, u := range m.Mentions {
		if u.ID == d.botID {
			return true
		}
	}
	if m.MessageReference != nil && m.ReferencedMessage != nil {
		if m.ReferencedMessage.Author != nil && m.ReferencedMessage.Author.ID == d.botID {
			return true
		}
	}
	return false
}

func (d *DiscordChannel) resolveDisplayName(m *discordgo.MessageCreate) string {
	if m.Member != nil && m.Member.Nick != "" {
		return m.Member.Nick
	}
	if m.Author.GlobalName != "" {
		return m.Author.GlobalName
	}
	return m.Author.Username
}

func (d *DiscordChannel) startTyping(channelID string) {
	d.session.ChannelTyping(channelID)
}

// --- Policy Checks ---

func (d *DiscordChannel) checkDMPolicy(ctx context.Context, senderID, channelID string) bool {
	policy := d.cfg.DMPolicy
	if policy == "" {
		policy = "open"
	}

	switch policy {
	case "disabled":
		return false
	case "open":
		return true
	case "allowlist":
		return d.IsAllowed(senderID)
	default: // "pairing"
		if d.IsAllowed(senderID) {
			return true
		}
		if d.pairingService != nil {
			paired, err := d.pairingService.IsPaired(ctx, senderID, d.Name())
			if err != nil {
				return true // fail-open
			}
			if paired {
				return true
			}
		}
		d.sendPairingReply(ctx, senderID, channelID)
		return false
	}
}

func (d *DiscordChannel) checkGroupPolicy(ctx context.Context, senderID, channelID string) bool {
	policy := d.cfg.GroupPolicy
	if policy == "" {
		policy = "open"
	}

	switch policy {
	case "disabled":
		return false
	case "allowlist":
		return d.IsAllowed(senderID)
	case "pairing":
		if d.IsAllowed(senderID) {
			return true
		}
		if _, cached := d.approvedGroups.Load(channelID); cached {
			return true
		}
		groupSenderID := "group:" + channelID
		if d.pairingService != nil {
			paired, err := d.pairingService.IsPaired(ctx, groupSenderID, d.Name())
			if err != nil {
				return true
			}
			if paired {
				d.approvedGroups.Store(channelID, true)
				return true
			}
		}
		d.sendPairingReply(ctx, groupSenderID, channelID)
		return false
	default:
		return true
	}
}

func (d *DiscordChannel) sendPairingReply(ctx context.Context, senderID, channelID string) {
	if d.pairingService == nil {
		return
	}

	if lastSent, ok := d.pairingDebounce.Load(senderID); ok {
		if time.Since(lastSent.(time.Time)) < pairingDebounceTime {
			return
		}
	}

	code, err := d.pairingService.RequestPairing(ctx, senderID, d.Name(), channelID, d.cfg.AgentID, nil)
	if err != nil {
		return
	}

	replyText := fmt.Sprintf(
		"Access not configured.\n\nYour Discord ID: %s\n\nPairing code: %s\n\nAsk the bot owner to approve with:\n  qorven pairing approve %s",
		senderID, code, code,
	)

	d.session.ChannelMessageSend(channelID, replyText)
	d.pairingDebounce.Store(senderID, time.Now())
	slog.Info("discord.pairing_sent", "sender", senderID, "code", code)
}

// --- Send ---

func (d *DiscordChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	if d.session == nil {
		return fmt.Errorf("not connected")
	}

	channelID := msg.ChatID
	if channelID == "" {
		channelID = msg.RecipientID
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		// Delete placeholder if exists
		if pk := msg.Metadata["placeholder_key"]; pk != "" {
			if pID, ok := d.placeholders.LoadAndDelete(pk); ok {
				d.session.ChannelMessageDelete(channelID, pID.(string))
			}
		}
		return nil
	}

	// Try to edit placeholder
	if pk := msg.Metadata["placeholder_key"]; pk != "" {
		if pID, ok := d.placeholders.LoadAndDelete(pk); ok {
			msgID := pID.(string)
			if len(content) <= maxMessageLen {
				_, err := d.session.ChannelMessageEdit(channelID, msgID, content)
				if err == nil {
					return nil
				}
			}
			// Delete placeholder if edit fails or content too long
			d.session.ChannelMessageDelete(channelID, msgID)
		}
	}

	// Send as new message(s)
	for _, chunk := range splitMessage(content, maxMessageLen) {
		if _, err := d.session.ChannelMessageSend(channelID, chunk); err != nil {
			return err
		}
	}
	return nil
}

// --- Streaming ---

func (d *DiscordChannel) StreamEnabled(isGroup bool) bool { return true }
func (d *DiscordChannel) ReasoningStreamEnabled() bool    { return true }

func (d *DiscordChannel) CreateStream(ctx context.Context, chatID string, firstStream bool) (channels.ChannelStream, error) {
	placeholder := "⏳ Thinking..."
	if !firstStream {
		placeholder = "..."
	}
	msg, err := d.session.ChannelMessageSend(chatID, placeholder)
	if err != nil {
		return nil, err
	}
	return &discordStream{session: d.session, channelID: chatID, msgID: msg.ID}, nil
}

func (d *DiscordChannel) FinalizeStream(ctx context.Context, chatID string, stream channels.ChannelStream) {
	// Placeholder finalization handled by Send
}

type discordStream struct {
	session   *discordgo.Session
	channelID string
	msgID     string
	lastEdit  time.Time
	lastText  string
	mu        sync.Mutex
	stopped   bool
}

func (ds *discordStream) Update(_ context.Context, text string) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.stopped || text == ds.lastText {
		return nil
	}
	if time.Since(ds.lastEdit) < time.Second {
		return nil
	}
	display := text
	if len(display) > maxMessageLen {
		display = display[len(display)-maxMessageLen:]
	}
	ds.session.ChannelMessageEdit(ds.channelID, ds.msgID, display+" ▌")
	ds.lastEdit = time.Now()
	ds.lastText = text
	return nil
}

func (ds *discordStream) Stop(_ context.Context) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.stopped = true
	if ds.lastText != "" {
		ds.session.ChannelMessageEdit(ds.channelID, ds.msgID, ds.lastText)
	}
	return nil
}

func (ds *discordStream) MessageID() string { return ds.msgID }

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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
