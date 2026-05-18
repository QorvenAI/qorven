// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// QOROS — Always-on background agent mode.
// Inspired by Claude Code's unreleased QOROS system.
//
// Five mechanisms:
//   1. Tick Loop — periodic check-ins that keep the agent alive
//   2. SleepTool — agent-controlled pacing (yield when idle)
//   3. Blocking Budget — auto-background commands after 15s
//   4. Daily Logs — append-only memory for perpetual sessions
//   5. SendMessage — dedicated output channel for proactive updates

const (
	// DefaultTickInterval is how often the tick fires when agent is active.
	DefaultTickInterval = 30 * time.Second

	// BlockingBudgetMS is how long a command can block before auto-backgrounding.
	BlockingBudgetMS = 15000

	// PromptCacheExpiry is how long before the LLM prompt cache expires.
	// Agent should wake before this to avoid expensive cache rebuilds.
	PromptCacheExpiry = 5 * time.Minute
)

// QorosMode represents the proactive agent state.
type QorosMode struct {
	mu       sync.Mutex
	active   bool
	paused   bool
	agentID  string
	cancel   context.CancelFunc

	// Tick state
	tickInterval time.Duration
	lastTick     time.Time
	tickCount    int

	// Sleep state
	sleeping     bool
	sleepUntil   time.Time
	sleepReason  string

	// Daily log
	logDir       string

	// Callbacks
	onTick       func(ctx context.Context, tickTime time.Time) error
	onMessage    func(agentID, content, status string) // proactive → user

	// Persistence
	stateDB  QorosStateDB
	tenantID string
}

// NewQoros creates a new QOROS proactive agent mode.
func NewQoros(agentID string, onTick func(ctx context.Context, tickTime time.Time) error, onMessage func(string, string, string)) *QorosMode {
	home, _ := os.UserHomeDir()
	return &QorosMode{
		agentID:      agentID,
		tickInterval: DefaultTickInterval,
		logDir:       filepath.Join(home, ".qorven", "logs", "daily"),
		onTick:       onTick,
		onMessage:    onMessage,
	}
}

// Start begins the proactive tick loop.
func (k *QorosMode) Start(ctx context.Context) {
	k.mu.Lock()
	if k.active {
		k.mu.Unlock()
		return
	}
	k.active = true
	ctx, k.cancel = context.WithCancel(ctx)
	k.mu.Unlock()

	slog.Info("qoros.started", "agent", k.agentID, "tick_interval", k.tickInterval)

	go k.tickLoop(ctx)
}

// Stop ends the proactive mode.
func (k *QorosMode) Stop() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.active = false
	if k.cancel != nil {
		k.cancel()
	}
	slog.Info("qoros.stopped", "agent", k.agentID, "ticks", k.tickCount)
}

// Pause temporarily suspends ticks (e.g., during user input).
func (k *QorosMode) Pause()  { k.mu.Lock(); k.paused = true; k.mu.Unlock() }
func (k *QorosMode) Resume() { k.mu.Lock(); k.paused = false; k.mu.Unlock() }

// IsActive returns whether QOROS mode is running.
func (k *QorosMode) IsActive() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.active
}

// Sleep tells the agent to yield for a duration.
// Each wake-up costs an API call, but prompt cache expires after 5 min.
func (k *QorosMode) Sleep(duration time.Duration, reason string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.sleeping = true
	k.sleepUntil = time.Now().Add(duration)
	k.sleepReason = reason
	slog.Info("qoros.sleep", "agent", k.agentID, "duration", duration, "reason", reason)
}

// Wake interrupts sleep (e.g., user sent a message).
func (k *QorosMode) Wake(reason string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.sleeping {
		k.sleeping = false
		k.sleepReason = ""
		slog.Info("qoros.wake", "agent", k.agentID, "reason", reason)
	}
}

