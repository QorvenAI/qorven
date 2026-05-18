// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package ssestream implements a generic Server-Sent Events client stream
// and event-source decoder.
//
// This package mirrors the structure of anomalyco/opencode-sdk-go's
// packages/ssestream — same generic Stream[T], same Decoder interface,
// same content-type-keyed decoder registry, same SSE framing semantics.
// It is written fresh here (no vendoring) but preserves the shape so that
// porting opencode services later is a line-for-line exercise.
//
// Framing (text/event-stream, per WHATWG HTML Living Standard):
//
//   - Lines terminated by \n, \r, or \r\n.
//   - Lines starting with ':' are comments and ignored.
//   - A "field: value" line contributes to the current event:
//     event  → sets event type (default "message")
//     data   → appended to data buffer; each 'data:' line adds a \n
//     id     → sets last event id
//     retry  → sets reconnection time (ms)
//   - A blank line dispatches the accumulated event.
//   - Whitespace after the colon is optional; SSE spec says exactly one
//     leading space after the colon is stripped. We strip a single leading
//     space from the value to match.
//
// The `data:` payloads in Qorven always carry JSON conforming to the
// api/events.Envelope shape; Stream[T] unmarshals into the caller's T.
//
// Licensing: the external SDK we match in shape is MIT-licensed; this
// file is fresh Qorven code.
package ssestream

import (
	"bufio"
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
)

// Event represents a single SSE event as framed off the wire. The fields are
// set by the decoder verbatim; JSON decoding into the caller's target type
// happens inside Stream[T].
type Event struct {
	Type  string // "event:" field, or "message" if absent
	Data  []byte // concatenated "data:" payload, newline-joined
	ID    string // "id:" field if present
	Retry int    // "retry:" field if present (milliseconds)
}

// Decoder streams SSE Events off an io.ReadCloser. Implementations must be
// safe to call Close multiple times.
type Decoder interface {
	Event() Event
	Next() bool
	Close() error
	Err() error
}

// DecoderFactory produces a fresh Decoder for the given response body.
// Registered per content-type via RegisterDecoder.
type DecoderFactory func(body io.ReadCloser) Decoder

var (
	registryMu sync.RWMutex
	registry   = map[string]DecoderFactory{
		"text/event-stream": newEventStreamDecoder,
	}
)

