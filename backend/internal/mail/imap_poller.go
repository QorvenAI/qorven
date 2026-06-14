// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// AgentTrigger is called when a new email arrives and should trigger the Soul.
// inReplyTo is the RFC 2822 In-Reply-To header value (may be empty for new threads).
// authResults is the Authentication-Results header value from the mail provider.
// Both are passed so the gateway can build anti-fabrication context via
// BuildVerifiedContext before routing to the agent brain.
type AgentTrigger func(ctx context.Context, agentID, sessionID, emailContent, subject, from, inReplyTo, authResults string)

// IMAPPoller polls UNSEEN messages from IMAP and routes them through the Router.
type IMAPPoller struct {
	store        *Store
	router       *Router
	agentTrigger AgentTrigger

	// running tracks the (cancel func, generation counter) for each identity's
	// IDLE goroutine so that AddIdentity can stop a stale goroutine before
	// launching a replacement and cleanup defers can avoid stomping a newer entry.
	// gen is a monotonically increasing counter; each launch gets its own value.
	mu      sync.Mutex
	running map[string]struct {
		cancel context.CancelFunc
		gen    uint64
	}
	genCounter atomic.Uint64
}

func NewIMAPPoller(store *Store, router *Router) *IMAPPoller {
	return &IMAPPoller{
		store:  store,
		router: router,
		running: make(map[string]struct {
			cancel context.CancelFunc
			gen    uint64
		}),
	}
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
	// Fetch envelope (From/Subject/MessageID/InReplyTo), body text, and the
	// Authentication-Results header so we can build anti-fabrication context.
	authHeaderSection := &imap.FetchItemBodySection{
		Specifier:    imap.PartSpecifierHeader,
		HeaderFields: []string{"Authentication-Results"},
	}
	fetchOptions := &imap.FetchOptions{
		Envelope: true,
		BodySection: []*imap.FetchItemBodySection{
			{Specifier: imap.PartSpecifierText},
			authHeaderSection,
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

		var from, subject, messageID, bodyText, authResults string
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
				if d.Section != nil && d.Section.Specifier == imap.PartSpecifierHeader {
					// Authentication-Results header block
					authResults = extractHeader(raw, "Authentication-Results")
				} else {
					// Fix #3: Handle multi-part MIME — extract plain text or strip HTML
					bodyText = extractPlainText(raw)
				}
			}
		}

		if from == "" || messageID == "" {
			continue
		}
		// Fallback: if no To addresses parsed, use identity address
		if len(toAddrs) == 0 {
			toAddrs = []string{id.Address}
		}

		targets, _ := p.router.RouteAndResolve(ctx, tenantID, from, "", subject, bodyText, "", messageID, inReplyTo, toAddrs)

		// Fire the agent brain for every resolved target — covers both
		// dedicated identities (one target = their agent) and shared-mailbox
		// / alias mail (one or more targets = the mapped agents).
		// Pass inReplyTo and authResults so the gateway can build full
		// anti-fabrication context (BuildVerifiedContext) before routing.
		if p.agentTrigger != nil {
			for _, t := range targets {
				agentID := t.AgentID
				go p.agentTrigger(ctx, agentID, "", bodyText, subject, from, inReplyTo, authResults)
			}
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

// startIDLETracked is the internal launcher used by both StartPolling and
// AddIdentity.  It creates a child context derived from parentCtx, registers
// its cancel func in p.running[identityID] (so AddIdentity can stop a stale
// goroutine before launching a replacement), and launches the retry goroutine.
// Callers MUST NOT hold p.mu when calling this function.
func (p *IMAPPoller) startIDLETracked(parentCtx context.Context, tenantID string, id *Identity, imapPass string) {
	idCtx, cancel := context.WithCancel(parentCtx)
	gen := p.genCounter.Add(1)

	p.mu.Lock()
	// If a goroutine is already running for this identity, cancel it first so we
	// don't leak goroutines (e.g. when credentials are refreshed via update).
	if old, ok := p.running[id.ID]; ok {
		old.cancel()
	}
	p.running[id.ID] = struct {
		cancel context.CancelFunc
		gen    uint64
	}{cancel, gen}
	p.mu.Unlock()

	go func() {
		defer func() {
			// Remove from the map when the goroutine exits, but only if this
			// goroutine is still the current owner.  A newer launch may have
			// already replaced the entry.
			p.mu.Lock()
			if entry, ok := p.running[id.ID]; ok && entry.gen == gen {
				delete(p.running, id.ID)
			}
			p.mu.Unlock()
		}()

		for {
			select {
			case <-idCtx.Done():
				return
			default:
			}

			err := p.idleLoop(idCtx, tenantID, id, imapPass)
			if err != nil {
				slog.Warn("imap.idle.error", "identity", id.Address, "error", err, "retry_in", "10s")
				select {
				case <-idCtx.Done():
					return
				case <-time.After(10 * time.Second):
				}
			}
		}
	}()
}

// StartIDLE connects to IMAP and uses IDLE to get push notifications.
// Falls back to polling if IDLE is not supported.
// Deprecated: prefer startIDLETracked for internal use; StartIDLE is kept for
// any existing external callers that don't need cancel tracking.
func (p *IMAPPoller) StartIDLE(ctx context.Context, tenantID string, id *Identity, imapPass string) {
	if id.IMAPHost == "" || imapPass == "" {
		return
	}
	p.startIDLETracked(ctx, tenantID, id, imapPass)
}

// AddIdentity starts polling for a newly-created mail identity without
// requiring a server restart.  It is safe to call from any goroutine
// (including an HTTP handler).  If a goroutine is already running for the
// given identity it is replaced (idempotent / restart-safe).
//
// ctx must be a long-lived context — use context.Background() from the
// call-site, NOT the HTTP request context, because the request context is
// cancelled as soon as the response is sent.
func (p *IMAPPoller) AddIdentity(ctx context.Context, tenantID string, id *Identity, imapPass string) {
	if id.IMAPHost == "" || imapPass == "" {
		return
	}
	if !id.IsActive {
		return
	}
	slog.Info("imap.poller.hot_add", "identity", id.Address)
	p.startIDLETracked(ctx, tenantID, id, imapPass)
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
			p.startIDLETracked(ctx, tenantID, &id, pass)
		}
	}()
}

// extractHeader parses a raw IMAP header block and returns the value of the
// named header field (case-insensitive).  Used to extract Authentication-Results
// from the header-only body section fetched for anti-fabrication context.
func extractHeader(raw, name string) string {
	lower := strings.ToLower(name) + ":"
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(strings.ToLower(line), lower) {
			return strings.TrimSpace(line[len(lower):])
		}
	}
	return ""
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
