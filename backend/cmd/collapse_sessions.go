// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/qorvenai/qorven/internal/config"
)

// collapseSessions runs the one-shot Phase-4 data migration for the
// "one Qor = one chat" refactor. For every Qor that ended up with
// multiple chat-family sessions (channel in web / webchat / tui /
// telegram / whatsapp / slack_dm / discord_dm), it:
//
//  1. Picks the most-recently-updated row as the canonical session.
//  2. For each older chat-family session belonging to the same Qor,
//     appends its messages onto the canonical row (preserving their
//     original channel tag so per-transport surfaces keep rendering
//     correctly), emits a single memory fact summarising the
//     archived slice so the Qor can still recall it on demand, and
//     marks the old row status = 'archived'.
//  3. Non-chat-family sessions (email, cron, voice, group chats,
//     self_build) are left untouched — those are genuinely different
//     threads per the plan.
//
// The migration is idempotent: archived rows are skipped on re-run,
// and already-merged messages aren't duplicated (we dedupe by
// timestamp+role+content before appending).
//
// Dry-run mode prints what would change without touching the DB.

var (
	collapseDryRun bool
	collapseYes    bool
)

var collapseSessionsCmd = &cobra.Command{
	Use:   "collapse-sessions",
	Short: "Collapse per-Qor chat-family sessions into one canonical chat (Phase 4 migration)",
	Long: `Collapses chat-family sessions per Qor into a single canonical chat row.

Use this once after upgrading past the one-Qor-one-chat refactor.
Subsequent chat-family writes already route to the canonical session,
so this only needs to run against databases with historical fan-out.

Archives old rows rather than deleting them; drop the archive later
once you're confident nothing depends on their ids.`,
	Example: `  qorven collapse-sessions --dry-run
  qorven collapse-sessions --yes`,
	RunE: runCollapseSessions,
}

func init() {
	collapseSessionsCmd.Flags().BoolVar(&collapseDryRun, "dry-run", false, "print what would change without writing")
	collapseSessionsCmd.Flags().BoolVar(&collapseYes, "yes", false, "skip interactive confirmation")
	rootCmd.AddCommand(collapseSessionsCmd)
}

// chatFamily lists the 1:1 chat channels that collapse into one
// canonical row. Keep in sync with gateway.isChatFamilyChannel.
var chatFamily = map[string]struct{}{
	"web":        {},
	"webchat":    {},
	"tui":        {},
	"telegram":   {},
	"whatsapp":   {},
	"slack_dm":   {},
	"discord_dm": {},
}

type sessRow struct {
	ID        string
	AgentID   string
	TenantID  string
	Channel   string
	UpdatedAt time.Time
	Messages  []byte
}