// RegisterDecoder installs a decoder factory for the given content-type.
// The content-type is matched case-insensitively against the media-type
// portion of the response Content-Type header (parameters are ignored).
func RegisterDecoder(contentType string, f DecoderFactory) {
	if f == nil {
		panic("ssestream: nil DecoderFactory")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[strings.ToLower(contentType)] = f
}

// lookupDecoder returns the factory for the given content-type header value,
// or nil if no decoder is registered.
func lookupDecoder(contentType string) DecoderFactory {
	mediaType := contentType
	if i := strings.Index(contentType, ";"); i >= 0 {
		mediaType = contentType[:i]
	}
	mediaType = strings.TrimSpace(strings.ToLower(mediaType))
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[mediaType]
}

// ─────────────────────────────── Stream ──────────────────────────────────

// Stream[T] iterates JSON-typed events off an SSE response body. Call Next
// until it returns false, then Err to check for a decoding error.
//
// Typical usage:
//
//	s, err := ssestream.NewStream[events.Envelope](resp)
//	if err != nil { return err }
//	defer s.Close()
//	for s.Next() {
//	    evt := s.Current()
//	    switch evt.Type { ... }
//	}
//	if err := s.Err(); err != nil { ... }
type Stream[T any] struct {
	decoder Decoder
	current T
	err     error
	closed  bool

	// If the server emits `data: [DONE]` — our historical sentinel — the
	// stream terminates cleanly without attempting to JSON-decode [DONE].
	doneSentinel string
}

// NewStream wraps an *http.Response whose body is an SSE stream. It picks
// a decoder based on the response's Content-Type (defaulting to
// text/event-stream when no content-type header is present). On error
// the response body is drained and closed.
func NewStream[T any](resp *http.Response) (*Stream[T], error) {
	if resp == nil {
		return nil, errors.New("ssestream: nil response")
	}
	if resp.Body == nil {
		return nil, errors.New("ssestream: response has no body")
	}
	ct := resp.Header.Get("Content-Type")
	factory := lookupDecoder(ct)
	if factory == nil {
		if ct == "" {
			factory = newEventStreamDecoder
		} else {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return nil, fmt.Errorf("ssestream: no decoder registered for content-type %q", ct)
		}
	}
	return &Stream[T]{
		decoder:      factory(resp.Body),
		doneSentinel: "[DONE]",
	}, nil
}

// NewStreamReader wraps a raw io.ReadCloser assuming text/event-stream
// framing. Useful for in-process tests and non-HTTP consumers.
func NewStreamReader[T any](body io.ReadCloser) *Stream[T] {
	return &Stream[T]{
		decoder:      newEventStreamDecoder(body),
		doneSentinel: "[DONE]",
	}
}

// SetDoneSentinel changes the literal `data:` payload that terminates the
// stream without being JSON-decoded. Default: "[DONE]". Pass "" to disable.
func (s *Stream[T]) SetDoneSentinel(sentinel string) { s.doneSentinel = sentinel }

// Next advances to the next event. Returns false on EOF or error. When it
// returns false, call Err() to distinguish clean EOF from a decode failure.
func (s *Stream[T]) Next() bool {
	if s.closed || s.err != nil {
		return false
	}
	for s.decoder.Next() {
		evt := s.decoder.Event()
		if len(evt.Data) == 0 {
			// Events with no `data:` are skipped per SSE spec.
			continue
		}
		if s.doneSentinel != "" && string(bytes.TrimSpace(evt.Data)) == s.doneSentinel {
			return false
		}
		var target T
		if err := json.Unmarshal(evt.Data, &target); err != nil {
			s.err = fmt.Errorf("ssestream: decode JSON: %w (payload: %s)", err, truncate(evt.Data, 256))
			return false
		}
		s.current = target
		return true
	}
	if err := s.decoder.Err(); err != nil {
		s.err = err
	}
	return false
}

// NextRaw advances to the next event without JSON-decoding it. Exposes the
// raw Event so callers can inspect the SSE Type/ID fields (not just the
// data payload). Same semantics as Next for EOF + error.
func (s *Stream[T]) NextRaw() (Event, bool) {
	if s.closed || s.err != nil {
		return Event{}, false
	}
	for s.decoder.Next() {
		evt := s.decoder.Event()
		if len(evt.Data) == 0 {
			continue
		}
		if s.doneSentinel != "" && string(bytes.TrimSpace(evt.Data)) == s.doneSentinel {
			return Event{}, false
		}
		return evt, true
	}
	if err := s.decoder.Err(); err != nil {
		s.err = err
	}
	return Event{}, false
}

// Current returns the most recent event decoded by Next. Only valid after
// Next returns true.
func (s *Stream[T]) Current() T { return s.current }

// Err returns the first error encountered during iteration, or nil on
// clean EOF.
func (s *Stream[T]) Err() error { return s.err }

// Close shuts down the underlying decoder. Safe to call multiple times.
func (s *Stream[T]) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.decoder.Close()
}

// ReadAll drains the entire stream, returning every successfully decoded
// event or the first error. Closes the stream before returning.
func (s *Stream[T]) ReadAll(ctx context.Context) ([]T, error) {
	defer s.Close()
	out := []T{}
	for {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			default:
			}
		}
		if !s.Next() {
			break
		}
		out = append(out, s.Current())
	}
	return out, s.Err()
}

// ───────────────────────── eventStreamDecoder ────────────────────────────

// eventStreamDecoder implements Decoder for text/event-stream. It accumulates
// `data:` / `event:` / `id:` / `retry:` fields per event until a blank line,
// then exposes the event via Event(). Implementation follows the WHATWG
// algorithm for parsing an event stream.
type eventStreamDecoder struct {
	body    io.ReadCloser
	scanner *bufio.Scanner

	// accumulator for the event currently being parsed
	eventType  string
	dataBuf    bytes.Buffer
	lastID     string
	retryMS    int
	retrySet   bool

	pending Event
	ready   bool
	err     error
	closed  bool
}

