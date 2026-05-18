// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package ssestream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Emitter writes server-sent events to an http.ResponseWriter (or any
// io.Writer with a Flush hook). It is safe for concurrent use — writes
// are serialized by an internal mutex, which matches SSE semantics
// (the browser/SDK consumes one event at a time).
//
// The emitter is one-shot: create it per HTTP handler, defer Close.
//
// It writes events using the standard SSE framing:
//
//	event: <type>      (optional, omitted when equal to "message")
//	id: <id>           (optional)
//	retry: <ms>        (optional)
//	data: <line>       (one line per embedded newline in payload)
//	\n                 (dispatch)
type Emitter struct {
	mu      sync.Mutex
	w       io.Writer
	flusher http.Flusher
	closed  bool
}

// NewEmitter wraps an http.ResponseWriter. It sets the canonical SSE
// response headers and writes them immediately (so the connection is
// "ready" before the first event). Returns an error if the underlying
// writer does not support Flush — streaming is impossible without it.
func NewEmitter(w http.ResponseWriter) (*Emitter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("ssestream: response writer does not support flushing")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	// Disable proxy buffering (NGINX) so events flow in real time.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	return &Emitter{w: w, flusher: flusher}, nil
}

// NewEmitterWriter wraps an arbitrary io.Writer + Flusher (for testing or
// non-HTTP consumers). Flusher may be nil — in which case Flush is a no-op.
func NewEmitterWriter(w io.Writer, f http.Flusher) *Emitter {
	return &Emitter{w: w, flusher: f}
}

// Send emits a single event with optional type and auto-marshaled JSON data.
// An empty evtType omits the "event:" line (SSE default is "message").
// Returns the first write error; subsequent calls on a failed emitter
// return that same error.
func (e *Emitter) Send(evtType string, data any) error {
	if data == nil {
		return e.SendRaw(evtType, "", 0, nil)
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("ssestream: marshal event payload: %w", err)
	}
	return e.SendRaw(evtType, "", 0, payload)
}

// SendEnvelope emits an event using the standardized {type, properties}
// envelope shape. This is the primary production path — API consumers
// read the type discriminator off the envelope, not the SSE "event:" field.
func (e *Emitter) SendEnvelope(evtType string, properties any) error {
	return e.Send("", map[string]any{
		"type":       evtType,
		"properties": properties,
	})
}

// SendRaw writes a single event with explicit control over every field.
// `data` is split on \n so multi-line payloads are correctly framed per
// the SSE spec.
func (e *Emitter) SendRaw(evtType, id string, retryMS int, data []byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return errors.New("ssestream: emitter closed")
	}

	var buf bytes.Buffer
	if evtType != "" && evtType != "message" {
		buf.WriteString("event: ")
		buf.WriteString(evtType)
		buf.WriteByte('\n')
	}
	if id != "" {
		buf.WriteString("id: ")
		buf.WriteString(id)
		buf.WriteByte('\n')
	}
	if retryMS > 0 {
		buf.WriteString("retry: ")
		buf.WriteString(strconv.Itoa(retryMS))
		buf.WriteByte('\n')
	}

	// Split payload on embedded newlines so each line is its own "data:" field.
	if len(data) == 0 {
		buf.WriteString("data: \n")
	} else {
		for _, line := range splitNewlines(data) {
			buf.WriteString("data: ")
			buf.Write(line)
			buf.WriteByte('\n')
		}
	}
	buf.WriteByte('\n') // dispatch

	if _, err := e.w.Write(buf.Bytes()); err != nil {
		e.closed = true
		return fmt.Errorf("ssestream: write event: %w", err)
	}
	if e.flusher != nil {
		e.flusher.Flush()
	}
	return nil
}

// SendComment writes a comment line. Useful as a keep-alive heartbeat or
// for inline debugging.
func (e *Emitter) SendComment(text string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return errors.New("ssestream: emitter closed")
	}
	// SSE comment: lines starting with ':'. Strip newlines from text.
	clean := strings.ReplaceAll(strings.ReplaceAll(text, "\r", " "), "\n", " ")
	line := ": " + clean + "\n\n"
	if _, err := e.w.Write([]byte(line)); err != nil {
		e.closed = true
		return fmt.Errorf("ssestream: write comment: %w", err)
	}
	if e.flusher != nil {
		e.flusher.Flush()
	}
	return nil
}

// SendDone emits the standard stream-termination sentinel ("[DONE]"). After
// this call, the underlying handler should return so the response body
// closes cleanly.
func (e *Emitter) SendDone() error {
	return e.SendRaw("", "", 0, []byte("[DONE]"))
}

// Heartbeat sends SSE comment lines at the given interval until ctx is
// cancelled. Runs in the caller's goroutine; use `go e.Heartbeat(...)`.
// Errors during heartbeat are returned but not fatal — the caller may
// continue emitting events. A zero or negative interval is treated as 15s.
func (e *Emitter) Heartbeat(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := e.SendComment("heartbeat"); err != nil {
				return err
			}
		}
	}
}

// Close marks the emitter as closed. Further sends return an error. Does
// NOT close the underlying http.ResponseWriter — that lifecycle belongs
// to the HTTP server.
func (e *Emitter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
	return nil
}

// splitNewlines splits on '\n' (keeping '\r' for Windows line endings
// absorbed into the previous line — we don't strip them because SSE
// allows arbitrary bytes in data fields per RFC). Empty payload returns
// one empty line.
func splitNewlines(b []byte) [][]byte {
	if len(b) == 0 {
		return [][]byte{nil}
	}
	lines := bytes.Split(b, []byte{'\n'})
	out := make([][]byte, len(lines))
	for i, line := range lines {
		// Strip a trailing \r so "a\r\nb" becomes ["a", "b"].
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		out[i] = line
	}
	return out
}