func runCollapseSessions(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Database.DSN == "" {
		cfg.Database.DSN = os.Getenv("QORVEN_POSTGRES_DSN")
	}
	if cfg.Database.DSN == "" {
		return fmt.Errorf("no database DSN — set QORVEN_POSTGRES_DSN or configure database.dsn")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.Database.DSN)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	if !collapseDryRun && !collapseYes {
		fmt.Print("This will archive old chat-family sessions and merge them into one per Qor. Continue? [y/N] ")
		var resp string
		fmt.Scanln(&resp)
		resp = strings.ToLower(strings.TrimSpace(resp))
		if resp != "y" && resp != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Pull every active chat-family session, ordered so the first row
	// per agent is the newest — that one becomes canonical.
	rows, err := pool.Query(ctx,
		`SELECT id, agent_id, tenant_id, COALESCE(channel,'web'), updated_at, messages
		 FROM sessions
		 WHERE (status = 'active' OR status IS NULL OR status = '')
		   AND COALESCE(channel,'web') = ANY($1)
		 ORDER BY agent_id, updated_at DESC`,
		chatFamilyKeys(),
	)
	if err != nil {
		return fmt.Errorf("query sessions: %w", err)
	}

	byAgent := map[string][]sessRow{}
	for rows.Next() {
		var r sessRow
		if err := rows.Scan(&r.ID, &r.AgentID, &r.TenantID, &r.Channel, &r.UpdatedAt, &r.Messages); err != nil {
			rows.Close()
			return fmt.Errorf("scan: %w", err)
		}
		byAgent[r.AgentID] = append(byAgent[r.AgentID], r)
	}
	rows.Close()

	var (
		agentsTouched   int
		sessionsArchived int
		messagesMerged  int
		memoriesEmitted int
	)

	for agentID, list := range byAgent {
		if len(list) < 2 {
			continue // already one chat; nothing to do
		}
		agentsTouched++
		canonical := list[0]
		others := list[1:]

		canonicalMsgs, err := parseMessages(canonical.Messages)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip agent %s: parse canonical messages: %v\n", agentID, err)
			continue
		}
		// Build a signature set to dedupe on re-run.
		seen := map[string]struct{}{}
		for _, m := range canonicalMsgs {
			seen[msgSig(m)] = struct{}{}
		}

		agentMerged := 0
		for _, old := range others {
			oldMsgs, err := parseMessages(old.Messages)
			if err != nil {
				fmt.Fprintf(os.Stderr, "skip session %s: parse messages: %v\n", old.ID, err)
				continue
			}
			for _, m := range oldMsgs {
				// Preserve the transport channel — the canonical
				// row is "web" by convention, but each merged
				// message keeps its own channel for per-surface
				// filtering. If the old session had channel set
				// and the message didn't, inherit from session.
				if _, hasChan := m["channel"]; !hasChan {
					m["channel"] = old.Channel
				}
				sig := msgSigMap(m)
				if _, dup := seen[sig]; dup {
					continue
				}
				seen[sig] = struct{}{}
				canonicalMsgs = append(canonicalMsgs, m)
				agentMerged++
			}
			sessionsArchived++
		}
		messagesMerged += agentMerged

		// Chronological sort after the merge so the UI renders them
		// in real time order regardless of which source row they
		// came from. Timestamp is stored under "timestamp" as a
		// Unix millisecond int.
		sortByTimestamp(canonicalMsgs)

		// Emit one memory per archived session summarising its slice
		// so the Qor can recall it by topic even though the raw row
		// is archived. We keep the summary short — it's an index
		// entry, not a full transcript.
		summaries := []string{}
		for _, old := range others {
			s, count := summarise(old)
			if count == 0 {
				continue
			}
			summaries = append(summaries, s)
			memoriesEmitted++
		}

		if collapseDryRun {
			fmt.Printf("[dry-run] agent=%s canonical=%s merge=%d archive=%d memories=%d\n",
				agentID[:8], canonical.ID[:8], agentMerged, len(others), len(summaries))
			continue
		}

		newMsgs, err := json.Marshal(canonicalMsgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip agent %s: marshal messages: %v\n", agentID, err)
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for agent %s: %w", agentID, err)
		}
		_, err = tx.Exec(ctx,
			`UPDATE sessions SET messages = $2, channel = 'web', updated_at = NOW() WHERE id = $1`,
			canonical.ID, newMsgs,
		)
		if err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("update canonical %s: %w", canonical.ID, err)
		}
		for _, old := range others {
			_, err = tx.Exec(ctx,
				`UPDATE sessions SET status = 'archived', updated_at = NOW() WHERE id = $1`,
				old.ID,
			)
			if err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("archive %s: %w", old.ID, err)
			}
		}
		for _, summary := range summaries {
			// memory_type must be one of the enum values on memories
			// (fact/preference/decision/identity/event/observation/
			// goal/todo). "event" fits best — each digest records
			// that a chat happened + its gist.
			_, err = tx.Exec(ctx,
				`INSERT INTO memories (tenant_id, agent_id, memory_type, content, source, source_type, importance, decay_exempt, tags)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				canonical.TenantID, agentID,
				"event",
				summary,
				"session_collapse",
				"compaction",
				0.5, false,
				[]string{"conversation_digest", "phase4_collapse"},
			)
			if err != nil {
				tx.Rollback(ctx)
				return fmt.Errorf("insert memory for %s: %w", agentID, err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit tx for %s: %w", agentID, err)
		}
	}

	verb := "would"
	if !collapseDryRun {
		verb = "did"
	}
	fmt.Printf("\nSummary: %s touch %d agent(s), archive %d session(s), merge %d message(s), emit %d memory fact(s).\n",
		verb, agentsTouched, sessionsArchived, messagesMerged, memoriesEmitted)
	if collapseDryRun {
		fmt.Println("Re-run without --dry-run to apply.")
	}
	return nil
}

func chatFamilyKeys() []string {
	out := make([]string, 0, len(chatFamily))
	for k := range chatFamily {
		out = append(out, k)
	}
	return out
}

// parseMessages accepts either [] (empty) or a JSON array of message
// objects. We keep them as map[string]any so we don't lose fields we
// don't model here (tool_calls, widgets, metadata) during round-trip.
func parseMessages(raw []byte) ([]map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func msgSig(m map[string]any) string { return msgSigMap(m) }

func msgSigMap(m map[string]any) string {
	role, _ := m["role"].(string)
	content, _ := m["content"].(string)
	ts := ""
	switch v := m["timestamp"].(type) {
	case float64:
		ts = fmt.Sprintf("%d", int64(v))
	case int64:
		ts = fmt.Sprintf("%d", v)
	case string:
		ts = v
	}
	return role + "|" + ts + "|" + content
}

func sortByTimestamp(msgs []map[string]any) {
	// Simple in-place insertion sort — message arrays are small
	// enough that allocating a slice of indices would cost more.
	for i := 1; i < len(msgs); i++ {
		for j := i; j > 0 && tsOf(msgs[j]) < tsOf(msgs[j-1]); j-- {
			msgs[j], msgs[j-1] = msgs[j-1], msgs[j]
		}
	}
}

func tsOf(m map[string]any) int64 {
	switch v := m["timestamp"].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	}
	return 0
}

// summarise produces a short memory-fact line describing an archived
// session so the Qor can still recall what was in it.
func summarise(s sessRow) (string, int) {
	msgs, err := parseMessages(s.Messages)
	if err != nil || len(msgs) == 0 {
		return "", 0
	}
	// Collect snippets: first + last user-role content + last
	// assistant reply — enough to scent the conversation.
	var first, lastUser, lastAsst string
	for _, m := range msgs {
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		if content == "" {
			continue
		}
		if first == "" && role == "user" {
			first = content
		}
		switch role {
		case "user":
			lastUser = content
		case "assistant":
			lastAsst = content
		}
	}
	startDate := "unknown"
	endDate := "unknown"
	if t, ok := tsUnix(msgs[0]); ok {
		startDate = time.Unix(t, 0).Format("2006-01-02")
	}
	if t, ok := tsUnix(msgs[len(msgs)-1]); ok {
		endDate = time.Unix(t, 0).Format("2006-01-02")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "[archived %s chat, %s → %s, %d messages] ",
		s.Channel, startDate, endDate, len(msgs))
	if first != "" {
		fmt.Fprintf(&sb, "opened with: %s. ", truncate(first, 120))
	}
	if lastUser != "" {
		fmt.Fprintf(&sb, "last ask: %s. ", truncate(lastUser, 120))
	}
	if lastAsst != "" {
		fmt.Fprintf(&sb, "last reply: %s", truncate(lastAsst, 120))
	}
	return strings.TrimSpace(sb.String()), len(msgs)
}

func tsUnix(m map[string]any) (int64, bool) {
	t := tsOf(m)
	if t == 0 {
		return 0, false
	}
	if t > 1_000_000_000_000 {
		// Stored as milliseconds — convert to seconds.
		return t / 1000, true
	}
	return t, true
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
