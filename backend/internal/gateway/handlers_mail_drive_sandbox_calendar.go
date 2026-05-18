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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/calendar"
	"github.com/qorvenai/qorven/internal/crypto"
)

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
	json.NewEncoder(w).Encode(id)
}

func (gw *Gateway) handleUpdateMailIdentity(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		DisplayName    string `json:"display_name"`
		SMTPHost       string `json:"smtp_host"`
		SMTPPort       int    `json:"smtp_port"`
		SMTPUser       string `json:"smtp_user"`
		SMTPPass       string `json:"smtp_pass"` // plain — encrypted before storage
		IMAPHost       string `json:"imap_host"`
		IMAPPort       int    `json:"imap_port"`
		IMAPUser       string `json:"imap_user"`
		IMAPPass       string `json:"imap_pass"` // plain — encrypted before storage
		PollInterval   int    `json:"poll_interval_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}

	// Encrypt passwords when provided; empty string = leave existing.
	var smtpPassEnc, imapPassEnc string
	encKey := gw.cfg.Auth.EncryptionKey
	if body.SMTPPass != "" {
		var err error
		if smtpPassEnc, err = crypto.EncryptString(body.SMTPPass, encKey); err != nil {
			writeJSON(w, 500, map[string]string{"error": "failed to encrypt smtp password"})
			return
		}
	}
	if body.IMAPPass != "" {
		var err error
		if imapPassEnc, err = crypto.EncryptString(body.IMAPPass, encKey); err != nil {
			writeJSON(w, 500, map[string]string{"error": "failed to encrypt imap password"})
			return
		}
	}

	// Build dynamic update — only overwrite encrypted pass columns if a new plaintext was supplied.
	if smtpPassEnc != "" && imapPassEnc != "" {
		_, err := gw.mailStore.Pool().Exec(r.Context(),
			`UPDATE soul_mail_identities
			 SET display_name=$1, smtp_host=$2, smtp_port=$3, smtp_user=$4, smtp_pass_enc=$5,
			     imap_host=$6, imap_port=$7, imap_user=$8, imap_pass_enc=$9, poll_interval_seconds=$10
			 WHERE id=$11 AND tenant_id=$12`,
			body.DisplayName, body.SMTPHost, body.SMTPPort, body.SMTPUser, smtpPassEnc,
			body.IMAPHost, body.IMAPPort, body.IMAPUser, imapPassEnc, body.PollInterval,
			id, defaultTenant)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
			return
		}
	} else if smtpPassEnc != "" {
		gw.mailStore.Pool().Exec(r.Context(),
			`UPDATE soul_mail_identities SET display_name=$1, smtp_host=$2, smtp_port=$3, smtp_user=$4, smtp_pass_enc=$5, imap_host=$6, imap_port=$7, imap_user=$8, poll_interval_seconds=$9 WHERE id=$10 AND tenant_id=$11`,
			body.DisplayName, body.SMTPHost, body.SMTPPort, body.SMTPUser, smtpPassEnc,
			body.IMAPHost, body.IMAPPort, body.IMAPUser, body.PollInterval, id, defaultTenant)
	} else if imapPassEnc != "" {
		gw.mailStore.Pool().Exec(r.Context(),
			`UPDATE soul_mail_identities SET display_name=$1, smtp_host=$2, smtp_port=$3, smtp_user=$4, imap_host=$5, imap_port=$6, imap_user=$7, imap_pass_enc=$8, poll_interval_seconds=$9 WHERE id=$10 AND tenant_id=$11`,
			body.DisplayName, body.SMTPHost, body.SMTPPort, body.SMTPUser,
			body.IMAPHost, body.IMAPPort, body.IMAPUser, imapPassEnc, body.PollInterval, id, defaultTenant)
	} else {
		gw.mailStore.Pool().Exec(r.Context(),
			`UPDATE soul_mail_identities SET display_name=$1, smtp_host=$2, smtp_port=$3, smtp_user=$4, imap_host=$5, imap_port=$6, imap_user=$7, poll_interval_seconds=$8 WHERE id=$9 AND tenant_id=$10`,
			body.DisplayName, body.SMTPHost, body.SMTPPort, body.SMTPUser,
			body.IMAPHost, body.IMAPPort, body.IMAPUser, body.PollInterval, id, defaultTenant)
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
		Subject    string   `json:"subject"`
		Body       string   `json:"body"`
		BodyHTML   string   `json:"body_html"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if len(body.To) == 0 {
		writeJSON(w, 400, map[string]string{"error": "at least one recipient required"})
		return
	}
	msgID := fmt.Sprintf("<%d@qorven.ai>", time.Now().UnixNano())
	msg, err := gw.mailStore.StoreSend(r.Context(), defaultTenant, body.AgentID, body.IdentityID, msgID, "", body.To[0], body.Subject, body.Body, body.BodyHTML, "pending_approval", body.To)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// Create approval
	gw.mailStore.CreateApproval(r.Context(), defaultTenant, msg.ID, body.AgentID, nil)
	json.NewEncoder(w).Encode(msg)
}

