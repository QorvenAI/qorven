// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// handlers_lsp.go — WebSocket endpoint that bridges browser ↔ language server.
//
// Route: GET /v1/lsp/{projectId}?lang=<go|typescript|javascript|python>
//
// The handler accepts a WebSocket upgrade (nhooyr.io/websocket), then calls
// lsp.Manager.Relay which spawns a language server process and pumps messages
// bidirectionally:
//
//	Browser WS (raw JSON-RPC) ↔ LSP stdio (Content-Length framed JSON-RPC)
//
// One language server process is spawned per connection and killed when the
// connection closes (v1 per-connection model).

package gateway

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/gateway/lsp"
	"nhooyr.io/websocket"
)

// handleLSPWS upgrades to WebSocket and relays JSON-RPC between the browser
// (monaco-languageclient via vscode-ws-jsonrpc) and a language server process.
func (gw *Gateway) handleLSPWS(w http.ResponseWriter, r *http.Request) {
	if gw.lspMgr == nil {
		http.Error(w, `{"error":"lsp service not available"}`, http.StatusServiceUnavailable)
		return
	}

	projectID := chi.URLParam(r, "projectId")
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		http.Error(w, `{"error":"lang query param required"}`, http.StatusBadRequest)
		return
	}

	// Resolve the project root path so the language server can find workspace files.
	var rootPath string
	if gw.projectReg != nil {
		if p := gw.projectReg.Get(projectID); p != nil {
			rootPath = p.Path
		}
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Origin already gated by wsAuth / AuthMiddlewareV2
	})
	if err != nil {
		slog.Warn("lsp.ws.accept_failed", "error", err)
		return
	}
	defer conn.CloseNow()

	ctx := r.Context()

	// Adapt nhooyr WebSocket to the wsRead/wsWrite closures Relay expects.
	wsRead := func() ([]byte, error) {
		_, msg, err := conn.Read(ctx)
		return msg, err
	}
	wsWrite := func(msg []byte) error {
		return conn.Write(ctx, websocket.MessageText, msg)
	}

	slog.Info("lsp.relay.start",
		"project", projectID, "lang", lang, "root", rootPath)

	if err := gw.lspMgr.Relay(ctx, projectID, lang, rootPath, wsRead, wsWrite); err != nil {
		// Most relay errors are benign disconnects; log at debug level.
		slog.Debug("lsp.relay.done", "project", projectID, "lang", lang, "reason", err)
		conn.Close(websocket.StatusNormalClosure, "relay ended")
		return
	}

	conn.Close(websocket.StatusNormalClosure, "done")
}

// handleLSPLanguage returns the LSP language id for a given file extension.
// Exposed as a helper for the frontend (GET /v1/lsp/lang?ext=.go → "go").
func (gw *Gateway) handleLSPLanguage(w http.ResponseWriter, r *http.Request) {
	ext := r.URL.Query().Get("ext")
	if ext == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ext param required"})
		return
	}
	lang := lsp.LanguageForExt(ext)
	if lang == "" {
		writeJSON(w, http.StatusOK, map[string]string{"lang": "", "supported": "false"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"lang": lang, "supported": "true"})
}
