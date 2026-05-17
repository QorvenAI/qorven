// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package telegram

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/qorvenai/qorven/internal/channels"
)

const (
	maxMessageLen    = 4096 // Telegram hard limit per sendMessage
	maxCaptionLen    = 1024
	maxFileDownload  = 20 << 20 // Telegram getFile limit: 20MB
	streamThrottleMs = 1200
	typingInterval   = 4 * time.Second
	pairingDebounce  = 60 * time.Second
	voiceGuardWindow = 30 * time.Second
	voiceGuardMax    = 3
)

type Config struct {
	AgentID        string `json:"agent_id"`
	BotToken       string `json:"bot_token"`
	BotName        string `json:"bot_name"`
	GroupPolicy    string `json:"group_policy"`    // open, mention_only, admin_only, disabled
	RequireMention bool   `json:"require_mention"`
	Proxy          string `json:"proxy"`
}

type TelegramChannel struct {
	cfg           Config
	handler       channels.InboundHandler
	b             *bot.Bot
	botUser       *models.User
	running       bool
	cancel        context.CancelFunc
	mu            sync.Mutex
	debouncer     *channels.Debouncer
	reconnect     *channels.Reconnector
	vGuard        *voiceGuard
	dedup         sync.Map // "chatID:msgID" → time.Time — prevents double-fire on platform retries
	typingCancels sync.Map // int64 chatID → context.CancelFunc
	Transcribe    func(ctx context.Context, audio []byte, format string) (string, error) // optional STT
}

func New(cfg Config, handler channels.InboundHandler) *TelegramChannel {
	if cfg.GroupPolicy == "" && cfg.RequireMention { cfg.GroupPolicy = "mention_only" }
	if cfg.GroupPolicy == "" { cfg.GroupPolicy = "open" }
	ch := &TelegramChannel{cfg: cfg, handler: handler, vGuard: newVoiceGuard()}
	ch.reconnect = channels.NewReconnector(10, func(ctx context.Context) error { return ch.connect(ctx) })
	return ch
}

func (t *TelegramChannel) Name() string    { return fmt.Sprintf("telegram:%s", t.cfg.BotName) }
func (t *TelegramChannel) Type() string    { return "telegram" }
func (t *TelegramChannel) AgentID() string { return t.cfg.AgentID }
func (t *TelegramChannel) IsRunning() bool { t.mu.Lock(); defer t.mu.Unlock(); return t.running }

func (t *TelegramChannel) Start(ctx context.Context) error { return t.connect(ctx) }

func (t *TelegramChannel) connect(_ context.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel

	opts := []bot.Option{
		bot.WithDefaultHandler(t.defaultHandler),
	}

	b, err := bot.New(t.cfg.BotToken, opts...)
	if err != nil { return fmt.Errorf("telegram init: %w", err) }
	t.b = b

	t.debouncer = channels.NewDebouncer(600*time.Millisecond, func(msg channels.InboundMessage) {
		if t.handler != nil { t.handler(ctx, msg) }
	})

	// Register commands
	t.b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, t.cmdStart)
	t.b.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, t.cmdHelp)
	t.b.RegisterHandler(bot.HandlerTypeMessageText, "/status", bot.MatchTypeExact, t.cmdStatus)
	t.b.RegisterHandler(bot.HandlerTypeMessageText, "/cancel", bot.MatchTypeExact, t.cmdCancel)
	t.b.RegisterHandler(bot.HandlerTypeMessageText, "/pair", bot.MatchTypeExact, t.cmdPair)
	t.b.RegisterHandler(bot.HandlerTypeMessageText, "/tasks", bot.MatchTypeExact, t.cmdTasks)
	t.b.RegisterHandler(bot.HandlerTypeMessageText, "/voice", bot.MatchTypePrefix, t.cmdVoice)

	me, _ := t.b.GetMe(ctx)
	t.botUser = me

	// Force-clear any stale polling session by toggling webhook
	t.b.DeleteWebhook(ctx, &bot.DeleteWebhookParams{DropPendingUpdates: false})

	// Register bot commands menu
	t.registerCommands(ctx)

	t.mu.Lock(); t.running = true; t.mu.Unlock()
	go t.b.Start(ctx)
	slog.Info("telegram.started", "bot", me.Username, "agent", t.cfg.AgentID, "api", "Bot API 9.5")
	return nil
}

