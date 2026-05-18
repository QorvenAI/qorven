// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package memory

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Dreamer runs background memory consolidation between sessions.
// Like REM sleep, it processes raw memories into structured knowledge:
// - Extracts patterns across conversations
// - Merges duplicate/similar memories
// - Builds digests for faster retrieval
// - Decays stale memories
type Dreamer struct {
	store     *Store
	tenantID  string
	interval  time.Duration
	mu        sync.Mutex
	running   bool
	lastRun   time.Time
	stats     DreamStats
}

type DreamStats struct {
	TotalRuns         int
	MemoriesMerged    int
	DigestsCreated    int
	MemoriesDecayed   int
	PromotedSession    int
	PromotedDiscussion int
	LastRunDuration   time.Duration
}

func NewDreamer(store *Store, tenantID string, interval time.Duration) *Dreamer {
	if interval <= 0 { interval = 30 * time.Minute }
	return &Dreamer{store: store, tenantID: tenantID, interval: interval}
}

// Start begins the dreaming loop. Runs consolidation at the configured interval.
func (d *Dreamer) Start(ctx context.Context) {
	d.mu.Lock()
	if d.running { d.mu.Unlock(); return }
	d.running = true
	d.mu.Unlock()

	slog.Info("dreamer.start", "tenant", d.tenantID, "interval", d.interval)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.mu.Lock()
			d.running = false
			d.mu.Unlock()
			slog.Info("dreamer.stop", "tenant", d.tenantID)
			return
		case <-ticker.C:
			d.consolidate(ctx)
		}
	}
}

// RunOnce performs a single consolidation pass (for testing or manual trigger).
func (d *Dreamer) RunOnce(ctx context.Context) DreamStats {
	return d.consolidate(ctx)
}

func (d *Dreamer) consolidate(ctx context.Context) DreamStats {
	start := time.Now()
	stats := DreamStats{}

	// Decay old memories
	decayed := d.decayMemories(ctx)
	stats.MemoriesDecayed = decayed

	// Merge duplicate memories
	merged := d.mergeDuplicates(ctx)
	stats.MemoriesMerged = merged

	// Build digests
	digests := d.buildDigests(ctx)
	stats.DigestsCreated = digests

	// Promote session memories to discussion scope
	promotedSession := d.promoteSessionToDiscussion(ctx)
	stats.PromotedSession = promotedSession

	// Promote discussion memories to agent scope
	promotedDiscussion := d.promoteDiscussionToAgent(ctx)
	stats.PromotedDiscussion = promotedDiscussion

	stats.LastRunDuration = time.Since(start)
	stats.TotalRuns = 1

	d.mu.Lock()
	d.stats.TotalRuns++
	d.stats.MemoriesMerged += merged
	d.stats.DigestsCreated += digests
	d.stats.MemoriesDecayed += decayed
	d.stats.PromotedSession += promotedSession
	d.stats.PromotedDiscussion += promotedDiscussion
	d.stats.LastRunDuration = stats.LastRunDuration
	d.lastRun = time.Now()
	d.mu.Unlock()

	slog.Info("dreamer.consolidated",
		"decayed", decayed, "merged", merged, "digests", digests,
		"promoted_session", promotedSession, "promoted_discussion", promotedDiscussion,
		"duration", stats.LastRunDuration.Round(time.Millisecond))

	return stats
}

// decayMemories reduces importance of old, rarely-accessed memories.
func (d *Dreamer) decayMemories(ctx context.Context) int {
	// Get all agents for this tenant
	rows, err := d.store.pool.Query(ctx,
		`SELECT DISTINCT agent_id FROM memories WHERE tenant_id = $1`, d.tenantID)
	if err != nil { return 0 }
	defer rows.Close()

	total := 0
	for rows.Next() {
		var agentID string
		rows.Scan(&agentID)
		n, _ := d.store.Decay(ctx, agentID)
		total += n
	}
	return total
}

// mergeDuplicates finds and merges memories with very similar content.
func (d *Dreamer) mergeDuplicates(ctx context.Context) int {
	// Find pairs of memories with high text similarity (same agent, same type)
	rows, err := d.store.pool.Query(ctx, `
		WITH pairs AS (
			SELECT a.id AS id_a, b.id AS id_b,
			       similarity(a.content, b.content) AS sim
			FROM memories a
			JOIN memories b ON a.agent_id = b.agent_id
			     AND a.memory_type = b.memory_type
			     AND a.id < b.id
			WHERE a.tenant_id = $1
			  AND similarity(a.content, b.content) > 0.8
			  AND a.decay_exempt = false AND b.decay_exempt = false
			LIMIT 50
		)
		SELECT id_a, id_b, sim FROM pairs ORDER BY sim DESC`,
		d.tenantID)
	if err != nil {
		slog.Debug("dreamer.merge.skip", "error", err)
		return 0
	}
	defer rows.Close()

	merged := 0
	for rows.Next() {
		var idA, idB string
		var sim float64
		rows.Scan(&idA, &idB, &sim)

		// Keep the one with higher importance, delete the other
		_, err := d.store.pool.Exec(ctx, `
			WITH keep AS (
				SELECT id FROM memories WHERE id IN ($1, $2) ORDER BY importance DESC, access_count DESC LIMIT 1
			)
			DELETE FROM memories WHERE id IN ($1, $2) AND id NOT IN (SELECT id FROM keep)`,
			idA, idB)
		if err == nil { merged++ }
	}
	return merged
}

