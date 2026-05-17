package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	whatsappch "github.com/qorvenai/qorven/internal/channels/whatsapp"
)

// handleWhatsAppQRStream is an SSE endpoint that pushes QR codes to the browser.
// GET /v1/channels/{id}/whatsapp/qr
func (gw *Gateway) handleWhatsAppQRStream(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ch := gw.findWhatsAppChannel(id)
	if ch == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "channel not found or not a whatsapp bridge channel"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	qrCh := make(chan string, 4)
	unsub := ch.SubscribeQREvents(func(qr string) {
		select {
		case qrCh <- qr:
		default:
		}
	})
	defer unsub()

	ch.RequestLatestQR()

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case qr := <-qrCh:
			data, _ := json.Marshal(map[string]string{"type": "qr", "qr": qr})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// handleWhatsAppListPending lists pending sender approvals for a channel.
// GET /v1/channels/{id}/whatsapp/pending
func (gw *Gateway) handleWhatsAppListPending(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")

	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, sender_jid, display_name, otp_code, otp_attempts, locked_until, created_at
		 FROM whatsapp_pending_senders
		 WHERE channel_id = $1 AND tenant_id = $2
		 ORDER BY created_at DESC`,
		id, defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	type pendingSender struct {
		ID          string  `json:"id"`
		SenderJID   string  `json:"sender_jid"`
		DisplayName string  `json:"display_name"`
		OTPCode     string  `json:"otp_code"`
		Attempts    int     `json:"attempts"`
		LockedUntil *string `json:"locked_until,omitempty"`
		CreatedAt   string  `json:"created_at"`
	}

	var result []pendingSender
	for rows.Next() {
		var s pendingSender
		var lockedUntil *time.Time
		var createdAt time.Time
		if err := rows.Scan(&s.ID, &s.SenderJID, &s.DisplayName, &s.OTPCode, &s.Attempts, &lockedUntil, &createdAt); err != nil {
			continue
		}
		s.CreatedAt = createdAt.Format(time.RFC3339)
		if lockedUntil != nil {
			ts := lockedUntil.Format(time.RFC3339)
			s.LockedUntil = &ts
		}
		result = append(result, s)
	}
	if result == nil {
		result = []pendingSender{}
	}
	writeJSON(w, http.StatusOK, result)
}

// handleWhatsAppApproveSender approves a pending sender.
// POST /v1/channels/{id}/whatsapp/pending/{pendingId}/approve
func (gw *Gateway) handleWhatsAppApproveSender(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	channelID := chi.URLParam(r, "id")
	pendingID := chi.URLParam(r, "pendingId")

	var senderJID, origMsg string
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT sender_jid, original_message FROM whatsapp_pending_senders
		 WHERE id = $1 AND channel_id = $2 AND tenant_id = $3`,
		pendingID, channelID, defaultTenant).Scan(&senderJID, &origMsg)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pending sender not found"})
		return
	}

	_, err = gw.db.Pool.Exec(r.Context(),
		`UPDATE channel_instances SET allowlist = array_append(COALESCE(allowlist, '{}'), $1)
		 WHERE id = $2 AND tenant_id = $3`,
		senderJID, channelID, defaultTenant)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	gw.db.Pool.Exec(r.Context(),
		`DELETE FROM whatsapp_pending_senders WHERE id = $1`, pendingID)

	ch := gw.findWhatsAppChannel(channelID)
	if ch != nil && origMsg != "" {
		ch.ReplayMessage(r.Context(), senderJID, origMsg)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "approved", "sender": senderJID})
}

// handleWhatsAppDenySender removes a pending sender without approving.
// POST /v1/channels/{id}/whatsapp/pending/{pendingId}/deny
func (gw *Gateway) handleWhatsAppDenySender(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	channelID := chi.URLParam(r, "id")
	pendingID := chi.URLParam(r, "pendingId")

	result, err := gw.db.Pool.Exec(r.Context(),
		`DELETE FROM whatsapp_pending_senders
		 WHERE id = $1 AND channel_id = $2 AND tenant_id = $3`,
		pendingID, channelID, defaultTenant)
	if err != nil || result.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pending sender not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "denied"})
}

// findWhatsAppChannel finds a running WhatsApp channel by instance ID.
func (gw *Gateway) findWhatsAppChannel(id string) *whatsappch.WhatsAppChannel {
	if gw.chanMgr == nil {
		return nil
	}
	ch := gw.chanMgr.GetChannel(id)
	if ch == nil {
		return nil
	}
	if waCh, ok := ch.(*whatsappch.WhatsAppChannel); ok {
		return waCh
	}
	return nil
}
