// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package zalo

import (
	"context"
	"log/slog"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// listener.go — WebSocket message listener for Zalo personal protocol.

// Listener connects to Zalo's WebSocket and emits parsed messages.
type Listener struct {
	sess          *Session
	ws            *WSClient
	messageCh     chan InboundMessage
	disconnectedCh chan CloseInfo
	errorCh       chan error
	stopCh        chan struct{}
}

// NewListener creates a listener for the given session.
func NewListener(sess *Session) (*Listener, error) {
	if sess.LoginInfo == nil { return nil, fmt.Errorf("zalo.listener: not logged in") }
	if len(sess.LoginInfo.WebSockets) == 0 { return nil, fmt.Errorf("zalo.listener: no websocket endpoints") }

	return &Listener{
		sess:           sess,
		messageCh:      make(chan InboundMessage, 64),
		disconnectedCh: make(chan CloseInfo, 1),
		errorCh:        make(chan error, 8),
		stopCh:         make(chan struct{}),
	}, nil
}

// Messages returns the channel for incoming messages.
func (ln *Listener) Messages() <-chan InboundMessage { return ln.messageCh }

// Disconnected returns the channel signaled on WebSocket disconnect.
func (ln *Listener) Disconnected() <-chan CloseInfo { return ln.disconnectedCh }

// Errors returns the channel for non-fatal errors.
func (ln *Listener) Errors() <-chan error { return ln.errorCh }

// Start connects to the WebSocket and begins listening.
func (ln *Listener) Start(ctx context.Context) error {
	wsURL := "wss://" + ln.sess.LoginInfo.WebSockets[0]
	headers := http.Header{}
	headers.Set("User-Agent", ln.sess.UserAgent)
	headers.Set("Origin", ZaloBaseURL)

	ws, err := DialWS(ctx, wsURL, headers, ln.sess.CookieJar)
	if err != nil { return fmt.Errorf("zalo.listener: connect: %w", err) }
	ln.ws = ws

	slog.Info("zalo.listener: connected", "url", wsURL)
	go ln.readLoop(ctx)
	return nil
}

// Stop closes the WebSocket connection.
func (ln *Listener) Stop() {
	close(ln.stopCh)
	if ln.ws != nil { ln.ws.Close(1000, "shutdown") }
}

func (ln *Listener) readLoop(ctx context.Context) {
	defer func() {
		close(ln.messageCh)
		close(ln.disconnectedCh)
	}()

	for {
		select {
		case <-ln.stopCh: return
		case <-ctx.Done(): return
		default:
		}

		data, err := ln.ws.ReadMessage(ctx)
		if err != nil {
			info := parseWSCloseInfo(err)
			slog.Warn("zalo.listener: disconnected", "code", info.Code, "reason", info.Reason)
			select {
			case ln.disconnectedCh <- info:
			default:
			}
			return
		}

		ln.handleFrame(data)
	}
}

func (ln *Listener) handleFrame(data []byte) {
	if len(data) < 4 { return }

	// Zalo frames: first 4 bytes = command type, rest = payload
	// Decrypt payload with session key
	decrypted, err := DecryptCBC(SecretKey(ln.sess.SecretKey).Bytes(), string(data[4:]))
	if err != nil {
		// Not all frames are encrypted — some are keepalive pings
		return
	}

	var msg struct {
		ActionID string `json:"actionId"`
		Data     struct {
			Content   string `json:"content"`
			FromUID   string `json:"fromUid"`
			ToUID     string `json:"toUid"`
			MsgID     string `json:"msgId"`
			MsgType   int    `json:"msgType"`
			Timestamp int64  `json:"ts"`
		} `json:"data"`
	}

	if err := json.Unmarshal(decrypted, &msg); err != nil { return }

	// Only handle text messages for now
	if msg.ActionID == "chat" || msg.ActionID == "group" {
		inbound := InboundMessage{
			MessageID: msg.Data.MsgID,
			SenderID:  msg.Data.FromUID,
			ChatID:    msg.Data.ToUID,
			Text:      msg.Data.Content,
			Timestamp: time.UnixMilli(msg.Data.Timestamp),
			IsGroup:   msg.ActionID == "group",
		}
		select {
		case ln.messageCh <- inbound:
		default:
			slog.Warn("zalo.listener: message channel full, dropping")
		}
	}
}