// Maximum single-event buffer: 1 MiB. SSE frames larger than this indicate a
// misbehaving server; we fail loudly rather than OOM.
const maxEventBuffer = 1 << 20

func newEventStreamDecoder(body io.ReadCloser) Decoder {
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), maxEventBuffer)
	sc.Split(scanSSELines)
	return &eventStreamDecoder{body: body, scanner: sc}
}

// scanSSELines is a bufio.SplitFunc that splits on \r\n, \n, or \r (per SSE
// spec). Returns the line WITHOUT the terminator.
func scanSSELines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		switch b {
		case '\n':
			return i + 1, data[:i], nil
		case '\r':
			// If \r\n, consume both bytes as one terminator.
			if i+1 < len(data) && data[i+1] == '\n' {
				return i + 2, data[:i], nil
			}
			// Bare \r is a line terminator only if we can see past it.
			if i+1 < len(data) || atEOF {
				return i + 1, data[:i], nil
			}
			// Need more data to disambiguate \r\n.
			return 0, nil, nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (d *eventStreamDecoder) Next() bool {
	if d.closed || d.err != nil {
		return false
	}
	for d.scanner.Scan() {
		line := d.scanner.Bytes()

		// Blank line: dispatch accumulated event.
		if len(line) == 0 {
			if d.dataBuf.Len() == 0 && d.eventType == "" && d.lastID == "" && !d.retrySet {
				continue
			}
			d.dispatch()
			if d.ready {
				return true
			}
			continue
		}

		// Comment line: ignore.
		if line[0] == ':' {
			continue
		}

		// Parse "field: value" or "field" (no colon → empty value).
		name, value := splitField(line)
		switch name {
		case "event":
			d.eventType = string(value)
		case "data":
			if d.dataBuf.Len() > 0 {
				d.dataBuf.WriteByte('\n')
			}
			d.dataBuf.Write(value)
		case "id":
			// Ignore id lines containing a NUL per SSE spec.
			if !bytes.ContainsRune(value, 0) {
				d.lastID = string(value)
			}
		case "retry":
			if n, err := strconv.Atoi(string(value)); err == nil && n >= 0 {
				d.retryMS = n
				d.retrySet = true
			}
		default:
			// Unknown fields are ignored per spec.
		}
	}

	// Flush any trailing event on EOF (some servers omit the final blank line).
	if d.dataBuf.Len() > 0 || d.eventType != "" || d.lastID != "" || d.retrySet {
		d.dispatch()
		if d.ready {
			return true
		}
	}

	if err := d.scanner.Err(); err != nil {
		d.err = fmt.Errorf("ssestream: scan: %w", err)
	}
	return false
}

// dispatch commits the accumulated state into d.pending and resets the
// accumulator for the next event.
func (d *eventStreamDecoder) dispatch() {
	et := d.eventType
	if et == "" {
		et = "message"
	}
	// Copy data because the scanner's slice is reused on the next Scan.
	payload := make([]byte, d.dataBuf.Len())
	copy(payload, d.dataBuf.Bytes())

	d.pending = Event{
		Type:  et,
		Data:  payload,
		ID:    d.lastID,
		Retry: d.retryMS,
	}
	d.ready = true
	d.eventType = ""
	d.dataBuf.Reset()
	d.retrySet = false
	d.retryMS = 0
}

func (d *eventStreamDecoder) Event() Event {
	evt := d.pending
	d.ready = false
	d.pending = Event{}
	return evt
}

func (d *eventStreamDecoder) Err() error { return d.err }

func (d *eventStreamDecoder) Close() error {
	if d.closed {
		return nil
	}
	d.closed = true
	return d.body.Close()
}

// splitField parses "name: value" → (name, value), stripping at most one
// leading space from value. Returns (name, nil) when no colon is present.
func splitField(line []byte) (name string, value []byte) {
	i := bytes.IndexByte(line, ':')
	if i < 0 {
		return string(line), nil
	}
	name = string(line[:i])
	value = line[i+1:]
	if len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	return name, value
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "...<truncated>"
}
