// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"log/slog"

	"github.com/qorvenai/qorven/internal/bus"
	"github.com/qorvenai/qorven/internal/channels"
)

// wireChannelEventSubscribers sets up event subscribers for channel lifecycle management.
// Called during gateway Start() after channels are loaded.
func (gw *Gateway) wireChannelEventSubscribers() {
	if gw.msgBus == nil {
		return
	}

	// 1. Channel reload on cache invalidation.
	// When channel instances are created/updated/deleted via API, reload all channels.
	gw.msgBus.Subscribe("channel-reload", func(evt bus.Event) {
		if evt.Name != "cache.invalidate.channels" {
			return
		}
		slog.Info("channel instances changed, reloading...")
		go gw.loadChannels()
	})

	// 2. Agent cascade disable: when an agent becomes inactive, stop its channels.
	// Without this, channels for deleted/inactive agents keep running and consuming resources.
	gw.msgBus.Subscribe("agent-cascade-channels", func(evt bus.Event) {
		if evt.Name != "agent.status.changed" {
			return
		}
		payload, ok := evt.Payload.(map[string]any)
		if !ok {
			return
		}
		newStatus, _ := payload["new_status"].(string)
		agentID, _ := payload["agent_id"].(string)
		if newStatus != "inactive" || agentID == "" {
			return
		}

		go func() {
			if gw.chanMgr == nil || gw.db == nil {
				return
			}
			// Find and disable channel instances for this agent
			tag, err := gw.db.Pool.Exec(context.Background(),
				`UPDATE channel_instances SET enabled = false, updated_at = NOW()
				 WHERE agent_id = $1 AND enabled = true`, agentID)
			if err != nil {
				slog.Warn("cascade_disable.failed", "agent", agentID, "error", err)
				return
			}
			if tag.RowsAffected() > 0 {
				slog.Info("cascade_disable.channels", "agent", agentID, "disabled", tag.RowsAffected())
				// Reload channels to stop the disabled ones
				gw.loadChannels()
			}
		}()
	})

	// 3. Pairing approval notification: send confirmation to the channel.
	gw.msgBus.Subscribe("pairing-approved", func(evt bus.Event) {
		if evt.Name != "pairing.approved" {
			return
		}
		payload, ok := evt.Payload.(map[string]any)
		if !ok {
			return
		}
		channel, _ := payload["channel"].(string)
		chatID, _ := payload["chat_id"].(string)
		if channel == "" || chatID == "" {
			return
		}

		msg := "✅ Access approved. Send a message to start chatting."
		if gw.chanMgr != nil {
			for _, ch := range gw.chanMgr.List() {
				if ch["running"] == true {
					_ = gw.chanMgr.Send(context.Background(), ch["id"].(string),
						channels.OutboundMessage{RecipientID: chatID, Content: msg})
					break
				}
			}
		}
	})

	slog.Info("channel event subscribers registered", "count", 3)
}
