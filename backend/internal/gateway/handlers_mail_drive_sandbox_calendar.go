// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/calendar"
	"github.com/qorvenai/qorven/internal/channels"
	"github.com/qorvenai/qorven/internal/drive"
	"github.com/qorvenai/qorven/internal/mail"
)

// isLoopbackRequest reports whether the request's TCP peer is the loopback
// interface. It checks RemoteAddr (the real socket peer), not X-Forwarded-For,
// which a client could spoof — a reverse proxy on the same host connects from
// loopback, so this correctly admits the documented localhost-proxy setup while
// rejecting direct network requests.
func isLoopbackRequest(r *http.Request) bool {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// safeGo runs fn in a goroutine with panic recovery so a panic in detached
// background work (inbound mail processing, etc.) logs instead of crashing the
// process. label identifies the work site in the recovery log.
func safeGo(label string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("safego.panic", "label", label, "panic", r)
			}
		}()
		fn()
	}()
}

func (gw *Gateway) handleListMailIdentities(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	ids, err := gw.mailStore.ListIdentities(r.Context(), defaultTenant)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(ids)
}

func (gw *Gateway) handleCreateMailIdentity(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	var body struct {
		AgentID     string `json:"agent_id"`
		Address     string `json:"address"`
		DisplayName string `json:"display_name"`
		Type        string `json:"identity_type"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Type == "" {
		body.Type = "dedicated"
	}
	id, err := gw.mailStore.CreateIdentity(r.Context(), defaultTenant, body.AgentID, body.Address, body.DisplayName, body.Type)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Newly-created identities won't have IMAP credentials yet (they are set via
	// the update endpoint), so AddIdentity is typically a no-op here. We call it
	// defensively so any future schema changes that do include IMAP at create time
	// are automatically polled without a restart.
	if gw.mailPoller != nil {
		imapPass, _ := gw.mailStore.IdentityIMAPPass(context.Background(), id.ID, gw.cfg.Auth.EncryptionKey)
		gw.mailPoller.AddIdentity(context.Background(), defaultTenant, id, imapPass)
	}

	json.NewEncoder(w).Encode(id)
}

func (gw *Gateway) handleUpdateMailIdentity(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		// Core connection fields.
		DisplayName  string `json:"display_name"`
		SMTPHost     string `json:"smtp_host"`
		SMTPPort     int    `json:"smtp_port"`
		SMTPUser     string `json:"smtp_user"`
		SMTPPass     string `json:"smtp_pass"` // plain — encrypted before storage
		IMAPHost     string `json:"imap_host"`
		IMAPPort     int    `json:"imap_port"`
		IMAPUser     string `json:"imap_user"`
		IMAPPass     string `json:"imap_pass"` // plain — encrypted before storage
		PollInterval int    `json:"poll_interval_seconds"`
		// Extended fields (Task 10).
		IsActive          *bool  `json:"is_active"`           // pointer — omitted ≠ false
		Transport         string `json:"transport"`
		ForwardURL        string `json:"forward_url"`
		ReplyTo           string `json:"reply_to"`
		DefaultImportance string `json:"default_importance"`
		SignatureHTML     string `json:"signature_html"`
		SignatureText     string `json:"signature_text"`
		InboundSecret     string `json:"inbound_secret"` // plain — encrypted before storage
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}

	encKey := gw.cfg.Auth.EncryptionKey

	// Encrypt secret fields when provided; empty string = leave existing.
	smtpPassEnc, err := mailEncryptIfProvided(body.SMTPPass, encKey)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to encrypt smtp password"})
		return
	}
	imapPassEnc, err := mailEncryptIfProvided(body.IMAPPass, encKey)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to encrypt imap password"})
		return
	}
	inboundSecretEnc, err := mailEncryptIfProvided(body.InboundSecret, encKey)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to encrypt inbound secret"})
		return
	}

	// Build a dynamic SET clause so we only overwrite encrypted-pass columns when
	// a new plaintext was supplied, and only touch is_active when the caller
	// explicitly included the field (pointer non-nil check).
	setClauses := []string{
		"display_name=$1",
		"smtp_host=$2",
		"smtp_port=$3",
		"smtp_user=$4",
		"imap_host=$5",
		"imap_port=$6",
		"imap_user=$7",
		"poll_interval_seconds=$8",
		"transport=$9",
		"forward_url=$10",
		"reply_to=$11",
		"default_importance=$12",
		"signature_html=$13",
		"signature_text=$14",
	}
	args := []any{
		body.DisplayName, body.SMTPHost, body.SMTPPort, body.SMTPUser,
		body.IMAPHost, body.IMAPPort, body.IMAPUser, body.PollInterval,
		body.Transport, body.ForwardURL, body.ReplyTo, body.DefaultImportance,
		body.SignatureHTML, body.SignatureText,
	}
	nextParam := func() string {
		n := len(args) + 1
		return fmt.Sprintf("$%d", n)
	}

	if smtpPassEnc != "" {
		setClauses = append(setClauses, "smtp_pass_enc="+nextParam())
		args = append(args, smtpPassEnc)
	}
	if imapPassEnc != "" {
		setClauses = append(setClauses, "imap_pass_enc="+nextParam())
		args = append(args, imapPassEnc)
	}
	if inboundSecretEnc != "" {
		setClauses = append(setClauses, "inbound_secret_enc="+nextParam())
		args = append(args, inboundSecretEnc)
	}
	if body.IsActive != nil {
		setClauses = append(setClauses, "is_active="+nextParam())
		args = append(args, *body.IsActive)
	}

	// Append WHERE clause params.
	idParam := nextParam()
	args = append(args, id)
	tenantParam := nextParam()
	args = append(args, defaultTenant)

	setSQL := ""
	for i, c := range setClauses {
		if i > 0 {
			setSQL += ", "
		}
		setSQL += c
	}
	q := "UPDATE soul_mail_identities SET " + setSQL + " WHERE id=" + idParam + " AND tenant_id=" + tenantParam
	if _, err := gw.mailStore.Pool().Exec(r.Context(), q, args...); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}

	// If IMAP credentials changed, (re-)start the IDLE goroutine for this
	// identity immediately so the new mailbox is polled without a restart.
	// Use context.Background() — not the request context — because the poll
	// loop must outlive the HTTP response.
	if gw.mailPoller != nil && body.IMAPHost != "" {
		updatedID, err := gw.mailStore.GetIdentity(r.Context(), id)
		if err == nil {
			imapPass, _ := gw.mailStore.IdentityIMAPPass(context.Background(), id, encKey)
			gw.mailPoller.AddIdentity(context.Background(), defaultTenant, updatedID, imapPass)
		} else {
			slog.Warn("mail.poller.hot_add.fetch_failed", "identity_id", id, "error", err)
		}
	}

	writeJSON(w, 200, map[string]string{"ok": "true"})
}

// ─── Aliases ──────────────────────────────────────────────────────────────────

type aliasRow struct {
	ID            string `json:"id"`
	AliasAddress  string `json:"alias_address"`
	TargetAgentID string `json:"target_agent_id"`
	CanSendAs     bool   `json:"can_send_as"`
	CanReceive    bool   `json:"can_receive"`
}

func (gw *Gateway) handleListMailAliases(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, alias_address, target_agent_id, can_send_as, can_receive
		 FROM mail_aliases WHERE tenant_id = $1 ORDER BY alias_address`, defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()
	result := []aliasRow{}
	for rows.Next() {
		var a aliasRow
		rows.Scan(&a.ID, &a.AliasAddress, &a.TargetAgentID, &a.CanSendAs, &a.CanReceive)
		result = append(result, a)
	}
	writeJSON(w, 200, result)
}

func (gw *Gateway) handleCreateMailAlias(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	var body struct {
		AliasAddress  string `json:"alias_address"`
		TargetAgentID string `json:"target_agent_id"`
		CanSendAs     bool   `json:"can_send_as"`
		CanReceive    bool   `json:"can_receive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AliasAddress == "" || body.TargetAgentID == "" {
		writeJSON(w, 400, map[string]string{"error": "alias_address and target_agent_id required"})
		return
	}
	var id string
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO mail_aliases (tenant_id, alias_address, target_agent_id, can_send_as, can_receive)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		defaultTenant, body.AliasAddress, body.TargetAgentID, body.CanSendAs, body.CanReceive,
	).Scan(&id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (gw *Gateway) handleDeleteMailAlias(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")
	gw.db.Pool.Exec(r.Context(), `DELETE FROM mail_aliases WHERE id = $1 AND tenant_id = $2`, id, defaultTenant)
	w.WriteHeader(204)
}

func (gw *Gateway) handleMailInbox(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	folder := r.URL.Query().Get("folder")
	msgs, err := gw.mailStore.ListInbox(r.Context(), agentID, folder, 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(msgs)
}

func (gw *Gateway) handleMailSent(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	msgs, err := gw.mailStore.ListInbox(r.Context(), agentID, "sent", 50)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(msgs)
}

func (gw *Gateway) handleGetMail(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	msg, err := gw.mailStore.GetMessage(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	gw.mailStore.MarkRead(r.Context(), msg.ID, true)
	json.NewEncoder(w).Encode(msg)
}

func (gw *Gateway) handleGetMailThread(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	msgs, err := gw.mailStore.GetThread(r.Context(), chi.URLParam(r, "thread_id"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(msgs)
}

func (gw *Gateway) handleSendMail(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}

	var body struct {
		AgentID    string   `json:"agent_id"`
		IdentityID string   `json:"identity_id"`
		To         []string `json:"to"`
		Cc         []string `json:"cc"`
		Bcc        []string `json:"bcc"`
		Subject    string   `json:"subject"`
		Body       string   `json:"body"`
		BodyHTML   string   `json:"body_html"`
		// Optional per-send overrides; identity defaults are used when absent.
		ReplyTo    string `json:"reply_to"`
		Importance string `json:"importance"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if len(body.To) == 0 {
		writeJSON(w, 400, map[string]string{"error": "at least one recipient required"})
		return
	}

	ctx := r.Context()

	// --- 1. Resolve sending identity ---
	var identity *mail.Identity
	var err error
	switch {
	case body.IdentityID != "":
		identity, err = gw.mailStore.GetIdentity(ctx, body.IdentityID)
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": "identity not found"})
			return
		}
		// Tenant ownership check.
		if identity.TenantID != defaultTenant {
			writeJSON(w, 403, map[string]string{"error": "identity belongs to a different tenant"})
			return
		}
	case body.AgentID != "":
		identity, err = gw.mailStore.GetIdentityForAgent(ctx, body.AgentID, defaultTenant)
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": "no active identity bound to agent"})
			return
		}
	default:
		writeJSON(w, 400, map[string]string{"error": "identity_id or agent_id required"})
		return
	}

	// --- 2. Apply identity signature to body ---
	bodyText := body.Body
	bodyHTML := body.BodyHTML
	if identity.SignatureText != "" {
		bodyText = bodyText + "\r\n\r\n--\r\n" + identity.SignatureText
	}
	if identity.SignatureHTML != "" {
		if bodyHTML != "" {
			bodyHTML = bodyHTML + "<br><br><div class=\"qorven-signature\">" + identity.SignatureHTML + "</div>"
		} else {
			// No HTML body provided — promote plain text to HTML if signature demands it.
			bodyHTML = "<p>" + bodyText + "</p><br><div class=\"qorven-signature\">" + identity.SignatureHTML + "</div>"
		}
	}

	msgID := fmt.Sprintf("<%d@qorven.ai>", time.Now().UnixNano())

	// --- 3. Transport branch ---
	var sendErr error
	if identity.Transport == "external" && identity.ForwardURL != "" {
		// Forward the message as JSON to the configured webhook URL (e.g. Pipedream, n8n).
		payload := map[string]any{
			"message_id": msgID,
			"from":       identity.Address,
			"from_name":  identity.DisplayName,
			"to":         body.To,
			"cc":         body.Cc,
			"bcc":        body.Bcc,
			"subject":    body.Subject,
			"body_text":  bodyText,
			"body_html":  bodyHTML,
			"reply_to":   body.ReplyTo,
			"importance": body.Importance,
		}
		payloadBytes, _ := json.Marshal(payload)
		resp, err := http.Post(identity.ForwardURL, "application/json", strings.NewReader(string(payloadBytes))) //nolint:noctx
		if err != nil {
			sendErr = err
		} else {
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				sendErr = fmt.Errorf("forward webhook returned %d", resp.StatusCode)
			}
		}
	} else {
		// Native SMTP send.
		encKey := gw.cfg.Auth.EncryptionKey
		smtpPass, err := gw.mailStore.IdentitySMTPPass(ctx, identity.ID, encKey)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "failed to retrieve SMTP credentials"})
			return
		}
		opt := &mail.SendOptions{
			Cc:         body.Cc,
			Bcc:        body.Bcc,
			ReplyTo:    body.ReplyTo,
			Importance: body.Importance,
		}
		sendErr = mail.NewSMTPSender().Send(identity, smtpPass, body.To, body.Subject, bodyText, bodyHTML, opt)
	}

	// --- 4. Persist the message ---
	// Cc recipients are stored alongside To in toAddrs so the full recipient
	// list is visible in the sent folder.  Bcc is SMTP-only (not stored).
	allVisible := append(body.To, body.Cc...)
	status := "sent"
	if sendErr != nil {
		slog.Warn("mail.send.failed", "identity", identity.ID, "error", sendErr)
		status = "failed"
	}

	msg, storeErr := gw.mailStore.StoreSend(ctx, defaultTenant, body.AgentID, identity.ID, msgID, "", body.To[0], body.Subject, bodyText, bodyHTML, status, allVisible)
	if storeErr != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(storeErr)})
		return
	}

	if sendErr != nil {
		writeJSON(w, 502, map[string]any{
			"error":   sanitizeError(sendErr),
			"message": msg,
		})
		return
	}

	writeJSON(w, 200, msg)
}

