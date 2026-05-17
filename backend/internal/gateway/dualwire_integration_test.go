// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/ssestream"
)

// TestDualWire_EndToEnd proves the handleDaemonStream dual-wire path
// (makeSender): each emit produces a legacy {type,data} frame AND a
// canonical {type,properties} envelope. Consumers that only read
// canonical frames see one logical event; consumers reading both
// streams see two.
//
// Phase 3 (FU-019): legacyAliases was emptied — the /code page no
// longer needs legacy-frame de-dup. The daemon stream still uses
// makeSender so we keep the test; expectConsume is now 2 for the
// non-oversized cases (both frames are well-formed and distinct).
//
// Table-driven across three scenarios:
//  1. "normal" — routine event with all fields populated.
//  2. "oversized" — payload above ssestream's max frame size; the frame
//     MUST be rejected by the decoder, not duplicated into the consumer.
//  3. "no_id" — envelope without an ID; both frames are distinct.
func TestDualWire_EndToEnd(t *testing.T) {
	cases := []struct {
		name          string
		legacyType    string // the flat name the legacy frame uses
		envType       string // the canonical namespaced name the envelope uses
		payload       any
		emitEnvelope  bool // whether to include an explicit id on the envelope
		expectConsume int  // 0 if we expect the stream to error/drop
		expectError   bool
	}{
		{
			name:          "normal",
			legacyType:    "agent_started",
			envType:       "agent.started",
			payload:       map[string]any{"project_id": "p1", "agent_key": "a1", "role": "frontend"},
			emitEnvelope:  true,
			expectConsume: 2, // legacy + canonical; legacyAliases empty so no de-dup
		},
		{
			name:          "no_id",
			legacyType:    "file_created",
			envType:       "file.edited",
			payload:       map[string]any{"project_id": "p1", "path": "src/app.tsx"},
			emitEnvelope:  false,
			expectConsume: 2, // legacy + canonical; distinct fingerprints post FU-019
		},
		{
			name:          "oversized",
			legacyType:    "tool_start",
			envType:       "agent.progress",
			payload:       map[string]any{"project_id": "p1", "agent_key": "a1", "blob": strings.Repeat("x", 2<<20)}, // 2 MiB > 1 MiB cap
			emitEnvelope:  true,
			expectConsume: 0,
			expectError:   true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Spin up an httptest server that runs the dual-wire emitter in
			// the shape project_handlers.makeSender uses: one legacy frame
			// plus one canonical envelope per event.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				flusher, ok := w.(http.Flusher)
				if !ok {
					t.Fatalf("no flusher")
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Connection", "keep-alive")
				w.WriteHeader(http.StatusOK)

				// Legacy frame.
				legacy, _ := json.Marshal(map[string]any{
					"type": tc.legacyType,
					"data": tc.payload,
				})
				fmt.Fprintf(w, "data: %s\n\n", legacy)

				// Canonical envelope.
				env := apievents.Envelope{
					Type:        apievents.Type(tc.envType),
					EmittedAtMS: time.Now().UnixMilli(),
				}
				if tc.emitEnvelope {
					env.ID = "evt-1"
				}
				if err := env.Encode(tc.payload); err != nil {
					t.Fatalf("envelope encode: %v", err)
				}
				b, _ := json.Marshal(env)
				fmt.Fprintf(w, "data: %s\n\n", b)

				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
			}))
			defer srv.Close()

			resp, err := http.Get(srv.URL)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			s, err := ssestream.NewStream[dualWireFrame](resp)
			if err != nil {
				t.Fatalf("NewStream: %v", err)
			}
			defer s.Close()

			// Consumer-side de-dup matches the production TUI and web
			// path. Two rules fire on every frame:
			//   1. Canonicalize the type name (legacy → namespaced)
			//      BEFORE fingerprinting. Without step 1, "agent_started"
			//      and "agent.started" fingerprint differently and both
			//      pass through.
			//   2. Record BOTH the envelope ID (when present) and the
			//      fingerprint, so the legacy twin (no id) matches the
			//      canonical one (id+fingerprint) via the shared
			//      fingerprint key.
			consumed := make([]dualWireFrame, 0, 2)
			seenIDs := make(map[string]struct{})
			seenFP := make(map[string]struct{})
			for s.Next() {
				fr := s.Current()
				canonical := string(apievents.CanonicalType(fr.Type))
				p := fr.payloadBytes()
				if len(p) > 128 {
					p = p[:128]
				}
				fp := canonical + "|" + string(p)

				if fr.ID != "" {
					if _, ok := seenIDs[fr.ID]; ok {
						continue
					}
					if _, ok := seenFP[fp]; ok {
						// Fingerprint already seen (legacy arrived first).
						seenIDs[fr.ID] = struct{}{}
						continue
					}
					seenIDs[fr.ID] = struct{}{}
					seenFP[fp] = struct{}{}
				} else {
					if _, ok := seenFP[fp]; ok {
						continue
					}
					seenFP[fp] = struct{}{}
				}
				consumed = append(consumed, fr)
			}

			if tc.expectError {
				if s.Err() == nil {
					t.Fatalf("expected decode error, got none (consumed=%d)", len(consumed))
				}
				if len(consumed) != 0 {
					t.Fatalf("oversized event must not reach consumer (got %d)", len(consumed))
				}
				return
			}
			if s.Err() != nil {
				t.Fatalf("unexpected err: %v", s.Err())
			}
			if len(consumed) != tc.expectConsume {
				t.Fatalf("expected %d consumed events, got %d: %+v",
					tc.expectConsume, len(consumed), consumed)
			}
		})
	}
}

