// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
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

// Send delivers an email via the identity's SMTP server.
func (s *SMTPSender) Send(identity *Identity, smtpPass string, to []string, subject, bodyText, bodyHTML string) error {
	if identity.SMTPHost == "" || smtpPass == "" {
		return fmt.Errorf("SMTP not configured for %s", identity.Address)
	}

	addr := fmt.Sprintf("%s:%d", identity.SMTPHost, identity.SMTPPort)
	auth := smtp.PlainAuth("", identity.SMTPUser, smtpPass, identity.SMTPHost)

	// Build RFC 5322 message
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", identity.DisplayName, identity.Address))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))

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
		return s.sendTLS(addr, identity.SMTPHost, auth, identity.Address, to, msg.String())
	}
	err := smtp.SendMail(addr, auth, identity.Address, to, []byte(msg.String()))
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
