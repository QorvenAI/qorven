// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package bus

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// MessageBus routes messages between channels and the agent runtime,
// and broadcasts events to WebSocket subscribers.
type MessageBus struct {
	inbound  chan InboundMessage
	outbound chan OutboundMessage

	handlers   map[string]MessageHandler
	handlerMu  sync.RWMutex

	subscribers map[string]EventHandler
	subMu       sync.RWMutex
}

// New creates a new MessageBus.
func New() *MessageBus {
	return &MessageBus{
		inbound:     make(chan InboundMessage, 1000),
		outbound:    make(chan OutboundMessage, 1000),
		handlers:    make(map[string]MessageHandler),
		subscribers: make(map[string]EventHandler),
	}
}

// PublishInbound queues an inbound message from a channel.
func (mb *MessageBus) PublishInbound(msg InboundMessage) {
	mb.inbound <- msg
}

// TryPublishInbound attempts to queue an inbound message without blocking.
func (mb *MessageBus) TryPublishInbound(msg InboundMessage) bool {
	select {
	case mb.inbound <- msg:
		return true
	default:
		return false
	}
}

// ConsumeInbound blocks until an inbound message is available or ctx is cancelled.
func (mb *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, bool) {
	select {
	case msg := <-mb.inbound:
		return msg, true
	case <-ctx.Done():
		return InboundMessage{}, false
	}
}

// PublishOutbound queues an outbound message to a channel.
func (mb *MessageBus) PublishOutbound(msg OutboundMessage) {
	mb.outbound <- msg
}

// TryPublishOutbound attempts to queue an outbound message without blocking.
func (mb *MessageBus) TryPublishOutbound(msg OutboundMessage) bool {
	select {
	case mb.outbound <- msg:
		return true
	default:
		return false
	}
}

// SubscribeOutbound blocks until an outbound message is available or ctx is cancelled.
func (mb *MessageBus) SubscribeOutbound(ctx context.Context) (OutboundMessage, bool) {
	select {
	case msg := <-mb.outbound:
		return msg, true
	case <-ctx.Done():
		return OutboundMessage{}, false
	}
}

// RegisterHandler registers a message handler for a channel.
func (mb *MessageBus) RegisterHandler(channel string, handler MessageHandler) {
	mb.handlerMu.Lock()
	defer mb.handlerMu.Unlock()
	mb.handlers[channel] = handler
}

// GetHandler returns the message handler for a channel.
func (mb *MessageBus) GetHandler(channel string) (MessageHandler, bool) {
	mb.handlerMu.RLock()
	defer mb.handlerMu.RUnlock()
	handler, ok := mb.handlers[channel]
	return handler, ok
}

// Subscribe registers an event subscriber.
func (mb *MessageBus) Subscribe(id string, handler EventHandler) {
	mb.subMu.Lock()
	defer mb.subMu.Unlock()
	mb.subscribers[id] = handler
}

// Unsubscribe removes an event subscriber.
func (mb *MessageBus) Unsubscribe(id string) {
	mb.subMu.Lock()
	defer mb.subMu.Unlock()
	delete(mb.subscribers, id)
}

// Broadcast sends an event to all subscribers (non-blocking per subscriber).
func (mb *MessageBus) Broadcast(event Event) {
	mb.subMu.RLock()
	defer mb.subMu.RUnlock()
	for id, handler := range mb.subscribers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("bus: subscriber panicked",
						"subscriber", id,
						"event", event.Name,
						"panic", fmt.Sprint(r),
					)
				}
			}()
			handler(event)
		}()
	}
}

// Close shuts down the message bus.
func (mb *MessageBus) Close() {
	close(mb.inbound)
	close(mb.outbound)
}
