// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package ssestream

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type payload struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
}

// fakeFlusher satisfies http.Flusher without needing a full ResponseWriter.
type fakeFlusher struct{ flushed int }

func (f *fakeFlusher) Flush() { f.flushed++ }

// nopCloser wraps bytes.Buffer for readback in tests.
type nopCloser struct{ io.Reader }

func (nopCloser) Close() error { return nil }

func TestDecoder_SingleEvent(t *testing.T) {
	body := "data: {\"type\":\"plan.proposed\",\"properties\":{\"id\":\"p1\"}}\n\n"
	d := newEventStreamDecoder(nopCloser{strings.NewReader(body)})
	if !d.Next() {
		t.Fatalf("expected one event, got none; err=%v", d.Err())
	}
	evt := d.Event()
	if evt.Type != "message" {
		t.Fatalf("expected default event type 'message', got %q", evt.Type)
	}
	want := `{"type":"plan.proposed","properties":{"id":"p1"}}`
	if string(evt.Data) != want {
		t.Fatalf("payload mismatch: got %q", evt.Data)
	}
	if d.Next() {
		t.Fatalf("expected EOF, got another event")
	}
}

func TestDecoder_MultilineData(t *testing.T) {
	body := "data: line1\ndata: line2\ndata: line3\n\n"
	d := newEventStreamDecoder(nopCloser{strings.NewReader(body)})
	if !d.Next() {
		t.Fatalf("expected event; err=%v", d.Err())
	}
	got := string(d.Event().Data)
	want := "line1\nline2\nline3"
	if got != want {
		t.Fatalf("multiline: got %q want %q", got, want)
	}
}

func TestDecoder_CommentsIgnored(t *testing.T) {
	body := ": keepalive\n: another comment\ndata: real\n\n"
	d := newEventStreamDecoder(nopCloser{strings.NewReader(body)})
	if !d.Next() {
		t.Fatalf("expected event")
	}
	if string(d.Event().Data) != "real" {
		t.Fatalf("expected 'real', got %q", d.Event().Data)
	}
}

func TestDecoder_EventTypeAndID(t *testing.T) {
	body := "event: message.updated\nid: 42\ndata: hello\n\n"
	d := newEventStreamDecoder(nopCloser{strings.NewReader(body)})
	if !d.Next() {
		t.Fatalf("expected event")
	}
	evt := d.Event()
	if evt.Type != "message.updated" {
		t.Fatalf("type: got %q", evt.Type)
	}
	if evt.ID != "42" {
		t.Fatalf("id: got %q", evt.ID)
	}
}

func TestDecoder_LineEndings(t *testing.T) {
	cases := map[string]string{
		"LF":   "data: a\n\n",
		"CRLF": "data: a\r\n\r\n",
		"CR":   "data: a\r\r",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			d := newEventStreamDecoder(nopCloser{strings.NewReader(body)})
			if !d.Next() {
				t.Fatalf("expected event")
			}
			if string(d.Event().Data) != "a" {
				t.Fatalf("got %q", d.Event().Data)
			}
		})
	}
}

func TestDecoder_NoTerminatorOnLastEvent(t *testing.T) {
	body := "data: trailing" // no blank line; still valid on EOF
	d := newEventStreamDecoder(nopCloser{strings.NewReader(body)})
	if !d.Next() {
		t.Fatalf("expected trailing event flushed at EOF; err=%v", d.Err())
	}
	if string(d.Event().Data) != "trailing" {
		t.Fatalf("got %q", d.Event().Data)
	}
}

func TestDecoder_EmptyBlanksIgnored(t *testing.T) {
	body := "\n\n\ndata: once\n\n"
	d := newEventStreamDecoder(nopCloser{strings.NewReader(body)})
	if !d.Next() {
		t.Fatalf("expected event")
	}
	if d.Next() {
		t.Fatalf("unexpected second event")
	}
}

func TestStream_DecodesJSONTyped(t *testing.T) {
	body := `data: {"type":"plan.proposed","properties":{"id":"p1"}}` + "\n\n" +
		`data: {"type":"plan.approved","properties":{"id":"p1"}}` + "\n\n" +
		"data: [DONE]\n\n"
	s := NewStreamReader[payload](nopCloser{strings.NewReader(body)})
	defer s.Close()

	events, err := s.ReadAll(context.Background())
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "plan.proposed" || events[1].Type != "plan.approved" {
		t.Fatalf("event types: %+v", events)
	}
}

func TestStream_DecodeError(t *testing.T) {
	body := "data: not json\n\n"
	s := NewStreamReader[payload](nopCloser{strings.NewReader(body)})
	defer s.Close()
	if s.Next() {
		t.Fatalf("expected decode failure")
	}
	if s.Err() == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestNewStream_UnknownContentType(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{"Content-Type": {"application/json"}},
		Body:   nopCloser{strings.NewReader("{}")},
	}
	if _, err := NewStream[payload](resp); err == nil {
		t.Fatalf("expected error for unregistered content-type")
	}
}

func TestNewStream_NilResponse(t *testing.T) {
	if _, err := NewStream[payload](nil); err == nil {
		t.Fatalf("expected error for nil response")
	}
}

func TestEmitter_SendEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	em, err := NewEmitter(rec)
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	if err := em.SendEnvelope("agent.started", map[string]any{"role": "frontend"}); err != nil {
		t.Fatalf("SendEnvelope: %v", err)
	}
	_ = em.SendDone()

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type: %s", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"type":"agent.started"`) {
		t.Fatalf("missing envelope: %s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("missing DONE: %s", body)
	}
}

func TestEmitter_HeartbeatAndCancel(t *testing.T) {
	var buf bytes.Buffer
	ff := &fakeFlusher{}
	em := NewEmitterWriter(&buf, ff)

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()
	// Use a very short interval to exercise the ticker.
	if err := em.Heartbeat(ctx, 10*time.Millisecond); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if !strings.Contains(buf.String(), ": heartbeat") {
		t.Fatalf("expected heartbeat comment, got %q", buf.String())
	}
	if ff.flushed == 0 {
		t.Fatalf("expected at least one flush")
	}
}

func TestEmitter_AfterClose(t *testing.T) {
	rec := httptest.NewRecorder()
	em, _ := NewEmitter(rec)
	_ = em.Close()
	if err := em.Send("x", "y"); err == nil {
		t.Fatalf("expected error after close")
	}
}

func TestEmitter_MultilineDataSplits(t *testing.T) {
	rec := httptest.NewRecorder()
	em, _ := NewEmitter(rec)
	_ = em.SendRaw("m", "", 0, []byte("a\nb\nc"))
	body := rec.Body.String()
	// Expect three data lines.
	if strings.Count(body, "data: ") != 3 {
		t.Fatalf("expected 3 data lines, got: %q", body)
	}
}

func TestRegisterDecoder_CustomContentType(t *testing.T) {
	// Register a no-op decoder for an application/x-ndjson-ish content type to
	// prove the registry accepts it.
	RegisterDecoder("application/x-test-sse", newEventStreamDecoder)
	resp := &http.Response{
		Header: http.Header{"Content-Type": {"application/x-test-sse; charset=utf-8"}},
		Body:   nopCloser{strings.NewReader(`data: "ok"` + "\n\n")},
	}
	s, err := NewStream[string](resp)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	defer s.Close()
	if !s.Next() {
		t.Fatalf("expected event; err=%v", s.Err())
	}
	if s.Current() != "ok" {
		t.Fatalf("payload: got %q", s.Current())
	}
}