// dualWireFrame understands both shapes: legacy {type, data} and canonical
// {type, properties, id}. Test-only — consumers in production have their
// own types.
type dualWireFrame struct {
	Type       string          `json:"type"`
	ID         string          `json:"id,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	Properties json.RawMessage `json:"properties,omitempty"`
}

func (f dualWireFrame) payloadBytes() []byte {
	if len(f.Properties) > 0 {
		return f.Properties
	}
	return f.Data
}

// TestDualWire_ConcurrentSenders verifies that when multiple goroutines
// emit through the mutex-guarded sender we produce correctly-framed
// output (no interleaved frames, no dropped events). This is the
// property that makes orchestrateBuild's concurrent broadcast() calls
// safe.
func TestDualWire_ConcurrentSenders(t *testing.T) {
	rec := httptest.NewRecorder()
	em, err := ssestream.NewEmitter(rec)
	if err != nil {
		t.Fatalf("emitter: %v", err)
	}

	const goroutines = 20
	const eventsPerGoroutine = 10
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				_ = em.SendEnvelope("test.event", map[string]any{
					"worker": i, "seq": j,
				})
			}
		}(i)
	}
	wg.Wait()
	_ = em.SendDone()

	// Pipe the recorded body through the ssestream decoder and count
	// well-formed envelopes.
	resp := &http.Response{
		Header: http.Header{"Content-Type": {"text/event-stream"}},
		Body:   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
	}
	s, err := ssestream.NewStream[dualWireFrame](resp)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	defer s.Close()

	count := 0
	for s.Next() {
		f := s.Current()
		if f.Type != "test.event" {
			t.Fatalf("unexpected type: %q", f.Type)
		}
		count++
	}
	if err := s.Err(); err != nil {
		t.Fatalf("stream err: %v", err)
	}
	if count != goroutines*eventsPerGoroutine {
		t.Fatalf("expected %d events, got %d", goroutines*eventsPerGoroutine, count)
	}
}

// TestDualWire_ContextCancellationStopsConsumer confirms that a cancelled
// context delivered to ReadAll aborts the loop without blocking forever
// on a slow-emitting server. This exercises the seam we rely on when
// the /code page's AbortController fires.
func TestDualWire_ContextCancellationStopsConsumer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(5 * time.Millisecond):
				env, _ := apievents.NewEnvelope(apievents.Type("ping.tick"), map[string]int{"x": 1})
				b, _ := json.Marshal(env)
				fmt.Fprintf(w, "data: %s\n\n", b)
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	s, err := ssestream.NewStream[dualWireFrame](resp)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	defer s.Close()

	// Draining should terminate when the client-side context expires
	// (resp.Body read returns an error).
	_, err = s.ReadAll(ctx)
	if err == nil && s.Err() == nil {
		t.Fatalf("expected context cancellation to surface an error")
	}
}
