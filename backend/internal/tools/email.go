// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sanitizeEmailHeader strips CR and LF to prevent header injection.
func sanitizeEmailHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// --- Config types ---

type SMTPConfig struct {
	Host, User, Password, From, FromName string
	Port                                  int
}

type IMAPConfig struct {
	Host, User, Password string
	Port                 int
}

// BoundIdentity carries the resolved sending identity for an agent.
// Address is the locked from-address (cannot be overridden by the LLM).
type BoundIdentity struct {
	IdentityID  string
	Address     string
	DisplayName string
	SMTPHost    string
	SMTPPort    int
	SMTPUser    string
	SMTPPass    string // already decrypted
	Transport   string
	ForwardURL  string
	SignatureText string
	SignatureHTML string
	ReplyTo     string
	Importance  string
	TenantID    string
}

type MailboxConfig struct {
	SMTP    *SMTPConfig
	IMAP    *IMAPConfig
	AgentID string
	Pool    *pgxpool.Pool
	// ResolveIdentity returns the bound mailbox identity for the given agent.
	// Set by the gateway so the tool can send as the agent's own address without
	// importing the mail package (avoids a circular dependency).
	// Returns nil, "" if the agent has no bound identity.
	ResolveIdentity func(ctx context.Context, agentID, tenantID string) (*BoundIdentity, error)
	// EncKey is the encryption key used to decrypt SMTP passwords when
	// ResolveIdentity is not set and a legacy global SMTP config is in use.
	EncKey string
}

// --- Email Send Tool ---

type EmailSendTool struct{ cfg *MailboxConfig }

func NewEmailSendTool() *EmailSendTool { return &EmailSendTool{} }
func (t *EmailSendTool) SetMailbox(cfg *MailboxConfig) { t.cfg = cfg }
func (t *EmailSendTool) SetSMTP(cfg *SMTPConfig) {
	if t.cfg == nil { t.cfg = &MailboxConfig{} }
	t.cfg.SMTP = cfg
}

// SetResolveIdentity installs the per-agent identity resolver and the pool for
// approval gating + persistence.  Call this from the gateway whenever the mail
// store is available, independent of whether a global SMTP is configured.
func (t *EmailSendTool) SetResolveIdentity(pool *pgxpool.Pool, fn func(ctx context.Context, agentID, tenantID string) (*BoundIdentity, error)) {
	if t.cfg == nil {
		t.cfg = &MailboxConfig{}
	}
	t.cfg.Pool = pool
	t.cfg.ResolveIdentity = fn
}

func (t *EmailSendTool) Name() string        { return "email_send" }
func (t *EmailSendTool) Description() string  { return "Send an email. Requires to, subject, and body." }
func (t *EmailSendTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"to":      map[string]any{"type": "string", "description": "Recipient email address"},
			"subject": map[string]any{"type": "string", "description": "Email subject line"},
			"body":    map[string]any{"type": "string", "description": "Email body text"},
		},
		"required": []string{"to", "subject", "body"},
	}
}

func (t *EmailSendTool) Execute(ctx context.Context, args map[string]any) *Result {
	to, _ := args["to"].(string)
	subject, _ := args["subject"].(string)
	body, _ := args["body"].(string)
	to = sanitizeEmailHeader(to)
	subject = sanitizeEmailHeader(subject)
	if to == "" || body == "" {
		return ErrorResult("to and body are required")
	}
	if t.cfg == nil {
		return ErrorResult("mail not configured")
	}

	agentID := AgentIDFromCtx(ctx)
	sessionID := SessionIDFromCtx(ctx)

	// ── Outbound approval gate ─────────────────────────────────────────────────
	// This gate is the autonomous-send authorization check. It MUST remain here
	// and is never bypassed: if outbound_approval is 'none' proceed immediately;
	// any other mode (supervisor/user/both) queues for human review first.
	if t.cfg.Pool != nil && agentID != "" {
		proceed, queueID, _ := CheckApproval(ctx, t.cfg.Pool, agentID, "email_send", sessionID, args)
		if !proceed {
			return TextResult(fmt.Sprintf("📋 Email queued for approval (ID: %s). To: %s, Subject: %s\nAwaiting %s approval before sending.", queueID[:8], to, subject, getApprovalMode(ctx, t.cfg.Pool, agentID)))
		}
	}

	// ── Resolve sending identity ───────────────────────────────────────────────
	// Priority: bound identity (per-agent mailbox) > global SMTP fallback.
	// The FROM address is always locked to the bound identity — the LLM cannot
	// supply an arbitrary from address (no spoofing).
	tenantID := TenantIDFromCtx(ctx)
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}

	if t.cfg.ResolveIdentity != nil && agentID != "" {
		bound, err := t.cfg.ResolveIdentity(ctx, agentID, tenantID)
		if err == nil && bound != nil {
			return t.sendViaBoundIdentity(ctx, bound, agentID, tenantID, to, subject, body)
		}
		// No bound identity — fall through to global SMTP if available.
	}

	// ── Global SMTP fallback ───────────────────────────────────────────────────
	if t.cfg.SMTP == nil {
		return ErrorResult("no mailbox bound to this agent and no global SMTP configured — cannot send")
	}
	return t.sendViaGlobalSMTP(ctx, agentID, tenantID, to, subject, body)
}

