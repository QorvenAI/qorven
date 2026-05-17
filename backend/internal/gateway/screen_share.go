// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

// User-to-agent screen share.
//
// The user's browser captures their desktop via the standard
// `navigator.mediaDevices.getDisplayMedia()` API, draws each video
// frame to a <canvas>, encodes as JPEG, and POSTs (or streams over
// WebSocket) to /ws/screen/upload. The server keeps just the LATEST
// frame in memory — we don't buffer history, because the agent only
// ever needs "what the user is looking at right now".
//
// Tool flow:
//   1. User clicks "Share screen" in the web UI.
//   2. Frontend sets up getDisplayMedia stream + sends frames at
//      ~1 fps over the upload WS.
//   3. Agent calls user_screen_capture → returns the latest frame
//      as base64 JPEG, or an error if the user isn't sharing.
//
// The latest frame is scoped per tenant (not per session) so any
// agent in the same tenant can see the shared screen while it's live.
// When the user disconnects, the stored frame is cleared after a TTL
// so we don't keep stale visuals around.

// maxFrameBytes caps per-frame size (JPEG compressed). 2 MiB is
// comfortably above a 1080p JPEG at moderate quality; bigger
// payloads are rejected to prevent a misbehaving client from
// filling the server's heap.
const maxFrameBytes = 2 * 1024 * 1024

// frameTTL is how long a stored frame stays "current" after the
// user's upload WebSocket disconnects. After this elapses,
// user_screen_capture returns "stale" so the agent doesn't reason
// over an old image.
const frameTTL = 30 * time.Second

// latestFrame is one tenant's most recent shared-screen frame.
type latestFrame struct {
	JPEG      []byte
	Width     int
	Height    int
	ReceivedAt time.Time
}

// ScreenShareStore holds per-tenant latest frames. Package-level
// singleton wired into the Gateway so both the WS handler and the
// agent tool share the same state.
type ScreenShareStore struct {
	mu     sync.RWMutex
	frames map[string]*latestFrame // tenantID → latest frame
}

// NewScreenShareStore returns an empty store.
func NewScreenShareStore() *ScreenShareStore {
	return &ScreenShareStore{frames: make(map[string]*latestFrame)}
}

// Store replaces any prior frame for tenantID. Called by the upload
// WS on every received frame.
func (s *ScreenShareStore) Store(tenantID string, jpeg []byte, width, height int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames[tenantID] = &latestFrame{
		JPEG:       jpeg,
		Width:      width,
		Height:     height,
		ReceivedAt: time.Now(),
	}
}

// Latest returns the most recent frame for tenantID, or nil when
// none has been stored or the frame is stale (> frameTTL old).
func (s *ScreenShareStore) Latest(tenantID string) *latestFrame {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.frames[tenantID]
	if !ok {
		return nil
	}
	if time.Since(f.ReceivedAt) > frameTTL {
		return nil
	}
	return f
}

// Clear drops the frame for tenantID. Called when the upload WS
// disconnects so we don't keep bytes around for a tenant that's no
// longer sharing.
func (s *ScreenShareStore) Clear(tenantID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.frames, tenantID)
}

// --- WebSocket upload handler ---

// screenUploadFrame is the per-frame JSON payload the browser sends.
// Shape matches what the frontend ScreenShareWidget produces —
// simple base64 JPEG plus dimensions.
type screenUploadFrame struct {
	Type   string `json:"type"`       // "frame"
	JPEG64 string `json:"jpeg_b64"`   // base64-encoded JPEG bytes
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// handleScreenShareUpload handles /ws/screen/upload. One WebSocket
// per sharing client; frames stream in newline-less JSON messages.
//
// Auth: goes through wsAuth (Bearer token or ?token= query param)
// before reaching this handler, same as every other /ws/* endpoint.
func (gw *Gateway) handleScreenShareUpload(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
		// 2 MiB matches maxFrameBytes so the library doesn't reject
		// our own payloads before we can validate them.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		slog.Warn("screen_upload.accept_failed", "error", err)
		return
	}
	// nhooyr/websocket default read message limit is 32 KiB; raise
	// it so full-screen JPEGs get through.
	conn.SetReadLimit(maxFrameBytes + 64*1024)

	tenantID := defaultTenant // single-tenant build; multi-tenant wiring can override
	slog.Info("screen_upload.connected", "tenant", tenantID)
	defer func() {
		gw.screenShare.Clear(tenantID)
		_ = conn.Close(websocket.StatusNormalClosure, "")
		slog.Info("screen_upload.disconnected", "tenant", tenantID)
	}()

	for {
		_, r, err := conn.Reader(r.Context())
		if err != nil {
			return
		}
		// Cap read via io.LimitReader as a second line of defense.
		// SetReadLimit above already rejects oversized frames, but
		// LimitReader catches anything that sneaks through.
		body, err := io.ReadAll(io.LimitReader(r, maxFrameBytes+1024))
		if err != nil {
			slog.Debug("screen_upload.read_error", "error", err)
			return
		}
		if len(body) > maxFrameBytes {
			slog.Warn("screen_upload.oversize", "bytes", len(body))
			continue
		}
		var f screenUploadFrame
		if err := json.Unmarshal(body, &f); err != nil {
			continue // ignore malformed frames; keep stream alive
		}
		if f.Type != "frame" || f.JPEG64 == "" {
			continue
		}
		jpeg, err := base64.StdEncoding.DecodeString(f.JPEG64)
		if err != nil {
			continue
		}
		if len(jpeg) > maxFrameBytes {
			continue
		}
		gw.screenShare.Store(tenantID, jpeg, f.Width, f.Height)
	}
}
