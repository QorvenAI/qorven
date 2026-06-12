// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package mail

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

// SMTPSender sends emails via SMTP with TLS.
type SMTPSender struct{}

func NewSMTPSender() *SMTPSender { return &SMTPSender{} }

// SendOptions carries optional per-send overrides beyond the identity defaults.
type SendOptions struct {
	// Cc / Bcc are additional recipients.  Cc appears in headers; Bcc is
	// delivered to the SMTP server but not written into any header.
	Cc  []string
	Bcc []string
	// ReplyTo overrides the identity-level Reply-To if non-empty.
	ReplyTo string
	// Importance overrides the identity-level default_importance if non-empty.
	// Accepted values: "high", "normal", "low".
	Importance string
}

// Send delivers an email via the identity's SMTP server.
// opt may be nil; supply it to set Cc, Bcc, ReplyTo or Importance.
func (s *SMTPSender) Send(identity *Identity, smtpPass string, to []string, subject, bodyText, bodyHTML string, opt *SendOptions) error {
	if identity.SMTPHost == "" || smtpPass == "" {
		return fmt.Errorf("SMTP not configured for %s", identity.Address)
	}
	if opt == nil {
		opt = &SendOptions{}
	}

	addr := fmt.Sprintf("%s:%d", identity.SMTPHost, identity.SMTPPort)
	auth := smtp.PlainAuth("", identity.SMTPUser, smtpPass, identity.SMTPHost)

	// Resolve Reply-To: per-send override > identity default.
	replyTo := opt.ReplyTo
	if replyTo == "" {
		replyTo = identity.ReplyTo
	}

	// Resolve Importance: per-send override > identity default.
	importance := opt.Importance
	if importance == "" {
		importance = identity.DefaultImportance
	}

	// SMTP envelope = To + Cc + Bcc (all must receive the message).
	envelope := make([]string, 0, len(to)+len(opt.Cc)+len(opt.Bcc))
	envelope = append(envelope, to...)
	envelope = append(envelope, opt.Cc...)
	envelope = append(envelope, opt.Bcc...)

	// Build RFC 5322 message
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", identity.DisplayName, identity.Address))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
	if len(opt.Cc) > 0 {
		msg.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(opt.Cc, ", ")))
	}
	// Bcc is intentionally omitted from headers.
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	if replyTo != "" {
		msg.WriteString(fmt.Sprintf("Reply-To: %s\r\n", replyTo))
	}
	if importance == "high" {
		msg.WriteString("Importance: high\r\n")
		msg.WriteString("X-Priority: 1\r\n")
	} else if importance == "low" {
		msg.WriteString("Importance: low\r\n")
		msg.WriteString("X-Priority: 5\r\n")
	}

	if bodyHTML != "" {
		msg.WriteString("MIME-Version: 1.0\r\n")
		msg.WriteString("Content-Type: multipart/alternative; boundary=qorven-boundary\r\n\r\n")
		msg.WriteString("--qorven-boundary\r\n")
		msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		msg.WriteString(bodyText + "\r\n")
		msg.WriteString("--qorven-boundary\r\n")
		msg.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
		msg.WriteString(bodyHTML + "\r\n")
		msg.WriteString("--qorven-boundary--\r\n")
	} else {
		msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
		msg.WriteString(bodyText + "\r\n")
	}

	// Try STARTTLS on port 587, direct TLS on 465
	if identity.SMTPPort == 465 {
		return s.sendTLS(addr, identity.SMTPHost, auth, identity.Address, envelope, msg.String())
	}
	err := smtp.SendMail(addr, auth, identity.Address, envelope, []byte(msg.String()))
	if err != nil {
		slog.Warn("smtp.send.error", "identity", identity.Address, "error", err)
	} else {
		slog.Info("smtp.sent", "identity", identity.Address, "to", to, "subject", subject)
	}
	return err
}

func (s *SMTPSender) sendTLS(addr, host string, auth smtp.Auth, from string, to []string, msg string) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Auth(auth); err != nil {
		return err
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, t := range to {
		if err := c.Rcpt(t); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	w.Write([]byte(msg))
	w.Close()
	return c.Quit()
}