// sendViaBoundIdentity sends mail as the agent's own locked identity.
// The from address is identity.Address — it cannot be overridden.
func (t *EmailSendTool) sendViaBoundIdentity(ctx context.Context, bound *BoundIdentity, agentID, tenantID, to, subject, body string) *Result {
	// Apply identity signature.
	bodyText := body
	if bound.SignatureText != "" {
		bodyText = bodyText + "\r\n\r\n--\r\n" + bound.SignatureText
	}

	msgID := fmt.Sprintf("<%d.qorven@%s>", time.Now().UnixNano(), domainOf(bound.Address))

	var sendErr error
	if bound.Transport == "external" && bound.ForwardURL != "" {
		payload := fmt.Sprintf(`{"message_id":%q,"from":%q,"from_name":%q,"to":[%q],"subject":%q,"body_text":%q}`,
			msgID, bound.Address, bound.DisplayName, to, subject, bodyText)
		resp, err := http.Post(bound.ForwardURL, "application/json", strings.NewReader(payload)) //nolint:noctx
		if err != nil {
			sendErr = err
		} else {
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				sendErr = fmt.Errorf("forward webhook returned %d", resp.StatusCode)
			}
		}
	} else {
		// Native SMTP — from is locked to bound.Address (cannot be forged).
		addr := fmt.Sprintf("%s:%d", bound.SMTPHost, bound.SMTPPort)
		auth := smtp.PlainAuth("", bound.SMTPUser, bound.SMTPPass, bound.SMTPHost)
		var rawMsg strings.Builder
		rawMsg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", sanitizeEmailHeader(bound.DisplayName), sanitizeEmailHeader(bound.Address)))
		rawMsg.WriteString(fmt.Sprintf("To: %s\r\n", sanitizeEmailHeader(to)))
		rawMsg.WriteString(fmt.Sprintf("Subject: %s\r\n", sanitizeEmailHeader(subject)))
		rawMsg.WriteString(fmt.Sprintf("Message-ID: %s\r\n", msgID))
		if bound.ReplyTo != "" {
			rawMsg.WriteString(fmt.Sprintf("Reply-To: %s\r\n", sanitizeEmailHeader(bound.ReplyTo)))
		}
		rawMsg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		rawMsg.WriteString(bodyText + "\r\n")
		if bound.SMTPPort == 465 {
			sendErr = smtpSendTLS(addr, bound.SMTPHost, auth, bound.Address, []string{to}, rawMsg.String())
		} else {
			sendErr = smtp.SendMail(addr, auth, bound.Address, []string{to}, []byte(rawMsg.String()))
		}
	}

	if sendErr != nil {
		return ErrorResult(fmt.Sprintf("SMTP error: %v", sendErr))
	}

	// Persist to mailbox with the real identity_id and tenant.
	if t.cfg.Pool != nil && agentID != "" {
		t.cfg.Pool.Exec(ctx,
			`INSERT INTO mailbox_messages (tenant_id, agent_id, identity_id, message_id, folder, direction, from_address, from_name, to_addresses, subject, body_text, send_status, is_read, created_at)
			 VALUES ($1, $2, $3::uuid, $4, 'sent', 'outbound', $5, $6, ARRAY[$7], $8, $9, 'sent', true, NOW())`,
			tenantID, agentID, bound.IdentityID, msgID,
			bound.Address, bound.DisplayName, to, subject, bodyText)
	}

	return TextResult(fmt.Sprintf("✅ Email sent to %s — Subject: %s (sent as %s)", to, subject, bound.Address))
}

