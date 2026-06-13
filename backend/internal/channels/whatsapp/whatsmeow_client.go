// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package whatsapp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver with database/sql
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/qorvenai/qorven/internal/channels"
)

// startWhatsmeow initialises the whatsmeow client for QR / multi-device mode.
// It opens the persistent device store, creates the client, then either starts
// a QR flow (unpaired device) or reconnects directly (already paired).
//
// Concurrency: container and client are constructed locally, then assigned to
// the struct fields under w.mu before any network I/O so that concurrent
// sendWhatsmeow or stopWhatsmeow calls always see a consistent view.
func (w *WhatsAppChannel) startWhatsmeow(ctx context.Context) error {
	if w.cfg.DBDSN == "" {
		return fmt.Errorf("whatsmeow: DBDSN is required for qr mode")
	}

	container, err := sqlstore.New(ctx, "pgx", w.cfg.DBDSN, waLog.Noop)
	if err != nil {
		return fmt.Errorf("whatsmeow: open device store: %w", err)
	}

	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("whatsmeow: get device: %w", err)
	}

	client := whatsmeow.NewClient(device, waLog.Noop)

	client.AddEventHandler(func(evt any) {
		w.onWhatsmeowEvent(ctx, evt)
	})

	// Assign fields under the lock before any blocking I/O so that an early
	// inbound event (fired by Connect) sees a non-nil client.
	w.mu.Lock()
	w.wmClient = client
	w.wmContainer = container
	w.mu.Unlock()

	if client.Store.ID == nil {
		// Device not yet paired — start QR flow.
		qrChan, err := client.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("whatsmeow: get qr channel: %w", err)
		}
		// Fix 2: honour ctx so the goroutine exits when Stop is called even if
		// qrChan is never drained.
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-qrChan:
					if !ok {
						return
					}
					switch item.Event {
					case "code":
						w.publishQR(item.Code)
					case "success":
						slog.Info("whatsapp.qr.paired", "agent", w.cfg.AgentID)
					default:
						if item.Error != nil {
							slog.Warn("whatsapp.qr.error", "event", item.Event, "error", item.Error)
						} else {
							slog.Warn("whatsapp.qr.event", "event", item.Event)
						}
					}
				}
			}
		}()
		if err := client.Connect(); err != nil {
			return fmt.Errorf("whatsmeow: connect (qr): %w", err)
		}
	} else {
		// Already paired — reconnect directly.
		if err := client.Connect(); err != nil {
			return fmt.Errorf("whatsmeow: connect: %w", err)
		}
	}

	slog.Info("whatsapp.qr.started", "agent", w.cfg.AgentID, "paired", client.Store.ID != nil)
	return nil
}

// stopWhatsmeow disconnects the whatsmeow client and closes its device store.
// Concurrency: fields are swapped to nil under w.mu so that a concurrent
// sendWhatsmeow or a second stopWhatsmeow call sees nil immediately and does
// nothing, while this call operates on the local copies outside the lock.
func (w *WhatsAppChannel) stopWhatsmeow() {
	w.mu.Lock()
	c := w.wmClient
	cont := w.wmContainer
	w.wmClient = nil
	w.wmContainer = nil
	w.mu.Unlock()

	if c != nil {
		c.Disconnect()
	}
	if cont != nil {
		if err := cont.Close(); err != nil {
			slog.Warn("whatsapp.qr.container_close", "error", err)
		}
	}
}

// onWhatsmeowEvent is the single event handler registered with the whatsmeow client.
func (w *WhatsAppChannel) onWhatsmeowEvent(ctx context.Context, evt any) {
	switch e := evt.(type) {
	case *events.Message:
		w.handleWhatsmeowMessage(ctx, e)
	case *events.LoggedOut:
		slog.Warn("whatsapp.qr.logged_out", "agent", w.cfg.AgentID, "reason", e.Reason)
	}
}

