// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package rules implements the background rule execution engine.
// Rules are stored in the agent_rules table by Prime via set_rule.
//
// Trigger types:
//
//	cron      — fires on a 5-field cron schedule (UTC)
//	threshold — fires when FireEvent is called with a matching metric that
//	            crosses the configured operator/value
//	event     — fires when FireEvent is called with a matching event name
//
// Action types:
//
//	escalate  — wake Prime with a message so the user sees an alert in chat
//	notify    — send a UI notification via rtHub.BroadcastNotification
//	run_tool  — wake the rule's agent with an instruction to call a specific tool
package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── External interfaces ──────────────────────────────────────────────────────

// AgentWaker wakes a persistent agent runtime with a message.
// Satisfied by *agent.RuntimeManager.WakeAgent (via adapter).
type AgentWaker interface {
	WakeWithMessage(agentID, message string) bool
}

// Notifier sends a UI notification to all connected clients.
// Satisfied by *realtime.Hub.BroadcastNotification (via adapter).
type Notifier interface {
	BroadcastNotification(title, body string)
}

// AgentLookup resolves "chief" / "coder" key to an agent UUID.
type AgentLookup interface {
	// IDByKey returns the agent UUID for the given key, e.g. "chief".
	// Returns "" if not found.
	IDByKey(ctx context.Context, key string) string
}

// ─── Rule model ───────────────────────────────────────────────────────────────

type rule struct {
	id          string
	agentID     string
	description string
	triggerType string          // "cron" | "threshold" | "event"
	triggerSpec json.RawMessage // provider-specific spec
	actionType  string          // "run_tool" | "escalate" | "notify"
	actionSpec  json.RawMessage
	enabled     bool
}

// ─── Engine ───────────────────────────────────────────────────────────────────

// Engine polls agent_rules, fires due cron rules, and exposes FireEvent for
// threshold/event rules.
type Engine struct {
	db       *pgxpool.Pool
	tenantID string
	waker    AgentWaker
	notifier Notifier
	lookup   AgentLookup

	mu         sync.RWMutex
	rules      []rule
	cronNext   map[string]time.Time // ruleID → next fire time
	loadedAt   time.Time

	pollTick time.Duration // how often to re-read rules from DB (default 60s)
}

// New creates an Engine. Call Start to begin background processing.
func New(db *pgxpool.Pool, tenantID string, waker AgentWaker, notifier Notifier, lookup AgentLookup) *Engine {
	return &Engine{
		db:       db,
		tenantID: tenantID,
		waker:    waker,
		notifier: notifier,
		lookup:   lookup,
		cronNext: make(map[string]time.Time),
		pollTick: 60 * time.Second,
	}
}

// Start launches the engine background loops. ctx cancellation stops them.
func (e *Engine) Start(ctx context.Context) {
	if e.db == nil {
		slog.Info("rule_engine.disabled", "reason", "no database")
		return
	}
	// Load rules immediately so cron timers are set on startup.
	if err := e.loadRules(ctx); err != nil {
		slog.Warn("rule_engine.initial_load_failed", "error", err)
	}
	go e.runCronLoop(ctx)
	go e.runReloader(ctx)
	slog.Info("rule_engine.started", "rules", len(e.rules))
}

// FireEvent evaluates all enabled threshold and event rules against the given
// event name and data map. Matching rules are fired immediately.
//
// For threshold rules: data["metric"] and data["value"] must be present.
// For event rules: event name must match trigger_spec["event"].
func (e *Engine) FireEvent(ctx context.Context, eventName string, data map[string]any) {
	e.mu.RLock()
	snapshot := make([]rule, len(e.rules))
	copy(snapshot, e.rules)
	e.mu.RUnlock()

	for _, r := range snapshot {
		if !r.enabled {
			continue
		}
		switch r.triggerType {
		case "threshold":
			if e.matchesThreshold(r, data) {
				go e.fire(ctx, r, eventName, data)
			}
		case "event":
			if e.matchesEvent(r, eventName) {
				go e.fire(ctx, r, eventName, data)
			}
		}
	}
}

// ─── Background loops ─────────────────────────────────────────────────────────

func (e *Engine) runCronLoop(ctx context.Context) {
	// Tick every minute — minimum cron granularity is 1 minute.
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			e.fireDueCronRules(ctx, now)
		}
	}
}

func (e *Engine) runReloader(ctx context.Context) {
	ticker := time.NewTicker(e.pollTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := e.loadRules(ctx); err != nil {
				slog.Warn("rule_engine.reload_failed", "error", err)
			}
		}
	}
}

