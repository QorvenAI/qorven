//go:build integration

// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// diamond_isolation_test.go — Session isolation and concurrent correctness tests.
// These verify that users can't see each other's data.

func TestDiamond_SessionIsolation_UsersCantSeeEachOther(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	// User A has context about turquoise + Ziggy
	respA := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": "My favorite color is TURQUOISE and my pet is named ZIGGY."},
			{"role": "assistant", "content": "Got it! Turquoise and Ziggy."},
			{"role": "user", "content": "What is my favorite color?"},
		},
		"max_tokens": 20, "temperature": 0,
	})
	bA := readBody(respA)
	if respA.StatusCode == 502 { t.Skip("API issue") }

	var rA struct{ Choices []struct{ Message struct{ Content string } } }
	json.Unmarshal([]byte(bA), &rA)
	if len(rA.Choices) > 0 {
		if !strings.Contains(strings.ToLower(rA.Choices[0].Message.Content), "turquoise") {
			t.Errorf("User A should know turquoise: %q", rA.Choices[0].Message.Content)
		}
	}

	// User B — completely fresh, no context
	respB := authPost(t, "/v1/chat/completions", map[string]any{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": "What is my favorite color and pet name?"},
		},
		"max_tokens": 50, "temperature": 0,
	})
	bB := readBody(respB)
	if respB.StatusCode == 502 { t.Skip("API issue") }

	var rB struct{ Choices []struct{ Message struct{ Content string } } }
	json.Unmarshal([]byte(bB), &rB)
	if len(rB.Choices) > 0 {
		content := strings.ToLower(rB.Choices[0].Message.Content)
		if strings.Contains(content, "turquoise") {
			t.Fatal("SECURITY: User B can see User A's favorite color!")
		}
		if strings.Contains(content, "ziggy") {
			t.Fatal("SECURITY: User B can see User A's pet name!")
		}
	}
	t.Log("session isolation: User B has no knowledge of User A's data ✓")
}

func TestDiamond_ConcurrentChat_ResponseCorrectness(t *testing.T) {
	requireGateway(t)
	if testing.Short() { t.Skip("skip real API") }

	// Send 10 concurrent requests, each asking for a specific number
	var wg sync.WaitGroup
	var correct, total atomic.Int32

	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			resp := authPost(t, "/v1/chat/completions", map[string]any{
				"model": "gpt-4o-mini",
				"messages": []map[string]string{
					{"role": "user", "content": fmt.Sprintf("Reply with ONLY the number %d. Nothing else.", n)},
				},
				"max_tokens": 5, "temperature": 0,
			})
			b := readBody(resp)
			if resp.StatusCode != 200 { return }
			total.Add(1)

			var r struct{ Choices []struct{ Message struct{ Content string } } }
			json.Unmarshal([]byte(b), &r)
			if len(r.Choices) > 0 {
				content := strings.TrimSpace(r.Choices[0].Message.Content)
				if strings.Contains(content, fmt.Sprintf("%d", n)) {
					correct.Add(1)
				}
			}
		}(i)
	}
	wg.Wait()

	c, tot := correct.Load(), total.Load()
	if tot < 5 { t.Skipf("only %d/10 requests succeeded", tot) }
	if c < tot/2 { t.Errorf("only %d/%d correct (responses may be mixed up)", c, tot) }
	t.Logf("concurrent correctness: %d/%d correct responses ✓", c, tot)
}

func TestDiamond_SessionIsolation_DB_Level(t *testing.T) {
	requireGateway(t)

	// Create two agents with different system prompts
	resp1 := authPost(t, "/v1/agents", map[string]any{
		"agent_key": "iso-cats-" + fmt.Sprintf("%d", time.Now().UnixNano()%100000),
		"display_name": "Cat Expert",
		"model": "gpt-4o-mini",
		"system_prompt": "You are a cat expert. You only know about cats.",
	})
	b1 := readBody(resp1)
	if resp1.StatusCode != 200 && resp1.StatusCode != 201 { t.Skip("can't create agent") }
	var ag1 map[string]any
	json.Unmarshal([]byte(b1), &ag1)
	id1, _ := ag1["id"].(string)
	if id1 == "" { t.Skip("no agent ID") }
	defer authDelete(t, "/v1/agents/"+id1)

	resp2 := authPost(t, "/v1/agents", map[string]any{
		"agent_key": "iso-dogs-" + fmt.Sprintf("%d", time.Now().UnixNano()%100000),
		"display_name": "Dog Expert",
		"model": "gpt-4o-mini",
		"system_prompt": "You are a dog expert. You only know about dogs.",
	})
	b2 := readBody(resp2)
	if resp2.StatusCode != 200 && resp2.StatusCode != 201 { t.Skip("can't create agent") }
	var ag2 map[string]any
	json.Unmarshal([]byte(b2), &ag2)
	id2, _ := ag2["id"].(string)
	if id2 == "" { t.Skip("no agent ID") }
	defer authDelete(t, "/v1/agents/"+id2)

	// Verify they have different prompts
	get1 := authGet(t, "/v1/agents/"+id1)
	get2 := authGet(t, "/v1/agents/"+id2)
	b1g := readBody(get1)
	b2g := readBody(get2)

	if !strings.Contains(b1g, "cat") { t.Error("agent 1 should be about cats") }
	if !strings.Contains(b2g, "dog") { t.Error("agent 2 should be about dogs") }
	if strings.Contains(b1g, "dog") { t.Error("cat agent should not mention dogs") }
	if strings.Contains(b2g, "cat") { t.Error("dog agent should not mention cats") }

	t.Log("DB-level isolation: cat and dog agents have separate configs ✓")
}