// tickLoop is the core proactive engine.
func (k *QorosMode) tickLoop(ctx context.Context) {
	ticker := time.NewTicker(k.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			k.mu.Lock()
			if !k.active || k.paused {
				k.mu.Unlock()
				continue
			}
			if k.sleeping && time.Now().Before(k.sleepUntil) {
				k.mu.Unlock()
				continue
			}
			// Wake up from sleep
			if k.sleeping {
				k.sleeping = false
				slog.Info("qoros.wake_from_sleep", "agent", k.agentID)
			}
			k.tickCount++
			k.lastTick = t
			k.mu.Unlock()

			// Fire the tick callback
			if k.onTick != nil {
				if err := k.onTick(ctx, t); err != nil {
					slog.Warn("qoros.tick_error", "agent", k.agentID, "error", err)
				}
			}

			// Persist state after every tick so restarts resume from correct position
			k.persistState(ctx)
		}
	}
}

// --- Daily Log (append-only memory) ---

// AppendDailyLog writes an entry to today's log.
// Dual-write strategy: filesystem first (fast, local), then DB (durable backup).
// If the filesystem write fails, the DB write still proceeds — entry is never lost.
// Path: ~/.qorven/logs/daily/YYYY/MM/YYYY-MM-DD.md
func (k *QorosMode) AppendDailyLog(entry string) error {
	now := time.Now()
	timestamp := now.Format("15:04:05")
	formatted := fmt.Sprintf("\n## %s\n\n%s\n", timestamp, entry)

	// Primary: filesystem (fast, local read/write during normal operation)
	fsErr := k.appendToFile(now, formatted)
	if fsErr != nil {
		slog.Warn("qoros.daily_log.fs_write_failed", "agent", k.agentID, "error", fsErr)
	}

	// Secondary: DB (survives disk wipes, queryable)
	if k.stateDB != nil {
		ctx := context.Background()
		if dbErr := k.stateDB.AppendDailyLogDB(ctx, k.agentID, k.tenantID, now, now, entry); dbErr != nil {
			slog.Warn("qoros.daily_log.db_write_failed", "agent", k.agentID, "error", dbErr)
			// Return fs error if both failed, db error if only db failed
			if fsErr != nil { return fsErr }
		}
	}

	return fsErr
}

