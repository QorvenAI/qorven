// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/session"
)

// PrimeDigest generates a live digest of all agent activity and writes it
// into Prime's ScopePrime memory. This makes Prime genuinely omniscient —
// it knows what every agent is doing, their recent completions, and active blockers
// without needing to ask anyone.
//
// Runs every digestInterval (default 5 min) as a background goroutine.
// Replaces the previous summary in Prime's memory rather than appending,
// so it stays current without growing unbounded.
type PrimeDigest struct {
	agentStore   *Store
	sessionStore *session.Store
	hierMem      *memory.HierarchyStore
	primeID      string
	interval     time.Duration

	mu     sync.Mutex
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewPrimeDigest creates a digest runner.
// primeID is Prime's agent UUID. interval defaults to 5 minutes.
func NewPrimeDigest(agentStore *Store, sessionStore *session.Store, hierMem *memory.HierarchyStore, primeID string, interval time.Duration) *PrimeDigest {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &PrimeDigest{
		agentStore:   agentStore,
		sessionStore: sessionStore,
		hierMem:      hierMem,
		primeID:      primeID,
		interval:     interval,
		stopCh:       make(chan struct{}),
	}
}

// Start begins the background digest loop.
func (pd *PrimeDigest) Start(ctx context.Context) {
	pd.wg.Add(1)
	go func() {
		defer pd.wg.Done()
		ticker := time.NewTicker(pd.interval)
		defer ticker.Stop()

		// Run immediately on start so Prime has context right away
		pd.run(ctx)

		for {
			select {
			case <-pd.stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				pd.run(ctx)
			}
		}
	}()
	slog.Info("prime_digest.started", "interval", pd.interval, "prime", pd.primeID[:8])
}

// Stop shuts down the digest loop.
func (pd *PrimeDigest) Stop() {
	close(pd.stopCh)
	pd.wg.Wait()
}

// run generates and saves a fresh digest snapshot.
func (pd *PrimeDigest) run(ctx context.Context) {
	if pd.agentStore == nil || pd.hierMem == nil {
		return
	}

	agents, err := pd.agentStore.List(ctx, "")
	if err != nil || len(agents) == 0 {
		return
	}

	digest := pd.buildDigest(ctx, agents)
	if digest == "" {
		return
	}

	// Save to Prime's ScopePrime memory — replaces previous digest
	// We use a special source tag so we can find and overwrite it
	_, err = pd.hierMem.SavePrime(ctx, digest, "prime_digest")
	if err != nil {
		slog.Warn("prime_digest.save_failed", "error", err)
		return
	}

	slog.Debug("prime_digest.updated", "agents", len(agents))
}

// buildDigest constructs the digest string from all agent states.
func (pd *PrimeDigest) buildDigest(ctx context.Context, agents []*Agent) string {
	now := time.Now()
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Agent Status Digest — %s\n\n", now.Format("Mon Jan 2, 15:04")))

	activeCount := 0
	idleCount := 0
	errorCount := 0

	for _, ag := range agents {
		if ag.ID == pd.primeID {
			continue // Skip Prime itself
		}
		if ag.AgentKey == "chief" || strings.HasPrefix(ag.AgentKey, "__") {
			continue // Skip internal agents
		}

		// Get recent session activity
		recentSessions, _ := pd.sessionStore.ListForAgent(ctx, ag.ID, 2)

		status := "idle"
		if ag.Status == "error" || ag.Status == "crashed" {
			status = "error"
			errorCount++
		} else if len(recentSessions) > 0 {
			lastSess := recentSessions[0]
			if time.Since(lastSess.UpdatedAt) < 10*time.Minute {
				status = "active"
				activeCount++
			} else {
				idleCount++
			}
		} else {
			idleCount++
		}

		name := ag.DisplayName
		if name == "" {
			name = ag.AgentKey
		}

		icon := "🟢"
		switch status {
		case "idle":
			icon = "⚪"
		case "error":
			icon = "🔴"
		}

		sb.WriteString(fmt.Sprintf("### %s %s (%s)\n", icon, name, status))
		sb.WriteString(fmt.Sprintf("- Model: %s\n", ag.Model))

		if len(recentSessions) > 0 {
			sess := recentSessions[0]
			age := time.Since(sess.UpdatedAt)
			ageStr := formatAge(age)

			label := sess.Label
			if label == "" {
				label = "Untitled"
			}

			sb.WriteString(fmt.Sprintf("- Last active: %s ago — \"%s\" (channel: %s)\n", ageStr, label, sess.Channel))
			if sess.Summary != "" {
				summary := sess.Summary
				if len(summary) > 200 {
					summary = summary[:200] + "…"
				}
				sb.WriteString(fmt.Sprintf("- Last summary: %s\n", summary))
			}
		} else {
			sb.WriteString("- No recent activity\n")
		}
		sb.WriteString("\n")
	}

	// Summary line at top for quick scanning
	header := fmt.Sprintf("**Status**: %d active, %d idle, %d errors | %d total agents\n\n",
		activeCount, idleCount, errorCount, len(agents)-1) // -1 for Prime
	return header + sb.String()
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}
