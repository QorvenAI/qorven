// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/qorvenai/qorven/internal/qor/browser"
	"github.com/qorvenai/qorven/internal/realtime"
)

// Agent-to-user live browser stream control.
//
// HTTP surface:
//   POST /v1/browser/live/start  — begin emitting frames at requested fps
//   POST /v1/browser/live/stop   — halt the stream
//   GET  /v1/browser/live/status — check whether it's running + last fps
//
// Frames ride on the existing /ws/realtime socket as events of
// type "browser_frame" so no new WebSocket is needed.

// liveStreamState is the publisher-side state (separate from the
// manager-side streamState so we don't smear concerns across packages).
type liveStreamState struct {
	mu  sync.Mutex
	mgr *browser.Manager // the manager whose stream we control
	fps int              // current frames/sec; 0 = not running
}

var gwLiveStream liveStreamState // package-level; matches Gateway singleton

// wireBrowserLivePublisher installs a FramePublisher that pushes each
// captured frame onto the realtime hub. Does NOT start the stream —
// that's done explicitly via POST /v1/browser/live/start so idle
// servers don't burn CPU on a viewer nobody's watching.
func (gw *Gateway) wireBrowserLivePublisher(mgr *browser.Manager) {
	// Routes are mounted later via registerBrowserLiveRoutes() — keep
	// this fn side-effect-free on the router so it can be called
	// during tool registration, which runs before the middleware
	// stack is finalized.
	gwLiveStream.mgr = mgr
}

// registerBrowserLiveRoutes wires the three HTTP endpoints that control
// the live browser-frame stream. Called from registerRoutes so the
// global middleware stack is already in place.
func (gw *Gateway) registerBrowserLiveRoutes() {
	gw.router.Post("/v1/browser/live/start", gw.handleBrowserLiveStart)
	gw.router.Post("/v1/browser/live/stop", gw.handleBrowserLiveStop)
	gw.router.Get("/v1/browser/live/status", gw.handleBrowserLiveStatus)
}

// handleBrowserLiveStart POST body: {"fps": 2} (optional). Default 2 fps.
// Clamped to [1, 8] by the Manager.
func (gw *Gateway) handleBrowserLiveStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FPS int `json:"fps"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.FPS <= 0 {
		body.FPS = 2
	}

	gwLiveStream.mu.Lock()
	defer gwLiveStream.mu.Unlock()
	if gwLiveStream.mgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "browser manager not available"})
		return
	}

	// Publish each frame onto the realtime hub. Every connected
	// web client will see these as `browser_frame` events and can
	// render them into an <img>/<canvas> at whatever rate it wants.
	// The hub's send buffer handles backpressure per-client.
	publisher := func(f browser.Frame) {
		gw.rtHub.Broadcast(realtime.Event{
			Type: "browser_frame",
			Data: map[string]any{
				"data_url": f.DataURL,
				"width":    f.Width,
				"height":   f.Height,
				"url":      f.URL,
				"taken_at": f.TakenAt.UnixMilli(),
			},
		})
	}
	gwLiveStream.mgr.StartLiveStream(body.FPS, publisher)
	gwLiveStream.fps = body.FPS
	writeJSON(w, http.StatusOK, map[string]any{"status": "started", "fps": body.FPS})
}

// handleBrowserLiveStop halts the stream.
func (gw *Gateway) handleBrowserLiveStop(w http.ResponseWriter, r *http.Request) {
	gwLiveStream.mu.Lock()
	defer gwLiveStream.mu.Unlock()
	if gwLiveStream.mgr != nil {
		gwLiveStream.mgr.StopLiveStream()
	}
	gwLiveStream.fps = 0
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// handleBrowserLiveStatus reports whether streaming is live + the
// configured fps. Frontend polls this to show an indicator.
func (gw *Gateway) handleBrowserLiveStatus(w http.ResponseWriter, r *http.Request) {
	gwLiveStream.mu.Lock()
	defer gwLiveStream.mu.Unlock()
	running := false
	if gwLiveStream.mgr != nil {
		running = gwLiveStream.mgr.IsStreaming()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"running": running,
		"fps":     gwLiveStream.fps,
	})
}