func (t *TelegramChannel) Stop(_ context.Context) error {
	if t.cancel != nil { t.cancel() }
	if t.debouncer != nil { t.debouncer.FlushAll() }
	if t.reconnect != nil { t.reconnect.Stop() }
	t.mu.Lock(); t.running = false; t.mu.Unlock()
	return nil
}

// --- Default Handler (all non-command messages) ---

func (t *TelegramChannel) defaultHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.CallbackQuery != nil { t.handleCallback(ctx, b, update.CallbackQuery); return }
	msg := update.Message
	if msg == nil { return }
	if msg.NewChatMembers != nil || msg.LeftChatMember != nil { return } // skip service msgs

	// FIX: Ignore self-authored messages (Qorven #54530 — prevents bogus pairing + echo loops)
	if t.botUser != nil && msg.From != nil && msg.From.ID == t.botUser.ID { return }

	// Dedup: Telegram retries webhook delivery on timeout; skip already-seen messages.
	dedupKey := fmt.Sprintf("%d:%d", msg.Chat.ID, msg.ID)
	if _, already := t.dedup.LoadOrStore(dedupKey, time.Now()); already { return }
	go t.evictDedup(dedupKey, 10*time.Minute)

	chatID := msg.Chat.ID
	senderID := fmt.Sprintf("%d", msg.From.ID)
	senderName := msg.From.FirstName
	if msg.From.LastName != "" { senderName += " " + msg.From.LastName }

	// Group policy
	isGroup := msg.Chat.Type == "group" || msg.Chat.Type == "supergroup"
	if isGroup {
		switch t.cfg.GroupPolicy {
		case "disabled": return
		case "mention_only":
			if !t.isMentioned(msg) { return }
		case "admin_only":
			if !t.isAdmin(ctx, b, chatID, msg.From.ID) { return }
		}
	}

	text := msg.Text
	if text == "" { text = msg.Caption }

	// Media extraction
	var mediaType, mediaFileID string
	switch {
	case len(msg.Photo) > 0:
		mediaType, mediaFileID = "photo", msg.Photo[len(msg.Photo)-1].FileID
	case msg.Voice != nil:
		if !t.vGuard.Allow(senderID) { return }
		mediaType, mediaFileID = "voice", msg.Voice.FileID
	case msg.Audio != nil:
		mediaType, mediaFileID = "audio", msg.Audio.FileID
	case msg.Document != nil:
		mediaType, mediaFileID = "document", msg.Document.FileID
	case msg.Video != nil:
		mediaType, mediaFileID = "video", msg.Video.FileID
	case msg.Sticker != nil:
		text = "[Sticker: " + msg.Sticker.Emoji + "]"
	}

	if text == "" && mediaFileID == "" { return }

	meta := map[string]string{
		"chat_id": fmt.Sprintf("%d", chatID), "message_id": fmt.Sprintf("%d", msg.ID),
	}
	if msg.MessageThreadID != 0 {
		meta["thread_id"] = fmt.Sprintf("%d", msg.MessageThreadID)
	}
	if msg.MediaGroupID != "" {
		meta["media_group_id"] = msg.MediaGroupID
	}

	if mediaFileID != "" {
		meta["has_media"] = "true"; meta["media_type"] = mediaType
		if mc := t.processMedia(ctx, b, mediaType, mediaFileID); mc != "" {
			if text != "" { text += "\n\n" + mc } else { text = mc }
		}
	}

	// Typing + reaction — cancelled automatically when Send() fires for this chat
	typingCtx, typingCancel := context.WithCancel(ctx)
	if prev, loaded := t.typingCancels.Swap(chatID, typingCancel); loaded {
		prev.(context.CancelFunc)()
	}
	go t.startTyping(typingCtx, b, chatID)
	t.setReaction(ctx, b, chatID, msg.ID, "👀")

	t.debouncer.Push(channels.InboundMessage{
		ChannelName: t.Name(), ChannelType: "telegram", AgentID: t.cfg.AgentID,
		SenderID: senderID, SenderName: senderName, Content: text, Metadata: meta,
	})
}

