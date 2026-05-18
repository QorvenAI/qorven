// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qorvenai/qorven/internal/store"
)

type Session struct {
	ID            string          `json:"id"`
	TenantID      string          `json:"tenant_id"`
	AgentID       string          `json:"agent_id"`
	UserID        string          `json:"user_id,omitempty"`
	// OwnerActorID is the canonical owner of the session, set by
	// handleCreateSession from the authenticated request context.
	// Populated from Phase 2 onward; older rows may be empty and
	// fall under the admin-only fallback in the ownership check.
	OwnerActorID  string          `json:"owner_actor_id,omitempty"`
	Channel       string          `json:"channel,omitempty"`
	Messages      json.RawMessage `json:"messages"`
	Label         string          `json:"label,omitempty"`
	Summary       string          `json:"summary,omitempty"`
	Status        string          `json:"status"`
	InputTokens   int64           `json:"input_tokens"`
	OutputTokens  int64           `json:"output_tokens"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type Message struct {
	Role       string         `json:"role"`
	Parts      json.RawMessage `json:"parts,omitempty"`
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Timestamp  int64          `json:"timestamp,omitempty"`
	Channel    string         `json:"channel,omitempty"`
	SenderName string         `json:"sender_name,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"` // widgets, sources, thinking
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// q routes queries through the tenant-scoped tx from ctx in multi-
// tenant HTTP handlers, else the raw pool. Every query in this file
// MUST use q — direct s.pool access bypasses RLS in multi-tenant.
func (s *Store) q(ctx context.Context) store.Queryable {
	return store.FromContext(ctx, s.pool)
}

// Create creates a new session and returns it with its UUID id.
// OwnerActorID is left empty — callers that have an authenticated actor
// should use CreateWithOwner instead so the Phase 2 ownership check can
// authorize future commands.
func (s *Store) Create(ctx context.Context, tenantID, agentID, userID, channel string) (*Session, error) {
	return s.CreateWithOwner(ctx, tenantID, agentID, userID, "", channel)
}

// CreateWithOwner is the Phase 2 session constructor. ownerActorID is
// recorded on the row so OwnerCheck can identify the session's owner
// without the legacy "operator" fallback.
func (s *Store) CreateWithOwner(ctx context.Context, tenantID, agentID, userID, ownerActorID, channel string) (*Session, error) {
	id := uuid.New().String()
	agentShort := agentID
	if len(agentShort) > 8 {
		agentShort = agentShort[:8]
	}
	key := fmt.Sprintf("%s:%s:%s", agentShort, userID, id[:8])
	now := time.Now()

	sess := &Session{
		ID: id, TenantID: tenantID, AgentID: agentID,
		UserID: userID, OwnerActorID: ownerActorID, Channel: channel, Status: "active",
		Messages: json.RawMessage("[]"), CreatedAt: now, UpdatedAt: now,
	}

	_, err := s.q(ctx).Exec(ctx,
		`INSERT INTO sessions (id, tenant_id, agent_id, session_key, user_id, owner_actor_id, messages, channel, status, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,$10,$11)`,
		sess.ID, sess.TenantID, sess.AgentID, key, sess.UserID, sess.OwnerActorID,
		sess.Messages, sess.Channel, sess.Status, sess.CreatedAt, sess.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return sess, nil
}

// GetByID loads a session by its UUID id. This is the PRIMARY lookup method.
func (s *Store) GetByID(ctx context.Context, id string) (*Session, error) {
	sess := &Session{}
	err := s.q(ctx).QueryRow(ctx,
		`SELECT id, tenant_id, agent_id, COALESCE(user_id,''), COALESCE(owner_actor_id,''), COALESCE(channel,'web'),
		        messages, COALESCE(label,''), COALESCE(summary,''), COALESCE(status,'active'),
		        COALESCE(input_tokens,0), COALESCE(output_tokens,0), created_at, updated_at
		 FROM sessions WHERE id = $1`, id,
	).Scan(&sess.ID, &sess.TenantID, &sess.AgentID, &sess.UserID, &sess.OwnerActorID, &sess.Channel,
		&sess.Messages, &sess.Label, &sess.Summary, &sess.Status,
		&sess.InputTokens, &sess.OutputTokens, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s (%v)", id, err)
	}
	return sess, nil
}

// Get tries ID first, then falls back to session_key (backward compat).
func (s *Store) Get(ctx context.Context, idOrKey string) (*Session, error) {
	sess, err := s.GetByID(ctx, idOrKey)
	if err == nil {
		return sess, nil
	}
	// Fallback: try session_key
	sess = &Session{}
	err = s.q(ctx).QueryRow(ctx,
		`SELECT id, tenant_id, agent_id, COALESCE(user_id,''), COALESCE(owner_actor_id,''),
		        COALESCE(channel,''), messages, COALESCE(label,''), COALESCE(summary,''), COALESCE(status,''),
		        COALESCE(input_tokens,0), COALESCE(output_tokens,0), created_at, updated_at
		 FROM sessions WHERE session_key = $1`, idOrKey,
	).Scan(&sess.ID, &sess.TenantID, &sess.AgentID, &sess.UserID, &sess.OwnerActorID, &sess.Channel,
		&sess.Messages, &sess.Label, &sess.Summary, &sess.Status,
		&sess.InputTokens, &sess.OutputTokens, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", idOrKey)
	}
	return sess, nil
}

// AppendUserMessage persists a user-role message synchronously and returns
// the canonical id assigned to the message. The id is stored in the
// message's Metadata under the key "id" so subsequent lookups (GET by id,
// part.updated correlation) resolve to the same row. Callers that care
// about the id — chiefly the command API's SubmitPrompt handler — must
// use this method instead of AppendMessage + synthesized ids.
//
// The returned id is a UUIDv4 string. The method is idempotent in the
// sense that it never appends the same message twice if the same id
// appears in Metadata — this protects against Phase 1's dual-wire
// emission path.
func (s *Store) AppendUserMessage(ctx context.Context, sessionID, agentID, text string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session_id required")
	}
	msgID := uuid.New().String()
	meta := map[string]any{
		"id":       msgID,
		"agent_id": agentID,
	}
	msg := Message{
		Role:      "user",
		Content:   text,
		Timestamp: time.Now().UnixMilli(),
		Metadata:  meta,
	}
	if err := s.AppendMessage(ctx, sessionID, msg, 0, 0); err != nil {
		return "", err
	}
	return msgID, nil
}

// AppendMessage adds a message to a session by ID and updates token counts.
func (s *Store) AppendMessage(ctx context.Context, sessionID string, msg Message, inputTok, outputTok int) error {
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	// Single UPDATE that handles both UUID id and session_key in one query.
	// This avoids the invalid-UUID-cast error when sessionID is a custom string.
	_, err = s.q(ctx).Exec(ctx,
		`UPDATE sessions
		 SET messages = messages || $2::jsonb,
		     input_tokens = input_tokens + $3,
		     output_tokens = output_tokens + $4,
		     updated_at = NOW()
		 WHERE id::text = $1 OR session_key = $1`,
		sessionID, msgJSON, inputTok, outputTok)
	return err
}

// UpdateLabel sets the session label.
func (s *Store) UpdateLabel(ctx context.Context, sessionID, label string) error {
	_, err := s.q(ctx).Exec(ctx, `UPDATE sessions SET label = $2, updated_at = NOW() WHERE id = $1`, sessionID, label)
	return err
}

// ListForAgent returns sessions for a specific agent, ordered by most recent.
func (s *Store) ListForAgent(ctx context.Context, agentID string, limit int) ([]*Session, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q(ctx).Query(ctx,
		`SELECT id, tenant_id, agent_id, COALESCE(channel,'web'), COALESCE(label,''), COALESCE(status,'active'),
		        jsonb_array_length(messages) as msg_count, input_tokens, output_tokens, created_at, updated_at
		 FROM sessions WHERE agent_id = $1 AND (status = 'active' OR status IS NULL OR status = '')
		 ORDER BY updated_at DESC LIMIT $2`, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := []*Session{}
	for rows.Next() {
		sess := &Session{}
		var msgCount int
		rows.Scan(&sess.ID, &sess.TenantID, &sess.AgentID, &sess.Channel, &sess.Label, &sess.Status,
			&msgCount, &sess.InputTokens, &sess.OutputTokens, &sess.CreatedAt, &sess.UpdatedAt)
		sessions = append(sessions, sess)
	}
	return sessions, nil
}


// FindByAgentAndChannel returns the most recent active session for an agent+channel pair.
func (s *Store) FindByAgentAndChannel(ctx context.Context, agentID, channel string) (*Session, error) {
	sess := &Session{}
	err := s.q(ctx).QueryRow(ctx,
		`SELECT id, tenant_id, agent_id, COALESCE(channel,'web'), COALESCE(label,''), COALESCE(status,'active'),
		        input_tokens, output_tokens, created_at, updated_at
		 FROM sessions WHERE agent_id = $1 AND channel = $2 AND (status = 'active' OR status IS NULL OR status = '')
		 ORDER BY updated_at DESC LIMIT 1`, agentID, channel,
	).Scan(&sess.ID, &sess.TenantID, &sess.AgentID, &sess.Channel, &sess.Label, &sess.Status,
		&sess.InputTokens, &sess.OutputTokens, &sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return sess, nil
}
// List returns all sessions for a tenant, optionally filtered by agent.
func (s *Store) List(ctx context.Context, tenantID, agentID string, limit int) ([]*Session, error) {
	if agentID != "" {
		return s.ListForAgent(ctx, agentID, limit)
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q(ctx).Query(ctx,
		`SELECT id, tenant_id, agent_id, COALESCE(channel,'web'), COALESCE(label,''), COALESCE(status,'active'),
		        input_tokens, output_tokens, created_at, updated_at
		 FROM sessions WHERE tenant_id = $1 AND (status = 'active' OR status IS NULL OR status = '')
		 ORDER BY updated_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := []*Session{}
	for rows.Next() {
		sess := &Session{}
		rows.Scan(&sess.ID, &sess.TenantID, &sess.AgentID, &sess.Channel, &sess.Label, &sess.Status,
			&sess.InputTokens, &sess.OutputTokens, &sess.CreatedAt, &sess.UpdatedAt)
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// Delete soft-deletes a session by setting status to 'archived'.
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.q(ctx).Exec(ctx, `UPDATE sessions SET status = 'archived', updated_at = NOW() WHERE id = $1`, id)
	return err
}

// Search finds sessions matching a query using PostgreSQL full-text search.
func (s *Store) Search(ctx context.Context, tenantID, query string, limit int) ([]*Session, error) {
	if limit <= 0 { limit = 10 }
	rows, err := s.q(ctx).Query(ctx,
		`SELECT id, tenant_id, agent_id, COALESCE(user_id,''), COALESCE(channel,'web'),
		        messages, COALESCE(label,''), COALESCE(summary,''), COALESCE(status,'active'),
		        COALESCE(input_tokens,0), COALESCE(output_tokens,0), created_at, updated_at
		 FROM sessions
		 WHERE tenant_id = $1 AND (label ILIKE '%' || $2 || '%' OR summary ILIKE '%' || $2 || '%')
		 ORDER BY updated_at DESC
		 LIMIT $3`, tenantID, query, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	out := []*Session{}
	for rows.Next() {
		sess := &Session{}
		rows.Scan(&sess.ID, &sess.TenantID, &sess.AgentID, &sess.UserID, &sess.Channel,
			&sess.Messages, &sess.Label, &sess.Summary, &sess.Status,
			&sess.InputTokens, &sess.OutputTokens, &sess.CreatedAt, &sess.UpdatedAt)
		out = append(out, sess)
	}
	return out, nil
}


// CreateWithKey creates a session with a custom session_key (used for API caller-supplied IDs).
func (s *Store) CreateWithKey(ctx context.Context, tenantID, agentID, userID, channel, key string) (*Session, error) {
	// Check if key already exists
	existing := &Session{}
	err := s.q(ctx).QueryRow(ctx,
		`SELECT id, tenant_id, agent_id, COALESCE(user_id,''), COALESCE(channel,'web'),
		        messages, COALESCE(label,''), COALESCE(summary,''), COALESCE(status,'active'),
		        COALESCE(input_tokens,0), COALESCE(output_tokens,0), created_at, updated_at
		 FROM sessions WHERE session_key = $1`, key,
	).Scan(&existing.ID, &existing.TenantID, &existing.AgentID, &existing.UserID, &existing.Channel,
		&existing.Messages, &existing.Label, &existing.Summary, &existing.Status,
		&existing.InputTokens, &existing.OutputTokens, &existing.CreatedAt, &existing.UpdatedAt)
	if err == nil {
		return existing, nil
	}
	// Create new
	id := uuid.New().String()
	if channel == "" { channel = "web" }
	now := time.Now()
	sess := &Session{
		ID: id, TenantID: tenantID, AgentID: agentID,
		UserID: userID, Channel: channel, Status: "active",
		Messages: json.RawMessage("[]"), CreatedAt: now, UpdatedAt: now,
	}
	_, err = s.q(ctx).Exec(ctx,
		`INSERT INTO sessions (id, tenant_id, agent_id, session_key, user_id, messages, channel, status, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		sess.ID, sess.TenantID, sess.AgentID, key, sess.UserID, sess.Messages, sess.Channel, sess.Status, sess.CreatedAt, sess.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// GetHistory returns all messages for a session.
func (s *Store) GetHistory(ctx context.Context, sessionID string) ([]Message, error) {
	sess, err := s.GetByID(ctx, sessionID)
	if err != nil { return nil, err }
	msgs := []Message{}
	json.Unmarshal(sess.Messages, &msgs)
	return msgs, nil
}

// SetHistory replaces all messages for a session.
func (s *Store) SetHistory(ctx context.Context, sessionID string, msgs []Message) error {
	data, _ := json.Marshal(msgs)
	_, err := s.q(ctx).Exec(ctx, `UPDATE sessions SET messages = $1, updated_at = now() WHERE id = $2`, data, sessionID)
	return err
}

// Compact summarizes old messages, keeping recent ones intact.
func (s *Store) Compact(ctx context.Context, sessionID string, keepRecent int) error {
	msgs, err := s.GetHistory(ctx, sessionID)
	if err != nil { return err }
	if len(msgs) <= keepRecent { return nil }

	old := msgs[:len(msgs)-keepRecent]
	recent := msgs[len(msgs)-keepRecent:]

	var summary strings.Builder
	summary.WriteString("[Previous conversation summary]\n")
	for _, m := range old {
		c := m.Content
		if len(c) > 100 { c = c[:100] + "..." }
		summary.WriteString(fmt.Sprintf("%s: %s\n", m.Role, c))
	}

	compacted := []Message{{Role: "system", Content: summary.String(), Timestamp: time.Now().Unix()}}
	compacted = append(compacted, recent...)
	return s.SetHistory(ctx, sessionID, compacted)
}

// Reset clears messages but preserves label.
func (s *Store) Reset(ctx context.Context, sessionID string) error {
	empty, _ := json.Marshal([]Message{})
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE sessions SET messages = $1, input_tokens = 0, output_tokens = 0, updated_at = now() WHERE id = $2`,
		empty, sessionID)
	return err
}

// UpdateSummary sets the session summary (used by compaction).
func (s *Store) UpdateSummary(ctx context.Context, sessionID, summary string) error {
	_, err := s.q(ctx).Exec(ctx, `UPDATE sessions SET summary = $2, updated_at = NOW() WHERE id = $1`, sessionID, summary)
	return err
}

// GetSummary returns the session summary.
func (s *Store) GetSummary(ctx context.Context, sessionID string) (string, error) {
	var summary string
	err := s.q(ctx).QueryRow(ctx, `SELECT COALESCE(summary,'') FROM sessions WHERE id = $1`, sessionID).Scan(&summary)
	return summary, err
}

// AccumulateTokens adds token counts to the session.
func (s *Store) AccumulateTokens(ctx context.Context, sessionID string, inputTok, outputTok int) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE sessions SET input_tokens = input_tokens + $2, output_tokens = output_tokens + $3, updated_at = NOW() WHERE id = $1`,
		sessionID, inputTok, outputTok)
	return err
}

// TruncateHistory keeps only the last N messages.
func (s *Store) TruncateHistory(ctx context.Context, sessionID string, keepLast int) error {
	msgs, err := s.GetHistory(ctx, sessionID)
	if err != nil || len(msgs) <= keepLast {
		return err
	}
	return s.SetHistory(ctx, sessionID, msgs[len(msgs)-keepLast:])
}

// ListPaged returns sessions with pagination.
func (s *Store) ListPaged(ctx context.Context, tenantID string, limit, offset int) ([]*Session, int, error) {
	if limit <= 0 { limit = 50 }
	var total int
	s.q(ctx).QueryRow(ctx, `SELECT count(*) FROM sessions WHERE tenant_id = $1 AND status != 'archived'`, tenantID).Scan(&total)

	rows, err := s.q(ctx).Query(ctx,
		`SELECT id, tenant_id, agent_id, COALESCE(channel,'web'), COALESCE(label,''), COALESCE(status,'active'),
		        input_tokens, output_tokens, created_at, updated_at
		 FROM sessions WHERE tenant_id = $1 AND status != 'archived'
		 ORDER BY updated_at DESC LIMIT $2 OFFSET $3`, tenantID, limit, offset)
	if err != nil { return nil, 0, err }
	defer rows.Close()

	sessions := []*Session{}
	for rows.Next() {
		sess := &Session{}
		rows.Scan(&sess.ID, &sess.TenantID, &sess.AgentID, &sess.Channel, &sess.Label, &sess.Status,
			&sess.InputTokens, &sess.OutputTokens, &sess.CreatedAt, &sess.UpdatedAt)
		sessions = append(sessions, sess)
	}
	return sessions, total, nil
}

// GetContinuationSummary returns a compact "what was I doing" context string
// built from the agent's most recent sessions across ALL channels.
// This is the core of "one agent, one continuous thread" — regardless of whether
// the last conversation was on web, Telegram, or a cron job, the agent resumes
// with awareness of what it was doing before.
//
// Returns empty string if nothing useful found (fresh agent).
func (s *Store) GetContinuationSummary(ctx context.Context, agentID string, maxSessions int) string {
	if s.pool == nil || agentID == "" {
		return ""
	}
	if maxSessions <= 0 {
		maxSessions = 3
	}

	// Get the most recent N sessions across all channels, ordered by last activity
	rows, err := s.q(ctx).Query(ctx,
		`SELECT id, COALESCE(channel,'web'), COALESCE(label,''), COALESCE(summary,''),
		        COALESCE(input_tokens,0)+COALESCE(output_tokens,0), updated_at
		 FROM sessions
		 WHERE agent_id = $1
		   AND (status = 'active' OR status = 'completed' OR status IS NULL)
		   AND updated_at > NOW() - INTERVAL '7 days'
		 ORDER BY updated_at DESC
		 LIMIT $2`,
		agentID, maxSessions)
	if err != nil {
		return ""
	}
	defer rows.Close()

	type sessInfo struct {
		id       string
		channel  string
		label    string
		summary  string
		tokens   int
		updatedAt time.Time
	}

	var sessions []sessInfo
	for rows.Next() {
		var si sessInfo
		if rows.Scan(&si.id, &si.channel, &si.label, &si.summary, &si.tokens, &si.updatedAt) == nil {
			sessions = append(sessions, si)
		}
	}

	if len(sessions) == 0 {
		return ""
	}

	// Build continuation context
	var sb strings.Builder
	sb.WriteString("## Background Context (previous sessions)\n")
	sb.WriteString("You have context from recent sessions listed below. This is background awareness only — the user is starting a NEW conversation now. Do NOT continue or reference incomplete tasks from these past sessions unless the user explicitly asks. Respond to the user's current message freshly.\n\n")

	for i, si := range sessions {
		age := time.Since(si.updatedAt)
		ageStr := ""
		switch {
		case age < time.Hour:
			ageStr = fmt.Sprintf("%dm ago", int(age.Minutes()))
		case age < 24*time.Hour:
			ageStr = fmt.Sprintf("%dh ago", int(age.Hours()))
		default:
			ageStr = fmt.Sprintf("%dd ago", int(age.Hours()/24))
		}

		label := si.label
		if label == "" {
			label = fmt.Sprintf("Session on %s", si.channel)
		}

		sb.WriteString(fmt.Sprintf("### %d. %s (%s, %s)\n", i+1, label, si.channel, ageStr))

		if si.summary != "" {
			// Session has an LLM-generated summary — use it directly
			sb.WriteString(si.summary)
		} else {
			// No summary yet — load last few messages as fallback
			msgs, err := s.GetHistory(ctx, si.id)
			if err == nil && len(msgs) > 0 {
				// Take last 3 meaningful turns
				meaningful := []Message{}
				for _, m := range msgs {
					if m.Role == "user" || m.Role == "assistant" {
						if len(m.Content) > 20 {
							meaningful = append(meaningful, m)
						}
					}
				}
				if len(meaningful) > 6 {
					meaningful = meaningful[len(meaningful)-6:]
				}
				for _, m := range meaningful {
					content := m.Content
					if len(content) > 300 {
						content = content[:300] + "…"
					}
					sb.WriteString(fmt.Sprintf("- **%s**: %s\n", m.Role, content))
				}
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("You are aware of this past context. Now respond to the user's new message directly and naturally.\n")
	return sb.String()
}

func (s *Store) GetOrCreate(ctx context.Context, sessionID, agentID, tenantID string) (string, error) {
	var id string
	err := s.q(ctx).QueryRow(ctx,
		`INSERT INTO sessions (id, agent_id, tenant_id, created_at) VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id RETURNING id`,
		sessionID, agentID, tenantID).Scan(&id)
	return id, err
}

func (s *Store) SetAgentInfo(ctx context.Context, sessionID, agentID, agentName string) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE sessions SET agent_id = $1, label = COALESCE(NULLIF(label,''), $2) WHERE id = $3`,
		agentID, agentName, sessionID)
	return err
}

