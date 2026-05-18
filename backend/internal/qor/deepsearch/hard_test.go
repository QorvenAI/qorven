// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package deepsearch

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHard_DeepSearch_FullPipeline(t *testing.T) {
	var searchCount atomic.Int32
	search := func(ctx context.Context, q string) (string, error) {
		searchCount.Add(1)
		return "Result for: " + q, nil
	}
	decompose := func(ctx context.Context, q string) ([]SubQuery, error) {
		return []SubQuery{
			{Query: "What is " + q, Topic: "Definition"},
			{Query: "History of " + q, Topic: "History"},
			{Query: "Examples of " + q, Topic: "Examples"},
		}, nil
	}
	synthesize := func(ctx context.Context, q, sub string) (string, error) {
		return "SYNTHESIS: " + q + "\n\n" + sub, nil
	}

	e := NewEngine(search, decompose, synthesize)
	result, err := e.Search(context.Background(), "quantum computing")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(result, "SYNTHESIS") { t.Error("missing synthesis") }
	if searchCount.Load() != 3 { t.Errorf("expected 3 searches, got %d", searchCount.Load()) }
	t.Logf("deep search: 3 sub-queries, synthesized result (%d chars)", len(result))
}

func TestHard_DeepSearch_ErrorResilience(t *testing.T) {
	failCount := 0
	search := func(ctx context.Context, q string) (string, error) {
		failCount++
		if failCount <= 2 { return "", fmt.Errorf("search failed") }
		return "success: " + q, nil
	}
	decompose := func(ctx context.Context, q string) ([]SubQuery, error) {
		return []SubQuery{{Query: "a"}, {Query: "b"}, {Query: "c"}}, nil
	}
	synthesize := func(ctx context.Context, q, sub string) (string, error) { return sub, nil }

	e := NewEngine(search, decompose, synthesize)
	result, err := e.Search(context.Background(), "test")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(result, "success") { t.Error("should have partial results") }
	t.Logf("error resilience: %d failures, still got result ✅", failCount-1)
}

func TestHard_DeepSearch_Timeout(t *testing.T) {
	search := func(ctx context.Context, q string) (string, error) {
		time.Sleep(5 * time.Second)
		return "late", nil
	}
	decompose := func(ctx context.Context, q string) ([]SubQuery, error) {
		return []SubQuery{{Query: "slow"}}, nil
	}
	synthesize := func(ctx context.Context, q, sub string) (string, error) { return sub, nil }

	e := NewEngine(search, decompose, synthesize)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := e.Search(ctx, "test")
	// Should handle timeout gracefully
	t.Logf("timeout handling: err=%v", err)
}
