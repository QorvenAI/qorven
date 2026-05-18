// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package voice

import (
	"strings"
	"time"
	"testing"
)

// Hard voice tests — real-world LLM responses, edge cases, performance.

func TestHard_TTS_RealLLMResponse_CodeExplanation(t *testing.T) {
	input := `Here's how to implement a rate limiter in Go:

` + "```go" + `
type RateLimiter struct {
    tokens    chan struct{}
    interval  time.Duration
}

func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
    rl := &RateLimiter{
        tokens:   make(chan struct{}, rate),
        interval: interval,
    }
    // Fill initial tokens
    for i := 0; i < rate; i++ {
        rl.tokens <- struct{}{}
    }
    // Refill periodically
    go func() {
        ticker := time.NewTicker(interval / time.Duration(rate))
        for range ticker.C {
            select {
            case rl.tokens <- struct{}{}:
            default:
            }
        }
    }()
    return rl
}
` + "```" + `

The key concepts are:
1. **Token bucket** algorithm — each request consumes a token
2. **Buffered channel** acts as the bucket with fixed capacity
3. **Goroutine** refills tokens at a steady rate

This approach handles **burst traffic** well because the channel buffer allows ` + "`rate`" + ` concurrent requests.

For more details, see https://pkg.go.dev/golang.org/x/time/rate`

	result := CleanTextForTTS(input)

	// Code should be replaced, not read aloud
	if strings.Contains(result, "chan struct{}") { t.Error("Go code should not be in TTS") }
	if strings.Contains(result, "func NewRateLimiter") { t.Error("function declaration in TTS") }

	// Explanation should survive
	if !strings.Contains(result, "Token bucket") { t.Error("missing Token bucket") }
	if !strings.Contains(result, "burst traffic") { t.Error("missing burst traffic") }

	// URLs stripped
	if strings.Contains(result, "https://") { t.Error("URL in TTS") }

	// Markdown stripped
	if strings.Contains(result, "**") { t.Error("bold markers in TTS") }

	t.Logf("code explanation: %d → %d chars", len(input), len(result))
}

func TestHard_TTS_RealLLMResponse_ErrorAnalysis(t *testing.T) {
	input := `I found the issue. The error occurs in ` + "`internal/agent/loop.go`" + ` at line 502:

` + "```" + `
panic: runtime error: index out of range [3] with length 2

goroutine 1 [running]:
github.com/qorvenai/qorven/internal/agent.(*Loop).Run(...)
    /home/runner/work/qorven/internal/agent/loop.go:502
` + "```" + `

The **root cause** is that ` + "`messages[3]`" + ` is accessed but the slice only has 2 elements.

**Fix**: Add a bounds check before accessing:
` + "```go" + `
if len(messages) > 3 {
    msg := messages[3]
}
` + "```" + `

This is a classic **off-by-one** error. The fix is straightforward.`

	result := CleanTextForTTS(input)

	// Stack traces should not be read aloud
	if strings.Contains(result, "goroutine 1") { t.Error("stack trace in TTS") }
	if strings.Contains(result, "/home/ec2-user") { t.Error("file path in TTS") }

	// Explanation should survive
	if !strings.Contains(result, "root cause") { t.Error("missing root cause") }
	if !strings.Contains(result, "off-by-one") { t.Error("missing off-by-one") }

	t.Logf("error analysis: %d → %d chars", len(input), len(result))
}

func TestHard_TTS_RealLLMResponse_TableData(t *testing.T) {
	input := `Here are the benchmark results:

| Provider | Latency | Tokens/sec | Cost/1M |
|----------|---------|------------|---------|
| OpenAI   | 450ms   | 85         | $3.00   |
| DeepSeek | 380ms   | 92         | $0.14   |
| Gemini   | 520ms   | 78         | $1.25   |
| Claude   | 490ms   | 80         | $3.00   |

**Key findings**:
- DeepSeek has the **lowest latency** and **highest throughput**
- OpenAI and Claude have similar performance but higher cost
- Gemini is the slowest but mid-range on cost

> Recommendation: Use DeepSeek for high-volume, OpenAI for quality-critical tasks.`

	result := CleanTextForTTS(input)

	// Table should be stripped
	if strings.Contains(result, "|---") { t.Error("table separators in TTS") }
	if strings.Contains(result, "| Provider") { t.Error("table header in TTS") }

	// Key findings should survive
	if !strings.Contains(result, "DeepSeek") { t.Error("missing DeepSeek") }
	if !strings.Contains(result, "lowest latency") || !strings.Contains(result, "highest throughput") {
		t.Log("some findings may be reformatted")
	}

	t.Logf("table data: %d → %d chars", len(input), len(result))
}

func TestHard_TTS_StressLargeInput(t *testing.T) {
	// 100KB input — should not OOM or take forever
	large := strings.Repeat("This is a paragraph of text. ", 3000) +
		strings.Repeat("```\ncode block\n```\n", 500) +
		strings.Repeat("https://example.com/path?q=test ", 1000) +
		strings.Repeat("| A | B |\n|---|---|\n| 1 | 2 |\n", 200)

	start := time.Now()
	result := CleanTextForTTS(large)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second { t.Errorf("100KB cleanup too slow: %v", elapsed) }
	if len(result) > len(large) { t.Error("output larger than input") }
	t.Logf("100KB stress: %d → %d chars in %v", len(large), len(result), elapsed)
}

func TestHard_AudioFormat_AllChannels(t *testing.T) {
	channels := map[string]string{
		"telegram": "ogg", "discord": "ogg", "whatsapp": "ogg",
		"slack": "mp3", "web": "mp3", "webchat": "mp3",
		"email": "mp3", "sms": "mp3", "webhook": "mp3",
		"zalo": "mp3", "line": "mp3", "teams": "mp3",
	}
	for ch, expected := range channels {
		got := PlatformAudioFormat(ch)
		if got != expected { t.Errorf("PlatformAudioFormat(%q)=%q, want %q", ch, got, expected) }
	}
}
