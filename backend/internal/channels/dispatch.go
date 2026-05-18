// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package channels

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// WebhookRoute holds a path and handler pair for mounting on the main gateway mux.
type WebhookRoute struct {
	Path    string
	Handler http.Handler
}

// dispatchOutbound consumes outbound messages from the bus and routes them
// to the appropriate channel. Internal channels are silently skipped.
func (m *Manager) dispatchOutbound(ctx context.Context) {
	slog.Info("outbound dispatcher started")

	for {
		select {
		case <-ctx.Done():
			slog.Info("outbound dispatcher stopped")
			return
		default:
			msg, ok := m.bus.SubscribeOutbound(ctx)
			if !ok {
				continue
			}

			channelName := msg.Metadata["channel"]
			if channelName == "" {
				channelName = msg.RecipientID // fallback
			}

			if IsInternalChannel(channelName) {
				continue
			}

			m.mu.RLock()
			channel := m.channels[channelName]
			m.mu.RUnlock()

			if channel == nil {
				slog.Warn("unknown channel for outbound message", "channel", channelName)
				continue
			}

			// Filter out temp media files that no longer exist
			if len(msg.Media) > 0 {
				tmpDir := os.TempDir()
				filtered := msg.Media[:0]
				for _, media := range msg.Media {
					if media.URL != "" && strings.HasPrefix(media.URL, tmpDir) {
						if _, err := os.Stat(media.URL); err != nil {
							slog.Debug("skipping already-delivered temp media", "path", media.URL)
							continue
						}
					}
					filtered = append(filtered, media)
				}
				msg.Media = filtered
				if len(msg.Media) == 0 && msg.Content == "" {
					continue
				}
			}

			if err := channel.Send(ctx, msg); err != nil {
				slog.Error("error sending message to channel",
					"channel", channelName,
					"error", err,
				)
				// Send error notification for media failures
				if len(msg.Media) > 0 {
					notifyMsg := OutboundMessage{
						RecipientID: msg.ChatID,
						Content:     formatChannelSendError(err),
						Metadata:    sendErrorMeta(msg.Metadata),
					}
					if err2 := channel.Send(ctx, notifyMsg); err2 != nil {
						slog.Warn("failed to send error notification",
							"channel", channelName, "error", err2)
					}
				}
			}

			// Clean up temp media files
			tmpDir := os.TempDir()
			for _, media := range msg.Media {
				if media.URL != "" && strings.HasPrefix(media.URL, tmpDir) {
					os.Remove(media.URL)
				}
			}
		}
	}
}

// WebhookHandlers returns all webhook handlers from channels that implement WebhookChannel.
func (m *Manager) WebhookHandlers() []WebhookRoute {
	m.mu.RLock()
	defer m.mu.RUnlock()

	routes := []WebhookRoute{}
	for _, ch := range m.channels {
		if wh, ok := ch.(WebhookChannel); ok {
			if path, handler := wh.WebhookHandler(); path != "" && handler != nil {
				routes = append(routes, WebhookRoute{Path: path, Handler: handler})
			}
		}
	}
	return routes
}

// SendToChannel delivers a message to a specific channel by name.
func (m *Manager) SendToChannel(ctx context.Context, channelName, chatID, content string) error {
	m.mu.RLock()
	channel := m.channels[channelName]
	m.mu.RUnlock()

	if channel == nil {
		return nil
	}

	return channel.Send(ctx, OutboundMessage{
		RecipientID: chatID,
		Content:     content,
	})
}

var telegramAPIDescRe = regexp.MustCompile(`"Bad Request:\s*(.+?)"`)

func formatChannelSendError(err error) string {
	raw := err.Error()
	lower := strings.ToLower(raw)

	if m := telegramAPIDescRe.FindStringSubmatch(raw); len(m) == 2 {
		return "⚠️ Send failed: " + m[1]
	}

	switch {
	case strings.Contains(lower, "not enough rights"):
		return "⚠️ Send failed: bot doesn't have permission to send this type of message."
	case strings.Contains(lower, "chat not found"):
		return "⚠️ Send failed: chat not found."
	case strings.Contains(lower, "bot was blocked"):
		return "⚠️ Send failed: bot was blocked by the user."
	case strings.Contains(lower, "too many requests") || strings.Contains(lower, "flood"):
		return "⚠️ Send failed: rate limited. Please try again later."
	case strings.Contains(lower, "file is too big"):
		return "⚠️ Send failed: file is too large."
	}

	return "⚠️ Failed to deliver message. Check bot logs for details."
}

func sendErrorMeta(orig map[string]string) map[string]string {
	if orig == nil {
		return nil
	}
	meta := make(map[string]string)
	for _, k := range []string{"local_key", "message_thread_id"} {
		if v := orig[k]; v != "" {
			meta[k] = v
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}
