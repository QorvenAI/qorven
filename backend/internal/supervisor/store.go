// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package supervisor

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists supervisor exchanges and messages to PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new supervisor store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// SaveMessage persists an inter-agent message.
func (s *Store) SaveMessage(ctx context.Context, msg Message) error {
	ctxJSON, _ := json.Marshal(msg.Context)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO supervisor_messages (id, exchange_id, from_agent, to_agent, intent, content, context, risk, reply_to, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (id) DO NOTHING`,
		msg.ID, msg.ExchangeID, msg.From, msg.To, string(msg.Intent), msg.Content, ctxJSON, string(msg.Risk), msg.ReplyTo, msg.Timestamp)
	return err
}

// SaveExchange persists or updates an exchange.
func (s *Store) SaveExchange(ctx context.Context, ex *Exchange) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO supervisor_exchanges (id, agent_a, agent_b, status, started_at, closed_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (id) DO UPDATE SET status = $4, closed_at = $6`,
		ex.ID, ex.AgentA, ex.AgentB, ex.Status, ex.StartedAt, ex.ClosedAt)
	return err
}

// SaveFixResult persists an auto-fix result.
func (s *Store) SaveFixResult(ctx context.Context, result FixResult) error {
	paramsJSON, _ := json.Marshal(result.Params)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO supervisor_fix_history (fix_type, params, success, error, duration_ms, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		string(result.FixType), paramsJSON, result.Success, result.Error, result.Duration.Milliseconds(), result.Timestamp)
	return err
}

// LoadRecentMessages loads the last N messages from the audit log.
func (s *Store) LoadRecentMessages(ctx context.Context, limit int) ([]Message, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, exchange_id, from_agent, to_agent, intent, content, context, risk, reply_to, created_at
		 FROM supervisor_messages ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	msgs := []Message{}
	for rows.Next() {
		var msg Message
		var intent, risk string
		ctxJSON := []byte{}
		err := rows.Scan(&msg.ID, &msg.ExchangeID, &msg.From, &msg.To, &intent, &msg.Content, &ctxJSON, &risk, &msg.ReplyTo, &msg.Timestamp)
		if err != nil {
			continue
		}
		msg.Intent = Intent(intent)
		msg.Risk = RiskLevel(risk)
		json.Unmarshal(ctxJSON, &msg.Context)
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// LoadPendingEscalations loads unresolved escalations.
func (s *Store) LoadPendingEscalations(ctx context.Context) ([]Message, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT m.id, m.exchange_id, m.from_agent, m.to_agent, m.intent, m.content, m.context, m.risk, m.reply_to, m.created_at
		 FROM supervisor_messages m
		 WHERE m.intent = 'ESCALATION_NOTICE' AND m.to_agent = 'human'
		 AND NOT EXISTS (SELECT 1 FROM supervisor_messages r WHERE r.reply_to = m.id AND r.intent = 'ACK')
		 ORDER BY m.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	msgs := []Message{}
	for rows.Next() {
		var msg Message
		var intent, risk string
		ctxJSON := []byte{}
		rows.Scan(&msg.ID, &msg.ExchangeID, &msg.From, &msg.To, &intent, &msg.Content, &ctxJSON, &risk, &msg.ReplyTo, &msg.Timestamp)
		msg.Intent = Intent(intent)
		msg.Risk = RiskLevel(risk)
		json.Unmarshal(ctxJSON, &msg.Context)
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// LoadAuditLog loads messages from DB (for startup recovery).
func (s *Store) LoadAuditLog(ctx context.Context, since time.Duration) ([]Message, error) {
	cutoff := time.Now().Add(-since)
	return s.loadMessagesSince(ctx, cutoff)
}

func (s *Store) loadMessagesSince(ctx context.Context, since time.Time) ([]Message, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, exchange_id, from_agent, to_agent, intent, content, context, risk, reply_to, created_at
		 FROM supervisor_messages WHERE created_at > $1 ORDER BY created_at ASC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	msgs := []Message{}
	for rows.Next() {
		var msg Message
		var intent, risk string
		ctxJSON := []byte{}
		rows.Scan(&msg.ID, &msg.ExchangeID, &msg.From, &msg.To, &intent, &msg.Content, &ctxJSON, &risk, &msg.ReplyTo, &msg.Timestamp)
		msg.Intent = Intent(intent)
		msg.Risk = RiskLevel(risk)
		json.Unmarshal(ctxJSON, &msg.Context)
		msgs = append(msgs, msg)
	}
	return msgs, nil
}