// --- Mention Detection ---

func (t *TelegramChannel) isMentioned(msg *models.Message) bool {
	if t.botUser == nil { return false }
	botAt := "@" + t.botUser.Username
	for _, e := range msg.Entities {
		if e.Type == "mention" {
			mention := msg.Text[e.Offset : e.Offset+e.Length]
			if strings.EqualFold(mention, botAt) { return true }
		}
	}
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil {
		return msg.ReplyToMessage.From.ID == t.botUser.ID
	}
	return false
}

func (t *TelegramChannel) isAdmin(ctx context.Context, b *bot.Bot, chatID, userID int64) bool {
	member, err := b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: userID})
	if err != nil { return false }
	s := member.Administrator
	return s != nil || member.Owner != nil
}

// --- Typing ---

func (t *TelegramChannel) startTyping(ctx context.Context, b *bot.Bot, chatID int64) {
	for i := 0; i < 15; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}
		b.SendChatAction(ctx, &bot.SendChatActionParams{ChatID: chatID, Action: "typing"})
		select {
		case <-ctx.Done():
			return
		case <-time.After(typingInterval):
		}
	}
}

// --- Reactions (proper API via Bot API 9.5) ---

func (t *TelegramChannel) setReaction(ctx context.Context, b *bot.Bot, chatID int64, msgID int, emoji string) {
	b.SetMessageReaction(ctx, &bot.SetMessageReactionParams{
		ChatID:    chatID,
		MessageID: msgID,
		Reaction:  []models.ReactionType{{ReactionTypeEmoji: &models.ReactionTypeEmoji{Emoji: emoji}}},
	})
}

func (t *TelegramChannel) SetSuccessReaction(ctx context.Context, chatID int64, msgID int) {
	t.setReaction(ctx, t.b, chatID, msgID, "✅")
}

func (t *TelegramChannel) SetErrorReaction(ctx context.Context, chatID int64, msgID int) {
	t.setReaction(ctx, t.b, chatID, msgID, "❌")
}

// --- Media Processing ---

func (t *TelegramChannel) processMedia(ctx context.Context, b *bot.Bot, mediaType, fileID string) string {
	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil { return fmt.Sprintf("[%s — download failed]", mediaType) }
	fileURL := b.FileDownloadLink(file)

	switch mediaType {
	case "photo":
		data, err := downloadFile(fileURL)
		if err != nil { return "[Photo — download failed]" }
		return fmt.Sprintf("<attached_file name=\"photo.jpg\" type=\"image/jpeg\">\n[image data: %d bytes]\n</attached_file>", len(data))
	case "voice", "audio":
		slog.Info("telegram.voice.processing", "has_transcribe", t.Transcribe != nil, "file_url_len", len(fileURL))
		if t.Transcribe != nil {
			data, err := downloadFile(fileURL)
			if err != nil {
				slog.Warn("telegram.voice.download_failed", "error", err, "url", fileURL[:min(len(fileURL), 80)])
			} else if len(data) == 0 {
				slog.Warn("telegram.voice.empty_download", "url", fileURL[:min(len(fileURL), 80)])
			} else {
				slog.Info("telegram.voice.downloaded", "bytes", len(data))
				transcript, err := t.Transcribe(ctx, data, "ogg")
				if err == nil && transcript != "" {
					slog.Info("telegram.voice.transcribed", "text", transcript[:min(len(transcript), 80)])
					return transcript
				}
				slog.Warn("telegram.stt.failed", "error", err, "transcript_len", len(transcript))
			}
		} else {
			slog.Warn("telegram.voice.no_transcribe_func")
		}
		return fmt.Sprintf("[%s message — transcription not available]", mediaType)
	case "document":
		if file.FilePath != "" && isTextFile(file.FilePath) {
			data, err := downloadFile(fileURL)
			if err == nil && len(data) < 50000 {
				return fmt.Sprintf("<attached_file name=\"%s\">\n%s\n</attached_file>", file.FilePath, string(data))
			}
		}
		return fmt.Sprintf("[Document: %s]", file.FilePath)
	default:
		return fmt.Sprintf("[%s attachment]", mediaType)
	}
}

