// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// --- sessions_list ---

type SessionsListTool struct{ pool *pgxpool.Pool }

func NewSessionsListTool(pool *pgxpool.Pool) *SessionsListTool { return &SessionsListTool{pool: pool} }
func (t *SessionsListTool) Name() string                       { return "sessions_list" }
func (t *SessionsListTool) Description() string                { return "List active sessions for this agent." }
func (t *SessionsListTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"limit": map[string]any{"type": "integer", "description": "Max sessions (default 10)"},
	}}
}

func (t *SessionsListTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.pool == nil { return ErrorResult("database not configured") }
	agentID := AgentIDFromCtx(ctx)
	limit := 10
	if n, ok := toInt(args["limit"]); ok && n > 0 { limit = n }
	if limit > 50 { limit = 50 }

	rows, err := t.pool.Query(ctx,
		`SELECT session_key, channel, label, updated_at FROM sessions
		 WHERE agent_id = $1 ORDER BY updated_at DESC LIMIT $2`, agentID, limit)
	if err != nil { return ErrorResult(fmt.Sprintf("query failed: %v", err)) }
	defer rows.Close()

	var b strings.Builder
	for rows.Next() {
		var key, channel string
		var label *string
		var updated time.Time
		rows.Scan(&key, &channel, &label, &updated)
		l := ""
		if label != nil { l = *label }
		fmt.Fprintf(&b, "- %s [%s] %s (updated: %s)\n", key, channel, l, updated.Format(time.RFC3339))
	}
	if b.Len() == 0 { return TextResult("no active sessions") }
	return TextResult(b.String())
}

// --- sessions_history ---

type SessionsHistoryTool struct{ pool *pgxpool.Pool }

func NewSessionsHistoryTool(pool *pgxpool.Pool) *SessionsHistoryTool {
	return &SessionsHistoryTool{pool: pool}
}
func (t *SessionsHistoryTool) Name() string { return "sessions_history" }
func (t *SessionsHistoryTool) Description() string {
	return "View message history for a session."
}
func (t *SessionsHistoryTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"session_key": map[string]any{"type": "string", "description": "Session key"},
		"last_n":      map[string]any{"type": "integer", "description": "Last N messages (default 20)"},
	}, "required": []string{"session_key"}}
}

func (t *SessionsHistoryTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.pool == nil { return ErrorResult("database not configured") }
	key, _ := args["session_key"].(string)
	if key == "" { return ErrorResult("session_key is required") }
	lastN := 20
	if n, ok := toInt(args["last_n"]); ok && n > 0 { lastN = n }

	var messagesJSON []byte
	err := t.pool.QueryRow(ctx, `SELECT messages FROM sessions WHERE session_key = $1`, key).Scan(&messagesJSON)
	if err != nil { return ErrorResult("session not found: " + key) }

	var messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	json.Unmarshal(messagesJSON, &messages)

	// Take last N
	if len(messages) > lastN { messages = messages[len(messages)-lastN:] }

	var b strings.Builder
	for _, m := range messages {
		content := m.Content
		if len(content) > 500 { content = content[:500] + "..." }
		fmt.Fprintf(&b, "[%s] %s\n\n", m.Role, content)
	}
	if b.Len() == 0 { return TextResult("no messages in session") }
	return TextResult(b.String())
}

// --- session_status ---

type SessionStatusTool struct{ pool *pgxpool.Pool }

func NewSessionStatusTool(pool *pgxpool.Pool) *SessionStatusTool {
	return &SessionStatusTool{pool: pool}
}
func (t *SessionStatusTool) Name() string        { return "session_status" }
func (t *SessionStatusTool) Description() string { return "Get current session status." }
func (t *SessionStatusTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *SessionStatusTool) Execute(ctx context.Context, args map[string]any) *Result {
	sessionKey := SessionIDFromCtx(ctx)
	agentID := AgentIDFromCtx(ctx)
	return TextResult(fmt.Sprintf("session: %s\nagent: %s\ntime: %s", sessionKey, agentID, time.Now().Format(time.RFC3339)))
}

// --- datetime ---

type DateTimeTool struct{}