func (gw *Gateway) handleMailRead(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		w.WriteHeader(503)
		return
	}
	// Accept optional JSON body {"read": bool}; defaults to true for backward compat.
	read := true
	if r.ContentLength > 0 {
		var body struct {
			Read *bool `json:"read"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.Read != nil {
			read = *body.Read
		}
	}
	gw.mailStore.MarkRead(r.Context(), chi.URLParam(r, "id"), read)
	w.WriteHeader(204)
}

func (gw *Gateway) handleMailStar(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		w.WriteHeader(503)
		return
	}
	// Accept optional JSON body {"starred": bool}; defaults to true for backward compat.
	starred := true
	if r.ContentLength > 0 {
		var body struct {
			Starred *bool `json:"starred"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.Starred != nil {
			starred = *body.Starred
		}
	}
	gw.mailStore.MarkStarred(r.Context(), chi.URLParam(r, "id"), starred)
	w.WriteHeader(204)
}

func (gw *Gateway) handleMailInboundWebhook(w http.ResponseWriter, r *http.Request) {
	// HMAC-SHA256 signature verification — prevents anyone from injecting fake emails
	// via the webhook endpoint. Set MAIL_WEBHOOK_SECRET env var and configure your
	// mail processor (Postfix milter, Mailgun, SendGrid) to sign payloads.
	//
	// For local/VPS setups without a signing proxy: leave MAIL_WEBHOOK_SECRET empty
	// and restrict the webhook endpoint to localhost with a reverse proxy.
	webhookSecret := os.Getenv("MAIL_WEBHOOK_SECRET")
	if webhookSecret != "" {
		sig := r.Header.Get("X-Webhook-Signature")
		if sig == "" {
			sig = r.Header.Get("X-Hub-Signature-256")
		}
		if !verifyWebhookHMAC(r, webhookSecret, sig) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	} else if !isLoopbackRequest(r) {
		// No signing secret configured: this endpoint feeds the autonomous brain,
		// so it must NOT be reachable unauthenticated from the network. Accept only
		// loopback (the documented "reverse proxy on localhost" setup); reject
		// everything else so an unconfigured deployment fails closed.
		slog.Warn("mail.webhook.rejected.no_secret", "remote", r.RemoteAddr)
		http.Error(w, "webhook authentication not configured", http.StatusUnauthorized)
		return
	}

	var body struct {
		From        string   `json:"from"`
		FromName    string   `json:"from_name"`
		To          []string `json:"to"`
		Subject     string   `json:"subject"`
		BodyText    string   `json:"body_text"`
		BodyHTML    string   `json:"body_html"`
		MessageID   string   `json:"message_id"`
		InReplyTo   string   `json:"in_reply_to"`
		References  string   `json:"references"`
		AuthResults string   `json:"auth_results"` // Authentication-Results header passthrough
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.MessageID == "" {
		body.MessageID = fmt.Sprintf("<%d@inbound>", time.Now().UnixNano())
	}
	targets, _ := gw.mailRouter.RouteAndResolve(r.Context(), defaultTenant, body.From, body.FromName, body.Subject, body.BodyText, body.BodyHTML, body.MessageID, body.InReplyTo, body.To)

	// Fire the agent brain for each resolved target that wants a trigger.
	// Per-identity inbound secrets are not checked here because the global
	// MAIL_WEBHOOK_SECRET HMAC (verified above) already authenticates the
	// payload — the identity is not known until after routing resolves it.
	if gw.inbound != nil {
		rawBody := body.BodyText
		if rawBody == "" {
			rawBody = body.BodyHTML
		}
		// Resolve thread ID: prefer In-Reply-To, fall back to first References entry.
		threadID := body.InReplyTo
		if threadID == "" && body.References != "" {
			refs := strings.Fields(body.References)
			if len(refs) > 0 {
				threadID = refs[0]
			}
		}
		for _, t := range targets {
			// Build anti-fabrication context: TRUST & VERIFICATION instruction +
			// verified thread history from the DB (not from the webhook payload).
			// This is identical to what the channel path and IMAP poller produce.
			verifiedContent := mail.BuildVerifiedContext(
				r.Context(), gw.mailStore, t.AgentID, defaultTenant,
				threadID, body.From, body.FromName, body.Subject, rawBody, body.AuthResults,
			)
			// Detach from the request context — it is cancelled when this webhook
			// returns 200, which would kill the agent run mid-flight. The run is
			// wrapped so a panic in the brain cannot crash the server: this path
			// is reachable by any inbound email, so a single malformed message
			// must never take the process down.
			msg := channels.InboundMessage{
				ChannelType: "email",
				ChannelName: "mail",
				AgentID:     t.AgentID,
				SenderID:    body.From,
				SenderName:  body.FromName,
				Content:     verifiedContent,
				Subject:     body.Subject,
				ReplyTo:     body.MessageID,
				Metadata: map[string]string{
					"message_id":   body.MessageID,
					"in_reply_to":  body.InReplyTo,
					"references":   body.References,
					"auth_results": body.AuthResults,
				},
			}
			ctx := context.WithoutCancel(r.Context())
			safeGo("mail.inbound.webhook", func() { gw.inbound.Process(ctx, msg) })
		}
	}
	w.WriteHeader(200)
}

func (gw *Gateway) handleListMailApprovals(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	approvals, err := gw.mailStore.ListPendingApprovals(r.Context(), defaultTenant)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(approvals)
}

func (gw *Gateway) handleApproveMailFunc(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	ctx := r.Context()
	approvalID := chi.URLParam(r, "id")

	// 1. Flip the DB flag first (idempotent anchor — even if send fails the decision is recorded).
	if err := gw.mailStore.DecideApproval(ctx, approvalID, "approved", "user", ""); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to record approval"})
		return
	}

	// 2. Look up the linked message.
	msgDBID, err := gw.mailStore.GetApprovalMessageID(ctx, approvalID)
	if err != nil {
		slog.Warn("mail.approve.no_message", "approval", approvalID, "error", err)
		// Approval recorded; message not found — respond with flag only.
		writeJSON(w, 200, map[string]string{"status": "approved", "warning": "message not found — not sent"})
		return
	}
	msg, err := gw.mailStore.GetMessage(ctx, msgDBID)
	if err != nil {
		slog.Warn("mail.approve.load_failed", "msg", msgDBID, "error", err)
		writeJSON(w, 200, map[string]string{"status": "approved", "warning": "message load failed — not sent"})
		return
	}

	// 3. Resolve the sending identity from the message's bound identity.
	identity, err := gw.mailStore.GetIdentity(ctx, msg.IdentityID)
	if err != nil {
		slog.Warn("mail.approve.identity_missing", "identity", msg.IdentityID, "error", err)
		writeJSON(w, 200, map[string]string{"status": "approved", "warning": "identity not found — not sent"})
		return
	}

	// 4. Apply identity signature.
	bodyText := msg.BodyText
	bodyHTML := msg.BodyHTML
	if identity.SignatureText != "" {
		bodyText = bodyText + "\r\n\r\n--\r\n" + identity.SignatureText
	}
	if identity.SignatureHTML != "" {
		if bodyHTML != "" {
			bodyHTML = bodyHTML + "<br><br><div class=\"qorven-signature\">" + identity.SignatureHTML + "</div>"
		} else {
			bodyHTML = "<p>" + bodyText + "</p><br><div class=\"qorven-signature\">" + identity.SignatureHTML + "</div>"
		}
	}

	// 5. Send via the identity's transport.
	var sendErr error
	if identity.Transport == "external" && identity.ForwardURL != "" {
		payload := map[string]any{
			"message_id": msg.MessageID,
			"from":       identity.Address,
			"from_name":  identity.DisplayName,
			"to":         msg.ToAddresses,
			"subject":    msg.Subject,
			"body_text":  bodyText,
			"body_html":  bodyHTML,
		}
		payloadBytes, _ := json.Marshal(payload)
		resp, fwErr := http.Post(identity.ForwardURL, "application/json", strings.NewReader(string(payloadBytes))) //nolint:noctx
		if fwErr != nil {
			sendErr = fwErr
		} else {
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				sendErr = fmt.Errorf("forward webhook returned %d", resp.StatusCode)
			}
		}
	} else {
		encKey := gw.cfg.Auth.EncryptionKey
		smtpPass, credErr := gw.mailStore.IdentitySMTPPass(ctx, identity.ID, encKey)
		if credErr != nil {
			writeJSON(w, 500, map[string]string{"error": "failed to retrieve SMTP credentials"})
			return
		}
		sendErr = mail.NewSMTPSender().Send(identity, smtpPass, msg.ToAddresses, msg.Subject, bodyText, bodyHTML, nil)
	}

	// 6. Update send_status.
	status := "sent"
	if sendErr != nil {
		slog.Warn("mail.approve.send_failed", "identity", identity.ID, "error", sendErr)
		status = "failed"
		gw.mailStore.UpdateMessageSendStatus(ctx, msgDBID, status)
		writeJSON(w, 502, map[string]any{
			"status": "approved",
			"error":  sanitizeError(sendErr),
		})
		return
	}
	gw.mailStore.UpdateMessageSendStatus(ctx, msgDBID, status)
	writeJSON(w, 200, map[string]string{"status": "approved", "send_status": "sent"})
}