// --- Send (3-level fallback: MarkdownV2 → HTML → plain) ---

func (t *TelegramChannel) Send(_ context.Context, msg channels.OutboundMessage) error {
	if t.b == nil { return fmt.Errorf("not connected") }
	var chatID int64
	fmt.Sscanf(msg.RecipientID, "%d", &chatID)
	if chatID == 0 { return fmt.Errorf("invalid chat_id") }

	// Stop typing indicator for this chat as soon as we have a reply ready.
	if cancel, loaded := t.typingCancels.LoadAndDelete(chatID); loaded {
		cancel.(context.CancelFunc)()
	}

	ctx := context.Background()
	content := strings.TrimSpace(msg.Content)

	// FIX: Skip whitespace-only replies (Qorven #56620 — prevents 400 empty-text crash)
	if content == "" { return nil }

	// FIX: Suppress NO_REPLY control envelopes (Qorven #56612)
	if content == `{"action":"NO_REPLY"}` { return nil }

	if msg.ReplyTo != "" {
		// FIX: Validate replyToMessageId (Qorven #56587 — reject non-numeric)
		var msgID int
		fmt.Sscanf(msg.ReplyTo, "%d", &msgID)
		if msgID > 0 { return t.editMessage(ctx, chatID, msgID, content) }
	}

	// Get thread ID from metadata if present (forum topic support)
	var threadID int
	if tid, ok := msg.Metadata["thread_id"]; ok {
		fmt.Sscanf(tid, "%d", &threadID)
	}

	chunks := splitMessage(content, maxMessageLen)
	for i, chunk := range chunks {
		if i > 0 { time.Sleep(1100 * time.Millisecond) } // stay under 1 msg/sec per chat limit
		if err := t.sendFormatted(ctx, chatID, threadID, chunk); err != nil {
			// FIX: Treat 403 "bot not member" as permanent failure (Qorven #53635)
			if strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "Forbidden") {
				slog.Warn("telegram.send.permanent_failure", "chat", chatID, "error", err)
				return err // Don't retry doomed chats
			}
			return err
		}
	}
	return nil
}

// StopTyping cancels the typing indicator for a chat without sending a message.
// Call when the reply is delivered via EditMessage instead of Send.
func (t *TelegramChannel) StopTyping(chatID int64) {
	if cancel, loaded := t.typingCancels.LoadAndDelete(chatID); loaded {
		cancel.(context.CancelFunc)()
	}
}

// SendText sends a plain text message to any chat ID. Used for system notifications
// like OTP codes that are not tied to an agent conversation.
func (t *TelegramChannel) SendText(ctx context.Context, chatID int64, text string) error {
	if t.b == nil { return fmt.Errorf("not connected") }
	return t.sendFormatted(ctx, chatID, 0, text)
}

