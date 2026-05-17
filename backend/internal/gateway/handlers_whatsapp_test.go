package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleWhatsAppQRStream_NoChannel_Returns404(t *testing.T) {
	gw := &Gateway{}
	req := httptest.NewRequest("GET", "/v1/channels/nonexistent/whatsapp/qr", nil)
	w := httptest.NewRecorder()
	// Need chi context with URL param
	// Without chi routing context, chi.URLParam returns ""
	// findWhatsAppChannel("") with nil chanMgr returns nil → 404
	gw.handleWhatsAppQRStream(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