// handleWhatsmeowMessage converts an inbound whatsmeow event to a channels.InboundMessage
// and passes it to the registered handler.
func (w *WhatsAppChannel) handleWhatsmeowMessage(ctx context.Context, e *events.Message) {
	if e.Info.IsFromMe {
		return
	}

	text := whatsmeowMessageText(e)
	if text == "" {
		return
	}

	// Dedup by message ID — whatsmeow can re-deliver on reconnect.
	if _, already := w.dedup.LoadOrStore(e.Info.ID, time.Now()); already {
		return
	}
	go func() { time.Sleep(10 * time.Minute); w.dedup.Delete(e.Info.ID) }()

	senderJID := e.Info.Sender.ToNonAD().String()
	senderName := e.Info.PushName

	content := text
	if senderName != "" {
		content = fmt.Sprintf("[From: %s]\n%s", senderName, text)
	}

	chatID := senderJID
	peerKind := "direct"
	if e.Info.Chat.Server == "g.us" {
		chatID = e.Info.Chat.ToNonAD().String()
		peerKind = "group"
	}

	if w.handler != nil {
		w.handler(ctx, channels.InboundMessage{
			ChannelName: "whatsapp",
			ChannelType: "whatsapp",
			AgentID:     w.cfg.AgentID,
			SenderID:    senderJID,
			SenderName:  senderName,
			ChatID:      chatID,
			Content:     content,
			PeerKind:    peerKind,
			Metadata: map[string]string{
				"chat_id":    chatID,
				"message_id": e.Info.ID,
				"msg_type":   "text",
			},
		})
	}
}

// whatsmeowMessageText extracts the plain-text body from an inbound message event.
func whatsmeowMessageText(e *events.Message) string {
	if e.Message == nil {
		return ""
	}
	if t := e.Message.GetConversation(); t != "" {
		return t
	}
	return e.Message.GetExtendedTextMessage().GetText()
}

// sendWhatsmeow sends a text message via the whatsmeow client, chunking long messages.
// Concurrency: snapshots w.wmClient under w.mu and uses the local copy so that
// a concurrent stopWhatsmeow cannot nil the field mid-send.
func (w *WhatsAppChannel) sendWhatsmeow(ctx context.Context, to, text string) error {
	w.mu.Lock()
	c := w.wmClient
	w.mu.Unlock()

	if c == nil {
		return fmt.Errorf("whatsmeow: client not initialised")
	}

	jid, err := parseWhatsmeowJID(to)
	if err != nil {
		return fmt.Errorf("whatsmeow: invalid recipient %q: %w", to, err)
	}

	for _, chunk := range chunkText(text, 4096) {
		msg := &waE2E.Message{Conversation: proto.String(chunk)}
		if _, err := c.SendMessage(ctx, jid, msg); err != nil {
			return fmt.Errorf("whatsmeow: send: %w", err)
		}
	}
	return nil
}

// parseWhatsmeowJID parses a recipient string into a types.JID.
// Accepts a full JID ("15551234567@s.whatsapp.net") or a bare phone number.
// types.ParseJID("15551234567") puts the number in .Server, not .User — we detect
// that case (no @) and fall back to types.NewJID with the default user server.
func parseWhatsmeowJID(to string) (types.JID, error) {
	if to == "" {
		return types.JID{}, fmt.Errorf("recipient is empty")
	}
	// Only use ParseJID result when the input is a proper user@server string.
	for _, c := range to {
		if c == '@' {
			jid, err := types.ParseJID(to)
			if err != nil {
				return types.JID{}, err
			}
			return jid, nil
		}
	}
	// Bare phone number — attach the default user server.
	return types.NewJID(to, types.DefaultUserServer), nil
}

// chunkText splits s into chunks of at most max bytes.
// If s fits in one chunk (or max <= 0) it returns a single-element slice.
func chunkText(s string, max int) []string {
	if max <= 0 || len(s) <= max {
		return []string{s}
	}
	var out []string
	for len(s) > max {
		out = append(out, s[:max])
		s = s[max:]
	}
	if len(s) > 0 {
		out = append(out, s)
	}
	return out
}

