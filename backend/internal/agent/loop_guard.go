// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// LoopGuard detects when the agent is stuck in repetitive tool call patterns.
// Three detection mechanisms:
//  1. Same-args-same-result: identical tool+args+result repeated N times
//  2. Same-result: same tool returning identical results with different args
//  3. Read-only streak: N consecutive non-mutating tool calls without writes
//
type LoopGuard struct {
	history         []toolCallRecord
	maxHistory      int
	readOnlyStreak  int
	totalErrors     int
	lastToolName    string
	lastArgsHash    string
	consecutiveSame int
	perToolErrors   map[string]int // per-tool consecutive error count

	// Configurable thresholds
	SameArgsWarnAt     int // warn after N identical calls (default 3)
	SameArgsCriticalAt int // force-break after N identical calls (default 5)
	SameResultWarnAt   int // warn after N same-result calls (default 4)
	SameResultCritAt   int // force-break after N same-result calls (default 6)
	ReadOnlyWarnAt     int // warn after N consecutive reads (default 8)
	ReadOnlyCriticalAt int // force-break after N consecutive reads (default 12)
	ErrorCircuitBreak  int // force-break after N total errors (default 10)
}

type toolCallRecord struct {
	toolName   string
	argsHash   string
	resultHash string
}

// DetectionLevel indicates severity of a detected loop.
type DetectionLevel string

const (
	DetectionNone     DetectionLevel = ""
	DetectionWarning  DetectionLevel = "warning"
	DetectionCritical DetectionLevel = "critical"
)

// DetectionResult holds the outcome of a loop check.
type DetectionResult struct {
	Level   DetectionLevel
	Message string
}

// NewLoopGuard creates a guard with sensible defaults.
func NewLoopGuard() *LoopGuard {
	return &LoopGuard{
		maxHistory:         30,
		SameArgsWarnAt:     3,
		SameArgsCriticalAt: 5,
		SameResultWarnAt:   4,
		SameResultCritAt:   6,
		ReadOnlyWarnAt:     8,
		ReadOnlyCriticalAt: 12,
		ErrorCircuitBreak:  10,
	}
}

// RecordCall records a tool call and returns the args hash for later result recording.
func (g *LoopGuard) RecordCall(toolName string, args map[string]any) string {
	h := hashToolCall(toolName, args)

	g.history = append(g.history, toolCallRecord{
		toolName: toolName,
		argsHash: h,
	})

	// Trim to max history (sliding window)
	if len(g.history) > g.maxHistory {
		g.history = g.history[len(g.history)-g.maxHistory:]
	}

	return h
}

// RecordResult updates the latest matching record with the result hash.
func (g *LoopGuard) RecordResult(toolName, argsHash, result string) {
	rh := hashResult(result)
	// Walk backwards to find the matching call
	for i := len(g.history) - 1; i >= 0; i-- {
		rec := &g.history[i]
		if rec.toolName == toolName && rec.argsHash == argsHash && rec.resultHash == "" {
			rec.resultHash = rh
			break
		}
	}
}

// RecordError increments the global error counter.
func (g *LoopGuard) RecordError() {
	g.totalErrors++
}

// RecordToolError tracks consecutive errors for a specific tool.
func (g *LoopGuard) RecordToolError(toolName string) {
	if g.perToolErrors == nil { g.perToolErrors = make(map[string]int) }
	g.perToolErrors[toolName]++
	g.totalErrors++
}

// RecordToolSuccess resets the per-tool error counter.
func (g *LoopGuard) RecordToolSuccess(toolName string) {
	if g.perToolErrors != nil { delete(g.perToolErrors, toolName) }
}

// IsToolCircuitBroken returns true if a tool has failed too many times consecutively.
func (g *LoopGuard) IsToolCircuitBroken(toolName string) bool {
	if g.perToolErrors == nil { return false }
	return g.perToolErrors[toolName] >= 2 // break after 2 consecutive failures
}

// RecordMutation resets or increments the read-only streak.
// Mutating tools reset the streak; read-only tools increment it.
func (g *LoopGuard) RecordMutation(toolName string) {
	if isMutatingTool(toolName) {
		g.readOnlyStreak = 0
	} else if !isNeutralTool(toolName) {
		g.readOnlyStreak++
	}
	// Neutral tools (exec, bash, mcp_*) don't affect the streak
}

