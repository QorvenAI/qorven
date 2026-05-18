// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package mail

import (
	"context"
	"time"

	emailchan "github.com/qorvenai/qorven/internal/channels/email"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the underlying connection pool for callers that need direct SQL access.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// --- Identities ---

type Identity struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	AgentID      *string   `json:"agent_id"`
	Address      string    `json:"address"`
	DisplayName  string    `json:"display_name"`
	IdentityType string    `json:"identity_type"`
	IsActive     bool      `json:"is_active"`
	SMTPHost     string    `json:"smtp_host,omitempty"`
	SMTPPort     int       `json:"smtp_port,omitempty"`
	SMTPUser     string    `json:"smtp_user,omitempty"`
	IMAPHost     string    `json:"imap_host,omitempty"`
	IMAPPort     int       `json:"imap_port,omitempty"`
	IMAPUser     string    `json:"imap_user,omitempty"`
	PollInterval int       `json:"poll_interval_seconds,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Store) CreateIdentity(ctx context.Context, tenantID, agentID, address, displayName, idType string) (*Identity, error) {
	id := &Identity{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO soul_mail_identities (tenant_id, agent_id, address, display_name, identity_type)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id, tenant_id, agent_id, address, display_name, identity_type, is_active, created_at`,
		tenantID, agentID, address, displayName, idType,
	).Scan(&id.ID, &id.TenantID, &id.AgentID, &id.Address, &id.DisplayName, &id.IdentityType, &id.IsActive, &id.CreatedAt)
	return id, err
}

func (s *Store) ListIdentities(ctx context.Context, tenantID string) ([]Identity, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, tenant_id, agent_id, address, display_name, identity_type, is_active, created_at
		 FROM soul_mail_identities WHERE tenant_id = $1 ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []Identity{}
	for rows.Next() {
		var i Identity
		rows.Scan(&i.ID, &i.TenantID, &i.AgentID, &i.Address, &i.DisplayName, &i.IdentityType, &i.IsActive, &i.CreatedAt)
		ids = append(ids, i)
	}
	return ids, nil
}