func NewDateTimeTool() *DateTimeTool    { return &DateTimeTool{} }
func (t *DateTimeTool) Name() string    { return "datetime" }
func (t *DateTimeTool) Description() string {
	return "Get current date and time. Use before creating cron jobs or time-sensitive operations."
}
func (t *DateTimeTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"timezone": map[string]any{"type": "string", "description": "IANA timezone (default: UTC). Example: America/New_York, Asia/Kolkata"},
	}}
}

func (t *DateTimeTool) Execute(ctx context.Context, args map[string]any) *Result {
	tz, _ := args["timezone"].(string)
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil { loc = l }
	}
	now := time.Now().In(loc)
	return TextResult(fmt.Sprintf("Current time: %s\nTimezone: %s\nUnix: %d\nDay: %s",
		now.Format("2006-01-02 15:04:05 MST"), loc.String(), now.Unix(), now.Weekday()))
}

// --- cron ---

type CronTool struct{ pool *pgxpool.Pool }

func NewCronTool(pool *pgxpool.Pool) *CronTool { return &CronTool{pool: pool} }
func (t *CronTool) Name() string               { return "cron" }
func (t *CronTool) Description() string {
	return `Manage scheduled jobs. IMPORTANT: You MUST call this tool with action="create" to schedule tasks. Do NOT say "scheduled" or "done" without calling this tool first. The tool returns the real job ID — use THAT ID in your confirmation, not a made-up one. Server timezone is IST (Asia/Kolkata). Cron expressions use server time directly (e.g., 22:18 IST = "18 22 * * *").`
}
func (t *CronTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"action":     map[string]any{"type": "string", "enum": []string{"list", "create", "enable", "disable", "delete"}, "description": "Action to perform"},
		"name":       map[string]any{"type": "string", "description": "Job name (for create)"},
		"expression": map[string]any{"type": "string", "description": "Cron expression (for create). Example: 0 9 * * * (daily at 9am)"},
		"task":       map[string]any{"type": "string", "description": "The TASK to execute when the cron fires. Write as a direct imperative command for the executing Soul — NOT a description of the schedule. GOOD: 'Fetch current weather for Dubai and format as a daily report' BAD: 'Send weather report at 5pm'"},
		"executor_agent_id": map[string]any{"type": "string", "description": "Agent ID of the Soul that should execute this task. Use a specialist Soul (DevOps, Researcher, etc.) for best results. If omitted, current Soul runs it."},
		"job_id":     map[string]any{"type": "string", "description": "Job ID (for enable/disable/delete)"},
	}, "required": []string{"action"}}
}

func (t *CronTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.pool == nil { return ErrorResult("database not configured") }
	action, _ := args["action"].(string)
	agentID := AgentIDFromCtx(ctx)

	switch action {
	case "list":
		rows, err := t.pool.Query(ctx,
			`SELECT id, name, enabled, cron_expression, last_run_at FROM cron_jobs WHERE agent_id = $1 ORDER BY name`, agentID)
		if err != nil { return ErrorResult(err.Error()) }
		defer rows.Close()
		var b strings.Builder
		for rows.Next() {
			var id, name, expr string
			var enabled bool
			var lastRun *time.Time
			rows.Scan(&id, &name, &enabled, &expr, &lastRun)
			status := "disabled"
			if enabled { status = "enabled" }
			last := "never"
			if lastRun != nil { last = lastRun.Format(time.RFC3339) }
			fmt.Fprintf(&b, "- %s: %s [%s] expr=%s last=%s\n", id[:8], name, status, expr, last)
		}
		if b.Len() == 0 { return TextResult("no cron jobs") }
		return TextResult(b.String())

	case "create":
		name, _ := args["name"].(string)
		expr, _ := args["expression"].(string)
		task, _ := args["task"].(string)
		if task == "" { task, _ = args["payload"].(string) } // backward compat
		if name == "" || expr == "" { return ErrorResult("name and expression required") }
		tenantID := TenantIDFromCtx(ctx)
		payloadJSON, _ := json.Marshal(map[string]string{"instruction": task})
		var id string
		err := t.pool.QueryRow(ctx,
			`INSERT INTO cron_jobs (tenant_id, agent_id, name, cron_expression, payload, enabled)
			 VALUES ($1, $2, $3, $4, $5, true) RETURNING id`, tenantID, agentID, name, expr, payloadJSON).Scan(&id)
		if err != nil { return ErrorResult(err.Error()) }

		// Register with in-memory scheduler so the job actually fires
		if OnCronSchedule != nil {
			OnCronSchedule(ctx, tenantID, agentID, id, name, expr, task)
		}

		// Sync to calendar for GUI visibility
		if OnCronCreated != nil {
			OnCronCreated(ctx, agentID, id, name, expr, task)
		}

		return TextResult(fmt.Sprintf("created cron job %s: %s (%s)", id[:8], name, expr))

	case "enable", "disable":
		jobID, _ := args["job_id"].(string)
		if jobID == "" { return ErrorResult("job_id required") }
		enabled := action == "enable"
		t.pool.Exec(ctx, `UPDATE cron_jobs SET enabled = $1 WHERE id = $2`, enabled, jobID)
		return TextResult(fmt.Sprintf("job %s %sd", jobID[:8], action))

	case "delete":
		jobID, _ := args["job_id"].(string)
		if jobID == "" { return ErrorResult("job_id required") }
		t.pool.Exec(ctx, `DELETE FROM cron_jobs WHERE id = $1`, jobID)
		if OnCronRemove != nil {
			OnCronRemove(jobID)
		}
		return TextResult(fmt.Sprintf("deleted job %s", jobID[:8]))

	default:
		return ErrorResult("action must be: list, create, enable, disable, delete")
	}
}

