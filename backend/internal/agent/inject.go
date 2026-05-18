// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"github.com/qorvenai/qorven/internal/providers"
)

// InjectedMessage represents a user follow-up message injected mid-run.
type InjectedMessage struct {
	Content   string // user message content
	HideInput bool   // don't persist to session history
}

// InjectChannel is a channel for mid-run message injection.
// When set on RunRequest, the loop drains this channel at turn boundaries
// to inject user follow-up messages into the running conversation.
type InjectChannel <-chan InjectedMessage

// drainInjectChannel drains all pending messages from the inject channel.
// Returns two slices:
//   - forLLM: messages to append to the LLM context (all messages)
//   - forSession: messages to persist to session history (excludes HideInput)
func drainInjectChannel(ch InjectChannel, onEvent func(StreamEvent)) (forLLM, forSession []providers.Message) {
	if ch == nil {
		return nil, nil
	}

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return forLLM, forSession
			}

			// Emit event for UI
			if onEvent != nil {
				onEvent(StreamEvent{
					Type: "inject.received",
					Data: map[string]any{"content": truncateStr(msg.Content, 100)},
				})
			}

			// Build provider message
			pmsg := providers.Message{
				Role:    "user",
				Content: msg.Content,
			}

			forLLM = append(forLLM, pmsg)
			if !msg.HideInput {
				forSession = append(forSession, pmsg)
			}

		default:
			// Channel empty
			return forLLM, forSession
		}
	}
}

// MakeInjectChannel creates a buffered inject channel.
func MakeInjectChannel(bufSize int) chan InjectedMessage {
	if bufSize <= 0 {
		bufSize = 8
	}
	return make(chan InjectedMessage, bufSize)
}

// InjectMessage sends a message to the inject channel (non-blocking).
// Returns false if the channel is full or nil.
func InjectMessage(ch chan<- InjectedMessage, msg InjectedMessage) bool {
	if ch == nil {
		return false
	}
	select {
	case ch <- msg:
		return true
	default:
		return false // channel full
	}
}
