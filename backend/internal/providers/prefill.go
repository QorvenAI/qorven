// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import "strings"

// PrefillContinuation handles thinking-only responses.
// When a model returns reasoning but no visible text content,
// the assistant message is appended as prefill so the model
// can continue and produce the text portion on the next call.
// Up to maxAttempts prefill retries before giving up.

const MaxPrefillAttempts = 2

// NeedsPrefillContinuation returns true if the response has reasoning
// but no visible text — meaning the model needs another turn to produce output.
func NeedsPrefillContinuation(resp *ChatResponse) bool {
	if resp == nil {
		return false
	}
	hasThinking := resp.Thinking != ""
	hasContent := strings.TrimSpace(resp.Content) != ""
	hasToolCalls := len(resp.ToolCalls) > 0
	return hasThinking && !hasContent && !hasToolCalls
}

// BuildPrefillMessage creates an assistant message from a thinking-only response
// that can be appended to the conversation for continuation.
func BuildPrefillMessage(resp *ChatResponse) Message {
	return Message{
		Role:     "assistant",
		Thinking: resp.Thinking,
		Content:  "", // empty — model will fill this on next turn
	}
}

// StripThinkingSignatures removes thinking/redacted_thinking blocks from
// all assistant messages EXCEPT the last one. This prevents Anthropic's
// "Invalid signature in thinking block" HTTP 400 error that occurs when
// context compression, session truncation, or message merging invalidates
// the signature on older thinking blocks.
func StripThinkingSignatures(messages []Message) []Message {
	// Find the last assistant message index
	lastAssistant := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			lastAssistant = i
			break
		}
	}

	out := make([]Message, len(messages))
	for i, m := range messages {
		out[i] = m
		if m.Role == "assistant" && i != lastAssistant && m.Thinking != "" {
			out[i].Thinking = "" // strip stale signature
		}
	}
	return out
}

