// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package telegram

import (
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
)

// Rich message context extraction: forward info, reply info, location, user names.

type MessageContext struct {
	ForwardFrom     string // "Forwarded from: User Name" or "Forwarded from: Channel Name"
	ReplyToText     string // text of the message being replied to
	ReplyToSender   string // who sent the replied-to message
	Location        string // "Location: lat, lng (name)"
	Contact         string // "Contact: Name (phone)"
	SenderFullName  string // "First Last (@username)"
}

// buildMessageContext extracts rich context from a Telegram message for the LLM
func buildMessageContext(msg *models.Message, botUsername string) *MessageContext {
	ctx := &MessageContext{}

	// Sender name
	if msg.From != nil {
		ctx.SenderFullName = buildUserName(msg.From)
	}

	// Forward info
	if msg.ForwardOrigin != nil {
		ctx.ForwardFrom = extractForwardOrigin(msg.ForwardOrigin)
	}

	// Reply context
	if msg.ReplyToMessage != nil {
		reply := msg.ReplyToMessage
		if reply.From != nil {
			ctx.ReplyToSender = buildUserName(reply.From)
		}
		if reply.Text != "" {
			text := reply.Text
			if len(text) > 200 { text = text[:200] + "..." }
			ctx.ReplyToText = text
		} else if reply.Caption != "" {
			text := reply.Caption
			if len(text) > 200 { text = text[:200] + "..." }
			ctx.ReplyToText = "[media] " + text
		}
	}

	// Location
	if msg.Location != nil {
		ctx.Location = fmt.Sprintf("%.6f, %.6f", msg.Location.Latitude, msg.Location.Longitude)
	}
	if msg.Venue != nil {
		ctx.Location = fmt.Sprintf("%s — %s (%.6f, %.6f)",
			msg.Venue.Title, msg.Venue.Address, msg.Venue.Location.Latitude, msg.Venue.Location.Longitude)
	}

	// Contact
	if msg.Contact != nil {
		name := msg.Contact.FirstName
		if msg.Contact.LastName != "" { name += " " + msg.Contact.LastName }
		ctx.Contact = fmt.Sprintf("%s (%s)", name, msg.Contact.PhoneNumber)
	}

	return ctx
}

// enrichContentWithContext prepends context info to the message content for the LLM
func enrichContentWithContext(content string, ctx *MessageContext) string {
	var parts []string

	if ctx.ForwardFrom != "" {
		parts = append(parts, fmt.Sprintf("[Forwarded from: %s]", ctx.ForwardFrom))
	}
	if ctx.ReplyToText != "" {
		sender := ctx.ReplyToSender
		if sender == "" { sender = "someone" }
		parts = append(parts, fmt.Sprintf("[Replying to %s: \"%s\"]", sender, ctx.ReplyToText))
	}
	if ctx.Location != "" {
		parts = append(parts, fmt.Sprintf("[Location shared: %s]", ctx.Location))
	}
	if ctx.Contact != "" {
		parts = append(parts, fmt.Sprintf("[Contact shared: %s]", ctx.Contact))
	}

	if len(parts) > 0 {
		return strings.Join(parts, "\n") + "\n\n" + content
	}
	return content
}

func extractForwardOrigin(origin *models.MessageOrigin) string {
	if origin == nil { return "" }
	if origin.MessageOriginUser != nil {
		return buildUserName(&origin.MessageOriginUser.SenderUser)
	}
	if origin.MessageOriginChannel != nil {
		ch := origin.MessageOriginChannel
		if ch.Chat.Title != "" { return ch.Chat.Title }
	}
	if origin.MessageOriginHiddenUser != nil {
		return origin.MessageOriginHiddenUser.SenderUserName
	}
	return "unknown"
}

func buildUserName(user *models.User) string {
	if user == nil { return "" }
	name := user.FirstName
	if user.LastName != "" { name += " " + user.LastName }
	if user.Username != "" { name += " (@" + user.Username + ")" }
	return name
}