// buildDigests compiles raw memories into structured digests per agent.
func (d *Dreamer) buildDigests(ctx context.Context) int {
	rows, err := d.store.pool.Query(ctx,
		`SELECT DISTINCT agent_id FROM memories WHERE tenant_id = $1`, d.tenantID)
	if err != nil { return 0 }
	defer rows.Close()

	created := 0
	for rows.Next() {
		var agentID string
		rows.Scan(&agentID)
		if d.buildAgentDigest(ctx, agentID) { created++ }
	}
	return created
}

func (d *Dreamer) buildAgentDigest(ctx context.Context, agentID string) bool {
	// Get recent high-importance memories
	rows, err := d.store.pool.Query(ctx, `
		SELECT memory_type, content, importance
		FROM memories
		WHERE agent_id = $1 AND importance >= 0.5
		ORDER BY importance DESC, created_at DESC
		LIMIT 20`, agentID)
	if err != nil { return false }
	defer rows.Close()

	var sections = map[string][]string{}
	for rows.Next() {
		var memType, content string
		var importance float64
		rows.Scan(&memType, &content, &importance)
		sections[memType] = append(sections[memType], content)
	}

	if len(sections) == 0 { return false }

	// Build digest
	var digest strings.Builder
	digest.WriteString(fmt.Sprintf("# Memory Digest for %s\n", agentID[:8]))
	digest.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format(time.RFC3339)))

	for memType, items := range sections {
		digest.WriteString(fmt.Sprintf("## %s\n", memType))
		for _, item := range items {
			preview := item
			if len(preview) > 200 { preview = preview[:200] + "..." }
			digest.WriteString(fmt.Sprintf("- %s\n", preview))
		}
		digest.WriteString("\n")
	}

	// Save as bulletin
	_, err = d.store.pool.Exec(ctx, `
		INSERT INTO memory_bulletins (agent_id, content, created_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (agent_id) DO UPDATE SET content = $2, created_at = NOW()`,
		agentID, digest.String())

	return err == nil
}

// Stats returns the cumulative dreaming statistics.
func (d *Dreamer) Stats() DreamStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stats
}

// promoteSessionToDiscussion promotes session-scoped memories to their discussion cluster
// for sessions that have been inactive for at least 1 hour.
func (d *Dreamer) promoteSessionToDiscussion(ctx context.Context) int {
	rows, err := d.store.pool.Query(ctx,
		`SELECT m.id, m.content, m.agent_id, s.discussion_id::text
		 FROM memories m
		 JOIN sessions s ON m.memory_type = 'session:' || s.id::text
		 WHERE m.tenant_id = $1
		   AND s.discussion_id IS NOT NULL
		   AND s.updated_at < now() - interval '1 hour'
		   AND m.memory_type NOT LIKE 'discussion:%'
		 LIMIT 100`,
		d.tenantID,
	)
	if err != nil {
		slog.Warn("dreamer.promote_session: query failed", "error", err)
		return 0
	}
	defer rows.Close()
	promoted := 0
	for rows.Next() {
		var memID, content, agentID, discussionID string
		if err := rows.Scan(&memID, &content, &agentID, &discussionID); err != nil {
			continue
		}
		_, _ = d.store.Save(ctx, d.tenantID, Memory{
			AgentID:    agentID,
			Type:       "discussion:" + discussionID,
			Content:    content,
			Source:     "promoted_from_session",
			Importance: 0.7,
		})
		promoted++
	}
	if promoted > 0 {
		slog.Info("dreamer.promoted_session_to_discussion", "count", promoted)
	}
	return promoted
}

// promoteDiscussionToAgent promotes discussion-scoped memories to agent scope
// for discussions that have been inactive for at least 7 days.
func (d *Dreamer) promoteDiscussionToAgent(ctx context.Context) int {
	rows, err := d.store.pool.Query(ctx,
		`SELECT m.id, m.content, m.agent_id
		 FROM memories m
		 JOIN discussions disc ON m.memory_type = 'discussion:' || disc.id::text
		 WHERE m.tenant_id = $1
		   AND disc.last_active_at < now() - interval '7 days'
		   AND m.memory_type NOT LIKE 'agent:%'
		 LIMIT 100`,
		d.tenantID,
	)
	if err != nil {
		slog.Warn("dreamer.promote_discussion: query failed", "error", err)
		return 0
	}
	defer rows.Close()
	promoted := 0
	for rows.Next() {
		var memID, content, agentID string
		if err := rows.Scan(&memID, &content, &agentID); err != nil {
			continue
		}
		_, _ = d.store.Save(ctx, d.tenantID, Memory{
			AgentID:    agentID,
			Type:       "agent",
			Content:    content,
			Source:     "promoted_from_discussion",
			Importance: 0.75,
		})
		promoted++
	}
	if promoted > 0 {
		slog.Info("dreamer.promoted_discussion_to_agent", "count", promoted)
	}
	return promoted
}
