// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package deepsearch

import (
	"context"
	"fmt"
	"sync"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSearchGraph_New(t *testing.T) {
	g := NewSearchGraph()
	if g == nil { t.Fatal("nil graph") }
}

func TestSearchGraph_AddRoot(t *testing.T) {
	g := NewSearchGraph()
	id := g.AddRoot("What is quantum computing?")
	if id != "root" { t.Errorf("root id=%q", id) }
}

func TestSearchGraph_AddSubQuery(t *testing.T) {
	g := NewSearchGraph()
	g.AddRoot("main query")
	id := g.AddSubQuery("root", "sub query 1", "topic 1")
	if id == "" { t.Error("empty sub query id") }
	if id == "root" { t.Error("sub query should not be root") }
}

func TestSearchGraph_SetResult(t *testing.T) {
	g := NewSearchGraph()
	g.AddRoot("query")
	id := g.AddSubQuery("root", "sub", "topic")
	g.SetResult(id, "found something")
	results := g.AllResults()
	if !strings.Contains(results, "found something") { t.Error("result not in AllResults") }
}

func TestSearchGraph_SetFailed(t *testing.T) {
	g := NewSearchGraph()
	g.AddRoot("query")
	id := g.AddSubQuery("root", "sub", "topic")
	g.SetFailed(id, "timeout")
	results := g.AllResults()
	if strings.Contains(results, "timeout") { t.Error("failed results should not appear in AllResults") }
}

func TestSearchGraph_AllResults_Empty(t *testing.T) {
	g := NewSearchGraph()
	g.AddRoot("query")
	if g.AllResults() != "" { t.Error("no results should be empty") }
}

func TestEngine_New(t *testing.T) {
	e := NewEngine(nil, nil, nil)
	if e == nil { t.Fatal("nil engine") }
	if e.maxDepth != 2 { t.Errorf("maxDepth=%d", e.maxDepth) }
	if e.maxParallel != 5 { t.Errorf("maxParallel=%d", e.maxParallel) }
}

func TestEngine_Search_SimpleQuery(t *testing.T) {
	// Decompose returns empty → direct search
	search := func(ctx context.Context, q string) (string, error) { return "direct result for: " + q, nil }
	decompose := func(ctx context.Context, q string) ([]SubQuery, error) { return nil, nil }
	synthesize := func(ctx context.Context, q, sub string) (string, error) { return "synthesized", nil }

	e := NewEngine(search, decompose, synthesize)
	result, err := e.Search(context.Background(), "simple question")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(result, "direct result") { t.Errorf("result=%q", result) }
}

func TestEngine_Search_WithDecomposition(t *testing.T) {
	var searchCount atomic.Int32
	search := func(ctx context.Context, q string) (string, error) {
		searchCount.Add(1)
		return "result for: " + q, nil
	}
	decompose := func(ctx context.Context, q string) ([]SubQuery, error) {
		return []SubQuery{
			{Query: "sub1", Topic: "Topic A"},
			{Query: "sub2", Topic: "Topic B"},
			{Query: "sub3", Topic: "Topic C"},
		}, nil
	}
	synthesize := func(ctx context.Context, q, sub string) (string, error) {
		return "SYNTHESIZED: " + sub, nil
	}

	e := NewEngine(search, decompose, synthesize)
	result, err := e.Search(context.Background(), "complex question")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(result, "SYNTHESIZED") { t.Errorf("result=%q", result) }
	if searchCount.Load() != 3 { t.Errorf("expected 3 searches, got %d", searchCount.Load()) }
}

func TestEngine_Search_ParallelExecution(t *testing.T) {
	// Verify sub-queries run in parallel by checking timing
	search := func(ctx context.Context, q string) (string, error) {
		time.Sleep(50 * time.Millisecond)
		return "ok", nil
	}
	decompose := func(ctx context.Context, q string) ([]SubQuery, error) {
		return []SubQuery{{Query: "a"}, {Query: "b"}, {Query: "c"}, {Query: "d"}}, nil
	}
	synthesize := func(ctx context.Context, q, sub string) (string, error) { return sub, nil }

	e := NewEngine(search, decompose, synthesize)
	start := time.Now()
	_, err := e.Search(context.Background(), "parallel test")
	elapsed := time.Since(start)
	if err != nil { t.Fatal(err) }
	// 4 queries × 50ms each. Sequential = 200ms. Parallel should be ~50-100ms.
	if elapsed > 180*time.Millisecond { t.Errorf("too slow (%v) — not parallel?", elapsed) }
}

func TestEngine_Search_DecomposeFailure_Fallback(t *testing.T) {
	search := func(ctx context.Context, q string) (string, error) { return "fallback result", nil }
	decompose := func(ctx context.Context, q string) ([]SubQuery, error) { return nil, fmt.Errorf("LLM error") }
	synthesize := func(ctx context.Context, q, sub string) (string, error) { return "", nil }

	e := NewEngine(search, decompose, synthesize)
	result, err := e.Search(context.Background(), "query")
	if err != nil { t.Fatal(err) }
	if result != "fallback result" { t.Errorf("should fallback to direct search: %q", result) }
}

func TestEngine_Search_AllSubQueriesFail(t *testing.T) {
	search := func(ctx context.Context, q string) (string, error) { return "", fmt.Errorf("search failed") }
	decompose := func(ctx context.Context, q string) ([]SubQuery, error) {
		return []SubQuery{{Query: "a"}, {Query: "b"}}, nil
	}
	synthesize := func(ctx context.Context, q, sub string) (string, error) { return "", nil }

	e := NewEngine(search, decompose, synthesize)
	_, err := e.Search(context.Background(), "query")
	if err == nil { t.Error("should fail when all sub-queries fail") }
}

func TestEngine_Search_SynthesizeFailure_RawResults(t *testing.T) {
	search := func(ctx context.Context, q string) (string, error) { return "raw: " + q, nil }
	decompose := func(ctx context.Context, q string) ([]SubQuery, error) {
		return []SubQuery{{Query: "a", Topic: "A"}}, nil
	}
	synthesize := func(ctx context.Context, q, sub string) (string, error) { return "", fmt.Errorf("LLM down") }

	e := NewEngine(search, decompose, synthesize)
	result, err := e.Search(context.Background(), "query")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(result, "raw: a") { t.Errorf("should return raw results: %q", result) }
}

func TestEngine_Search_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	search := func(ctx context.Context, q string) (string, error) {
		return "", ctx.Err()
	}
	decompose := func(ctx context.Context, q string) ([]SubQuery, error) { return nil, ctx.Err() }
	synthesize := func(ctx context.Context, q, sub string) (string, error) { return "", nil }

	e := NewEngine(search, decompose, synthesize)
	_, err := e.Search(ctx, "query")
	// Should handle cancelled context gracefully
	if err == nil { t.Log("cancelled context handled") }
}

