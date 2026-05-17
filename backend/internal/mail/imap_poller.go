// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// AgentTrigger is called when a new email arrives and should trigger the Soul.
type AgentTrigger func(ctx context.Context, agentID, sessionID, emailContent, subject, from string)

// IMAPPoller polls UNSEEN messages from IMAP and routes them through the Router.
type IMAPPoller struct {
	store        *Store
	router       *Router
	agentTrigger AgentTrigger
}

func NewIMAPPoller(store *Store, router *Router) *IMAPPoller {
	return &IMAPPoller{store: store, router: router}
}

// SetAgentTrigger wires the callback that wakes a Soul when mail arrives.
func (p *IMAPPoller) SetAgentTrigger(fn AgentTrigger) { p.agentTrigger = fn }

// PollIdentity connects to IMAP, fetches UNSEEN messages, routes each, marks as seen.
func (p *IMAPPoller) PollIdentity(ctx context.Context, tenantID string, id *Identity, imapPass string) error {
	if id.IMAPHost == "" || imapPass == "" {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", id.IMAPHost, id.IMAPPort)
	c, err := imapclient.DialTLS(addr, &imapclient.Options{
		TLSConfig: &tls.Config{ServerName: id.IMAPHost},
	})
	if err != nil {
		return fmt.Errorf("imap dial %s: %w", addr, err)
	}
	defer c.Close()

	if err := c.Login(id.IMAPUser, imapPass).Wait(); err != nil {
		return fmt.Errorf("imap login: %w", err)
	}

	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		return fmt.Errorf("imap select: %w", err)
	}

	// Search UNSEEN
	criteria := &imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}
	searchData, err := c.Search(criteria, nil).Wait()
	if err != nil {
		return fmt.Errorf("imap search: %w", err)
	}

	seqNums := searchData.AllSeqNums()
	if len(seqNums) == 0 {
		return nil
	}

	seqSet := imap.SeqSetNum(seqNums...)
	fetchOptions := &imap.FetchOptions{
		Envelope: true,
		BodySection: []*imap.FetchItemBodySection{
			{Specifier: imap.PartSpecifierText},
		},
	}

	fetchCmd := c.Fetch(seqSet, fetchOptions)
	defer fetchCmd.Close()

	count := 0
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		var from, subject, messageID, bodyText string
		toAddrs := []string{}
		var inReplyTo string

		for {
			item := msg.Next()
			if item == nil {
				break
			}
			switch d := item.(type) {
			case imapclient.FetchItemDataEnvelope:
				if d.Envelope != nil {
					subject = d.Envelope.Subject
					messageID = d.Envelope.MessageID
					if len(d.Envelope.InReplyTo) > 0 {
						inReplyTo = d.Envelope.InReplyTo[0]
					}
					if len(d.Envelope.From) > 0 {
						from = d.Envelope.From[0].Addr()
					}
					// Fix #2: Read actual To: addresses for plus-addressing
					for _, addr := range d.Envelope.To {
						toAddrs = append(toAddrs, addr.Addr())
					}
				}
			case imapclient.FetchItemDataBodySection:
				data, _ := io.ReadAll(d.Literal)
				raw := string(data)
				// Fix #3: Handle multi-part MIME — extract plain text or strip HTML
				bodyText = extractPlainText(raw)
			}
		}

		if from == "" || messageID == "" {
			continue
		}
		// Fallback: if no To addresses parsed, use identity address
		if len(toAddrs) == 0 {
			toAddrs = []string{id.Address}
		}

		p.router.Route(ctx, tenantID, from, "", subject, bodyText, "", messageID, inReplyTo, toAddrs)

		// Fix #5: Trigger agent loop when mail arrives
		if p.agentTrigger != nil && id.AgentID != nil {
			go p.agentTrigger(ctx, *id.AgentID, "", bodyText, subject, from)
		}

		count++
	}

	// Mark all fetched as seen
	if count > 0 {
		storeCmd := c.Store(seqSet, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagSeen}}, nil)
		storeCmd.Close()
		slog.Info("imap.polled", "identity", id.Address, "new_messages", count)
	}

	return nil
}

// StartIDLE connects to IMAP and uses IDLE to get push notifications.
// Falls back to polling if IDLE is not supported.
func (p *IMAPPoller) StartIDLE(ctx context.Context, tenantID string, id *Identity, imapPass string) {
	if id.IMAPHost == "" || imapPass == "" {
		return
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			err := p.idleLoop(ctx, tenantID, id, imapPass)
			if err != nil {
				slog.Warn("imap.idle.error", "identity", id.Address, "error", err, "retry_in", "10s")
				time.Sleep(10 * time.Second)
			}
		}
	}()
}

func (p *IMAPPoller) idleLoop(ctx context.Context, tenantID string, id *Identity, imapPass string) error {
	addr := fmt.Sprintf("%s:%d", id.IMAPHost, id.IMAPPort)
	c, err := imapclient.DialTLS(addr, &imapclient.Options{
		TLSConfig: &tls.Config{ServerName: id.IMAPHost},
	})
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Login(id.IMAPUser, imapPass).Wait(); err != nil {
		return err
	}
	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		return err
	}

	slog.Info("imap.idle.started", "identity", id.Address)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Start IDLE — server will push when new mail arrives
		idleCmd, err := c.Idle()
		if err != nil {
			// IDLE not supported — fall back to polling
			slog.Warn("imap.idle.not_supported", "identity", id.Address, "fallback", "polling")
			return p.pollFallback(ctx, tenantID, id, imapPass)
		}

		// Wait for server notification or timeout (29 min — RFC recommends < 30 min)
		timer := time.NewTimer(29 * time.Minute)
		select {
		case <-ctx.Done():
			timer.Stop()
			idleCmd.Close()
			return nil
		case <-timer.C:
			// Refresh IDLE connection
		}
		timer.Stop()

		if err := idleCmd.Close(); err != nil {
			return err
		}

		// New mail notification received — fetch UNSEEN
		p.PollIdentity(ctx, tenantID, id, imapPass)
	}
}

func (p *IMAPPoller) pollFallback(ctx context.Context, tenantID string, id *Identity, imapPass string) error {
	interval := time.Duration(id.PollInterval) * time.Second
	if interval < 15*time.Second {
		interval = 30 * time.Second
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
			p.PollIdentity(ctx, tenantID, id, imapPass)
		}
	}
}

// StartPolling runs a background loop polling all active IMAP identities.
// Uses IDLE when available, falls back to polling.
func (p *IMAPPoller) StartPolling(ctx context.Context, tenantID string, getPassword func(identityID string) string) {
	go func() {
		identities, err := p.store.ListIdentities(ctx, tenantID)
		if err != nil {
			slog.Warn("imap.start.error", "error", err)
			return
		}
		for _, id := range identities {
			if id.IMAPHost == "" || !id.IsActive {
				continue
			}
			pass := getPassword(id.ID)
			if pass == "" {
				continue
			}
			p.StartIDLE(ctx, tenantID, &id, pass)
		}
	}()
}

// extractPlainText handles multi-part MIME — extracts text/plain or strips HTML.
func extractPlainText(raw string) string {
	// Simple approach: if it looks like HTML, strip tags
	if strings.Contains(raw, "<html") || strings.Contains(raw, "<body") || strings.Contains(raw, "<div") {
		return stripHTML(raw)
	}
	return strings.TrimSpace(raw)
}

func stripHTML(html string) string {
	var result strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			result.WriteRune(' ')
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	// Clean up whitespace
	text := result.String()
	lines := strings.Split(text, "\n")
	clean := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	return strings.Join(clean, "\n")
}
