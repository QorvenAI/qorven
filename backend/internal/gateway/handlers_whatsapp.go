package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	whatsappch "github.com/qorvenai/qorven/internal/channels/whatsapp"
)

// handleWhatsAppQRStream is an SSE endpoint that pushes QR codes and pairing
// confirmation to the browser.
// GET /v1/channels/{id}/whatsapp/qr
func (gw *Gateway) handleWhatsAppQRStream(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ch := gw.findWhatsAppChannel(id)
	if ch == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "channel not found or not a whatsapp channel"})
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
	connCh := make(chan struct{}, 1)

	unsubQR := ch.SubscribeQREvents(func(qr string) {
		select {
		case qrCh <- qr:
		default:
		}
	})
	defer unsubQR()

	unsubConn := ch.SubscribeConnectedEvents(func() {
		select {
		case connCh <- struct{}{}:
		default:
		}
	})
	defer unsubConn()

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
		case <-connCh:
			data, _ := json.Marshal(map[string]string{"type": "connected"})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
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