// --- send_dm tool — Soul-initiated DM to user ---

// ChannelSender abstracts channel manager for the send_dm tool.
type ChannelSender interface {
	List() []map[string]any
	Send(ctx context.Context, instanceID string, msg OutboundMessage) error
}

// OutboundMessage mirrors channels.OutboundMessage to avoid import cycle.
type OutboundMessage struct {
	RecipientID string
	Content     string
}

type SendDMTool struct {
	pool    *pgxpool.Pool
	chanSnd ChannelSender // nil = web-only mode
}

func NewSendDMTool(pool *pgxpool.Pool, chanSnd ChannelSender) *SendDMTool {
	return &SendDMTool{pool: pool, chanSnd: chanSnd}
}
func (t *SendDMTool) Name() string { return "send_dm" }
func (t *SendDMTool) Description() string {
	return "Send a message to the user via web DM or Telegram. Set channel to 'telegram' to send via Telegram, or 'web' (default) for the web chat DM."
}
func (t *SendDMTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"content": map[string]any{"type": "string", "description": "The message content to send"},
		"channel": map[string]any{"type": "string", "description": "Delivery channel: 'web' (default) or 'telegram'", "enum": []string{"web", "telegram"}},
	}, "required": []string{"content"}}
}

func (t *SendDMTool) Execute(ctx context.Context, args map[string]any) *Result {
	content, _ := args["content"].(string)
	if content == "" {
		return ErrorResult("content is required")
	}
	agentID := AgentIDFromCtx(ctx)
	if agentID == "" {
		return ErrorResult("no agent context")
	}

	channel, _ := args["channel"].(string)
	if channel == "" {
		channel = "web"
	}

	fmt.Fprintf(os.Stderr, "[send_dm] channel=%s agent=%s content_len=%d\n", channel, agentID[:8], len(content))

	// Telegram delivery
	if channel == "telegram" {
		return t.sendTelegram(ctx, agentID, content)
	}

	// Web DM delivery
	return t.sendWebDM(ctx, agentID, content)
}

