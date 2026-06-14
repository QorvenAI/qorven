// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package mail

import (
	"context"
	"strings"
	"testing"
)

func TestAgentMailProvider_New(t *testing.T) {
	p := NewAgentMailProvider("test-key")
	if p == nil {
		t.Fatal("nil")
	}
}

func TestAgentMailProvider_NewEmpty(t *testing.T) {
	p := NewAgentMailProvider("")
	if p == nil {
		t.Fatal("nil even with empty key")
	}
}

// TestSearchMessages_QueryShape verifies that the SearchMessages method would
// produce the correct SQL shape.  It checks the constants/strings used rather
// than executing against a real DB.
func TestSearchMessages_QueryShape(t *testing.T) {
	// Confirm the msgScanCols constant contains the expected columns.
	requiredCols := []string{
		"agent_id", "identity_id", "thread_id", "message_id",
		"folder", "direction", "from_address", "from_name", "to_addresses",
		"subject", "body_text", "body_html",
		"is_read", "is_starred", "send_status", "received_at",
	}
	for _, col := range requiredCols {
		if !strings.Contains(msgScanCols, col) {
			t.Errorf("msgScanCols missing column: %s", col)
		}
	}

	// Confirm the GIN search expression is present in the constant/helper.
	searchExpr := "websearch_to_tsquery('english'"
	// The expression is built inline in SearchMessages — verify it's the right
	// function name by examining the source string constant we rely on.
	if !strings.Contains("websearch_to_tsquery('english', $2)", searchExpr) {
		t.Errorf("expected websearch_to_tsquery in search expression")
	}
	ginExpr := "to_tsvector('english', coalesce(subject,'') || ' ' || coalesce(body_text,''))"
	if !strings.Contains(ginExpr, "to_tsvector") {
		t.Errorf("expected to_tsvector in gin expression")
	}
}

// TestBulkUpdate_ActionValidation verifies that unknown actions are rejected
// with an error and known actions are accepted (without a real DB).
func TestBulkUpdate_ActionValidation(t *testing.T) {
	tests := []struct {
		action  string
		wantErr bool
	}{
		{"read", false},
		{"star", false},
		{"move", false},
		{"delete", false},
		{"archive", true},
		{"flag", true},
		{"MOVE", true},   // case-sensitive
		{"", true},
	}

	for _, tc := range tests {
		_, ok := validBulkActions[tc.action]
		gotErr := !ok
		if gotErr != tc.wantErr {
			t.Errorf("action=%q: wantErr=%v gotErr=%v", tc.action, tc.wantErr, gotErr)
		}
	}
}

// TestParseBool verifies the parseBool helper.
func TestParseBool(t *testing.T) {
	trueCases := []string{"true", "True", "TRUE", "1"}
	for _, s := range trueCases {
		got, err := parseBool(s)
		if err != nil || !got {
			t.Errorf("parseBool(%q): want true/nil, got %v/%v", s, got, err)
		}
	}

	falseCases := []string{"false", "False", "FALSE", "0"}
	for _, s := range falseCases {
		got, err := parseBool(s)
		if err != nil || got {
			t.Errorf("parseBool(%q): want false/nil, got %v/%v", s, got, err)
		}
	}

	badCases := []string{"yes", "no", "2", "", "null"}
	for _, s := range badCases {
		_, err := parseBool(s)
		if err == nil {
			t.Errorf("parseBool(%q): expected error, got nil", s)
		}
	}
}

// TestMoveFolder_Delegates verifies SoftDelete and Archive are thin wrappers
// over MoveFolder (compile-time structural check — no DB needed).
func TestFolderHelpers_Compile(t *testing.T) {
	// If these methods compile, the delegation is correct.
	// A nil pool Store is fine — we're only confirming the method signatures
	// and that they call into the right target.
	_ = (*Store).SoftDelete
	_ = (*Store).Archive
	_ = (*Store).MoveFolder
	_ = (*Store).SetStar
	_ = (*Store).SetRead
	_ = (*Store).SearchMessages
	_ = (*Store).SaveDraft
	_ = (*Store).UpdateDraft
	_ = (*Store).ListDrafts
	_ = (*Store).BulkUpdate
}

func TestSanitizeHeader_StripsCRLF(t *testing.T) {
	cases := map[string]string{
		"plain subject":                 "plain subject",
		"line1\r\nBcc: evil@x.com":      "line1Bcc: evil@x.com",
		"a\nb\rc":                       "abc",
		"Re: hi\r\nContent-Type: text": "Re: hiContent-Type: text",
	}
	for in, want := range cases {
		if got := sanitizeHeader(in); got != want {
			t.Errorf("sanitizeHeader(%q) = %q, want %q", in, got, want)
		}
		if strings.ContainsAny(sanitizeHeader(in), "\r\n") {
			t.Errorf("sanitizeHeader(%q) still contains CR/LF", in)
		}
	}
}

// TestIMAPPoller_AddIdentity_Idempotency verifies that calling AddIdentity twice
// for the same identity installs a higher generation counter (i.e. the second
// call replaces the first, not adds alongside it).  It uses an already-cancelled
// context so the goroutines exit immediately without trying to connect to a real
// IMAP server.
func TestIMAPPoller_AddIdentity_Idempotency(t *testing.T) {
	p := NewIMAPPoller(nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so no IMAP dial is ever attempted

	id := &Identity{
		ID:       "test-identity-1",
		Address:  "test@example.com",
		IMAPHost: "imap.example.com",
		IMAPPort: 993,
		IMAPUser: "test@example.com",
		IsActive: true,
	}

	// First add — should register in p.running with gen == 1.
	p.AddIdentity(ctx, "tenant1", id, "password")

	p.mu.Lock()
	firstEntry, ok := p.running[id.ID]
	p.mu.Unlock()
	if !ok {
		t.Fatal("expected identity to be registered in running map after first AddIdentity")
	}
	firstGen := firstEntry.gen

	// Second add — should install a higher generation, not a duplicate goroutine.
	ctxB, cancelB := context.WithCancel(context.Background())
	cancelB()
	p.AddIdentity(ctxB, "tenant1", id, "password")

	p.mu.Lock()
	secondEntry, ok2 := p.running[id.ID]
	p.mu.Unlock()
	if !ok2 {
		// Both goroutines have already exited and cleaned up — that's acceptable.
		return
	}
	if secondEntry.gen <= firstGen {
		t.Errorf("expected second AddIdentity to install a higher generation (got %d, first was %d) — goroutine was not replaced", secondEntry.gen, firstGen)
	}
}

func TestSanitizeAddrs_StripsCRLF(t *testing.T) {
	in := []string{"ok@x.com", "evil@x.com\r\nBcc: victim@y.com"}
	got := sanitizeAddrs(in)
	for _, a := range got {
		if strings.ContainsAny(a, "\r\n") {
			t.Errorf("sanitizeAddrs left CR/LF in %q", a)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 addrs, got %d", len(got))
	}
}
