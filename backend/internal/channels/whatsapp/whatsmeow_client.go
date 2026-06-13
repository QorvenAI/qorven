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
func (w *WhatsAppChannel) startWhatsmeow(ctx context.Context) error {
	if w.cfg.DBDSN == "" {
		return fmt.Errorf("whatsmeow: DBDSN is required for qr mode")
	}

	container, err := sqlstore.New(ctx, "pgx", w.cfg.DBDSN, waLog.Noop)
	if err != nil {
		return fmt.Errorf("whatsmeow: open device store: %w", err)
	}
	w.wmContainer = container

	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("whatsmeow: get device: %w", err)
	}

	client := whatsmeow.NewClient(device, waLog.Noop)
	w.wmClient = client

	client.AddEventHandler(func(evt any) {
		w.onWhatsmeowEvent(ctx, evt)
	})

	if client.Store.ID == nil {
		// Device not yet paired — start QR flow.
		qrChan, err := client.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("whatsmeow: get qr channel: %w", err)
		}
		go func() {
			for item := range qrChan {
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
func (w *WhatsAppChannel) stopWhatsmeow() {
	if w.wmClient != nil {
		w.wmClient.Disconnect()
		w.wmClient = nil
	}
	if w.wmContainer != nil {
		if err := w.wmContainer.Close(); err != nil {
			slog.Warn("whatsapp.qr.container_close", "error", err)
		}
		w.wmContainer = nil
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
func (w *WhatsAppChannel) sendWhatsmeow(ctx context.Context, to, text string) error {
	if w.wmClient == nil {
		return fmt.Errorf("whatsmeow: client not initialised")
	}

	jid, err := parseWhatsmeowJID(to)
	if err != nil {
		return fmt.Errorf("whatsmeow: invalid recipient %q: %w", to, err)
	}

	for _, chunk := range chunkText(text, 4096) {
		msg := &waE2E.Message{Conversation: proto.String(chunk)}
		if _, err := w.wmClient.SendMessage(ctx, jid, msg); err != nil {
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

