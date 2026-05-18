// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package zalo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// client.go — Zalo WebSocket client with thread-safe writes and cookie injection.

// WSClient wraps gorilla/websocket with thread-safe write.
type WSClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// CloseInfo holds WebSocket close frame details.
type CloseInfo struct {
	Code   int
	Reason string
}

// DialWS connects to a Zalo WebSocket endpoint.
// Manually injects cookies from the jar to bypass Go's cookiejar domain-matching
// limitations with wss:// URLs and host-only cookies.
func DialWS(ctx context.Context, wsURL string, headers http.Header, jar http.CookieJar) (*WSClient, error) {
	dialer := websocket.Dialer{EnableCompression: true}

	if jar != nil {
		injectCookies(headers, jar, wsURL)
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("zalo.ws: dial: %w", err)
	}
	conn.SetReadLimit(1 << 20) // 1MB
	return &WSClient{conn: conn}, nil
}

func injectCookies(headers http.Header, jar http.CookieJar, wsURL string) {
	baseURL := &url.URL{Scheme: "https", Host: "chat.zalo.me", Path: "/"}
	seen := make(map[string]string, 4)
	for _, c := range jar.Cookies(baseURL) {
		seen[c.Name] = c.Name + "=" + c.Value
	}
	if u, err := url.Parse(strings.Replace(wsURL, "wss://", "https://", 1)); err == nil {
		for _, c := range jar.Cookies(u) {
			seen[c.Name] = c.Name + "=" + c.Value
		}
	}
	if len(seen) == 0 { return }

	parts := make([]string, 0, len(seen))
	for _, v := range seen { parts = append(parts, v) }
	cookie := strings.Join(parts, "; ")
	if existing := headers.Get("Cookie"); existing != "" {
		cookie = existing + "; " + cookie
	}
	headers.Set("Cookie", cookie)
	slog.Debug("zalo.ws: cookies injected", "count", len(seen))
}

// ReadMessage reads the next WebSocket message. Blocks until message or close.
func (c *WSClient) ReadMessage(ctx context.Context) ([]byte, error) {
	_, data, err := c.conn.ReadMessage()
	return data, err
}

// WriteMessage sends a binary WebSocket message. Thread-safe.
func (c *WSClient) WriteMessage(ctx context.Context, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

// Close sends a close frame and shuts down the connection.
func (c *WSClient) Close(code int, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason))
	c.conn.Close()
}

func parseWSCloseInfo(err error) CloseInfo {
	var ce *websocket.CloseError
	if errors.As(err, &ce) { return CloseInfo{Code: ce.Code, Reason: ce.Text} }
	return CloseInfo{Code: 1006, Reason: err.Error()}
}