func (e *Engine) fireDueCronRules(ctx context.Context, now time.Time) {
	e.mu.RLock()
	snapshot := make([]rule, len(e.rules))
	copy(snapshot, e.rules)
	e.mu.RUnlock()

	for _, r := range snapshot {
		if !r.enabled || r.triggerType != "cron" {
			continue
		}
		var spec struct {
			Cron string `json:"cron"`
		}
		if err := json.Unmarshal(r.triggerSpec, &spec); err != nil || spec.Cron == "" {
			continue
		}

		e.mu.Lock()
		next, scheduled := e.cronNext[r.id]
		if !scheduled {
			// First time we see this rule — compute initial next-fire.
			t, err := nextCronTime(spec.Cron, now)
			if err != nil {
				slog.Warn("rule_engine.cron.parse_failed", "rule", r.id, "expr", spec.Cron, "error", err)
				e.mu.Unlock()
				continue
			}
			e.cronNext[r.id] = t
			e.mu.Unlock()
			continue
		}
		if now.Before(next) {
			e.mu.Unlock()
			continue
		}
		// Due — advance to next occurrence before releasing the lock so
		// concurrent ticks don't double-fire.
		t, _ := nextCronTime(spec.Cron, now)
		e.cronNext[r.id] = t
		e.mu.Unlock()

		slog.Info("rule_engine.cron.firing", "rule", r.id, "description", r.description, "next", t)
		go e.fire(ctx, r, "cron", map[string]any{"cron": spec.Cron})
	}
}

// ─── Rule loading ─────────────────────────────────────────────────────────────

func (e *Engine) loadRules(ctx context.Context) error {
	rows, err := e.db.Query(ctx,
		`SELECT id, agent_id, description, trigger_type, trigger_spec, action_type, action_spec, enabled
		 FROM agent_rules
		 WHERE tenant_id = $1
		 ORDER BY created_at`,
		e.tenantID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var loaded []rule
	for rows.Next() {
		var r rule
		if err = rows.Scan(&r.id, &r.agentID, &r.description,
			&r.triggerType, &r.triggerSpec,
			&r.actionType, &r.actionSpec,
			&r.enabled); err != nil {
			continue
		}
		loaded = append(loaded, r)
	}

	e.mu.Lock()
	// Preserve cronNext entries for rules that still exist and are still enabled.
	existing := make(map[string]bool, len(loaded))
	for _, r := range loaded {
		existing[r.id] = true
	}
	for id := range e.cronNext {
		if !existing[id] {
			delete(e.cronNext, id)
		}
	}
	e.rules = loaded
	e.loadedAt = time.Now()
	e.mu.Unlock()

	slog.Info("rule_engine.reloaded", "count", len(loaded))
	return nil
}

// ─── Action execution ─────────────────────────────────────────────────────────

func (e *Engine) fire(ctx context.Context, r rule, eventName string, data map[string]any) {
	slog.Info("rule_engine.fire", "rule", r.id, "action", r.actionType, "description", r.description)

	switch r.actionType {
	case "escalate":
		e.doEscalate(ctx, r, eventName, data)
	case "notify":
		e.doNotify(r, eventName, data)
	case "run_tool":
		e.doRunTool(ctx, r, eventName, data)
	default:
		slog.Warn("rule_engine.unknown_action", "action", r.actionType, "rule", r.id)
	}
}

func (e *Engine) doEscalate(ctx context.Context, r rule, eventName string, data map[string]any) {
	// Parse action_spec for optional message template.
	var spec struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(r.actionSpec, &spec)

	msg := spec.Message
	if msg == "" {
		msg = fmt.Sprintf("Automation rule triggered: %s (event: %s)", r.description, eventName)
	}
	msg = interpolateData(msg, data)

	// Wake Prime so the alert appears in the user's chat.
	primeID := e.lookup.IDByKey(ctx, "chief")
	if primeID == "" {
		slog.Warn("rule_engine.escalate.no_prime", "rule", r.id)
		return
	}
	prompt := fmt.Sprintf("[RULE ALERT] %s\n\nAlert the user immediately with a brief, clear message.", msg)
	if !e.waker.WakeWithMessage(primeID, prompt) {
		slog.Warn("rule_engine.escalate.wake_failed", "rule", r.id, "prime", primeID)
	}
}

func (e *Engine) doNotify(r rule, eventName string, data map[string]any) {
	var spec struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	_ = json.Unmarshal(r.actionSpec, &spec)

	title := spec.Title
	if title == "" {
		title = "Automation Rule"
	}
	body := spec.Body
	if body == "" {
		body = r.description
	}
	title = interpolateData(title, data)
	body = interpolateData(body, data)

	if e.notifier != nil {
		e.notifier.BroadcastNotification(title, body)
	}
	slog.Info("rule_engine.notified", "rule", r.id, "title", title)
}

func (e *Engine) doRunTool(ctx context.Context, r rule, eventName string, data map[string]any) {
	var spec struct {
		Tool    string            `json:"tool"`
		Params  map[string]any    `json:"params,omitempty"`
		Message string            `json:"message"`
	}
	_ = json.Unmarshal(r.actionSpec, &spec)

	// Determine which agent to wake: rule's own agent or "chief" as fallback.
	agentID := r.agentID
	if agentID == "" {
		agentID = e.lookup.IDByKey(ctx, "chief")
	}
	if agentID == "" {
		slog.Warn("rule_engine.run_tool.no_agent", "rule", r.id)
		return
	}

	msg := spec.Message
	if msg == "" && spec.Tool != "" {
		paramsJSON, _ := json.Marshal(spec.Params)
		msg = fmt.Sprintf("[RULE ACTION] Run tool: %s\nParams: %s\nContext: %s",
			spec.Tool, paramsJSON, r.description)
	}
	if msg == "" {
		msg = fmt.Sprintf("[RULE ACTION] %s (triggered by %s)", r.description, eventName)
	}
	msg = interpolateData(msg, data)

	if !e.waker.WakeWithMessage(agentID, msg) {
		slog.Warn("rule_engine.run_tool.wake_failed", "rule", r.id, "agent", agentID)
	}
}

// ─── Trigger matching ─────────────────────────────────────────────────────────

func (e *Engine) matchesThreshold(r rule, data map[string]any) bool {
	var spec struct {
		Metric   string  `json:"metric"`
		Operator string  `json:"operator"` // "gt" | "lt" | "gte" | "lte" | "eq"
		Value    float64 `json:"value"`
	}
	if err := json.Unmarshal(r.triggerSpec, &spec); err != nil {
		return false
	}
	if spec.Metric == "" {
		return false
	}

	// Extract the metric value from the event data.
	raw, ok := data[spec.Metric]
	if !ok {
		return false
	}
	var actual float64
	switch v := raw.(type) {
	case float64:
		actual = v
	case int:
		actual = float64(v)
	case int64:
		actual = float64(v)
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return false
		}
		actual = f
	default:
		return false
	}

	switch spec.Operator {
	case "gt", ">":
		return actual > spec.Value
	case "lt", "<":
		return actual < spec.Value
	case "gte", ">=":
		return actual >= spec.Value
	case "lte", "<=":
		return actual <= spec.Value
	case "eq", "==", "=":
		return actual == spec.Value
	}
	return false
}