func (t *TelegramChannel) sendFormatted(ctx context.Context, chatID int64, threadID int, text string) error {
	// Try HTML (most reliable — no escaping landmines)
	params := &bot.SendMessageParams{ChatID: chatID, Text: markdownToHTML(text), ParseMode: models.ParseModeHTML}
	if threadID > 0 { params.MessageThreadID = threadID }
	_, err := t.b.SendMessage(ctx, params)
	if err == nil { return nil }

	// Fallback: plain
	params = &bot.SendMessageParams{ChatID: chatID, Text: stripFormatting(text)}
	if threadID > 0 { params.MessageThreadID = threadID }
	_, err = t.b.SendMessage(ctx, params)
	return err
}

func (t *TelegramChannel) editMessage(ctx context.Context, chatID int64, msgID int, text string) error {
	_, err := t.b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: chatID, MessageID: msgID, Text: markdownToHTML(text), ParseMode: models.ParseModeHTML,
	})
	if err != nil {
		_, err = t.b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID: chatID, MessageID: msgID, Text: stripFormatting(text),
		})
	}
	return err
}

// --- Send Media ---

func (t *TelegramChannel) SendPhoto(ctx context.Context, chatID int64, url, caption string) error {
	p := &bot.SendPhotoParams{ChatID: chatID, Photo: &models.InputFileString{Data: url}}
	if caption != "" { p.Caption = caption; p.ParseMode = models.ParseModeHTML }
	_, err := t.b.SendPhoto(ctx, p)
	if err != nil {
		// FIX: Fallback to document when photo fails (Qorven #52545 — PHOTO_INVALID_DIMENSIONS)
		if strings.Contains(err.Error(), "PHOTO_INVALID") || strings.Contains(err.Error(), "wrong file") {
			slog.Info("telegram.photo.fallback_to_document", "url", url)
			return t.SendDocument(ctx, chatID, url, caption)
		}
		return err
	}
	return nil
}

func (t *TelegramChannel) SendDocument(ctx context.Context, chatID int64, url, caption string) error {
	p := &bot.SendDocumentParams{ChatID: chatID, Document: &models.InputFileString{Data: url}}
	if caption != "" { p.Caption = caption }
	_, err := t.b.SendDocument(ctx, p)
	return err
}

func (t *TelegramChannel) SendVoice(ctx context.Context, chatID int64, filePathOrURL string) error {
	// If it's a local file, upload it; otherwise treat as URL
	if strings.HasPrefix(filePathOrURL, "/") || strings.HasPrefix(filePathOrURL, "./") {
		f, err := os.Open(filePathOrURL)
		if err != nil { return fmt.Errorf("open voice file: %w", err) }
		defer f.Close()
		_, err = t.b.SendVoice(ctx, &bot.SendVoiceParams{
			ChatID: chatID,
			Voice:  &models.InputFileUpload{Filename: "voice.ogg", Data: f},
		})
		return err
	}
	_, err := t.b.SendVoice(ctx, &bot.SendVoiceParams{ChatID: chatID, Voice: &models.InputFileString{Data: filePathOrURL}})
	return err
}

func (t *TelegramChannel) SendVideo(ctx context.Context, chatID int64, url, caption string) error {
	p := &bot.SendVideoParams{ChatID: chatID, Video: &models.InputFileString{Data: url}}
	if caption != "" { p.Caption = caption }
	_, err := t.b.SendVideo(ctx, p)
	return err
}

// --- Inline Keyboard ---

func (t *TelegramChannel) SendWithButtons(ctx context.Context, chatID int64, text string, buttons [][]InlineButton) error {
	var rows [][]models.InlineKeyboardButton
	for _, row := range buttons {
		var kbRow []models.InlineKeyboardButton
		for _, btn := range row {
			if btn.URL != "" {
				kbRow = append(kbRow, models.InlineKeyboardButton{Text: btn.Text, URL: btn.URL})
			} else {
				kbRow = append(kbRow, models.InlineKeyboardButton{Text: btn.Text, CallbackData: btn.Data})
			}
		}
		rows = append(rows, kbRow)
	}
	_, err := t.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID, Text: markdownToHTML(text), ParseMode: models.ParseModeHTML,
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

type InlineButton struct{ Text, Data, URL string }

// --- Streaming (placeholder → edit → final) ---

func (t *TelegramChannel) CreateStream(ctx context.Context, chatIDStr string, isThinking bool) (channels.ChannelStream, error) {
	var chatID int64
	fmt.Sscanf(chatIDStr, "%d", &chatID)
	// Use a minimal placeholder — the typing indicator already shows activity.
	placeholder := "..."
	sent, err := t.b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: placeholder})
	if err != nil { return nil, err }
	return &tgStream{b: t.b, chatID: chatID, msgID: sent.ID, throttle: time.Duration(streamThrottleMs) * time.Millisecond}, nil
}

