// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/crypto"
)

// ─── Search ───────────────────────────────────────────────────────────────────

// handleMailSearch is GET /mail/search?q=&agent_id=
func (gw *Gateway) handleMailSearch(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, 400, map[string]string{"error": "q is required"})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	msgs, err := gw.mailStore.SearchMessages(r.Context(), defaultTenant, agentID, q, 50)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, msgs)
}

// ─── Trash / Archive / Move ───────────────────────────────────────────────────

// handleMailDelete is DELETE /mail/{id} — soft-deletes by moving to trash.
func (gw *Gateway) handleMailDelete(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := gw.mailStore.SoftDelete(r.Context(), id); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(204)
}

// handleMailArchive is POST /mail/{id}/archive.
func (gw *Gateway) handleMailArchive(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := gw.mailStore.Archive(r.Context(), id); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(204)
}

// handleMailMove is POST /mail/{id}/move — body: {"folder":"<name>"}
func (gw *Gateway) handleMailMove(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Folder string `json:"folder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Folder == "" {
		writeJSON(w, 400, map[string]string{"error": "folder is required"})
		return
	}
	if err := gw.mailStore.MoveFolder(r.Context(), id, body.Folder); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(204)
}

// ─── Mail Drafts ─────────────────────────────────────────────────────────────

// handleListMailDrafts is GET /mail/drafts?agent_id=
func (gw *Gateway) handleListMailDrafts(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	drafts, err := gw.mailStore.ListDrafts(r.Context(), defaultTenant, agentID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, drafts)
}

// handleSaveMailDraft is POST /mail/drafts
// Body: agent_id, identity_id, to, subject, body_text, body_html, cc[], bcc[]
func (gw *Gateway) handleSaveMailDraft(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	var body struct {
		AgentID    string   `json:"agent_id"`
		IdentityID string   `json:"identity_id"`
		To         string   `json:"to"`
		Subject    string   `json:"subject"`
		BodyText   string   `json:"body_text"`
		BodyHTML   string   `json:"body_html"`
		Cc         []string `json:"cc"`
		Bcc        []string `json:"bcc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	msg, err := gw.mailStore.SaveDraft(r.Context(), defaultTenant, body.AgentID, body.IdentityID, body.To, body.Subject, body.BodyText, body.BodyHTML, body.Cc, body.Bcc)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 201, msg)
}

// handleUpdateMailDraft is PUT /mail/drafts/{id}
func (gw *Gateway) handleUpdateMailDraft(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		To       string   `json:"to"`
		Subject  string   `json:"subject"`
		BodyText string   `json:"body_text"`
		BodyHTML string   `json:"body_html"`
		Cc       []string `json:"cc"`
		Bcc      []string `json:"bcc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if err := gw.mailStore.UpdateDraft(r.Context(), id, body.To, body.Subject, body.BodyText, body.BodyHTML, body.Cc, body.Bcc); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(204)
}

// handleDeleteMailDraft is DELETE /mail/drafts/{id} — soft-deletes (moves to trash).
func (gw *Gateway) handleDeleteMailDraft(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := gw.mailStore.SoftDelete(r.Context(), id); err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	w.WriteHeader(204)
}

// ─── Bulk ─────────────────────────────────────────────────────────────────────

// handleMailBulk is POST /mail/bulk
// Body: {"ids":["..."],"action":"read|star|move|delete","value":"..."}
func (gw *Gateway) handleMailBulk(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	var body struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
		Value  string   `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid body"})
		return
	}
	if len(body.IDs) == 0 {
		writeJSON(w, 400, map[string]string{"error": "ids must not be empty"})
		return
	}
	if body.Action == "" {
		writeJSON(w, 400, map[string]string{"error": "action is required"})
		return
	}
	if err := gw.mailStore.BulkUpdate(r.Context(), body.IDs, body.Action, body.Value); err != nil {
		writeJSON(w, 400, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, map[string]string{"ok": "true"})
}

// ─── Attachment serving ───────────────────────────────────────────────────────

// mailAttachmentMeta is the shape of one entry in the mailbox_messages.attachments jsonb array.
type mailAttachmentMeta struct {
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	// Data is base64-encoded raw bytes when stored inline.  Most deployments
	// store only metadata here; in that case Data is empty and this endpoint
	// returns 404 with an explanatory message.
	Data string `json:"data"`
}

// handleMailAttachment is GET /mail/{id}/attachments/{name}
// Fetches the message's attachment list from the jsonb column and streams the
// named attachment's bytes.  Returns 404 if the attachment is not found or if
// only metadata (no inline data) was stored at ingest time.
func (gw *Gateway) handleMailAttachment(w http.ResponseWriter, r *http.Request) {
	if gw.mailStore == nil {
		writeJSON(w, 503, map[string]string{"error": "mail not configured"})
		return
	}
	msgID := chi.URLParam(r, "id")
	name := chi.URLParam(r, "name")

	var attJSON []byte
	err := gw.mailStore.Pool().QueryRow(r.Context(),
		`SELECT COALESCE(attachments::text, '[]') FROM mailbox_messages WHERE id = $1`, msgID,
	).Scan(&attJSON)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "message not found"})
		return
	}

	var attachments []mailAttachmentMeta
	if err := json.Unmarshal(attJSON, &attachments); err != nil {
		writeJSON(w, 500, map[string]string{"error": fmt.Sprintf("corrupt attachments column: %v", err)})
		return
	}

	for _, att := range attachments {
		if att.Name == name {
			if att.Data == "" {
				writeJSON(w, 404, map[string]string{
					"error": "attachment metadata stored but raw bytes were not persisted",
					"name":  att.Name,
				})
				return
			}
			ct := att.ContentType
			if ct == "" {
				ct = "application/octet-stream"
			}
			data, err := base64.StdEncoding.DecodeString(att.Data)
			if err != nil {
				// Try unpadded variant.
				data, err = base64.RawStdEncoding.DecodeString(att.Data)
				if err != nil {
					writeJSON(w, 500, map[string]string{"error": "could not decode attachment data"})
					return
				}
			}
			w.Header().Set("Content-Type", ct)
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, att.Name))
			w.Write(data) //nolint:errcheck
			return
		}
	}
	writeJSON(w, 404, map[string]string{"error": "attachment not found", "name": name})
}

// ─── Extended identity update helpers ─────────────────────────────────────────

// mailEncryptIfProvided returns an encrypted copy of plain when plain is non-empty,
// or ("", nil) to indicate "leave existing value in DB untouched".
func mailEncryptIfProvided(plain, encKey string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return crypto.EncryptString(plain, encKey)
}
