// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"strings"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/tools"
)

// parallelToolResult holds the output of a single parallel tool execution.
type parallelToolResult struct {
	idx       int
	tc        providers.ToolCall
	result    *tools.Result
	argsJSON  string
	startTime time.Time
	duration  time.Duration
}

// executeToolsParallel runs multiple tool calls in parallel and returns results in order.
// This is the Qorven pattern: emit all tool.call events upfront, execute in goroutines,
// collect results, sort by original index for deterministic ordering.
func (l *Loop) executeToolsParallel(
	ctx context.Context,
	req RunRequest,
	toolCalls []providers.ToolCall,
	toolCtx context.Context,
	allowedTools map[string]bool,
	loopGuard *LoopGuard,
	onEvent func(StreamEvent),
) []parallelToolResult {
	if len(toolCalls) == 0 {
		return nil
	}

	// 1. Emit all tool.call events upfront (client sees all calls starting)
	for _, tc := range toolCalls {
		onEvent(ToolStart(tc.Name))
		onEvent(PartEvent(ToolCallPart(tc.Name, tc.ID, tc.Arguments)))
	}

	// 2. Execute all tools in parallel
	resultCh := make(chan parallelToolResult, len(toolCalls))
	var wg sync.WaitGroup

	for i, tc := range toolCalls {
		wg.Add(1)
		go func(idx int, tc providers.ToolCall) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					resultCh <- parallelToolResult{
						idx:    idx,
						tc:     tc,
						result: tools.ErrorResult(fmt.Sprintf("tool %q panicked: %v", tc.Name, r)),
					}
				}
			}()

			argsJSON, _ := json.Marshal(tc.Arguments)
			startTime := time.Now()
			slog.Info("tool call", "tool", tc.Name, "args_len", len(argsJSON), "parallel", true)

			var result *tools.Result

			// Policy check
			if allowedTools != nil && !allowedTools[tc.Name] {
				slog.Warn("security.tool_policy_blocked", "tool", tc.Name)
				result = tools.ErrorResult("tool not allowed by policy: " + tc.Name)
			}

			// Execute if not blocked
			if result == nil {
				result = l.executeTool(toolCtx, req, tc.Name, tc.Arguments)
			}

			resultCh <- parallelToolResult{
				idx:       idx,
				tc:        tc,
				result:    result,
				argsJSON:  string(argsJSON),
				startTime: startTime,
				duration:  time.Since(startTime),
			}
		}(i, tc)
	}

	// Close channel after all goroutines complete
	go func() { wg.Wait(); close(resultCh) }()

	// 3. Collect results (respect context cancellation)
	collected := make([]parallelToolResult, 0, len(toolCalls))
	for range toolCalls {
		select {
		case r, ok := <-resultCh:
			if !ok {
				break
			}
			collected = append(collected, r)
		case <-ctx.Done():
			return collected // partial results on cancellation
		}
	}

	// 4. Sort by original index for deterministic ordering
	sort.Slice(collected, func(i, j int) bool {
		return collected[i].idx < collected[j].idx
	})

	return collected
}

// uniquifyToolCallIDs ensures globally unique tool call IDs.
// OpenAI-compatible APIs return 400 on duplicate IDs.
func uniquifyToolCallIDs(calls []providers.ToolCall, runID string, iteration int) []providers.ToolCall {
	seen := make(map[string]int)
	for i := range calls {
		id := calls[i].ID
		if id == "" {
			calls[i].ID = fmt.Sprintf("%s-%d-%d", runID, iteration, i)
			continue
		}
		if count := seen[id]; count > 0 {
			calls[i].ID = fmt.Sprintf("%s-%d", id, count)
		}
		seen[id]++
	}
	return calls
}

// TruncationHandler manages max_tokens truncation retry logic.
type TruncationHandler struct {
	retries    int
	maxRetries int
}

// NewTruncationHandler creates a handler with default max retries.
func NewTruncationHandler() *TruncationHandler {
	return &TruncationHandler{maxRetries: 3}
}