// sendViaGlobalSMTP is the legacy path used when no per-agent identity is bound.
func (t *EmailSendTool) sendViaGlobalSMTP(ctx context.Context, agentID, tenantID, to, subject, body string) *Result {
	s := t.cfg.SMTP

	msgID := fmt.Sprintf("<%d.qorven@%s>", time.Now().UnixNano(), domainOf(s.From))

	var rawMsg strings.Builder
	rawMsg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", sanitizeEmailHeader(s.FromName), sanitizeEmailHeader(s.From)))
	rawMsg.WriteString(fmt.Sprintf("To: %s\r\n", sanitizeEmailHeader(to)))
	rawMsg.WriteString(fmt.Sprintf("Subject: %s\r\n", sanitizeEmailHeader(subject)))
	rawMsg.WriteString(fmt.Sprintf("Message-ID: %s\r\n", msgID))
	rawMsg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	rawMsg.WriteString(body + "\r\n")

	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	auth := smtp.PlainAuth("", s.User, s.Password, s.Host)

	var err error
	if s.Port == 465 {
		err = smtpSendTLS(addr, s.Host, auth, s.From, []string{to}, rawMsg.String())
	} else {
		err = smtp.SendMail(addr, auth, s.From, []string{to}, []byte(rawMsg.String()))
	}
	if err != nil {
		return ErrorResult(fmt.Sprintf("SMTP error: %v", err))
	}

	if t.cfg.Pool != nil && agentID != "" {
		t.cfg.Pool.Exec(ctx,
			`INSERT INTO mailbox_messages (tenant_id, agent_id, message_id, folder, direction, from_address, from_name, to_addresses, subject, body_text, send_status, is_read, created_at)
			 VALUES ($1, $2, $3, 'sent', 'outbound', $4, $5, ARRAY[$6], $7, $8, 'sent', true, NOW())`,
			tenantID, agentID, msgID, s.From, s.FromName, to, subject, body)
	}

	return TextResult(fmt.Sprintf("✅ Email sent to %s — Subject: %s (tracked in sent folder)", to, subject))
}

// domainOf extracts the domain from an email address, falling back to "qorven.ai".
func domainOf(addr string) string {
	if idx := strings.LastIndex(addr, "@"); idx >= 0 {
		return addr[idx+1:]
	}
	return "qorven.ai"
}

func smtpSendTLS(addr, host string, auth smtp.Auth, from string, to []string, msg string) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil { return err }
	c, err := smtp.NewClient(conn, host)
	if err != nil { return err }
	defer c.Close()
	if err := c.Auth(auth); err != nil { return err }
	if err := c.Mail(from); err != nil { return err }
	for _, t := range to {
		if err := c.Rcpt(t); err != nil { return err }
	}
	w, err := c.Data()
	if err != nil { return err }
	w.Write([]byte(msg))
	w.Close()
	return c.Quit()
}

// --- Email Read Tool ---

type EmailReadTool struct{ cfg *MailboxConfig }

func NewEmailReadTool() *EmailReadTool { return &EmailReadTool{} }
func (t *EmailReadTool) SetMailbox(cfg *MailboxConfig) { t.cfg = cfg }
func (t *EmailReadTool) SetIMAP(cfg *IMAPConfig) {
	if t.cfg == nil { t.cfg = &MailboxConfig{} }
	t.cfg.IMAP = cfg
}

func (t *EmailReadTool) Name() string        { return "email_read" }
func (t *EmailReadTool) Description() string  { return "Check email inbox. Returns subject, from, date, and flags suspicious replies." }
func (t *EmailReadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"limit": map[string]any{"type": "integer", "description": "Max messages (default 5)"},
		},
	}
}

