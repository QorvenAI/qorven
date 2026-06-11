// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package lsp

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestEncodeDecodeFraming(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	var buf bytes.Buffer
	if err := writeFramed(&buf, payload); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "Content-Length: ") {
		t.Errorf("missing header: %q", buf.String())
	}
	got, err := readFramed(bufio.NewReader(&buf))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Errorf("roundtrip: got %q", got)
	}
}

func TestWriteFramedHeader(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","method":"textDocument/didOpen"}`)
	var buf bytes.Buffer
	if err := writeFramed(&buf, payload); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	expected := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if !strings.HasPrefix(s, expected) {
		t.Errorf("expected header %q, got prefix %q", expected, s[:min(len(s), 40)])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