func (t *TelegramChannel) SendPlaceholder(chatID int64, text string) (int, error) {
	sent, err := t.b.SendMessage(context.Background(), &bot.SendMessageParams{ChatID: chatID, Text: text})
	if err != nil { return 0, err }
	return sent.ID, nil
}

func (t *TelegramChannel) EditMessage(chatID int64, msgID int, text string) error {
	return t.editMessage(context.Background(), chatID, msgID, text)
}

type tgStream struct {
	b        *bot.Bot
	chatID   int64
	msgID    int
	throttle time.Duration
	lastEdit time.Time
	lastText string
	mu       sync.Mutex
	stopped  bool
}

func (s *tgStream) Update(_ context.Context, text string) error {
	s.mu.Lock(); defer s.mu.Unlock()
	if s.stopped || text == s.lastText { return nil }
	if time.Since(s.lastEdit) < s.throttle { return nil }
	display := text + "▌"
	if len(display) > maxMessageLen { display = display[len(display)-maxMessageLen:] }
	s.b.EditMessageText(context.Background(), &bot.EditMessageTextParams{ChatID: s.chatID, MessageID: s.msgID, Text: display})
	s.lastEdit = time.Now(); s.lastText = text
	return nil
}

func (s *tgStream) UpdateThinking(_ context.Context, thinking string) error {
	s.mu.Lock(); defer s.mu.Unlock()
	if s.stopped { return nil }
	if time.Since(s.lastEdit) < s.throttle { return nil }
	display := "💭 Thinking...\n\n" + thinking
	if len(display) > maxMessageLen { display = display[:maxMessageLen] }
	s.b.EditMessageText(context.Background(), &bot.EditMessageTextParams{ChatID: s.chatID, MessageID: s.msgID, Text: display})
	s.lastEdit = time.Now()
	return nil
}

func (s *tgStream) UpdateToolStatus(_ context.Context, tool, status string) error {
	s.mu.Lock(); defer s.mu.Unlock()
	if s.stopped { return nil }
	if time.Since(s.lastEdit) < s.throttle { return nil }
	display := s.lastText + "\n\n🔧 " + tool + "... " + status
	if len(display) > maxMessageLen { display = display[len(display)-maxMessageLen:] }
	s.b.EditMessageText(context.Background(), &bot.EditMessageTextParams{ChatID: s.chatID, MessageID: s.msgID, Text: display})
	s.lastEdit = time.Now()
	return nil
}

func (s *tgStream) Stop(_ context.Context) error {
	s.mu.Lock(); defer s.mu.Unlock()
	s.stopped = true
	if s.lastText != "" {
		s.b.EditMessageText(context.Background(), &bot.EditMessageTextParams{
			ChatID: s.chatID, MessageID: s.msgID, Text: markdownToHTML(s.lastText), ParseMode: models.ParseModeHTML,
		})
	}
	return nil
}

func (s *tgStream) MessageID() string { return fmt.Sprintf("%d", s.msgID) }

// --- Commands ---