func (gw *Gateway) handleMailRead(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		w.WriteHeader(503)
		return
	}
	gw.mailStore.MarkRead(r.Context(), chi.URLParam(r, "id"), true)
	w.WriteHeader(204)
}

func (gw *Gateway) handleMailStar(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		w.WriteHeader(503)
		return
	}
	gw.mailStore.MarkStarred(r.Context(), chi.URLParam(r, "id"), true)
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
	gw.mailRouter.Route(r.Context(), defaultTenant, body.From, body.FromName, body.Subject, body.BodyText, body.BodyHTML, body.MessageID, body.InReplyTo, body.To)
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
	gw.mailStore.DecideApproval(r.Context(), chi.URLParam(r, "id"), "approved", "user", "")
	json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
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

func (gw *Gateway) handleListDriveFiles(w http.ResponseWriter, r *http.Request) {
	if gw.driveStore == nil {
		json.NewEncoder(w).Encode([]any{})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	parentID := r.URL.Query().Get("parent_id")
	search := r.URL.Query().Get("q")
	var pid *string
	if parentID != "" {
		pid = &parentID
	}
	// Full-text search ignores folder hierarchy — return flat matches across all files.
	if search != "" {
		files, err := gw.driveStore.SearchFiles(r.Context(), agentID, search)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(files)
		return
	}
	files, err := gw.driveStore.ListFiles(r.Context(), agentID, pid)
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
	storagePath := fmt.Sprintf("/tmp/qorven-drive/%s/%s/%s", defaultTenant, agentID, header.Filename)
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
	json.NewEncoder(w).Encode(f)
}

func (gw *Gateway) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		http.Error(w, "not found", 404)
		return
	}
	// Get file path from DB, serve it
	var path string
	gw.db.Pool.QueryRow(r.Context(), `SELECT path FROM drive_files WHERE id = $1`, chi.URLParam(r, "id")).Scan(&path)
	if path == "" {
		http.Error(w, "not found", 404)
		return
	}
	// Validate path is within workspace directory to prevent arbitrary file reads
	workspace := "/tmp/qorven-workspace"
	cleanPath := filepath.Clean(path)
	if !strings.HasPrefix(cleanPath, workspace) {
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
	json.NewDecoder(r.Body).Decode(&body)
	gw.driveStore.ShareFile(r.Context(), chi.URLParam(r, "id"), body.GranteeType, body.GranteeID, body.Permission)
	w.WriteHeader(204)
}

func (gw *Gateway) handleDeleteDriveFile(w http.ResponseWriter, r *http.Request) {
	if gw.driveStore == nil {
		w.WriteHeader(503)
		return
	}
	gw.driveStore.DeleteFile(r.Context(), chi.URLParam(r, "id"))
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