func (t *EmailReadTool) Execute(ctx context.Context, args map[string]any) *Result {
	if t.cfg == nil || t.cfg.IMAP == nil {
		return ErrorResult("IMAP not configured")
	}
	limit := 5
	if l, ok := args["limit"].(float64); ok && l > 0 { limit = int(l) }

	im := t.cfg.IMAP
	addr := fmt.Sprintf("%s:%d", im.Host, im.Port)
	c, err := imapclient.DialTLS(addr, &imapclient.Options{})
	if err != nil { return ErrorResult(fmt.Sprintf("IMAP connect: %v", err)) }
	defer c.Close()

	if err := c.Login(im.User, im.Password).Wait(); err != nil {
		return ErrorResult(fmt.Sprintf("IMAP login: %v", err))
	}
	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		return ErrorResult(fmt.Sprintf("IMAP select: %v", err))
	}

	criteria := &imap.SearchCriteria{Since: time.Now().Add(-7 * 24 * time.Hour)}
	searchData, err := c.Search(criteria, nil).Wait()
	if err != nil { return ErrorResult(fmt.Sprintf("IMAP search: %v", err)) }

	ids := searchData.AllSeqNums()
	if len(ids) == 0 { return TextResult("📧 Inbox empty (last 7 days)") }

	start := 0
	if len(ids) > limit { start = len(ids) - limit }

	seqSet := new(imap.SeqSet)
	for _, id := range ids[start:] { seqSet.AddNum(id) }

	fetchCmd := c.Fetch(*seqSet, &imap.FetchOptions{Envelope: true})

	var results []string
	for {
		msg := fetchCmd.Next()
		if msg == nil { break }
		buf, err := msg.Collect()
		if err != nil || buf.Envelope == nil { continue }

		env := buf.Envelope
		from := ""
		if len(env.From) > 0 { from = env.From[0].Addr() }
		date := ""
		if !env.Date.IsZero() { date = env.Date.Format("Jan 2 15:04") }

		// Thread verification: check if In-Reply-To matches our sent messages
		trustFlag := ""
		if len(env.InReplyTo) > 0 && t.cfg.Pool != nil && AgentIDFromCtx(ctx) != "" {
			var count int
			t.cfg.Pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM mailbox_messages WHERE agent_id = $1 AND message_id = $2 AND direction = 'outbound'`,
				AgentIDFromCtx(ctx), env.InReplyTo[0]).Scan(&count)
			if count == 0 {
				trustFlag = " ⚠️ UNVERIFIED REPLY (no matching sent message)"
			} else {
				trustFlag = " ✅ verified reply"
			}
		}

		entry := fmt.Sprintf("From: %s | Subject: %s | Date: %s%s", from, env.Subject, date, trustFlag)

		// Store in mailbox (inbox folder)
		agentID := AgentIDFromCtx(ctx)
	if t.cfg.Pool != nil && agentID != "" {
			inReplyTo := ""
			if len(env.InReplyTo) > 0 { inReplyTo = env.InReplyTo[0] }
			t.cfg.Pool.Exec(ctx,
				`INSERT INTO mailbox_messages (tenant_id, agent_id, message_id, in_reply_to, folder, direction, from_address, subject, is_read, created_at)
				 VALUES ('00000000-0000-0000-0000-000000000001', $1, $2, $3, 'inbox', 'inbound', $4, $5, false, NOW())
				 ON CONFLICT DO NOTHING`,
				AgentIDFromCtx(ctx), env.MessageID, inReplyTo, from, env.Subject)
		}

		results = append(results, entry)
	}

	if len(results) == 0 { return TextResult("📧 No messages found") }
	return TextResult(fmt.Sprintf("📧 Inbox (%d messages):\n\n%s", len(results), strings.Join(results, "\n")))
}

func getApprovalMode(ctx context.Context, pool *pgxpool.Pool, agentID string) string {
	var mode string
	pool.QueryRow(ctx, `SELECT COALESCE(outbound_approval, 'supervisor') FROM agents WHERE id = $1`, agentID).Scan(&mode)
	if mode == "" { return "supervisor" }
	return mode
}