func (k *QorosMode) appendToFile(now time.Time, content string) error {
	dir := filepath.Join(k.logDir, now.Format("2006"), now.Format("01"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	logFile := filepath.Join(dir, now.Format("2006-01-02")+".md")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// ReadDailyLog reads today's log.
// Falls back to DB if the filesystem file is missing (e.g., after disk wipe/migration).
func (k *QorosMode) ReadDailyLog() (string, error) {
	now := time.Now()
	logFile := filepath.Join(k.logDir, now.Format("2006"), now.Format("01"), now.Format("2006-01-02")+".md")
	data, err := os.ReadFile(logFile)
	if err == nil {
		return string(data), nil
	}

	// Filesystem missing — try DB fallback
	if k.stateDB != nil {
		ctx := context.Background()
		entries, dbErr := k.stateDB.ReadDailyLogDB(ctx, k.agentID, now)
		if dbErr != nil || len(entries) == 0 {
			return "", nil
		}
		// Reconstruct markdown format from DB entries
		var sb strings.Builder
		for _, e := range entries {
			sb.WriteString(e)
			sb.WriteString("\n")
		}
		content := sb.String()
		// Opportunistically restore the filesystem file so future reads are fast
		_ = k.appendToFile(now, content)
		return content, nil
	}

	return "", nil
}

// --- SendMessage (proactive output channel) ---

// SendProactive sends a proactive message to the user.
// Status: "normal" for replies, "proactive" for unsolicited updates.
func (k *QorosMode) SendProactive(content string) {
	if k.onMessage != nil {
		k.onMessage(k.agentID, content, "proactive")
	}
}

// --- SleepTool (for agent to call) ---

// SleepToolParams are the parameters for the sleep tool.
type SleepToolParams struct {
	DurationSeconds int    `json:"duration_seconds"`
	Reason          string `json:"reason,omitempty"`
}

// HandleSleepTool processes a sleep tool call from the agent.
func (k *QorosMode) HandleSleepTool(params SleepToolParams) string {
	duration := time.Duration(params.DurationSeconds) * time.Second

	// Cap at prompt cache expiry to avoid expensive rebuilds
	if duration > PromptCacheExpiry {
		duration = PromptCacheExpiry
		slog.Info("qoros.sleep_capped", "requested", params.DurationSeconds, "capped_to", duration)
	}

	k.Sleep(duration, params.Reason)
	return fmt.Sprintf("Sleeping for %s. Will wake on next tick or user message.", duration)
}

// --- Blocking Budget ---

// BlockingBudgetTimer creates a timer that auto-backgrounds a command after the budget.
// Returns a cancel function to call if the command finishes before the budget.
func BlockingBudgetTimer(onBackground func()) func() {
	timer := time.AfterFunc(time.Duration(BlockingBudgetMS)*time.Millisecond, func() {
		slog.Info("qoros.blocking_budget_exceeded", "budget_ms", BlockingBudgetMS)
		onBackground()
	})
	return func() { timer.Stop() }
}

// --- DB Persistence (Gap 3 fix: QOROS state survives restarts) ---

// QorosStateDB is a DB-backed snapshot of QorosMode state.
// Written after each tick so the engine can restore state on restart.
type QorosStateDB interface {
	// SaveQorosState persists the current state to DB.
	SaveQorosState(ctx context.Context, agentID, tenantID string, tickCount int, sleeping bool, sleepUntil *time.Time, sleepReason string, lastTickAt *time.Time) error
	// LoadQorosState loads persisted state. Returns (found, tickCount, sleeping, sleepUntil, err).
	LoadQorosState(ctx context.Context, agentID string) (found bool, tickCount int, sleeping bool, sleepUntil time.Time, err error)
	// MarkQorosActive sets the active flag for a QOROS instance.
	MarkQorosActive(ctx context.Context, agentID, tenantID string, active bool) error
	// ListActiveQoros returns agent IDs that were active before the last restart.
	ListActiveQoros(ctx context.Context, tenantID string) ([]string, error)

	// AppendDailyLogDB writes a log entry to the DB (durable backup alongside filesystem).
	AppendDailyLogDB(ctx context.Context, agentID, tenantID string, logDate time.Time, entryTime time.Time, content string) error
	// ReadDailyLogDB reads all entries for a given date from DB.
	// Used to reconstruct filesystem logs after a disk wipe.
	ReadDailyLogDB(ctx context.Context, agentID string, logDate time.Time) ([]string, error)
}

// SetDB wires a persistence backend so tick state survives restarts.
func (k *QorosMode) SetDB(db QorosStateDB, tenantID string) {
	k.mu.Lock()
	k.stateDB = db
	k.tenantID = tenantID
	k.mu.Unlock()
}

// persistState writes current state to DB after each tick.
func (k *QorosMode) persistState(ctx context.Context) {
	if k.stateDB == nil {
		return
	}
	k.mu.Lock()
	tickCount := k.tickCount
	sleeping := k.sleeping
	var sleepUntil *time.Time
	if k.sleeping {
		t := k.sleepUntil
		sleepUntil = &t
	}
	sleepReason := k.sleepReason
	now := time.Now()
	tenantID := k.tenantID
	k.mu.Unlock()

	if err := k.stateDB.SaveQorosState(ctx, k.agentID, tenantID, tickCount, sleeping, sleepUntil, sleepReason, &now); err != nil {
		slog.Warn("qoros.persist_failed", "agent", k.agentID, "error", err)
	}
}

// RestoreState loads persisted state from DB on startup.
// Call this before Start() to resume from the last known tick count and sleep state.
func (k *QorosMode) RestoreState(ctx context.Context) {
	if k.stateDB == nil {
		return
	}
	found, tickCount, sleeping, sleepUntil, err := k.stateDB.LoadQorosState(ctx, k.agentID)
	if err != nil || !found {
		return
	}
	k.mu.Lock()
	k.tickCount = tickCount
	if sleeping && time.Now().Before(sleepUntil) {
		k.sleeping = true
		k.sleepUntil = sleepUntil
		slog.Info("qoros.restored_sleeping", "agent", k.agentID, "until", sleepUntil)
	} else {
		k.sleeping = false // sleep expired during downtime — wake up
	}
	k.mu.Unlock()
	slog.Info("qoros.state_restored", "agent", k.agentID, "ticks", tickCount, "sleeping", sleeping)
}