func (e *Engine) matchesEvent(r rule, eventName string) bool {
	var spec struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(r.triggerSpec, &spec); err != nil {
		return false
	}
	return spec.Event != "" && strings.EqualFold(spec.Event, eventName)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// interpolateData replaces {key} placeholders in template with values from data.
func interpolateData(template string, data map[string]any) string {
	for k, v := range data {
		template = strings.ReplaceAll(template, "{"+k+"}", fmt.Sprintf("%v", v))
	}
	return template
}

// ─── Cron parser ─────────────────────────────────────────────────────────────
// Minimal 5-field cron parser (minute hour dom month dow) in UTC.
// Supports: * (wildcard), N (exact), */N (step), N-M (range).
// Does NOT support @yearly/@monthly/etc aliases.

// nextCronTime returns the next time at or after `from` that matches expr.
func nextCronTime(expr string, from time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}
	minuteSpec, hourSpec, domSpec, monthSpec, dowSpec := fields[0], fields[1], fields[2], fields[3], fields[4]

	// Advance by at least 1 minute to avoid re-firing in the same minute.
	t := from.UTC().Truncate(time.Minute).Add(time.Minute)

	// Search up to 4 years ahead (catches leap-year edge cases).
	limit := t.Add(4 * 365 * 24 * time.Hour)
	for t.Before(limit) {
		// Check month (1-12)
		if !matchField(monthSpec, int(t.Month()), 1, 12) {
			// Skip to next month
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
			continue
		}
		// Check day-of-month (1-31)
		if !matchField(domSpec, t.Day(), 1, 31) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, time.UTC)
			continue
		}
		// Check day-of-week (0=Sun, 6=Sat)
		if !matchField(dowSpec, int(t.Weekday()), 0, 6) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, time.UTC)
			continue
		}
		// Check hour (0-23)
		if !matchField(hourSpec, t.Hour(), 0, 23) {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, time.UTC)
			continue
		}
		// Check minute (0-59)
		if !matchField(minuteSpec, t.Minute(), 0, 59) {
			t = t.Add(time.Minute)
			continue
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cron: no match within 4 years for %q", expr)
}

// matchField returns true if value matches the cron field spec.
func matchField(spec string, value, min, max int) bool {
	if spec == "*" {
		return true
	}
	// Step: */N
	if strings.HasPrefix(spec, "*/") {
		n, err := strconv.Atoi(spec[2:])
		if err != nil || n <= 0 {
			return false
		}
		return (value-min)%n == 0
	}
	// Range: N-M
	if idx := strings.Index(spec, "-"); idx >= 0 {
		lo, err1 := strconv.Atoi(spec[:idx])
		hi, err2 := strconv.Atoi(spec[idx+1:])
		if err1 != nil || err2 != nil {
			return false
		}
		return value >= lo && value <= hi
	}
	// List: N,M,K
	for _, part := range strings.Split(spec, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && n == value {
			return true
		}
	}
	return false
}
