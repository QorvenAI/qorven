// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const (
	logRingSize   = 100
	redactedValue = "***"
)

var sensitiveKeys = []string{"key", "token", "secret", "password", "dsn", "credential", "authorization", "cookie"}

// LogTee is a slog.Handler that forwards log records to subscribed WS clients
// while delegating to an underlying handler for normal output.
type LogTee struct {
	inner slog.Handler

	mu      sync.RWMutex
	clients map[string]*logSubscriber

	ringMu  sync.RWMutex
	ring    []map[string]any
	ringPos int
	ringFul bool
}

type logSubscriber struct {
	client *WSClient
	level  slog.Level
}

// NewLogTee wraps an existing slog.Handler so log records are also forwarded
// to any WebSocket clients that have started log tailing.
func NewLogTee(inner slog.Handler) *LogTee {
	return &LogTee{
		inner:   inner,
		clients: make(map[string]*logSubscriber),
		ring:    make([]map[string]any, logRingSize),
	}
}

func (t *LogTee) Enabled(ctx context.Context, level slog.Level) bool {
	if t.inner.Enabled(ctx, level) {
		return true
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, sub := range t.clients {
		if level >= sub.level {
			return true
		}
	}
	return false
}

func (t *LogTee) Handle(ctx context.Context, r slog.Record) error {
	entry := t.buildEntry(r)

	// Store in ring buffer
	t.ringMu.Lock()
	t.ring[t.ringPos] = entry
	t.ringPos = (t.ringPos + 1) % logRingSize
	if t.ringPos == 0 {
		t.ringFul = true
	}
	t.ringMu.Unlock()

	// Forward to subscribers
	t.mu.RLock()
	for _, sub := range t.clients {
		if r.Level >= sub.level {
			sub.client.Send(RPCResponse{Result: map[string]any{"type": "log", "data": entry}})
		}
	}
	t.mu.RUnlock()

	if t.inner.Enabled(ctx, r.Level) {
		return t.inner.Handle(ctx, r)
	}
	return nil
}

func (t *LogTee) buildEntry(r slog.Record) map[string]any {
	entry := map[string]any{
		"timestamp": r.Time.UnixMilli(),
		"level":     levelName(r.Level),
		"message":   r.Message,
	}
	attrs := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		key := a.Key
		val := a.Value.String()
		if key == "component" || key == "source" || key == "module" {
			entry["source"] = val
			return true
		}
		if isSensitiveKey(key) {
			attrs[key] = redactedValue
		} else {
			attrs[key] = val
		}
		return true
	})
	if len(attrs) > 0 {
		entry["attrs"] = attrs
	}
	return entry
}

func (t *LogTee) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogTee{inner: t.inner.WithAttrs(attrs), clients: t.clients, ring: t.ring}
}

func (t *LogTee) WithGroup(name string) slog.Handler {
	return &LogTee{inner: t.inner.WithGroup(name), clients: t.clients, ring: t.ring}
}

// Subscribe adds a client to the log tailing set at the given level.
func (t *LogTee) Subscribe(clientID string, client *WSClient, level slog.Level) {
	t.mu.Lock()
	t.clients[clientID] = &logSubscriber{client: client, level: level}
	t.mu.Unlock()

	// Replay ring buffer
	t.ringMu.RLock()
	var entries []map[string]any
	if t.ringFul {
		for i := range logRingSize {
			idx := (t.ringPos + i) % logRingSize
			if e := t.ring[idx]; e != nil && logLevelValue(e["level"]) >= level {
				entries = append(entries, e)
			}
		}
	} else {
		for i := 0; i < t.ringPos; i++ {
			if e := t.ring[i]; e != nil && logLevelValue(e["level"]) >= level {
				entries = append(entries, e)
			}
		}
	}
	t.ringMu.RUnlock()

	for _, e := range entries {
		client.Send(RPCResponse{Result: map[string]any{"type": "log", "data": e}})
	}
	client.Send(RPCResponse{Result: map[string]any{"type": "log", "data": map[string]any{
		"timestamp": time.Now().UnixMilli(),
		"level":     "info",
		"message":   "Log tailing started",
		"source":    "gateway",
	}}})
}

// Unsubscribe removes a client from the log tailing set.
func (t *LogTee) Unsubscribe(clientID string) {
	t.mu.Lock()
	delete(t.clients, clientID)
	t.mu.Unlock()
}

func levelName(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	case l >= slog.LevelInfo:
		return "info"
	default:
		return "debug"
	}
}

func logLevelValue(v any) slog.Level {
	s, _ := v.(string)
	switch s {
	case "error":
		return slog.LevelError
	case "warn":
		return slog.LevelWarn
	case "info":
		return slog.LevelInfo
	default:
		return slog.LevelDebug
	}
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
