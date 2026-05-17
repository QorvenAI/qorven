// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// ToolMetrics tracks per-tool execution statistics.
type ToolMetrics struct {
	mu    sync.RWMutex
	tools map[string]*ToolStat
}

// ToolStat holds stats for a single tool.
type ToolStat struct {
	Name         string        `json:"name"`
	CallCount    int64         `json:"call_count"`
	SuccessCount int64         `json:"success_count"`
	ErrorCount   int64         `json:"error_count"`
	TotalLatency time.Duration `json:"-"`
	AvgLatencyMs float64       `json:"avg_latency_ms"`
	MaxLatencyMs float64       `json:"max_latency_ms"`
	LastCallAt   time.Time     `json:"last_call_at"`
	LastError    string        `json:"last_error,omitempty"`
	SuccessRate  float64       `json:"success_rate"`
}

// NewToolMetrics creates a new metrics store.
func NewToolMetrics() *ToolMetrics {
	return &ToolMetrics{tools: make(map[string]*ToolStat)}
}

// Record records a tool execution.
func (tm *ToolMetrics) Record(name string, latency time.Duration, success bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	stat, ok := tm.tools[name]
	if !ok {
		stat = &ToolStat{Name: name}
		tm.tools[name] = stat
	}

	stat.CallCount++
	stat.TotalLatency += latency
	stat.LastCallAt = time.Now()

	if success {
		stat.SuccessCount++
	} else {
		stat.ErrorCount++
	}

	// Update computed fields
	stat.AvgLatencyMs = float64(stat.TotalLatency.Milliseconds()) / float64(stat.CallCount)
	ms := float64(latency.Milliseconds())
	if ms > stat.MaxLatencyMs {
		stat.MaxLatencyMs = ms
	}
	if stat.CallCount > 0 {
		stat.SuccessRate = float64(stat.SuccessCount) / float64(stat.CallCount)
	}
}

// RecordError records a tool error with the error message.
func (tm *ToolMetrics) RecordError(name string, latency time.Duration, errMsg string) {
	tm.Record(name, latency, false)
	tm.mu.Lock()
	if stat, ok := tm.tools[name]; ok {
		stat.LastError = errMsg
	}
	tm.mu.Unlock()
}

// Get returns stats for a specific tool.
func (tm *ToolMetrics) Get(name string) *ToolStat {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if stat, ok := tm.tools[name]; ok {
		copy := *stat
		return &copy
	}
	return nil
}

// List returns stats for all tools, sorted by call count descending.
func (tm *ToolMetrics) List() []ToolStat {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	stats := make([]ToolStat, 0, len(tm.tools))
	for _, s := range tm.tools {
		stats = append(stats, *s)
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].CallCount > stats[j].CallCount
	})
	return stats
}

// Summary returns a JSON-serializable summary.
func (tm *ToolMetrics) Summary() map[string]any {
	stats := tm.List()
	var totalCalls, totalErrors int64
	for _, s := range stats {
		totalCalls += s.CallCount
		totalErrors += s.ErrorCount
	}
	return map[string]any{
		"total_calls":  totalCalls,
		"total_errors": totalErrors,
		"tool_count":   len(stats),
		"tools":        stats,
	}
}

// JSON returns the metrics as JSON bytes.
func (tm *ToolMetrics) JSON() []byte {
	data, _ := json.Marshal(tm.Summary())
	return data
}

// Reset clears all metrics.
func (tm *ToolMetrics) Reset() {
	tm.mu.Lock()
	tm.tools = make(map[string]*ToolStat)
	tm.mu.Unlock()
}