func (gw *Gateway) handleRejectMailFunc(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	gw.mailStore.DecideApproval(r.Context(), chi.URLParam(r, "id"), "rejected", "user", body.Reason)
	json.NewEncoder(w).Encode(map[string]string{"status": "rejected"})
}

// driveCaller resolves the calling identity for ACL checks: the agent id the
// request acts as (from ?agent_id=, used when a user browses an agent's drive,
// or the agent's own id when an agent tool calls), and whether the human user
// is an admin (admins read everything).
func (gw *Gateway) driveCaller(r *http.Request) (callerAgent string, isAdminUser bool) {
	callerAgent = r.URL.Query().Get("agent_id")
	if u := userFromContext(r.Context()); u != nil && u.Role == "admin" {
		isAdminUser = true
	}
	return
}

func (gw *Gateway) handleListDriveFiles(w http.ResponseWriter, r *http.Request) {
	if gw.driveStore == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	parentID := r.URL.Query().Get("parent_id")
	search := r.URL.Query().Get("q")
	var pid *string
	if parentID != "" {
		pid = &parentID
	}
	// Full-text search ignores folder hierarchy — return flat matches across all files.
	if search != "" {
		// TODO(drive-s2): ACL-filter search results by scope (names/metadata only;
		// content download is already ACL-gated). Tenant-scoped below.
		agentID := r.URL.Query().Get("agent_id")
		files, err := gw.driveStore.SearchFiles(r.Context(), defaultTenant, agentID, search)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(files)
		return
	}
	callerAgent, isAdmin := gw.driveCaller(r)
	files, err := gw.driveStore.ListVisible(r.Context(), defaultTenant, callerAgent, isAdmin, pid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(files)
}

func (gw *Gateway) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	if gw.driveStore == nil {
		writeJSON(w, 503, map[string]string{"error": "drive not configured"})
		return
	}
	r.ParseMultipartForm(32 << 20)
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file required", 400)
		return
	}
	defer file.Close()
	agentID := r.FormValue("agent_id")
	parentID := r.FormValue("parent_id")
	storagePath := drive.DriveFilePath(defaultTenant, agentID, header.Filename)
	os.MkdirAll(filepath.Dir(storagePath), 0755)
	dst, _ := os.Create(storagePath)
	defer dst.Close()
	written, _ := io.Copy(dst, file)
	var pid *string
	if parentID != "" {
		pid = &parentID
	}
	f, err := gw.driveStore.CreateFile(r.Context(), defaultTenant, agentID, header.Filename, storagePath, header.Header.Get("Content-Type"), written, false, pid)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	gw.enqueueMirrorPush(f)
	json.NewEncoder(w).Encode(f)
}

