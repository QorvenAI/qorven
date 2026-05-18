// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"nhooyr.io/websocket"

	"github.com/qorvenai/qorven/internal/voice"
)

// ─── Realtime voice proxy ──────────────────────────────────────────────
//
// The browser-side Prime widget can't talk to OpenAI Realtime directly
// because that would leak the user's API key into the page. This handler
// is a minimal pass-through WebSocket proxy:
//
//   browser ──ws──→  /ws/voice/realtime  ──ws──→  api.openai.com
//                         │
//                    (auth'd via gateway — we look up the user's
//                     configured realtime provider row and use its
//                     stored API key)
//
// Protocol-agnostic: we forward raw frames in both directions. The
// browser speaks the OpenAI Realtime event schema directly, so there's
// no normalisation to do in Go — the schema is wherever the upstream
// provider defines it. Future realtime drivers (Gemini Live, Moshi)
// will each get their own handler with whatever normalisation their
// schema needs.

// handleRealtimeVoiceProxy accepts a browser WebSocket, finds the
// user's default realtime provider, dials the upstream WebSocket
// using the provider's credentials, and pumps frames both ways.
func (gw *Gateway) handleRealtimeVoiceProxy(w http.ResponseWriter, r *http.Request) {
	if gw.voiceStore == nil {
		http.Error(w, "voice store not configured", http.StatusServiceUnavailable)
		return
	}

	// Find the default realtime provider for this tenant.
	rows, err := gw.voiceStore.List(r.Context(), defaultTenant)
	if err != nil {
		http.Error(w, "voice provider lookup failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var rt *voice.ProviderRow
	for i := range rows {
		if rows[i].Kind == "realtime" && rows[i].Enabled && rows[i].IsDefault {
			rt = &rows[i]
			break
		}
	}
	if rt == nil {
		// Second pass: any enabled realtime provider, default or not.
		for i := range rows {
			if rows[i].Kind == "realtime" && rows[i].Enabled {
				rt = &rows[i]
				break
			}
		}
	}
	if rt == nil {
		http.Error(w, "no realtime provider configured — add one in Settings → Voice",
			http.StatusPreconditionFailed)
		return
	}

	// Upgrade the browser connection.
	browserConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"}, // gateway auth already happened
	})
	if err != nil {
		slog.Warn("realtime.browser_upgrade_failed", "error", err)
		return
	}
	defer browserConn.CloseNow()

	// Dial the upstream provider. Only OpenAI Realtime is wired today
	// — Gemini Live gets its own case when it lands.
	var upstream *websocket.Conn
	var model string
	settings := parseSettings(rt.Settings)
	switch rt.Driver {
	case "openai_realtime":
		model = stringOr(settings, "model", "gpt-4o-realtime-preview")
		url := "wss://api.openai.com/v1/realtime?model=" + model
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		c, _, derr := websocket.Dial(ctx, url, &websocket.DialOptions{
			HTTPHeader: map[string][]string{
				"Authorization": {"Bearer " + rt.APIKey},
				"OpenAI-Beta":   {"realtime=v1"},
			},
		})
		cancel()
		if derr != nil {
			_ = browserConn.Close(websocket.StatusInternalError,
				"upstream dial failed: "+truncateForWS(derr.Error()))
			slog.Warn("realtime.upstream_dial_failed", "driver", rt.Driver, "error", derr)
			return
		}
		upstream = c
	default:
		_ = browserConn.Close(websocket.StatusPolicyViolation,
			"driver not supported for realtime proxy: "+rt.Driver)
		return
	}
	defer upstream.CloseNow()

	// Pipe both directions until either side closes. We use two
	// goroutines because nhooyr.io/websocket.Read blocks and there's
	// no epoll-style multiplexer. Any read error on either leg closes
	// the whole session.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go pipeFrames(ctx, upstream, browserConn, "upstream→browser")
	pipeFrames(ctx, browserConn, upstream, "browser→upstream")
}

// pipeFrames forwards every message from src to dst until one of them
// errors or ctx cancels. Context errors cause a graceful close on dst.
func pipeFrames(ctx context.Context, src, dst *websocket.Conn, dir string) {
	for {
		typ, data, err := src.Read(ctx)
		if err != nil {
			_ = dst.Close(websocket.StatusNormalClosure, dir+" closed")
			return
		}
		if err := dst.Write(ctx, typ, data); err != nil {
			_ = src.Close(websocket.StatusNormalClosure, dir+" write failed")
			return
		}
	}
}

// handleRealtimeEphemeralToken mints a short-lived client token for
// direct browser-to-OpenAI WebRTC sessions — avoids running our
// gateway on the audio hot path.
//
// OpenAI's spec: POST /v1/realtime/sessions with the model + voice,
// returns {client_secret:{value,expires_at}} usable by the browser
// for ~1 minute. We proxy that exchange so the user's real API key
// never reaches the page.
func (gw *Gateway) handleRealtimeEphemeralToken(w http.ResponseWriter, r *http.Request) {
	if gw.voiceStore == nil {
		writeJSON(w, 503, map[string]string{"error": "voice store not configured"})
		return
	}
	rows, err := gw.voiceStore.List(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	var rt *voice.ProviderRow
	for i := range rows {
		if rows[i].Kind == "realtime" && rows[i].Enabled &&
			rows[i].Driver == "openai_realtime" && rows[i].IsDefault {
			rt = &rows[i]; break
		}
	}
	if rt == nil {
		writeJSON(w, 412, map[string]string{"error": "no OpenAI Realtime provider configured"})
		return
	}

	settings := parseSettings(rt.Settings)
	payload := map[string]any{
		"model": stringOr(settings, "model", "gpt-4o-realtime-preview"),
		"voice": stringOr(settings, "voice", "alloy"),
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(r.Context(), "POST",
		"https://api.openai.com/v1/realtime/sessions", stringsReader(body))
	req.Header.Set("Authorization", "Bearer "+rt.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		writeJSON(w, resp.StatusCode, map[string]any{
			"error": "openai realtime: " + truncateForWS(string(raw)),
		})
		return
	}
	// Pass the upstream response through verbatim so the browser
	// gets {client_secret:{value,expires_at}, ...}.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(raw)
}

// ─── tiny helpers ──────────────────────────────────────────────────────

func parseSettings(raw []byte) map[string]any {
	if len(raw) == 0 { return map[string]any{} }
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil { return map[string]any{} }
	return m
}

func stringOr(m map[string]any, key, fallback string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" { return s }
	}
	return fallback
}

// truncateForWS keeps error messages short enough for a WS close frame
// (126-byte limit for the close reason).
func truncateForWS(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 100 { s = s[:100] + "…" }
	return s
}

// stringsReader returns an io.Reader over the supplied bytes without
// pulling in the bytes package just for one call site.
func stringsReader(b []byte) io.Reader {
	return &byteReader{b: b}
}

type byteReader struct {
	b   []byte
	pos int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.b) { return 0, io.EOF }
	n := copy(p, r.b[r.pos:])
	r.pos += n
	return n, nil
}

// keep base64 import alive for a future auth helper
var _ = base64.StdEncoding
