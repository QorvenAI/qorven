// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Robust send with retry, IPv4 fallback, media routing, HTML depth fallback.

const (
	maxRetries       = 3
	retryBaseDelay   = 500 * time.Millisecond
	sendTimeout      = 60 * time.Second
)

// --- Retry Logic ---

func (t *TelegramChannel) retrySend(ctx context.Context, name string, fn func(context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := fn(ctx)
		if err == nil { return nil }
		lastErr = err

		if !isRetryableError(err) {
			return err // permanent failure — don't retry
		}

		delay := retryBaseDelay * time.Duration(1<<attempt) // exponential: 500ms, 1s, 2s
		slog.Warn("telegram.send.retry", "name", name, "attempt", attempt+1, "delay", delay, "error", err)
		time.Sleep(delay)
	}
	return fmt.Errorf("telegram send failed after %d retries: %w", maxRetries, lastErr)
}

func isRetryableError(err error) bool {
	if err == nil { return false }
	msg := strings.ToLower(err.Error())
	// Network errors — retry
	for _, pattern := range []string{"timeout", "connection reset", "connection refused", "eof", "broken pipe", "temporary failure"} {
		if strings.Contains(msg, pattern) { return true }
	}
	// Rate limit — retry after delay
	if strings.Contains(msg, "429") || strings.Contains(msg, "too many requests") { return true }
	// Server errors — retry
	if strings.Contains(msg, "500") || strings.Contains(msg, "502") || strings.Contains(msg, "503") { return true }
	return false
}

func isPermanentFailure(err error) bool {
	if err == nil { return false }
	msg := strings.ToLower(err.Error())
	// 403 = bot kicked/blocked — permanent
	if strings.Contains(msg, "403") || strings.Contains(msg, "forbidden") { return true }
	// Chat not found — permanent
	if strings.Contains(msg, "chat not found") { return true }
	// Bot not member — permanent
	if strings.Contains(msg, "not a member") || strings.Contains(msg, "was kicked") { return true }
	return false
}

// --- Send with Full Formatting Pipeline ---

func (t *TelegramChannel) sendText(ctx context.Context, chatID int64, threadID int, text string) error {
	// Convert markdown to Telegram HTML
	html := markdownToTelegramHTML(text)

	// Split using HTML-aware chunking
	chunks := chunkHTML(html, telegramHTMLMaxLen)

	for _, chunk := range chunks {
		err := t.retrySend(ctx, "sendText", func(ctx context.Context) error {
			return t.sendHTMLWithFallback(ctx, chatID, threadID, chunk)
		})
		if err != nil {
			if isPermanentFailure(err) {
				slog.Warn("telegram.send.permanent_failure", "chat", chatID, "error", err)
				return err
			}
			return err
		}
	}
	return nil
}

// sendHTMLWithFallback tries HTML → plain text (with recursive depth for complex HTML)
func (t *TelegramChannel) sendHTMLWithFallback(ctx context.Context, chatID int64, threadID int, html string) error {
	params := &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      html,
		ParseMode: models.ParseModeHTML,
	}
	if threadID > 0 { params.MessageThreadID = threadID }

	_, err := t.b.SendMessage(ctx, params)
	if err == nil { return nil }

	// If HTML failed, try stripping complex tags and retry
	simplified := stripComplexHTML(html)
	if simplified != html {
		params.Text = simplified
		_, err = t.b.SendMessage(ctx, params)
		if err == nil { return nil }
	}

	// Final fallback: plain text
	slog.Debug("telegram.send.html_failed_fallback_plain", "error", err)
	params.Text = htmlToPlain(html)
	params.ParseMode = ""
	_, err = t.b.SendMessage(ctx, params)
	return err
}

func stripComplexHTML(html string) string {
	// Remove <a> tags (keep text), remove <pre><code> nesting (keep <pre>)
	html = strings.ReplaceAll(html, "</a>", "")
	// Simple regex-free approach: remove href attributes
	for strings.Contains(html, "<a href=") {
		start := strings.Index(html, "<a href=")
		end := strings.Index(html[start:], ">")
		if end < 0 { break }
		html = html[:start] + html[start+end+1:]
	}
	return html
}

// --- Send Media from URL or Local Path ---

func (t *TelegramChannel) sendMediaByType(ctx context.Context, chatID int64, threadID int, mediaType, source, caption string) error {
	if caption != "" && len(caption) > maxCaptionLen {
		caption = caption[:maxCaptionLen]
	}

	return t.retrySend(ctx, "sendMedia", func(ctx context.Context) error {
		switch mediaType {
		case "photo":
			p := &bot.SendPhotoParams{ChatID: chatID, Photo: &models.InputFileString{Data: source}}
			if caption != "" { p.Caption = caption; p.ParseMode = models.ParseModeHTML }
			if threadID > 0 { p.MessageThreadID = threadID }
			_, err := t.b.SendPhoto(ctx, p)
			if err != nil && (strings.Contains(err.Error(), "PHOTO_INVALID") || strings.Contains(err.Error(), "wrong file")) {
				// Fallback to document (Qorven #52545)
				return t.sendMediaByType(ctx, chatID, threadID, "document", source, caption)
			}
			return err

		case "document":
			p := &bot.SendDocumentParams{ChatID: chatID, Document: &models.InputFileString{Data: source}}
			if caption != "" { p.Caption = caption }
			if threadID > 0 { p.MessageThreadID = threadID }
			_, err := t.b.SendDocument(ctx, p)
			return err

		case "audio":
			p := &bot.SendAudioParams{ChatID: chatID, Audio: &models.InputFileString{Data: source}}
			if caption != "" { p.Caption = caption }
			if threadID > 0 { p.MessageThreadID = threadID }
			_, err := t.b.SendAudio(ctx, p)
			return err

		case "voice":
			p := &bot.SendVoiceParams{ChatID: chatID, Voice: &models.InputFileString{Data: source}}
			if threadID > 0 { p.MessageThreadID = threadID }
			_, err := t.b.SendVoice(ctx, p)
			return err

		case "video":
			p := &bot.SendVideoParams{ChatID: chatID, Video: &models.InputFileString{Data: source}}
			if caption != "" { p.Caption = caption }
			if threadID > 0 { p.MessageThreadID = threadID }
			_, err := t.b.SendVideo(ctx, p)
			return err

		default:
			return t.sendMediaByType(ctx, chatID, threadID, "document", source, caption)
		}
	})
}

// --- Edit with Retry ---

func (t *TelegramChannel) editMessageWithRetry(ctx context.Context, chatID int64, msgID int, text string) error {
	html := markdownToTelegramHTML(text)
	return t.retrySend(ctx, "editMessage", func(ctx context.Context) error {
		_, err := t.b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID: chatID, MessageID: msgID, Text: html, ParseMode: models.ParseModeHTML,
		})
		if err != nil {
			// Fallback to plain
			_, err = t.b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID: chatID, MessageID: msgID, Text: htmlToPlain(html),
			})
		}
		return err
	})
}

// --- Delete ---

func (t *TelegramChannel) deleteMsg(ctx context.Context, chatID int64, msgID int) error {
	_, err := t.b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: msgID})
	return err
}