func (t *SendDMTool) sendTelegram(ctx context.Context, agentID, content string) *Result {
	fmt.Fprintf(os.Stderr, "[send_telegram] START agent=%s\n", agentID[:8])
	if t.chanSnd == nil {
		fmt.Fprintf(os.Stderr, "[send_telegram] chanSnd is nil!\n")
		return ErrorResult("channel manager not available")
	}

	// Find running telegram instance for this agent
	var instanceID string
	chList := t.chanSnd.List()
	slog.Info("send_telegram.find", "agent_id", agentID, "channels", len(chList))
	for _, ch := range chList {
		if fmt.Sprintf("%v", ch["agent_id"]) == agentID && ch["type"] == "telegram" && ch["running"] == true {
			instanceID, _ = ch["id"].(string)
			break
		}
	}
	if instanceID == "" {
		return ErrorResult("no running Telegram channel for this agent")
	}

	// Find paired Telegram user
	var chatID string
	qctx, qcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer qcancel()
	t.pool.QueryRow(qctx,
		`SELECT sender_id FROM paired_devices WHERE channel = 'telegram' LIMIT 1`).Scan(&chatID)
	if chatID == "" {
		t.pool.QueryRow(qctx,
			`SELECT sender_id FROM pairing_requests WHERE channel_type = 'telegram' ORDER BY created_at DESC LIMIT 1`).Scan(&chatID)
	}
	if chatID == "" {
		return ErrorResult("no paired Telegram user found. Ask the user to message the bot on Telegram first.")
	}

	slog.Info("send_telegram.sending", "instance", instanceID, "chat_id", chatID)
	// Use a fresh context with timeout — don't inherit the potentially-cancelled request context
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer sendCancel()
	err := t.chanSnd.Send(sendCtx, instanceID, OutboundMessage{RecipientID: chatID, Content: content})
	if err != nil {
		slog.Error("send_telegram.failed", "error", err)
		return ErrorResult("telegram send failed: " + err.Error())
	}
	slog.Info("send_telegram.success", "chat_id", chatID)
	return TextResult(fmt.Sprintf("Message sent to Telegram (chat: %s)", chatID))
}

func (t *SendDMTool) sendWebDM(ctx context.Context, agentID, content string) *Result {
	var sessionID string
	t.pool.QueryRow(ctx,
		`SELECT id FROM sessions WHERE agent_id = $1 AND channel = 'web' ORDER BY updated_at DESC LIMIT 1`, agentID).Scan(&sessionID)
	if sessionID == "" {
		t.pool.QueryRow(ctx,
			`INSERT INTO sessions (tenant_id, agent_id, user_id, channel) VALUES ((SELECT id FROM tenants LIMIT 1), $1, 'user', 'web') RETURNING id`,
			agentID).Scan(&sessionID)
	}
	if sessionID == "" {
		return ErrorResult("could not find or create session")
	}

	_, err := t.pool.Exec(ctx,
		`UPDATE sessions SET messages = messages || $1::jsonb, updated_at = NOW() WHERE id = $2`,
		fmt.Sprintf(`[{"role":"assistant","content":%q,"timestamp":%d}]`, content, time.Now().Unix()), sessionID)
	if err != nil {
		return ErrorResult("failed to save message: " + err.Error())
	}
	return TextResult(fmt.Sprintf("Message sent to web DM (session: %s)", sessionID[:8]))
}

// --- send_telegram tool — dedicated Telegram sender ---

type SendTelegramTool struct{ dm *SendDMTool }

func NewSendTelegramTool(dm *SendDMTool) *SendTelegramTool { return &SendTelegramTool{dm: dm} }
func (t *SendTelegramTool) Name() string { return "send_telegram" }
func (t *SendTelegramTool) Description() string {
	return "Send a message to the user's Telegram. The recipient is auto-resolved from paired devices — no chat ID needed. Just provide the message content."
}
func (t *SendTelegramTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"content": map[string]any{"type": "string", "description": "The message to send to Telegram"},
	}, "required": []string{"content"}}
}
func (t *SendTelegramTool) Execute(ctx context.Context, args map[string]any) *Result {
	args["channel"] = "telegram"
	return t.dm.Execute(ctx, args)
}

// OnCronCreated is called after a cron job is created. Set by gateway to sync to calendar.
var OnCronCreated func(ctx context.Context, agentID, jobID, name, expression, task string)

// OnCronSchedule is called after a cron job is created. Set by gateway to register with the in-memory scheduler.
var OnCronSchedule func(ctx context.Context, tenantID, agentID, jobID, name, expression, task string)

// OnCronRemove is called when a cron job is deleted. Set by gateway to remove from in-memory scheduler.
var OnCronRemove func(jobID string)

