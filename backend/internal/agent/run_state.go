// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"github.com/qorvenai/qorven/internal/providers"
)

// runState encapsulates all mutable state for a single Loop.Run execution.
// Grouping these fields enables cleaner code and prevents passing 20+ variables.
type runState struct {
	// Loop control
	iteration      int
	totalToolCalls int

	// Output accumulators
	finalContent  string
	finalThinking string
	toolsUsed     []string
	pendingMsgs   []providers.Message

	// Token tracking
	totalUsage providers.Usage

	// Event state
	blockReplies   int
	lastBlockReply string

	// Compaction state
	midLoopCompacted   bool
	overheadTokens     int  // non-history token overhead
	overheadCalibrated bool

	// Truncation retry
	truncationRetries int

	// Loop detection
	loopKilled bool

	// Budget nudges sent
	budgetNudge75Sent bool
	budgetNudge90Sent bool

	// Web tool budgets
	webSearchCalls int
	webFetchCalls  int

	// Consecutive tool-only iterations (no text output)
	consecutiveToolIters int
}

// newRunState creates a fresh run state.
func newRunState() *runState {
	return &runState{}
}

// addUsage accumulates token usage from an LLM response.
func (rs *runState) addUsage(usage *providers.Usage) {
	if usage == nil {
		return
	}
	rs.totalUsage.PromptTokens += usage.PromptTokens
	rs.totalUsage.CompletionTokens += usage.CompletionTokens
	rs.totalUsage.TotalTokens += usage.TotalTokens
}

// addToolCall records a tool call and returns warning if budget exceeded.
func (rs *runState) addToolCall(toolName string, budget int) string {
	rs.totalToolCalls++
	rs.toolsUsed = append(rs.toolsUsed, toolName)

	if budget > 0 && rs.totalToolCalls > budget {
		return "[System] Tool call budget reached. Summarize results and respond now."
	}
	return ""
}

// checkIterationBudget returns a nudge message if budget is running low.
func (rs *runState) checkIterationBudget(current, max int) string {
	if max <= 0 {
		return ""
	}

	pct := float64(current) / float64(max)

	// 75% nudge: only if no content yet
	if pct >= 0.75 && !rs.budgetNudge75Sent && rs.finalContent == "" {
		rs.budgetNudge75Sent = true
		return "[System] You have used 75% of your iteration budget without providing a text response. Start summarizing your findings and respond to the user within the next few iterations."
	}

	// 90% nudge
	if pct >= 0.90 && !rs.budgetNudge90Sent {
		rs.budgetNudge90Sent = true
		return "[System] You have used 90% of your iteration budget. Wrap up now and provide your final response."
	}

	return ""
}

// trackWebTool tracks web_search and web_fetch calls for budget enforcement.
// Returns an error message if budget exceeded.
func (rs *runState) trackWebTool(toolName string) string {
	switch toolName {
	case "web_search":
		rs.webSearchCalls++
		if rs.webSearchCalls > 2 {
			return "Web search budget exhausted (2/2). Summarize your findings and answer from the search results you have."
		}
	case "web_fetch":
		rs.webFetchCalls++
		if rs.webFetchCalls > 1 {
			return "Web fetch budget exhausted (1/1). Use the content you already fetched to answer."
		}
	}
	return ""
}

// RunResult is the output of a completed agent run.
// Extended with fields from source for better observability.
type RunResultExt struct {
	Content        string           `json:"content"`
	Thinking       string           `json:"thinking,omitempty"`
	RunID          string           `json:"runId"`
	Iterations     int              `json:"iterations"`
	ToolsUsed      []string         `json:"toolsUsed,omitempty"`
	Usage          *providers.Usage `json:"usage,omitempty"`
	InputTokens    int              `json:"inputTokens"`
	OutputTokens   int              `json:"outputTokens"`
	BlockReplies   int              `json:"blockReplies,omitempty"`
	LastBlockReply string           `json:"lastBlockReply,omitempty"`
	LoopKilled     bool             `json:"loopKilled,omitempty"`
	Sources        []string         `json:"sources,omitempty"`
	Parts          []MessagePart    `json:"parts,omitempty"`
	Metadata       map[string]any   `json:"metadata,omitempty"`
}

// toResult converts runState to RunResult.
func (rs *runState) toResult(runID string) *RunResultExt {
	return &RunResultExt{
		Content:        rs.finalContent,
		Thinking:       rs.finalThinking,
		RunID:          runID,
		Iterations:     rs.iteration,
		ToolsUsed:      rs.toolsUsed,
		Usage:          &rs.totalUsage,
		InputTokens:    rs.totalUsage.PromptTokens,
		OutputTokens:   rs.totalUsage.CompletionTokens,
		BlockReplies:   rs.blockReplies,
		LastBlockReply: rs.lastBlockReply,
		LoopKilled:     rs.loopKilled,
	}
}
