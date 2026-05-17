// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// hard_dedup_test.go — Tests for response deduplication and stream guard.

func TestHard_ResponseDedup_DetectsDuplicate(t *testing.T) {
	d := NewResponseDedup(5*time.Second, 100)

	// First request — not a duplicate
	_, isDup := d.CheckRequest("sess1", "What is Go?")
	if isDup { t.Error("first request should not be duplicate") }

	// Record the response
	d.RecordRequest("sess1", "What is Go?", "Go is a programming language.")

	// Same request again — should be duplicate
	cached, isDup := d.CheckRequest("sess1", "What is Go?")
	if !isDup { t.Error("same request should be duplicate") }
	if cached != "Go is a programming language." { t.Errorf("cached: %q", cached) }
}

func TestHard_ResponseDedup_DifferentSessionNotDuplicate(t *testing.T) {
	d := NewResponseDedup(5*time.Second, 100)
	d.RecordRequest("sess1", "What is Go?", "answer")

	_, isDup := d.CheckRequest("sess2", "What is Go?")
	if isDup { t.Error("different session should not be duplicate") }
}

func TestHard_ResponseDedup_DifferentMessageNotDuplicate(t *testing.T) {
	d := NewResponseDedup(5*time.Second, 100)
	d.RecordRequest("sess1", "What is Go?", "answer")

	_, isDup := d.CheckRequest("sess1", "What is Rust?")
	if isDup { t.Error("different message should not be duplicate") }
}

func TestHard_ResponseDedup_ExpiresAfterTTL(t *testing.T) {
	d := NewResponseDedup(50*time.Millisecond, 100)
	d.RecordRequest("sess1", "test", "answer")

	time.Sleep(100 * time.Millisecond)

	_, isDup := d.CheckRequest("sess1", "test")
	if isDup { t.Error("should expire after TTL") }
}

func TestHard_ResponseDedup_StreamGuard(t *testing.T) {
	d := NewResponseDedup(5*time.Second, 100)

	// Acquire stream
	ok := d.AcquireStream("sess1")
	if !ok { t.Error("first acquire should succeed") }

	// Second acquire for same session should fail
	ok = d.AcquireStream("sess1")
	if ok { t.Error("second acquire should fail — stream already active") }

	// Different session should succeed
	ok = d.AcquireStream("sess2")
	if !ok { t.Error("different session should succeed") }

	// Release and re-acquire
	d.ReleaseStream("sess1")
	ok = d.AcquireStream("sess1")
	if !ok { t.Error("should succeed after release") }

	d.ReleaseStream("sess1")
	d.ReleaseStream("sess2")
}

func TestHard_ResponseDedup_ConcurrentAccess(t *testing.T) {
	d := NewResponseDedup(5*time.Second, 1000)
	var wg sync.WaitGroup
	panicked := false

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			defer func() { if r := recover(); r != nil { panicked = true } }()
			sess := "sess" + string(rune('0'+n%10))
			msg := "message " + string(rune('A'+n%26))
			d.CheckRequest(sess, msg)
			d.RecordRequest(sess, msg, "response")
			d.AcquireStream(sess)
			d.ReleaseStream(sess)
		}(i)
	}
	wg.Wait()
	if panicked { t.Fatal("concurrent access caused panic") }

	// Verify state is consistent — at least some requests should be cached
	_, isDup := d.CheckRequest("sess0", "message A")
	if !isDup { t.Error("recorded request should be cached") }
}

func TestHard_ResponseDedup_HashCollisionResistance(t *testing.T) {
	d := NewResponseDedup(5*time.Second, 100)

	// These should produce different hashes
	d.RecordRequest("sess1", "abc", "response1")
	d.RecordRequest("sess1", "abd", "response2")

	cached, isDup := d.CheckRequest("sess1", "abc")
	if !isDup { t.Fatal("abc should be cached") }
	if cached != "response1" { t.Error("wrong cached response for abc") }

	cached, isDup = d.CheckRequest("sess1", "abd")
	if !isDup { t.Fatal("abd should be cached") }
	if cached != "response2" { t.Error("wrong cached response for abd") }
}

// ── Sanitize Error (existing function) ──

func TestHard_SanitizeError_HidesInternalPaths(t *testing.T) {
	dangerous := []string{
		"/home/ec2-user/secret/config.toml",
		"postgres://user:password@localhost:5432/db",
		"api_key=sk-proj-abc123",
		"Bearer eyJhbGciOiJIUzI1NiJ9",
	}
	for _, input := range dangerous {
		result := sanitizeError(errFromString(input))
		if strings.Contains(result, "ec2-user") { t.Errorf("leaked path: %q", result) }
		if strings.Contains(result, "password") { t.Errorf("leaked password: %q", result) }
		if strings.Contains(result, "sk-proj") { t.Errorf("leaked API key: %q", result) }
		if strings.Contains(result, "eyJhbG") { t.Errorf("leaked JWT: %q", result) }
	}
	t.Log("error sanitization: no secrets leaked ✓")
}
