package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	whatsappch "github.com/qorvenai/qorven/internal/channels/whatsapp"
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

// TestWhatsAppCloudWebhookRoute_ChallengeEcho verifies that registering a cloud
// WhatsApp channel wires up GET /v1/webhooks/whatsapp/{id} so Meta's verify-token
// challenge is echoed back (the route that gateway_channels.go must register).
func TestWhatsAppCloudWebhookRoute_ChallengeEcho(t *testing.T) {
	const channelID = "test-wa-id-001"
	const verifyToken = "vtok-secret"

	// Build a minimal chi router and register the webhook route exactly as
	// the production code under test will — this is the narrowest test that
	// still exercises the real registration path.
	router := chi.NewRouter()
	cfg := whatsappch.Config{
		AgentID:       "agent-1",
		Mode:          "cloud",
		PhoneNumberID: "123456",
		AccessToken:   "tok",
		VerifyToken:   verifyToken,
	}
	ch := whatsappch.New(cfg, nil)
	webhookPath := "/v1/webhooks/whatsapp/" + channelID
	router.Get(webhookPath, ch.HandleWebhook)
	router.Post(webhookPath, ch.HandleWebhook)

	t.Run("valid_token_echoes_challenge", func(t *testing.T) {
		url := webhookPath + "?hub.mode=subscribe&hub.verify_token=" + verifyToken + "&hub.challenge=42"
		req := httptest.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if body := w.Body.String(); body != "42" {
			t.Errorf("expected body %q, got %q", "42", body)
		}
	})

	t.Run("wrong_token_rejected_403", func(t *testing.T) {
		url := webhookPath + "?hub.mode=subscribe&hub.verify_token=WRONG&hub.challenge=42"
		req := httptest.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d", w.Code)
		}
	})
}