func (t *TelegramChannel) cmdStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	t.sendFormatted(ctx, update.Message.Chat.ID, 0, fmt.Sprintf("👋 Hi %s! I'm **%s**, your Qorven AI assistant.\n\nJust send me a message!", update.Message.From.FirstName, t.cfg.BotName))
}
func (t *TelegramChannel) cmdHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	t.sendFormatted(ctx, update.Message.Chat.ID, 0, "**Commands:**\n/start — Start\n/help — Help\n/status — Status\n/pair — Pair account\n/tasks — View tasks\n/cancel — Cancel")
}
func (t *TelegramChannel) cmdStatus(ctx context.Context, b *bot.Bot, update *models.Update) {
	t.sendFormatted(ctx, update.Message.Chat.ID, 0, fmt.Sprintf("✅ **%s** online.\nAgent: `%s`", t.cfg.BotName, t.cfg.AgentID[:8]))
}
func (t *TelegramChannel) cmdCancel(ctx context.Context, b *bot.Bot, update *models.Update) {
	t.sendFormatted(ctx, update.Message.Chat.ID, 0, "🛑 Cancelled.")
}
func (t *TelegramChannel) cmdPair(ctx context.Context, b *bot.Bot, update *models.Update) {
	t.handlePairCommand(ctx, update.Message.Chat.ID, update.Message.From.ID, update.Message.From.FirstName)
}
func (t *TelegramChannel) cmdTasks(ctx context.Context, b *bot.Bot, update *models.Update) {
	t.sendFormatted(ctx, update.Message.Chat.ID, 0, "📋 View tasks at the Qorven dashboard.")
}

func (t *TelegramChannel) cmdVoice(ctx context.Context, b *bot.Bot, update *models.Update) {
	text := strings.TrimSpace(update.Message.Text)
	chatID := update.Message.Chat.ID
	switch {
	case strings.Contains(text, "off"):
		t.sendFormatted(ctx, chatID, 0, "🔇 Voice mode **OFF** — I'll reply with text only.")
	case strings.Contains(text, "on"), strings.Contains(text, "always"):
		t.sendFormatted(ctx, chatID, 0, "🔊 Voice mode **ON** — I'll reply with voice to all messages.")
	case strings.Contains(text, "reply"):
		t.sendFormatted(ctx, chatID, 0, "🎤 Voice mode **REPLY** — I'll reply with voice only when you send voice messages.")
	default:
		t.sendFormatted(ctx, chatID, 0, "🎤 **Voice Mode**\n\n`/voice off` — Text replies only\n`/voice reply` — Voice reply to voice messages\n`/voice on` — Voice reply to everything")
	}
}

func (t *TelegramChannel) handleCallback(ctx context.Context, b *bot.Bot, cb *models.CallbackQuery) {
	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
	if cb.Data != "" && t.handler != nil {
		t.handler(ctx, channels.InboundMessage{
			ChannelName: t.Name(), ChannelType: "telegram", AgentID: t.cfg.AgentID,
			SenderID: fmt.Sprintf("%d", cb.From.ID), SenderName: cb.From.FirstName, Content: cb.Data,
			Metadata: map[string]string{"chat_id": fmt.Sprintf("%d", cb.Message.Message.Chat.ID), "callback": "true"},
		})
	}
}

// --- Voice Guard ---

type voiceGuard struct {
	mu     sync.Mutex
	counts map[string][]time.Time
}

func newVoiceGuard() *voiceGuard { return &voiceGuard{counts: make(map[string][]time.Time)} }

func (vg *voiceGuard) Allow(senderID string) bool {
	vg.mu.Lock(); defer vg.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-voiceGuardWindow)
	var valid []time.Time
	for _, t := range vg.counts[senderID] {
		if t.After(cutoff) { valid = append(valid, t) }
	}
	if len(valid) >= voiceGuardMax { return false }
	vg.counts[senderID] = append(valid, now)
	return true
}

// --- Helpers ---

func downloadFile(url string) ([]byte, error) {
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Get(url)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, maxFileDownload))
}