func (gw *Gateway) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	if gw.driveStore == nil {
		http.Error(w, "not found", 404)
		return
	}
	f, err := gw.driveStore.GetFile(r.Context(), defaultTenant, chi.URLParam(r, "id"))
	if err != nil || f == nil {
		http.Error(w, "not found", 404)
		return
	}
	callerAgent, isAdmin := gw.driveCaller(r)
	if ok, _ := gw.driveStore.CanAccess(r.Context(), f, callerAgent, isAdmin); !ok {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	cleanPath := filepath.Clean(f.Path)
	if err := drive.ValidateUnderRoot(cleanPath); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.ServeFile(w, r, cleanPath)
}

func (gw *Gateway) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	if gw.driveStore == nil {
		writeJSON(w, 503, map[string]string{"error": "drive not configured"})
		return
	}
	var body struct {
		AgentID  string  `json:"agent_id"`
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	f, err := gw.driveStore.CreateFile(r.Context(), defaultTenant, body.AgentID, body.Name, "", "", 0, true, body.ParentID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(f)
}

func (gw *Gateway) handleShareFile(w http.ResponseWriter, r *http.Request) {
	if gw.driveStore == nil {
		w.WriteHeader(503)
		return
	}
	var body struct {
		GranteeType string `json:"grantee_type"`
		GranteeID   string `json:"grantee_id"`
		Permission  string `json:"permission"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	// Only the owning agent or an admin may grant access to a file — otherwise
	// any authed caller could hand out access to files they don't own, defeating
	// the custom-scope ACL.
	f, err := gw.driveStore.GetFile(r.Context(), defaultTenant, chi.URLParam(r, "id"))
	if err != nil || f == nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	callerAgent, isAdmin := gw.driveCaller(r)
	if !(isAdmin || (callerAgent != "" && f.AgentID != nil && callerAgent == *f.AgentID)) {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	if err := gw.driveStore.ShareFile(r.Context(), f.ID, body.GranteeType, body.GranteeID, body.Permission); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(204)
}

func (gw *Gateway) handleDeleteDriveFile(w http.ResponseWriter, r *http.Request) {
	if gw.driveStore == nil {
		w.WriteHeader(503)
		return
	}
	f, err := gw.driveStore.GetFile(r.Context(), defaultTenant, chi.URLParam(r, "id"))
	if err != nil || f == nil {
		w.WriteHeader(404)
		return
	}
	callerAgent, isAdmin := gw.driveCaller(r)
	if !(isAdmin || (callerAgent != "" && f.AgentID != nil && callerAgent == *f.AgentID)) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	gw.driveStore.DeleteFile(r.Context(), f.ID)
	w.WriteHeader(204)
}

func (gw *Gateway) handleSetDriveScope(w http.ResponseWriter, r *http.Request) {
	if gw.driveStore == nil {
		writeJSON(w, 503, map[string]string{"error": "drive not configured"})
		return
	}
	var body struct {
		Scope   string  `json:"scope"`
		ScopeID *string `json:"scope_id"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	switch body.Scope {
	case drive.ScopePrivate, drive.ScopeCompany, drive.ScopeDepartment, drive.ScopeCustom:
	default:
		writeJSON(w, 400, map[string]string{"error": "invalid scope"})
		return
	}
	f, err := gw.driveStore.GetFile(r.Context(), defaultTenant, chi.URLParam(r, "id"))
	if err != nil || f == nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	callerAgent, isAdmin := gw.driveCaller(r)
	if !(isAdmin || (callerAgent != "" && f.AgentID != nil && callerAgent == *f.AgentID)) {
		writeJSON(w, 403, map[string]string{"error": "forbidden"})
		return
	}
	if err := gw.driveStore.SetScope(r.Context(), defaultTenant, f.ID, body.Scope, body.ScopeID); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(204)
}

func (gw *Gateway) handleDriveQuota(w http.ResponseWriter, r *http.Request) {
	if gw.driveStore == nil {
		writeJSON(w, 503, map[string]string{"error": "drive not configured"})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	used, total, err := gw.driveStore.GetQuota(r.Context(), agentID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"used_bytes": used, "total_bytes": total, "percent": float64(used) / float64(total) * 100})
}

func (gw *Gateway) handleEnrichFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	go gw.enrichDriveFile(context.Background(), id)
	writeJSON(w, 202, map[string]string{"status": "enrichment_queued", "id": id})
}

func (gw *Gateway) enrichDriveFile(ctx context.Context, fileID string) {
	if gw.db == nil {
		return
	}
	// 1. Get file info
	var content, name string
	err := gw.db.Pool.QueryRow(ctx,
		"SELECT COALESCE(extracted_text,''), name FROM drive_files WHERE id = $1", fileID,
	).Scan(&content, &name)
	if err != nil || content == "" {
		gw.db.Pool.Exec(ctx, "UPDATE drive_files SET enrichment_status='failed' WHERE id=$1", fileID)
		return
	}

	// 2. Update status to processing
	gw.db.Pool.Exec(ctx, "UPDATE drive_files SET enrichment_status='processing' WHERE id=$1", fileID)

	// 3. Ask the agent loop to summarize
	if gw.agentLoop == nil {
		gw.db.Pool.Exec(ctx, "UPDATE drive_files SET enrichment_status='failed' WHERE id=$1", fileID)
		return
	}

	prompt := fmt.Sprintf(`Analyze this document and provide:
1. A 2-sentence summary
2. 5-10 keywords (comma-separated)
3. Named entities (people, organizations, places) as a comma-separated list

Document: %s

Respond in JSON format:
{"summary": "...", "keywords": ["...", ...], "entities": ["...", ...]}`,
		truncateStr(content, 4000))

	// Use prime agent for enrichment
	agentID := gw.agentLoop.PrimeID
	if agentID == "" {
		gw.db.Pool.Exec(ctx, "UPDATE drive_files SET enrichment_status='failed' WHERE id=$1", fileID)
		return
	}
	resp, err := gw.agentLoop.Chat(ctx, agentID, prompt)
	if err != nil {
		gw.db.Pool.Exec(ctx, "UPDATE drive_files SET enrichment_status='failed' WHERE id=$1", fileID)
		return
	}

	// 4. Parse response — extract JSON block from the reply
	var result struct {
		Summary  string   `json:"summary"`
		Keywords []string `json:"keywords"`
		Entities []string `json:"entities"`
	}
	start := strings.Index(resp, "{")
	end := strings.LastIndex(resp, "}") + 1
	if start >= 0 && end > start {
		json.Unmarshal([]byte(resp[start:end]), &result) //nolint:errcheck
	}

	// 5. Store results
	kwJSON, _ := json.Marshal(result.Keywords)
	entJSON, _ := json.Marshal(result.Entities)
	gw.db.Pool.Exec(ctx,
		"UPDATE drive_files SET enrichment_status='done', summary=$1, keywords=$2, entities_extracted=$3 WHERE id=$4",
		result.Summary, string(kwJSON), string(entJSON), fileID)

	slog.Info("drive.enrichment.done", "id", fileID, "name", name, "keywords", len(result.Keywords))
}

func (gw *Gateway) handleSandboxRun(w http.ResponseWriter, r *http.Request) {
	if gw.sandboxStore == nil {
		writeJSON(w, 503, map[string]string{"error": "sandbox not configured"})
		return
	}
	var body struct {
		AgentID  string `json:"agent_id"`
		Command  string `json:"command"`
		Language string `json:"language"`
		Code     string `json:"code"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	run, err := gw.sandboxStore.Execute(r.Context(), body.AgentID, body.Command, body.Language, body.Code)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(run)
}

func (gw *Gateway) handleListSandboxRuns(w http.ResponseWriter, r *http.Request) {
	if gw.sandboxStore == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	runs, err := gw.sandboxStore.ListRuns(r.Context(), agentID, 20)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(runs)
}

func (gw *Gateway) handleGetSandboxRun(w http.ResponseWriter, r *http.Request) {
	if gw.sandboxStore == nil {
		writeJSON(w, 503, map[string]string{"error": "sandbox not configured"})
		return
	}
	run, err := gw.sandboxStore.GetRun(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	json.NewEncoder(w).Encode(run)
}

func (gw *Gateway) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	if gw.sandboxStore == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	artifacts, _ := gw.sandboxStore.ListArtifacts(r.Context(), agentID)
	json.NewEncoder(w).Encode(artifacts)
}

func (gw *Gateway) handleListCalendarEvents(w http.ResponseWriter, r *http.Request) {
	if gw.calendarStore == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	start, _ := time.Parse(time.RFC3339, startStr)
	end, _ := time.Parse(time.RFC3339, endStr)
	if start.IsZero() {
		start = time.Now().AddDate(0, -1, 0)
	}
	if end.IsZero() {
		end = time.Now().AddDate(0, 1, 0)
	}
	var aid *string
	if agentID != "" {
		aid = &agentID
	}
	events, err := gw.calendarStore.List(r.Context(), defaultTenant, aid, start, end)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(events)
}

func (gw *Gateway) handleCreateCalendarEvent(w http.ResponseWriter, r *http.Request) {
	if gw.calendarStore == nil {
		writeJSON(w, 503, map[string]string{"error": "calendar not configured"})
		return
	}
	var body calendar.Event
	json.NewDecoder(r.Body).Decode(&body)
	if body.EventType == "" {
		body.EventType = "event"
	}
	evt, err := gw.calendarStore.Create(r.Context(), defaultTenant, body)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(evt)
}

func (gw *Gateway) handleUpdateCalendarEvent(w http.ResponseWriter, r *http.Request) {
	if gw.calendarStore == nil {
		w.WriteHeader(503)
		return
	}
	var body calendar.Event
	json.NewDecoder(r.Body).Decode(&body)
	gw.calendarStore.Update(r.Context(), chi.URLParam(r, "id"), body)
	w.WriteHeader(204)
}

func (gw *Gateway) handleDeleteCalendarEvent(w http.ResponseWriter, r *http.Request) {
	if gw.calendarStore == nil {
		w.WriteHeader(503)
		return
	}
	gw.calendarStore.Delete(r.Context(), chi.URLParam(r, "id"))
	w.WriteHeader(204)
}

// ─── Remote Drive Browsing ───────────────────────────────────────────────────

// handleListDriveRemotes returns storage platforms and their connection status.
func (gw *Gateway) handleListDriveRemotes(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	ctx := r.Context()

	// Query storage platforms directly from DB.
	rows, err := gw.db.Pool.Query(ctx,
		`SELECT id, name, icon FROM connector_platforms WHERE category = 'storage' AND enabled = true ORDER BY name`)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	type remoteEntry struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Icon      string `json:"icon"`
		Connected bool   `json:"connected"`
	}

	result := []remoteEntry{}
	for rows.Next() {
		var e remoteEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.Icon); err != nil {
			continue
		}

		// Check vault for direct credentials.
		if gw.vault != nil {
			if _, err := gw.vault.GetToken(ctx, defaultTenant, e.ID, nil); err == nil {
				e.Connected = true
			}
		}

		// If not connected via vault, check relay accounts.
		if !e.Connected && gw.relayStore != nil {
			acc, _ := gw.relayStore.GetAccountForPlatform(ctx, defaultTenant, e.ID)
			if acc != nil {
				e.Connected = true
			}
		}

		result = append(result, e)
	}

	writeJSON(w, 200, result)
}

// handleListRemoteFiles browses files from a connected cloud storage provider.
func (gw *Gateway) handleListRemoteFiles(w http.ResponseWriter, r *http.Request) {
	if gw.connExec == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "connectors not configured"})
		return
	}

	provider := chi.URLParam(r, "provider")
	path := r.URL.Query().Get("path")

	// Build provider-specific params for the list_files action.
	params := map[string]any{}
	switch provider {
	case "google-drive":
		if path != "" {
			params["q"] = fmt.Sprintf("'%s' in parents", path)
		}
		params["pageSize"] = "50"
	case "dropbox":
		if path != "" {
			params["path"] = path
		} else {
			params["path"] = ""
		}
	case "microsoft-onedrive":
		if path != "" {
			params["path"] = path
		}
	case "box":
		if path != "" {
			params["folder_id"] = path
		} else {
			params["folder_id"] = "0"
		}
	default:
		writeJSON(w, 400, map[string]string{"error": "unsupported provider: " + provider})
		return
	}

	result, err := gw.connExec.Execute(r.Context(), provider, "list_files", params)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": sanitizeError(err)})
		return
	}

	// Parse the raw JSON result into a normalized file list.
	type remoteFile struct {
		Name     string `json:"name"`
		Path     string `json:"path"`
		IsFolder bool   `json:"is_folder"`
		Size     int64  `json:"size"`
		Modified string `json:"modified"`
		RemoteID string `json:"remote_id"`
	}

	// Try to parse as a JSON array or object and normalize.
	normalized := gw.normalizeRemoteFiles(provider, result)
	writeJSON(w, 200, normalized)
	_ = remoteFile{} // type used in normalizeRemoteFiles
}

// normalizeRemoteFiles parses provider-specific JSON responses into a common format.
func (gw *Gateway) normalizeRemoteFiles(provider, raw string) []map[string]any {
	var result []map[string]any

	switch provider {
	case "google-drive":
		// Google Drive returns {"files": [{id, name, mimeType, size, modifiedTime, parents}]}
		var resp struct {
			Files []struct {
				ID           string `json:"id"`
				Name         string `json:"name"`
				MimeType     string `json:"mimeType"`
				Size         string `json:"size"`
				ModifiedTime string `json:"modifiedTime"`
			} `json:"files"`
		}
		if err := json.Unmarshal([]byte(raw), &resp); err == nil {
			for _, f := range resp.Files {
				isFolder := f.MimeType == "application/vnd.google-apps.folder"
				var size int64
				fmt.Sscanf(f.Size, "%d", &size)
				result = append(result, map[string]any{
					"name":      f.Name,
					"path":      f.ID,
					"is_folder": isFolder,
					"size":      size,
					"modified":  f.ModifiedTime,
					"remote_id": f.ID,
				})
			}
		}

	case "dropbox":
		// Dropbox returns {"entries": [{".tag": "file"|"folder", "name", "path_lower", "size", "server_modified", "id"}]}
		var resp struct {
			Entries []struct {
				Tag            string `json:".tag"`
				Name           string `json:"name"`
				PathLower      string `json:"path_lower"`
				Size           int64  `json:"size"`
				ServerModified string `json:"server_modified"`
				ID             string `json:"id"`
			} `json:"entries"`
		}
		if err := json.Unmarshal([]byte(raw), &resp); err == nil {
			for _, f := range resp.Entries {
				result = append(result, map[string]any{
					"name":      f.Name,
					"path":      f.PathLower,
					"is_folder": f.Tag == "folder",
					"size":      f.Size,
					"modified":  f.ServerModified,
					"remote_id": f.ID,
				})
			}
		}

	case "microsoft-onedrive":
		// OneDrive returns {"value": [{id, name, size, lastModifiedDateTime, folder?, file?}]}
		var resp struct {
			Value []struct {
				ID                   string `json:"id"`
				Name                 string `json:"name"`
				Size                 int64  `json:"size"`
				LastModifiedDateTime string `json:"lastModifiedDateTime"`
				Folder               *struct {
					ChildCount int `json:"childCount"`
				} `json:"folder"`
			} `json:"value"`
		}
		if err := json.Unmarshal([]byte(raw), &resp); err == nil {
			for _, f := range resp.Value {
				result = append(result, map[string]any{
					"name":      f.Name,
					"path":      f.ID,
					"is_folder": f.Folder != nil,
					"size":      f.Size,
					"modified":  f.LastModifiedDateTime,
					"remote_id": f.ID,
				})
			}
		}

	case "box":
		// Box returns {"entries": [{type: "file"|"folder", id, name, size, modified_at}]}
		var resp struct {
			Entries []struct {
				Type       string `json:"type"`
				ID         string `json:"id"`
				Name       string `json:"name"`
				Size       int64  `json:"size"`
				ModifiedAt string `json:"modified_at"`
			} `json:"entries"`
		}
		if err := json.Unmarshal([]byte(raw), &resp); err == nil {
			for _, f := range resp.Entries {
				result = append(result, map[string]any{
					"name":      f.Name,
					"path":      f.ID,
					"is_folder": f.Type == "folder",
					"size":      f.Size,
					"modified":  f.ModifiedAt,
					"remote_id": f.ID,
				})
			}
		}

	default:
		// Fallback: try to return raw JSON as-is.
		var arr []map[string]any
		if json.Unmarshal([]byte(raw), &arr) == nil {
			return arr
		}
	}

	if result == nil {
		result = []map[string]any{}
	}
	return result
}

// handleDownloadRemoteFile downloads a file from a remote provider into the local workspace.
func (gw *Gateway) handleDownloadRemoteFile(w http.ResponseWriter, r *http.Request) {
	if gw.connExec == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "connectors not configured"})
		return
	}
	if gw.driveStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "drive not configured"})
		return
	}

	provider := chi.URLParam(r, "provider")

	var body struct {
		RemoteID string `json:"remote_id"`
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request body"})
		return
	}
	if body.RemoteID == "" || body.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "remote_id and name are required"})
		return
	}

	params := map[string]any{"file_id": body.RemoteID}

	result, err := gw.connExec.Execute(r.Context(), provider, "download_file", params)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": sanitizeError(err)})
		return
	}

	// Save the downloaded content to the persistent drive root (not /tmp, which
	// is ephemeral and would also fail the download handler's root check). The
	// "remote" pseudo-agent segment keeps imported files in their own subtree.
	storagePath := drive.DriveFilePath(defaultTenant, "remote", body.Name)
	os.MkdirAll(filepath.Dir(storagePath), 0755)

	if err := os.WriteFile(storagePath, []byte(result), 0644); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to save file locally"})
		return
	}

	// Detect mime type from file extension.
	mimeType := "application/octet-stream"
	ext := strings.ToLower(filepath.Ext(body.Name))
	switch ext {
	case ".pdf":
		mimeType = "application/pdf"
	case ".txt", ".md", ".csv":
		mimeType = "text/plain"
	case ".json":
		mimeType = "application/json"
	case ".doc", ".docx":
		mimeType = "application/msword"
	case ".xls", ".xlsx":
		mimeType = "application/vnd.ms-excel"
	case ".png":
		mimeType = "image/png"
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	}

	size := int64(len(result))
	var parentID *string
	if body.ParentID != "" {
		parentID = &body.ParentID
	}

	// Create a local drive record for the downloaded file.
	f, err := gw.driveStore.CreateFile(r.Context(), defaultTenant, "", body.Name, storagePath, mimeType, size, false, parentID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}

	writeJSON(w, 201, f)
}