// DetectSameArgs checks for no-progress loops (same tool + same args repeated).
func (g *LoopGuard) DetectSameArgs(toolName, argsHash string) DetectionResult {
	if toolName == g.lastToolName && argsHash == g.lastArgsHash {
		g.consecutiveSame++
	} else {
		g.consecutiveSame = 1
		g.lastToolName = toolName
		g.lastArgsHash = argsHash
	}

	if g.consecutiveSame >= g.SameArgsCriticalAt {
		return DetectionResult{
			Level: DetectionCritical,
			Message: fmt.Sprintf(
				"CRITICAL: %s has been called %d times with identical arguments and results. "+
					"Stopping to prevent runaway loop.",
				toolName, g.consecutiveSame),
		}
	}

	if g.consecutiveSame >= g.SameArgsWarnAt {
		return DetectionResult{
			Level: DetectionWarning,
			Message: fmt.Sprintf(
				"[System: WARNING — %s has been called %d times with the same arguments and identical results. "+
					"This is not making progress. Try a completely different approach, use different tools, "+
					"or respond directly to the user with what you know.]",
				toolName, g.consecutiveSame),
		}
	}

	return DetectionResult{}
}

// DetectSameResult checks for same tool returning identical results with different args.
func (g *LoopGuard) DetectSameResult(toolName, resultHash string) DetectionResult {
	if resultHash == "" {
		return DetectionResult{}
	}

	count := 0
	for i := len(g.history) - 1; i >= 0 && i >= len(g.history)-g.maxHistory; i-- {
		rec := g.history[i]
		if rec.toolName == toolName && rec.resultHash == resultHash {
			count++
		}
	}

	if count >= g.SameResultCritAt {
		return DetectionResult{
			Level: DetectionCritical,
			Message: fmt.Sprintf(
				"CRITICAL: %s keeps returning the same result despite different arguments (%d times). "+
					"The approach is not working. Try a completely different strategy.",
				toolName, count),
		}
	}

	if count >= g.SameResultWarnAt {
		return DetectionResult{
			Level: DetectionWarning,
			Message: fmt.Sprintf(
				"[System: WARNING — %s has returned the same result %d times with different arguments. "+
					"Consider trying a different tool or approach.]",
				toolName, count),
		}
	}

	return DetectionResult{}
}

// DetectReadOnlyStreak checks for unproductive read-only loops.
func (g *LoopGuard) DetectReadOnlyStreak() DetectionResult {
	if g.readOnlyStreak >= g.ReadOnlyCriticalAt {
		return DetectionResult{
			Level:   DetectionCritical,
			Message: fmt.Sprintf("CRITICAL: %d consecutive read-only tool calls without any writes. Stopping.", g.readOnlyStreak),
		}
	}

	if g.readOnlyStreak >= g.ReadOnlyWarnAt {
		return DetectionResult{
			Level: DetectionWarning,
			Message: fmt.Sprintf(
				"[System: WARNING — You have made %d consecutive read-only tool calls without writing or editing any file. "+
					"If you already have the information you need, use the edit or write_file tool to take action.]",
				g.readOnlyStreak),
		}
	}

	return DetectionResult{}
}

// DetectErrorCircuitBreak checks if total errors exceed the circuit breaker threshold.
func (g *LoopGuard) DetectErrorCircuitBreak() DetectionResult {
	if g.totalErrors >= g.ErrorCircuitBreak {
		return DetectionResult{
			Level:   DetectionCritical,
			Message: fmt.Sprintf("CRITICAL: %d total tool errors encountered. Circuit breaker triggered.", g.totalErrors),
		}
	}
	return DetectionResult{}
}

// Reset clears all state (e.g., on new session).
func (g *LoopGuard) Reset() {
	g.history = g.history[:0]
	g.readOnlyStreak = 0
	g.totalErrors = 0
	g.lastToolName = ""
	g.lastArgsHash = ""
	g.consecutiveSame = 0
}

// --- Hashing ---

func hashToolCall(name string, args map[string]any) string {
	// Sort keys for deterministic hashing
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	h.Write([]byte(name))
	for _, k := range keys {
		h.Write([]byte(k))
		v, _ := json.Marshal(args[k])
		h.Write(v)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func hashResult(content string) string {
	if content == "" {
		return ""
	}
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])[:16]
}

// --- Tool Classification ---

// Mutating tools reset the read-only streak.
var mutatingTools = map[string]bool{
	"write_file":   true,
	"edit":         true,
	"apply_patch":  true,
	"spawn":        true,
	"team_tasks":   true,
	"message":      true,
	"create_image": true,
	"create_video": true,
	"create_audio": true,
	"tts":          true,
	"cron":         true,
	"publish_skill": true,
	"sessions_send": true,
}

// Neutral tools don't affect the read-only streak (ambiguous — could be read or write).
var neutralTools = map[string]bool{
	"exec": true,
	"bash": true,
}

func isMutatingTool(name string) bool {
	if mutatingTools[name] {
		return true
	}
	// MCP tools are neutral
	return false
}

func isNeutralTool(name string) bool {
	if neutralTools[name] {
		return true
	}
	return strings.HasPrefix(name, "mcp_")
}
