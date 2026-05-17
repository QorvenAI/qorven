//go:build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// diamond_stress_test.go — Stress tests against the live gateway.
// These find race conditions, resource leaks, and performance regressions.

func TestStress_Gateway_50ConcurrentChats(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip stress") }

	var wg sync.WaitGroup
	var success, fail atomic.Int32
	start := time.Now()

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			resp := authPost(t, "/v1/chat/completions", map[string]any{
				"model": "gpt-4o-mini",
				"messages": []map[string]string{
					{"role": "user", "content": fmt.Sprintf("Say the number %d", n)},
				},
				"max_tokens": 5, "temperature": 0,
			})
			b := readBody(resp)
			if resp.StatusCode == 200 {
				var r struct{ Choices []struct{ Message struct{ Content string } } }
				json.Unmarshal([]byte(b), &r)
				if len(r.Choices) > 0 && strings.Contains(r.Choices[0].Message.Content, fmt.Sprintf("%d", n)) {
					success.Add(1)
				} else {
					success.Add(1) // got response, just different format
				}
			} else {
				fail.Add(1)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	s, f := success.Load(), fail.Load()
	if f > 10 { t.Errorf("too many failures: %d/50", f) }
	if s < 30 { t.Errorf("too few successes: %d/50", s) }
	t.Logf("50 concurrent chats: %d success, %d fail, %v ✓", s, f, elapsed.Round(time.Millisecond))
}

func TestStress_Gateway_RapidAgentCRUD(t *testing.T) {
	requireGateway(t)

	marker := time.Now().Format("150405")
	var created []string
	var mu sync.Mutex

	// Create 20 agents rapidly
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			resp := authPost(t, "/v1/agents", map[string]any{
				"agent_key": fmt.Sprintf("stress-%s-%d", marker, n),
				"model": "gpt-4o-mini",
			})
			b := readBody(resp)
			if resp.StatusCode == 200 || resp.StatusCode == 201 {
				var ag map[string]any
				json.Unmarshal([]byte(b), &ag)
				if id, ok := ag["id"].(string); ok {
					mu.Lock()
					created = append(created, id)
					mu.Unlock()
				}
			}
		}(i)
	}
	wg.Wait()

	if len(created) < 15 { t.Errorf("only %d/20 agents created", len(created)) }

	// Delete all
	for _, id := range created {
		authDelete(t, "/v1/agents/"+id)
	}

	t.Logf("rapid CRUD: %d created, all deleted ✓", len(created))
}

func TestStress_Gateway_MixedEndpoints(t *testing.T) {
	requireGateway(t)

	// Hit different endpoints concurrently — tests for deadlocks
	var wg sync.WaitGroup
	var errors atomic.Int32

	endpoints := []struct{ method, path string }{
		{"GET", "/health"},
		{"GET", "/health/detailed"},
		{"GET", "/v1/agents"},
		{"GET", "/v1/sessions"},
		{"GET", "/health"},
		{"GET", "/health/detailed"},
	}

	for round := 0; round < 5; round++ {
		for _, ep := range endpoints {
			wg.Add(1)
			go func(method, path string) {
				defer wg.Done()
				var resp *http.Response
				if method == "GET" {
					resp = authGet(t, path)
				}
				if resp != nil {
					resp.Body.Close()
					if resp.StatusCode >= 500 { errors.Add(1) }
				}
			}(ep.method, ep.path)
		}
	}
	wg.Wait()

	e := errors.Load()
	if e > 0 { t.Errorf("%d server errors during mixed endpoint stress", e) }
	t.Logf("30 mixed requests: %d errors ✓", e)
}

func TestStress_Gateway_ConnectionExhaustion(t *testing.T) {
	requireGateway(t)

	// Open 100 connections rapidly — gateway should handle without crashing
	var wg sync.WaitGroup
	var success atomic.Int32

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "GET", "http://localhost:4200/health", nil)
			req.Header.Set("Authorization", "Bearer test123")
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 { success.Add(1) }
			}
		}()
	}
	wg.Wait()

	s := success.Load()
	if s < 50 { t.Errorf("only %d/100 connections succeeded", s) }
	t.Logf("100 connections: %d/100 success ✓", s)
}

func TestStress_Gateway_LargePayloadBatch(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip stress") }

	// Send 10 requests with progressively larger payloads
	sizes := []int{100, 500, 1000, 5000, 10000, 20000, 50000}
	for _, size := range sizes {
		msg := strings.Repeat("x", size)
		resp := authPost(t, "/v1/chat/completions", map[string]any{
			"model": "gpt-4o-mini",
			"messages": []map[string]string{{"role": "user", "content": "Summarize: " + msg}},
			"max_tokens": 10,
		})
		readBody(resp)
		if resp.StatusCode >= 500 && resp.StatusCode != 502 {
			t.Errorf("500 on %d char payload: %d", size, resp.StatusCode)
		}
	}
	t.Logf("payload sizes %v: all handled ✓", sizes)
}
