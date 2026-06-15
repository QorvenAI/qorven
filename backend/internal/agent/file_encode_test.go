// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncodeAttachedFiles(t *testing.T) {
	// base64 data URI → decoded UTF-8 inside an <attached_file> block
	parts := []FilePart{{URL: "data:text/csv;base64," + base64.StdEncoding.EncodeToString([]byte("a,b\n1,2")), MediaType: "text/csv", Filename: "d.csv"}}
	out := EncodeAttachedFiles("hi", parts)
	if !strings.Contains(out, `<attached_file name="d.csv" type="text/csv">`) {
		t.Fatalf("missing block header: %q", out)
	}
	if !strings.Contains(out, "a,b") {
		t.Fatalf("missing decoded content: %q", out)
	}
	if !strings.HasPrefix(out, "hi") {
		t.Fatalf("original message should be preserved: %q", out)
	}
	// round-trips through the existing parser
	clean, files := ExtractFilesFromMessage(out)
	if len(files) != 1 {
		t.Fatalf("want 1 extracted file, got %d", len(files))
	}
	if !strings.Contains(clean, "hi") {
		t.Fatalf("clean message lost original text: %q", clean)
	}
}

func TestEncodeAttachedFiles_RemoteURL(t *testing.T) {
	parts := []FilePart{{URL: "https://example.com/x.pdf", MediaType: "application/pdf", Filename: "x.pdf"}}
	out := EncodeAttachedFiles("", parts)
	if !strings.Contains(out, "https://example.com/x.pdf") {
		t.Fatalf("remote URL should be referenced: %q", out)
	}
}

func TestEncodeAttachedFiles_Empty(t *testing.T) {
	if got := EncodeAttachedFiles("just text", nil); got != "just text" {
		t.Fatalf("no parts should return msg unchanged, got %q", got)
	}
}

func TestEncodeAttachedFiles_Cap(t *testing.T) {
	big := strings.Repeat("x", 100000)
	parts := []FilePart{{URL: "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte(big)), MediaType: "text/plain", Filename: "big.txt"}}
	out := EncodeAttachedFiles("", parts)
	if len(out) > 90000 { // capped at 80K + block overhead + truncation note
		t.Fatalf("content should be capped ~80K, got %d", len(out))
	}
}