func markdownToHTML(md string) string {
	md = strings.ReplaceAll(md, "&", "&amp;")
	md = strings.ReplaceAll(md, "<", "&lt;")
	md = strings.ReplaceAll(md, ">", "&gt;")
	lines := strings.Split(md, "\n")
	var result []string
	inCode := false
	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if inCode { result = append(result, "</code></pre>"); inCode = false } else {
				lang := strings.TrimPrefix(line, "```")
				if lang != "" { result = append(result, fmt.Sprintf("<pre><code class=\"language-%s\">", lang)) } else { result = append(result, "<pre><code>") }
				inCode = true
			}
			continue
		}
		if inCode { result = append(result, line); continue }
		for strings.Contains(line, "**") {
			i := strings.Index(line, "**"); j := strings.Index(line[i+2:], "**")
			if j < 0 { break }
			line = line[:i] + "<b>" + line[i+2:i+2+j] + "</b>" + line[i+2+j+2:]
		}
		for strings.Contains(line, "`") {
			i := strings.Index(line, "`"); j := strings.Index(line[i+1:], "`")
			if j < 0 { break }
			line = line[:i] + "<code>" + line[i+1:i+1+j] + "</code>" + line[i+1+j+1:]
		}
		result = append(result, line)
	}
	if inCode { result = append(result, "</code></pre>") }
	return strings.Join(result, "\n")
}

func stripFormatting(t string) string {
	t = strings.ReplaceAll(t, "**", ""); t = strings.ReplaceAll(t, "`", ""); t = strings.ReplaceAll(t, "```", "")
	return t
}

func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen { return []string{text} }
	var chunks []string
	for len(text) > 0 {
		if len(text) <= maxLen { chunks = append(chunks, text); break }
		cut := maxLen
		if idx := strings.LastIndex(text[:maxLen], "\n"); idx > maxLen/2 { cut = idx + 1 }
		chunks = append(chunks, text[:cut]); text = text[cut:]
	}
	return chunks
}

// Pairing command handler
func (t *TelegramChannel) handlePairCommand(ctx context.Context, chatID int64, senderID int64, senderName string) {
	t.sendFormatted(ctx, chatID, 0, fmt.Sprintf("🔐 **Pairing**\n\nYour Telegram ID: `%d`\nName: %s\n\nAsk your admin to approve from Qorven dashboard → Pairing.", senderID, senderName))
}

// Strip bot @mention from text
func (t *TelegramChannel) stripBotMention(text string) string {
	if t.botUser == nil { return text }
	mention := "@" + t.botUser.Username
	text = strings.ReplaceAll(text, mention, "")
	return strings.TrimSpace(text)
}

// Polling conflict detection (409 error)
func isPollConflict(err error) bool {
	if err == nil { return false }
	return strings.Contains(err.Error(), "409") || strings.Contains(strings.ToLower(err.Error()), "conflict")
}

// Register bot commands menu
func (t *TelegramChannel) registerCommands(ctx context.Context) {
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	t.b.SetMyCommands(tctx, &bot.SetMyCommandsParams{Commands: []models.BotCommand{
		{Command: "start", Description: "Start chatting"},
		{Command: "help", Description: "Show commands"},
		{Command: "status", Description: "Soul status"},
		{Command: "cancel", Description: "Cancel operation"},
		{Command: "pair", Description: "Pair account"},
		{Command: "tasks", Description: "View tasks"},
		{Command: "voice", Description: "Voice mode (off/reply/on)"},
	}})
}

func (t *TelegramChannel) evictDedup(key string, after time.Duration) {
	time.Sleep(after)
	t.dedup.Delete(key)
}

// Pairing reply debounce
var pairingDebounceMap sync.Map

func shouldSendPairingReply(senderID string) bool {
	if last, ok := pairingDebounceMap.Load(senderID); ok {
		if time.Since(last.(time.Time)) < pairingDebounce { return false }
	}
	pairingDebounceMap.Store(senderID, time.Now())
	return true
}