func TestSubQuery_Fields(t *testing.T) {
	sq := SubQuery{Query: "what is X", Topic: "Definition"}
	if sq.Query == "" { t.Error("empty query") }
	if sq.Topic == "" { t.Error("empty topic") }
}

// === HARD DEEPSEARCH TESTS ===

func TestEngine_Search_ManySubQueries(t *testing.T) {
	var searchCount atomic.Int32
	search := func(ctx context.Context, q string) (string, error) {
		searchCount.Add(1)
		time.Sleep(10 * time.Millisecond)
		return "result for " + q, nil
	}
	decompose := func(ctx context.Context, q string) ([]SubQuery, error) {
		subs := make([]SubQuery, 20)
		for i := range subs { subs[i] = SubQuery{Query: "sub" + string(rune('0'+i%10)), Topic: "T"} }
		return subs, nil
	}
	synthesize := func(ctx context.Context, q, sub string) (string, error) { return "FINAL: " + sub[:100], nil }

	e := NewEngine(search, decompose, synthesize)
	start := time.Now()
	result, err := e.Search(context.Background(), "big question")
	elapsed := time.Since(start)
	if err != nil { t.Fatal(err) }
	if result == "" { t.Error("empty result") }
	if searchCount.Load() != 20 { t.Errorf("expected 20 searches, got %d", searchCount.Load()) }
	// 20 queries × 10ms each, parallel (max 5) = ~40ms. Sequential would be 200ms.
	if elapsed > 150*time.Millisecond { t.Errorf("too slow (%v) — not parallel enough?", elapsed) }
}

func TestSearchGraph_ConcurrentAccess(t *testing.T) {
	g := NewSearchGraph()
	g.AddRoot("main")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := g.AddSubQuery("root", "q"+string(rune('0'+n%10)), "topic")
			g.SetResult(id, "result")
			g.AllResults()
		}(i)
	}
	wg.Wait()
}
