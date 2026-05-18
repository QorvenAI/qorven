// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"crypto/tls"
	"fmt"
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

type MailboxConfig struct {
	SMTP    *SMTPConfig
	IMAP    *IMAPConfig
	AgentID string
	Pool    *pgxpool.Pool
}

// --- Email Send Tool ---

type EmailSendTool struct{ cfg *MailboxConfig }

func NewEmailSendTool() *EmailSendTool { return &EmailSendTool{} }
func (t *EmailSendTool) SetMailbox(cfg *MailboxConfig) { t.cfg = cfg }
func (t *EmailSendTool) SetSMTP(cfg *SMTPConfig) {
	if t.cfg == nil { t.cfg = &MailboxConfig{} }
	t.cfg.SMTP = cfg
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
	if t.cfg == nil || t.cfg.SMTP == nil {
		return ErrorResult("SMTP not configured")
	}

	agentID := AgentIDFromCtx(ctx)
	sessionID := SessionIDFromCtx(ctx)
	// Outbound approval gate
	
	if t.cfg.Pool != nil && agentID != "" {
		proceed, queueID, _ := CheckApproval(ctx, t.cfg.Pool, agentID, "email_send", sessionID, args)
		if !proceed {
			return TextResult(fmt.Sprintf("📋 Email queued for approval (ID: %s). To: %s, Subject: %s\nAwaiting %s approval before sending.", queueID[:8], to, subject, getApprovalMode(ctx, t.cfg.Pool, agentID)))
		}
	}
	s := t.cfg.SMTP

	// Generate message ID for thread tracking
	msgID := fmt.Sprintf("<%d.qorven@%s>", time.Now().UnixNano(), strings.Split(s.From, "@")[1])

	// Build message
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", sanitizeEmailHeader(s.FromName), sanitizeEmailHeader(s.From)))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", sanitizeEmailHeader(to)))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", sanitizeEmailHeader(subject)))
	msg.WriteString(fmt.Sprintf("Message-ID: %s\r\n", msgID))
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	msg.WriteString(body + "\r\n")

	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	auth := smtp.PlainAuth("", s.User, s.Password, s.Host)

	var err error
	if s.Port == 465 {
		err = smtpSendTLS(addr, s.Host, auth, s.From, []string{to}, msg.String())
	} else {
		err = smtp.SendMail(addr, auth, s.From, []string{to}, []byte(msg.String()))
	}
	if err != nil {
		return ErrorResult(fmt.Sprintf("SMTP error: %v", err))
	}

	// Store in mailbox (sent folder) for thread tracking
	
	if t.cfg.Pool != nil && agentID != "" {
		t.cfg.Pool.Exec(ctx,
			`INSERT INTO mailbox_messages (tenant_id, agent_id, message_id, folder, direction, from_address, from_name, to_addresses, subject, body_text, send_status, is_read, created_at)
			 VALUES ('00000000-0000-0000-0000-000000000001', $1, $2, 'sent', 'outbound', $3, $4, ARRAY[$5], $6, $7, 'sent', true, NOW())`,
			agentID, msgID, s.From, s.FromName, to, subject, body)
	}

	return TextResult(fmt.Sprintf("✅ Email sent to %s — Subject: %s (tracked in sent folder)", to, subject))
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