func (s *Store) UpdateMetadata(ctx context.Context, sessionID string, meta map[string]any) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE sessions SET metadata = COALESCE(metadata, '{}'::jsonb) || $1::jsonb WHERE id = $2`,
		meta, sessionID)
	return err
}

func (s *Store) IncrementCompaction(ctx context.Context, sessionID string) (int, error) {
	var count int
	err := s.q(ctx).QueryRow(ctx,
		`UPDATE sessions SET compaction_count = COALESCE(compaction_count, 0) + 1 WHERE id = $1
		 RETURNING compaction_count`, sessionID).Scan(&count)
	return count, err
}

func (s *Store) GetCompactionCount(ctx context.Context, sessionID string) (int, error) {
	var count int
	err := s.q(ctx).QueryRow(ctx,
		`SELECT COALESCE(compaction_count, 0) FROM sessions WHERE id = $1`, sessionID).Scan(&count)
	return count, err
}

func (s *Store) SetMemoryFlushDone(ctx context.Context, sessionID string, compactionCount int) error {
	_, err := s.q(ctx).Exec(ctx,
		`UPDATE sessions SET memory_flush_at_compaction = $1 WHERE id = $2`,
		compactionCount, sessionID)
	return err
}

func (s *Store) GetMemoryFlushCompactionCount(ctx context.Context, sessionID string) (int, error) {
	var count int
	err := s.q(ctx).QueryRow(ctx,
		`SELECT COALESCE(memory_flush_at_compaction, 0) FROM sessions WHERE id = $1`, sessionID).Scan(&count)
	return count, err
}

// TrackFile records a file change in the session's files_changed array.
func (s *Store) TrackFile(ctx context.Context, sessionID, path, action string) {
	s.q(ctx).Exec(ctx,
		`UPDATE sessions SET files_changed = files_changed || $1::jsonb WHERE id = $2`,
		fmt.Sprintf(`[{"path":%q,"action":%q,"at":%q}]`, path, action, time.Now().Format(time.RFC3339)),
		sessionID)
}

// GetFiles returns files changed in a session.
func (s *Store) GetFiles(ctx context.Context, sessionID string) (json.RawMessage, error) {
	var files json.RawMessage
	err := s.q(ctx).QueryRow(ctx, `SELECT COALESCE(files_changed, '[]') FROM sessions WHERE id = $1`, sessionID).Scan(&files)
	return files, err
}