func (s *Store) FindIdentityByAddress(ctx context.Context, address, tenantID string) (*Identity, error) {
	i := &Identity{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, tenant_id, agent_id, address, display_name, identity_type, is_active, created_at
		 FROM soul_mail_identities WHERE address = $1 AND tenant_id = $2 AND is_active = true`, address, tenantID,
	).Scan(&i.ID, &i.TenantID, &i.AgentID, &i.Address, &i.DisplayName, &i.IdentityType, &i.IsActive, &i.CreatedAt)
	return i, err
}

// --- Aliases ---

type Alias struct {
	ID            string `json:"id"`
	AliasAddress  string `json:"alias_address"`
	TargetAgentID string `json:"target_agent_id"`
	CanSendAs     bool   `json:"can_send_as"`
	CanReceive    bool   `json:"can_receive"`
}

func (s *Store) FindAliasesByAddress(ctx context.Context, address, tenantID string) ([]Alias, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, alias_address, target_agent_id, can_send_as, can_receive
		 FROM mail_aliases WHERE alias_address = $1 AND tenant_id = $2 AND can_receive = true`, address, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	aliases := []Alias{}
	for rows.Next() {
		var a Alias
		rows.Scan(&a.ID, &a.AliasAddress, &a.TargetAgentID, &a.CanSendAs, &a.CanReceive)
		aliases = append(aliases, a)
	}
	return aliases, nil
}

// --- Messages ---

type Message struct {
	ID          string    `json:"id"`
	AgentID     *string   `json:"agent_id"`
	IdentityID  string    `json:"identity_id"`
	ThreadID    string    `json:"thread_id"`
	MessageID   string    `json:"message_id"`
	Folder      string    `json:"folder"`
	Direction   string    `json:"direction"`
	FromAddress string    `json:"from_address"`
	FromName    string    `json:"from_name"`
	ToAddresses []string  `json:"to_addresses"`
	Subject     string    `json:"subject"`
	BodyText    string    `json:"body_text"`
	BodyHTML    string    `json:"body_html"`
	IsRead      bool      `json:"is_read"`
	IsStarred   bool      `json:"is_starred"`
	SendStatus  string    `json:"send_status"`
	ReceivedAt  time.Time `json:"received_at"`
}

func (s *Store) StoreInbound(ctx context.Context, tenantID, agentID, identityID, messageID, threadID, from, fromName, subject, bodyText, bodyHTML string, to []string) (*Message, error) {
	m := &Message{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO mailbox_messages (tenant_id, agent_id, identity_id, message_id, thread_id, folder, direction, from_address, from_name, to_addresses, subject, body_text, body_html)
		 VALUES ($1, $2, $3, $4, $5, 'inbox', 'inbound', $6, $7, $8, $9, $10, $11)
		 RETURNING id, agent_id, identity_id, thread_id, message_id, folder, direction, from_address, from_name, to_addresses, subject, body_text, body_html, is_read, is_starred, send_status, received_at`,
		tenantID, agentID, identityID, messageID, threadID, from, fromName, to, subject, bodyText, bodyHTML,
	).Scan(&m.ID, &m.AgentID, &m.IdentityID, &m.ThreadID, &m.MessageID, &m.Folder, &m.Direction, &m.FromAddress, &m.FromName, &m.ToAddresses, &m.Subject, &m.BodyText, &m.BodyHTML, &m.IsRead, &m.IsStarred, &m.SendStatus, &m.ReceivedAt)
	return m, err
}

func (s *Store) ListInbox(ctx context.Context, agentID, folder string, limit int) ([]Message, error) {
	if folder == "" {
		folder = "inbox"
	}
	if limit <= 0 {
		limit = 50
	}
	var rows pgx.Rows
	var err error
	q := `SELECT id, agent_id, COALESCE(identity_id::text,''), COALESCE(thread_id,''), message_id, folder, direction, from_address, COALESCE(from_name,''), to_addresses, COALESCE(subject,''), COALESCE(body_text,''), '', is_read, is_starred, send_status, received_at
		 FROM mailbox_messages WHERE folder = $1`
	if agentID != "" {
		rows, err = s.pool.Query(ctx, q+` AND agent_id = $2 ORDER BY received_at DESC LIMIT $3`, folder, agentID, limit)
	} else {
		rows, err = s.pool.Query(ctx, q+` ORDER BY received_at DESC LIMIT $2`, folder, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	msgs := []Message{}
	for rows.Next() {
		var m Message
		rows.Scan(&m.ID, &m.AgentID, &m.IdentityID, &m.ThreadID, &m.MessageID, &m.Folder, &m.Direction, &m.FromAddress, &m.FromName, &m.ToAddresses, &m.Subject, &m.BodyText, &m.BodyHTML, &m.IsRead, &m.IsStarred, &m.SendStatus, &m.ReceivedAt)
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (s *Store) GetMessage(ctx context.Context, id string) (*Message, error) {
	m := &Message{}
	err := s.pool.QueryRow(ctx,
		`SELECT id, agent_id, identity_id, COALESCE(thread_id,''), message_id, folder, direction, from_address, COALESCE(from_name,''), to_addresses, COALESCE(subject,''), COALESCE(body_text,''), COALESCE(body_html,''), is_read, is_starred, send_status, received_at
		 FROM mailbox_messages WHERE id = $1`, id,
	).Scan(&m.ID, &m.AgentID, &m.IdentityID, &m.ThreadID, &m.MessageID, &m.Folder, &m.Direction, &m.FromAddress, &m.FromName, &m.ToAddresses, &m.Subject, &m.BodyText, &m.BodyHTML, &m.IsRead, &m.IsStarred, &m.SendStatus, &m.ReceivedAt)
	return m, err
}

func (s *Store) GetThread(ctx context.Context, threadID string) ([]Message, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, agent_id, identity_id, thread_id, message_id, folder, direction, from_address, COALESCE(from_name,''), to_addresses, COALESCE(subject,''), COALESCE(body_text,''), COALESCE(body_html,''), is_read, is_starred, send_status, received_at
		 FROM mailbox_messages WHERE thread_id = $1 ORDER BY received_at`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	msgs := []Message{}
	for rows.Next() {
		var m Message
		rows.Scan(&m.ID, &m.AgentID, &m.IdentityID, &m.ThreadID, &m.MessageID, &m.Folder, &m.Direction, &m.FromAddress, &m.FromName, &m.ToAddresses, &m.Subject, &m.BodyText, &m.BodyHTML, &m.IsRead, &m.IsStarred, &m.SendStatus, &m.ReceivedAt)
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (s *Store) MarkRead(ctx context.Context, id string, read bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE mailbox_messages SET is_read = $1 WHERE id = $2`, read, id)
	return err
}

func (s *Store) MarkStarred(ctx context.Context, id string, starred bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE mailbox_messages SET is_starred = $1 WHERE id = $2`, starred, id)
	return err
}

func (s *Store) StoreSend(ctx context.Context, tenantID, agentID, identityID, messageID, threadID, to, subject, bodyText, bodyHTML, status string, toAddrs []string) (*Message, error) {
	m := &Message{}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO mailbox_messages (tenant_id, agent_id, identity_id, message_id, thread_id, folder, direction, from_address, to_addresses, subject, body_text, body_html, send_status)
		 VALUES ($1, $2, $3, $4, $5, 'sent', 'outbound', $6, $7, $8, $9, $10, $11)
		 RETURNING id, agent_id, identity_id, COALESCE(thread_id,''), message_id, folder, direction, from_address, '', to_addresses, subject, body_text, body_html, is_read, is_starred, send_status, received_at`,
		tenantID, agentID, identityID, messageID, threadID, to, toAddrs, subject, bodyText, bodyHTML, status,
	).Scan(&m.ID, &m.AgentID, &m.IdentityID, &m.ThreadID, &m.MessageID, &m.Folder, &m.Direction, &m.FromAddress, &m.FromName, &m.ToAddresses, &m.Subject, &m.BodyText, &m.BodyHTML, &m.IsRead, &m.IsStarred, &m.SendStatus, &m.ReceivedAt)
	return m, err
}

// --- Approval ---

func (s *Store) CreateApproval(ctx context.Context, tenantID, messageID, agentID string, expiresAt *time.Time) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO mail_approval_queue (tenant_id, message_id, agent_id, expires_at) VALUES ($1, $2, $3, $4) RETURNING id`,
		tenantID, messageID, agentID, expiresAt).Scan(&id)
	return id, err
}

func (s *Store) ListPendingApprovals(ctx context.Context, tenantID string) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT q.id, q.message_id, q.agent_id, q.status, q.expires_at, q.created_at,
		        m.from_address, m.to_addresses, m.subject, m.body_text
		 FROM mail_approval_queue q JOIN mailbox_messages m ON q.message_id = m.id
		 WHERE q.tenant_id = $1 AND q.status = 'pending' ORDER BY q.created_at`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := []map[string]any{}
	for rows.Next() {
		var id, msgID, agentID, status, from, subject, body string
		to := []string{}
		var expiresAt *time.Time
		var createdAt time.Time
		rows.Scan(&id, &msgID, &agentID, &status, &expiresAt, &createdAt, &from, &to, &subject, &body)
		results = append(results, map[string]any{
			"id": id, "message_id": msgID, "agent_id": agentID, "status": status,
			"expires_at": expiresAt, "created_at": createdAt,
			"from": from, "to": to, "subject": subject, "body_preview": body,
		})
	}
	return results, nil
}

// ─── ThreadLoader implementation (email channel interface) ────────────────────

// GetVerifiedThread loads the full thread history from the agent's own DB records.
// Uses the canonical thread_id to find all messages in the same conversation.
// This is the "Outlook model" — agent reads its own verified sent/received records,
// not the potentially-forged quoted text in the email body.
func (s *Store) GetVerifiedThread(ctx context.Context, threadID string) ([]emailchan.ThreadMessage, error) {
	msgs, err := s.GetThread(ctx, threadID)
	if err != nil || len(msgs) == 0 {
		return nil, err
	}
	out := make([]emailchan.ThreadMessage, 0, len(msgs))
	for _, m := range msgs {
		body := m.BodyText
		if body == "" { body = m.BodyHTML }
		out = append(out, emailchan.ThreadMessage{
			Direction:  m.Direction,
			From:       m.FromAddress,
			Subject:    m.Subject,
			Body:       body,
			ReceivedAt: m.ReceivedAt.Format("Mon Jan 2, 15:04"),
		})
	}
	return out, nil
}

// IsKnownSender checks if this sender has ever corresponded with this agent.
// Known = any inbound message from this address exists in the agent's mailbox.
func (s *Store) IsKnownSender(ctx context.Context, tenantID, agentID, fromAddress string) bool {
	if s.pool == nil { return false }
	var count int
	s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM mailbox_messages
		 WHERE tenant_id = $1 AND agent_id = $2 AND from_address = $3 AND direction = 'inbound'
		 LIMIT 1`,
		tenantID, agentID, fromAddress).Scan(&count)
	return count > 0
}

func (s *Store) DecideApproval(ctx context.Context, id, decision, reviewedBy, notes string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE mail_approval_queue SET status = $1, reviewed_by = $2, reviewed_at = now(), notes = $3 WHERE id = $4`,
		decision, reviewedBy, notes, id)
	if decision == "approved" {
		// Update the message send_status
		s.pool.Exec(ctx,
			`UPDATE mailbox_messages SET send_status = 'approved' WHERE id = (SELECT message_id FROM mail_approval_queue WHERE id = $1)`, id)
	}
	return err
}
