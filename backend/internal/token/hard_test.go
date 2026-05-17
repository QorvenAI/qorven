// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package token

import (
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/providers"
)

// hard_test.go — Token counting accuracy tests.
// Wrong token count = wrong billing = lost money or overcharged users.

func TestHard_Estimate_EmptyMessages(t *testing.T) {
	c := &Counter{}
	if c.Estimate(nil) != 0 { t.Error("nil should be 0") }
	if c.Estimate([]providers.Message{}) != 0 { t.Error("empty should be 0") }
}

func TestHard_Estimate_SingleMessage(t *testing.T) {
	c := &Counter{}
	// "Hello world" = 11 chars → ~3 tokens + 4 overhead = 6-7
	msgs := []providers.Message{{Role: "user", Content: "Hello world"}}
	tokens := c.Estimate(msgs)
	if tokens < 5 || tokens > 10 { t.Errorf("'Hello world' should be ~6 tokens, got %d", tokens) }
}

func TestHard_Estimate_LargeMessage(t *testing.T) {
	c := &Counter{}
	// 4000 chars → ~1000 tokens + 4 overhead
	content := strings.Repeat("x", 4000)
	msgs := []providers.Message{{Role: "user", Content: content}}
	tokens := c.Estimate(msgs)
	if tokens < 900 || tokens > 1100 { t.Errorf("4000 chars should be ~1004 tokens, got %d", tokens) }
}

func TestHard_Estimate_MultipleMessages(t *testing.T) {
	c := &Counter{}
	msgs := []providers.Message{
		{Role: "system", Content: strings.Repeat("x", 400)},  // ~104
		{Role: "user", Content: strings.Repeat("x", 200)},    // ~54
		{Role: "assistant", Content: strings.Repeat("x", 800)}, // ~204
	}
	tokens := c.Estimate(msgs)
	// Total: 1400 chars / 4 + 12 overhead = 362
	if tokens < 300 || tokens > 400 { t.Errorf("expected ~362 tokens, got %d", tokens) }
}

func TestHard_Estimate_UnicodeContent(t *testing.T) {
	c := &Counter{}
	// Unicode chars are multi-byte — token estimate should still work
	msgs := []providers.Message{{Role: "user", Content: "日本語テスト 🚀 café"}}
	tokens := c.Estimate(msgs)
	if tokens <= 0 { t.Error("unicode should produce positive token count") }
}

func TestHard_WillExceedLimit_UnderLimit(t *testing.T) {
	c := &Counter{}
	msgs := []providers.Message{{Role: "user", Content: "short"}}
	if c.WillExceedLimit(msgs, 128000) { t.Error("short message should not exceed 128K limit") }
}

func TestHard_WillExceedLimit_OverLimit(t *testing.T) {
	c := &Counter{}
	// 500K chars → ~125K tokens, which exceeds 85% of 128K (108K)
	msgs := []providers.Message{{Role: "user", Content: strings.Repeat("x", 500000)}}
	if !c.WillExceedLimit(msgs, 128000) { t.Error("500K chars should exceed 128K limit") }
}

func TestHard_WillExceedLimit_AtBoundary(t *testing.T) {
	c := &Counter{}
	// 128K * 0.85 * 4 chars/token = ~435K chars is the boundary
	boundary := int(128000 * 0.85 * 4)
	msgs := []providers.Message{{Role: "user", Content: strings.Repeat("x", boundary-100)}}
	if c.WillExceedLimit(msgs, 128000) { t.Error("just under boundary should not exceed") }

	msgs2 := []providers.Message{{Role: "user", Content: strings.Repeat("x", boundary+100)}}
	if !c.WillExceedLimit(msgs2, 128000) { t.Error("just over boundary should exceed") }
}

func TestHard_EstimateCostCents_Accuracy(t *testing.T) {
	c := &Counter{}
	// 10K tokens → 10 * 0.1 = 1 cent
	msgs := []providers.Message{{Role: "user", Content: strings.Repeat("x", 40000)}} // ~10K tokens
	cost := c.EstimateCostCents(msgs)
	if cost < 0 { t.Error("cost should never be negative") }
	if cost > 10 { t.Errorf("10K tokens should cost ~1 cent, got %d", cost) }
	t.Logf("40K chars → %d tokens → %d cents ✓", c.Estimate(msgs), cost)
}

func TestHard_EstimateCostCents_Zero(t *testing.T) {
	c := &Counter{}
	cost := c.EstimateCostCents(nil)
	if cost != 0 { t.Errorf("empty should cost 0, got %d", cost) }
}
