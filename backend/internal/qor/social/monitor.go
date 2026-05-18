// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// monitor.go — Qorven Topic Monitor.
// Auto-updating topic tracking across platforms. Uses cron + memory
// to continuously monitor topics and notify agents when something changes.

// TopicMonitor tracks topics across platforms and detects changes.
type TopicMonitor struct {
	engine   *IntelligenceEngine
	mu       sync.Mutex
	monitors map[string]*MonitorConfig
	snapshots map[string]*TopicSnapshot
	onChange func(agentID, topic, diff string) // callback when topic changes
}

// MonitorConfig defines what to monitor and how often.
type MonitorConfig struct {
	ID        string     `json:"id"`
	AgentID   string     `json:"agent_id"`
	Topic     string     `json:"topic"`
	Platforms []Platform `json:"platforms"` // empty = all
	Interval  time.Duration `json:"interval"`
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"created_at"`
}

// TopicSnapshot captures the state of a topic at a point in time.
type TopicSnapshot struct {
	MonitorID   string         `json:"monitor_id"`
	Topic       string         `json:"topic"`
	TakenAt     time.Time      `json:"taken_at"`
	ResultCount int            `json:"result_count"`
	TopResults  []ScoredResult `json:"top_results"`
	ContentHash string         `json:"content_hash"` // for change detection
	Summary     string         `json:"summary"`
}

func NewTopicMonitor(engine *IntelligenceEngine, onChange func(agentID, topic, diff string)) *TopicMonitor {
	return &TopicMonitor{
		engine:    engine,
		monitors:  make(map[string]*MonitorConfig),
		snapshots: make(map[string]*TopicSnapshot),
		onChange:  onChange,
	}
}

// Add creates a new topic monitor.
func (tm *TopicMonitor) Add(agentID, topic string, platforms []Platform, interval time.Duration) string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	id := fmt.Sprintf("mon_%s_%d", agentID[:8], time.Now().UnixMilli())
	if interval <= 0 { interval = 1 * time.Hour }

	tm.monitors[id] = &MonitorConfig{
		ID: id, AgentID: agentID, Topic: topic,
		Platforms: platforms, Interval: interval,
		Active: true, CreatedAt: time.Now(),
	}
	return id
}

// Remove stops and removes a monitor.
func (tm *TopicMonitor) Remove(id string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.monitors, id)
	delete(tm.snapshots, id)
}

// List returns all monitors for an agent.
func (tm *TopicMonitor) List(agentID string) []*MonitorConfig {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	var out []*MonitorConfig
	for _, m := range tm.monitors {
		if m.AgentID == agentID { out = append(out, m) }
	}
	return out
}

// CheckAll runs all due monitors. Called by cron or ticker.
func (tm *TopicMonitor) CheckAll(ctx context.Context) int {
	tm.mu.Lock()
	configs := make([]*MonitorConfig, 0, len(tm.monitors))
	for _, m := range tm.monitors {
		if m.Active { configs = append(configs, m) }
	}
	tm.mu.Unlock()

	checked := 0
	for _, cfg := range configs {
		// Check if due
		tm.mu.Lock()
		prev, hasPrev := tm.snapshots[cfg.ID]
		tm.mu.Unlock()

		if hasPrev && time.Since(prev.TakenAt) < cfg.Interval { continue }

		snap, diff := tm.checkTopic(ctx, cfg, prev)
		if snap == nil { continue }
		checked++

		tm.mu.Lock()
		tm.snapshots[cfg.ID] = snap
		tm.mu.Unlock()

		if diff != "" && tm.onChange != nil {
			tm.onChange(cfg.AgentID, cfg.Topic, diff)
		}
	}
	return checked
}

// checkTopic searches for a topic and compares with previous snapshot.
func (tm *TopicMonitor) checkTopic(ctx context.Context, cfg *MonitorConfig, prev *TopicSnapshot) (*TopicSnapshot, string) {
	opts := SearchOpts{MaxResults: 15, TimeRange: 7 * 24 * time.Hour}

	var results []ScoredResult
	var err error
	if len(cfg.Platforms) > 0 {
		results, err = tm.engine.SearchPlatforms(ctx, cfg.Topic, cfg.Platforms, opts)
	} else {
		results, err = tm.engine.SearchAll(ctx, cfg.Topic, opts)
	}
	if err != nil {
		slog.Warn("monitor.check.error", "topic", cfg.Topic, "error", err)
		return nil, ""
	}

	// Take top 10
	top := results
	if len(top) > 10 { top = top[:10] }

	// Compute content hash for change detection
	hash := hashResults(top)

	snap := &TopicSnapshot{
		MonitorID:   cfg.ID,
		Topic:       cfg.Topic,
		TakenAt:     time.Now(),
		ResultCount: len(results),
		TopResults:  top,
		ContentHash: hash,
	}

	// Detect changes
	if prev == nil {
		snap.Summary = fmt.Sprintf("Initial scan: %d results across platforms", len(results))
		return snap, "" // first scan — no diff
	}

	if hash == prev.ContentHash {
		return snap, "" // no change
	}

	// Build diff
	diff := buildDiff(cfg.Topic, prev, snap)
	snap.Summary = diff
	return snap, diff
}

// hashResults creates a content hash from the top results for change detection.
func hashResults(results []ScoredResult) string {
	var b strings.Builder
	for _, r := range results {
		b.WriteString(r.URL)
		b.WriteString(fmt.Sprintf("%d", r.Upvotes+r.Likes))
	}
	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:8])
}

// buildDiff compares two snapshots and describes what changed.
func buildDiff(topic string, prev, curr *TopicSnapshot) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("📊 Topic Update: %q\n\n", topic))

	// Find new results (URLs not in previous)
	prevURLs := map[string]bool{}
	for _, r := range prev.TopResults { prevURLs[r.URL] = true }

	var newResults []ScoredResult
	for _, r := range curr.TopResults {
		if !prevURLs[r.URL] { newResults = append(newResults, r) }
	}

	if len(newResults) > 0 {
		b.WriteString(fmt.Sprintf("🆕 %d new results:\n", len(newResults)))
		for _, r := range newResults {
			engagement := ""
			if r.Upvotes > 0 { engagement = fmt.Sprintf(" (↑%d)", r.Upvotes) }
			if r.Likes > 0 { engagement = fmt.Sprintf(" (♥%d)", r.Likes) }
			b.WriteString(fmt.Sprintf("  • [%s] %s%s\n    %s\n", r.Platform, r.Title, engagement, r.URL))
		}
	}

	// Check for engagement changes on existing results
	prevByURL := map[string]ScoredResult{}
	for _, r := range prev.TopResults { prevByURL[r.URL] = r }

	var trending []string
	for _, r := range curr.TopResults {
		if p, ok := prevByURL[r.URL]; ok {
			upDiff := r.Upvotes - p.Upvotes
			commentDiff := r.Comments - p.Comments
			if upDiff > 50 || commentDiff > 10 {
				trending = append(trending, fmt.Sprintf("  • %s: +%d votes, +%d comments", truncate(r.Title, 60), upDiff, commentDiff))
			}
		}
	}

	if len(trending) > 0 {
		b.WriteString(fmt.Sprintf("\n📈 Trending up:\n%s\n", strings.Join(trending, "\n")))
	}

	b.WriteString(fmt.Sprintf("\nTotal: %d results (was %d)", curr.ResultCount, prev.ResultCount))
	return b.String()
}

// Start begins the monitor loop. Checks all monitors at a fixed tick rate.
func (tm *TopicMonitor) Start(ctx context.Context, tickRate time.Duration) {
	if tickRate <= 0 { tickRate = 5 * time.Minute }
	ticker := time.NewTicker(tickRate)
	defer ticker.Stop()

	slog.Info("monitor.start", "tick_rate", tickRate)
	for {
		select {
		case <-ctx.Done():
			slog.Info("monitor.stop")
			return
		case <-ticker.C:
			checked := tm.CheckAll(ctx)
			if checked > 0 { slog.Info("monitor.tick", "checked", checked) }
		}
	}
}