// Check returns true if the response was truncated and should retry.
// Returns the hint message to inject if retrying.
func (h *TruncationHandler) Check(resp *providers.ChatResponse) (shouldRetry bool, hint string) {
	if resp == nil {
		return false, ""
	}

	// Truncated: finish_reason is "length" and there are tool calls
	truncated := resp.FinishReason == "length" && len(resp.ToolCalls) > 0

	// Parse error: tool call arguments are malformed (likely truncated JSON)
	parseErr := !truncated && hasToolParseErrors(resp.ToolCalls)

	if !truncated && !parseErr {
		h.retries = 0 // reset on success
		return false, ""
	}

	h.retries++
	if h.retries >= h.maxRetries {
		slog.Warn("truncation retry limit reached", "retries", h.retries)
		return false, "" // give up
	}

	if truncated {
		return true, "[System] Your output was truncated because it exceeded max_tokens. Your tool call arguments were incomplete. Please retry with shorter content — split large writes into multiple smaller calls."
	}
	return true, "[System] One or more tool call arguments were malformed (truncated JSON). This usually means your output was too long. Please retry with shorter content."
}

func hasToolParseErrors(calls []providers.ToolCall) bool {
	// Check if any tool call has malformed arguments (empty or invalid JSON)
	for _, tc := range calls {
		if len(tc.Arguments) == 0 {
			// Empty arguments might indicate truncation
			return true
		}
	}
	return false
}

// BlockReplyEmitter handles intermediate assistant content for non-streaming channels.
type BlockReplyEmitter struct {
	count   int
	last    string
	onEvent func(StreamEvent)
}

// NewBlockReplyEmitter creates an emitter.
func NewBlockReplyEmitter(onEvent func(StreamEvent)) *BlockReplyEmitter {
	return &BlockReplyEmitter{onEvent: onEvent}
}

// Emit sends a block.reply event if content is non-empty and not silent.
func (e *BlockReplyEmitter) Emit(content string) {
	if content == "" || e.onEvent == nil {
		return
	}
	sanitized := sanitizeToolCallMarkup(content)
	if sanitized == "" || IsSilentReply(sanitized) {
		return
	}
	e.count++
	e.last = sanitized
	e.onEvent(StreamEvent{
		Type: "block.reply",
		Data: map[string]string{"content": sanitized},
	})
}

// sanitizeToolCallMarkup removes tool call markup from assistant content.
func sanitizeToolCallMarkup(content string) string {
	for _, pattern := range []string{
		"<tool_call>", "</tool_call>",
		"<function_call>", "</function_call>",
		"<|tool_call", "|>",
	} {
		content = removePattern(content, pattern)
	}
	return content
}

// IsSilentReply returns true if the content is just filler/acknowledgment.
func IsSilentReply(content string) bool {
	silent := []string{
		"I'll", "Let me", "I will", "I'm going to",
		"Searching", "Looking", "Checking",
	}
	for _, s := range silent {
		if len(content) < 50 && strings.HasPrefix(content, s) {
			return true
		}
	}
	return false
}

func removePattern(s, pattern string) string {
	for {
		idx := indexOf(s, pattern)
		if idx < 0 {
			return s
		}
		s = s[:idx] + s[idx+len(pattern):]
	}
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func contains(s, substr string) bool {
	return indexOf(s, substr) >= 0
}

// IterationBudgetNudger injects warnings when iteration budget is running low.
type IterationBudgetNudger struct {
	sent75 bool
	sent90 bool
}

// Check returns a nudge message if the agent should be warned about budget.
func (n *IterationBudgetNudger) Check(current, max int, hasContent bool) string {
	if max <= 0 {
		return ""
	}

	pct := float64(current) / float64(max)

	// 75% nudge: only if no content yet
	if pct >= 0.75 && !n.sent75 && !hasContent {
		n.sent75 = true
		return "[System] You have used 75% of your iteration budget without providing a text response. Start summarizing your findings and respond to the user within the next few iterations."
	}

	// 90% nudge: always
	if pct >= 0.90 && !n.sent90 {
		n.sent90 = true
		return "[System] You have used 90% of your iteration budget. Wrap up now and provide your final response."
	}

	return ""
}

// ToolBudgetChecker enforces per-run tool call limits.
type ToolBudgetChecker struct {
	total int
	limit int
}

// NewToolBudgetChecker creates a checker with the given limit (0 = unlimited).
func NewToolBudgetChecker(limit int) *ToolBudgetChecker {
	return &ToolBudgetChecker{limit: limit}
}

// Add records tool calls and returns a warning message if budget exceeded.
func (c *ToolBudgetChecker) Add(count int) string {
	c.total += count
	if c.limit > 0 && c.total > c.limit {
		return fmt.Sprintf("[System] Tool call budget reached (%d/%d). Do NOT call any more tools. Summarize results so far and respond to the user.", c.total, c.limit)
	}
	return ""
}
