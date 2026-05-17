// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package browser

import (
	"context"
	"encoding/base64"
	"log/slog"
	"time"
)

// Agent-to-user live stream: the web UI can watch what the backend
// browser is doing in real time, like a Loom recording of the
// automation run. This is strictly observational — the user can't
// intervene in the agent's actions from the viewer (for that, use
// the browser tools from chat).
//
// Mechanism:
//   1. Manager.StartLiveStream(fps) begins a ticker goroutine that
//      takes a screenshot at `fps` frames/sec (cap 8 — anything
//      higher saturates CDP and the agent loop feels sluggish).
//   2. Each frame is JPEG-encoded (smaller than PNG, good enough
//      for a thumbnail preview) and pushed via the provided publish
//      function. The gateway wires this to realtime.Hub.Broadcast.
//   3. Manager.StopLiveStream cancels the goroutine.
//
// Rate-limiting is the watcher's job — if no one's watching, we still
// capture frames. Wasted work? Sure, but only a few KB/s. The
// alternative (coupling capture to subscribers) creates a stateful
// protocol we don't need yet. If bandwidth becomes an issue, add a
// "pause when no subscribers" path.

// FramePublisher is what StartLiveStream calls with each new frame.
// Signature kept tiny so the gateway can adapt it to whatever
// broadcast channel it has (realtime.Hub, SSE, NATS, etc.).
type FramePublisher func(frame Frame)

// Frame is one captured screenshot — the payload is always JPEG
// (with a data: URL header) so viewers can set it directly on an
// <img> src without extra decoding.
type Frame struct {
	// DataURL is "data:image/jpeg;base64,<bytes>".
	DataURL string
	// Width + Height report viewport size so the viewer can size
	// its container correctly without reading the JPEG header.
	Width, Height int
	// TakenAt is the server clock at capture time. Viewers use it
	// to detect missed frames and throttle their render loop.
	TakenAt time.Time
	// URL is the tab's current URL when the frame was captured.
	// Lets the viewer show "now on: example.com" above the image.
	URL string
}

// streamState is the manager's per-instance live stream controller.
// Guarded by Manager.liveMu.
type streamState struct {
	cancel context.CancelFunc
	fps    int
}

// StartLiveStream begins emitting frames at the requested rate. If a
// stream is already running, the prior one is cancelled — there's no
// concept of multiple concurrent streams off the same manager (every
// watcher consumes the same broadcast fan-out downstream).
//
// fps is clamped to [1, 8]. Values outside are silently adjusted.
func (m *Manager) StartLiveStream(fps int, publish FramePublisher) {
	if fps < 1 {
		fps = 1
	}
	if fps > 8 {
		fps = 8
	}
	m.liveMu.Lock()
	defer m.liveMu.Unlock()

	// Cancel any prior stream — idempotent start.
	if m.liveState != nil && m.liveState.cancel != nil {
		m.liveState.cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.liveState = &streamState{cancel: cancel, fps: fps}

	interval := time.Second / time.Duration(fps)
	go m.runLiveStream(ctx, interval, publish)
	slog.Info("browser.livestream.started", "fps", fps)
}

// StopLiveStream ends the current stream. No-op if none is running.
func (m *Manager) StopLiveStream() {
	m.liveMu.Lock()
	defer m.liveMu.Unlock()
	if m.liveState != nil && m.liveState.cancel != nil {
		m.liveState.cancel()
		m.liveState = nil
		slog.Info("browser.livestream.stopped")
	}
}

// IsStreaming reports whether a live stream is currently running.
func (m *Manager) IsStreaming() bool {
	m.liveMu.Lock()
	defer m.liveMu.Unlock()
	return m.liveState != nil
}

func (m *Manager) runLiveStream(ctx context.Context, interval time.Duration, publish FramePublisher) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Sentinel for frame capture failures — a transient Chrome hiccup
	// shouldn't kill the stream. After 10 consecutive failures we
	// give up (browser is probably dead).
	consecutiveErr := 0
	const maxConsecutive = 10

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Don't bother capturing if the browser isn't running.
			// Saves a CDP round-trip per tick on an idle server.
			if !m.IsRunning() {
				continue
			}
			capCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			png, err := m.JPEGScreenshot(capCtx, 60)
			info, infoErr := m.PageInfo(capCtx)
			cancel()

			if err != nil {
				consecutiveErr++
				slog.Debug("browser.livestream.capture_failed",
					"error", err, "consecutive", consecutiveErr)
				if consecutiveErr >= maxConsecutive {
					slog.Warn("browser.livestream.giving_up", "consecutive_errs", consecutiveErr)
					return
				}
				continue
			}
			consecutiveErr = 0

			var w, h int
			var url string
			if infoErr == nil {
				if vp, ok := info["viewport"].(map[string]any); ok {
					if f, ok := vp["width"].(float64); ok {
						w = int(f)
					}
					if f, ok := vp["height"].(float64); ok {
						h = int(f)
					}
				}
				if s, ok := info["url"].(string); ok {
					url = s
				}
			}

			publish(Frame{
				DataURL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(png),
				Width:   w,
				Height:  h,
				TakenAt: time.Now(),
				URL:     url,
			})
		}
	}
}

