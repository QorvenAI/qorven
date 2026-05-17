// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/webui"
)

func (gw *Gateway) registerRoutes() {
	r := gw.router
	r.Use(PathTraversalGuard)
	if gw.auditStore != nil {
		r.Use(gw.auditMiddleware)
	}

	// Browser live-frame endpoints. Wired here (not in registerTools)
	// so they inherit the middleware stack above without tripping
	// chi's "Use after routes" rule.
	gw.registerBrowserLiveRoutes()

	// App Platform — public bundle serving (no auth, JS-only, short TTL).
	r.Get("/app-assets/{slug}/bundle.js", gw.handleAppAsset)

	r.Get("/health/detailed", gw.handleDetailedHealth)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","version":%q}`, buildInfo.Version)
	})
	// k8s/docker liveness + readiness. /livez = "process up",
	// /readyz = "dependencies warmed". Separate so a DB blip doesn't
	// restart the process — it just pulls us out of the load balancer.
	r.Get("/livez", gw.handleLivez)
	r.Get("/readyz", gw.handleReadyz)
	// Port discovery for the web client. Served unauthenticated — the
	// only thing it reveals is the port we're already listening on.
	r.Get("/__qorven_runtime", gw.handleRuntimeInfo)
	// CA certificate download — unauthenticated so browsers on other
	// machines can fetch and install the CA without logging in first.
	// This is a public certificate (not a secret); anyone who can reach
	// the server can already observe the cert in the TLS handshake.
	r.Get("/ca.pem", gw.handleCADownload)
	r.Get("/metrics", HandleMetrics)
	r.Get("/openapi.json", HandleOpenAPI)

	// WebSocket endpoints — auth via ?token= query param
	r.Get("/ws", gw.wsAuth(gw.handleWebSocket))
	r.Get("/ws/realtime", gw.wsAuth(gw.rtHub.HandleWebSocket))
	// Voice realtime WebSocket
	if gw.voiceMgr != nil && gw.agentLoop != nil {
		r.Get("/ws/voice", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			agentID := r.URL.Query().Get("agent_id")
			// Resolve agent key (like "chief") to actual UUID
			if agentID == "" || agentID == "chief" {
				if gw.agents != nil {
					if agents, err := gw.agents.List(r.Context(), defaultTenant); err == nil {
						for _, a := range agents {
							if a.AgentKey == "chief" {
								agentID = a.ID
								break
							}
						}
						if agentID == "" || agentID == "chief" {
							if len(agents) > 0 {
								agentID = agents[0].ID
							}
						}
					}
				}
			}
			// Find or create a voice session for this agent
			var sessID string
			if gw.sessions != nil && agentID != "" {
				existing, _ := gw.sessions.ListForAgent(r.Context(), agentID, 10)
				for _, s := range existing {
					if s.Channel == "voice" {
						sessID = s.ID
						break
					}
				}
				if sessID == "" {
					s, err := gw.sessions.Create(r.Context(), defaultTenant, agentID, "operator", "voice")
					if err == nil {
						sessID = s.ID
					}
				}
			}
			if sessID == "" {
				sessID = "voice-" + agentID
			}
			gw.voiceMgr.HandleRealtimeVoice(func(ctx context.Context, aID, sID, message string) (string, error) {
				result, err := gw.agentLoop.Run(ctx, agent.RunRequest{AgentID: agentID, SessionID: sessID, UserMessage: message, Channel: "voice"}, func(event agent.StreamEvent) {})
				if err != nil {
					return "", err
				}
				return result.Content, nil
			}).ServeHTTP(w, r)
		}))
		r.Get("/ws/voice/webrtc", gw.voiceMgr.HandleWebRTCSignaling(gw.agentLoop.Chat).ServeHTTP)

		// OpenAI Realtime (and future realtime providers) — the browser
		// speaks the upstream protocol directly, we just proxy and
		// inject the stored API key.
		r.Get("/ws/voice/realtime", gw.wsAuth(gw.handleRealtimeVoiceProxy))
		r.Get("/v1/voice/realtime/ephemeral", gw.handleRealtimeEphemeralToken)
	}

	// Screen share (user → agent). User's browser captures their
	// desktop via getDisplayMedia, streams JPEG frames here; the
	// user_screen_capture tool reads the latest one. Auth via the
	// standard wsAuth so no stranger can inject frames.
	r.Get("/ws/screen/upload", gw.wsAuth(gw.handleScreenShareUpload))

	// A2A Protocol (public, no auth — discovery + task endpoints)
	if gw.a2aServer != nil {
		r.Route("/a2a", gw.a2aServer.Routes())
	}

	// Rate limit login: 10 requests/minute per IP
	loginRL := NewIPRateLimit(0, 10) // 10 burst, refills slowly
	r.With(loginRL.Middleware()).Post("/auth/login", gw.handleLogin)
	r.With(loginRL.Middleware()).Post("/auth/setup", gw.handleSetup)
	r.Post("/auth/logout", gw.handleLogout)
	r.Post("/auth/refresh", gw.handleRefresh)
	r.Get("/auth/setup-check", gw.handleSetupCheck)
	r.Get("/auth/me", gw.handleMe)
	r.Post("/auth/change-password", gw.handleChangePassword)
	r.Post("/auth/api-keys", gw.handleCreateAPIKey)

	// Forgot password / magic link — stricter rate limit (3/min per IP)
	// because the endpoints DO real work (DB write + email send) and
	// are attractive to spray-attackers.
	forgotRL := NewIPRateLimit(0, 3)
	r.With(forgotRL.Middleware()).Post("/auth/forgot-password", gw.handleForgotPassword)
	r.With(forgotRL.Middleware()).Post("/auth/verify-otp", gw.handleVerifyOTP)
	r.With(forgotRL.Middleware()).Post("/auth/reset-password", gw.handleResetPassword)
	r.Post("/v1/setup/finalize", gw.handleSetupFinalize)

	// Mail inbound webhook (public, HMAC verified)
	r.Post("/v1/webhooks/mail/inbound", gw.handleMailInboundWebhook)

	// AI SDK v6 chat bridge — served at /api/chat so the static-export
	// web bundle's DefaultChatTransport(api:'/api/chat') works without a
	// Node.js runtime. Requires auth (agent access); quota is enforced
	// inside the agent loop, not here.
	r.With(gw.AuthMiddlewareV2, gw.TenantScopeMiddleware).Post("/api/chat", gw.handleAISDKChat)

	// Phase 1 protocol surfaces + all authenticated /v1/* routes. The
	// implementation lives in routes_v1.go; tests share this same helper
	// so the middleware chain / mount order cannot silently drift.
	buildV1Router(gw, r)

	// Sandbox app proxy — prefix UUID is the access token, no auth middleware
	if gw.appRunner != nil {
		r.Handle("/sandbox/{prefix}/*", gw.appRunner.ProxyHandler())
	}

	// Serve static web UI. Resolution order:
	//   1. External web/ folder (dev: /home/../web/out, or custom
	//      operator build at /var/lib/qorven/web). Wins so users
	//      can override the bundled UI without rebuilding the
	//      binary.
	//   2. Embedded FS (release builds have the Next.js static
	//      export baked in via go:embed — see internal/webui).
	//   3. Neither: 404 the not-found handler so API-only installs
	//      still work.
	webDir := gw.findWebDir()
	switch {
	case webDir != "":
		fs := http.FileServer(http.Dir(webDir))
		r.NotFound(func(w http.ResponseWriter, req *http.Request) {
			// Validate path stays within webDir (prevent path traversal)
			cleanPath := filepath.Clean(req.URL.Path)
			fullPath := filepath.Join(webDir, cleanPath)
			if !strings.HasPrefix(fullPath, filepath.Clean(webDir)+string(filepath.Separator)) && fullPath != filepath.Clean(webDir) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			// Try exact file first
			if _, err := os.Stat(fullPath); err == nil {
				fs.ServeHTTP(w, req)
				return
			}
			// SPA fallback — serve index.html
			http.ServeFile(w, req, filepath.Join(webDir, "index.html"))
		})
		slog.Info("routes registered", "health", "/health", "api", "/v1/*", "ws", "/ws", "web", webDir)
	case webui.HasIndex():
		sub, _ := webui.FS()
		fsrv := http.FileServer(http.FS(sub))
		// serveEmbeddedFile writes the named file from the embed FS
		// with the right content-type. Use this instead of
		// http.FileServer when we've chosen a specific HTML file to
		// return, since the FileServer loves to 301 /x/index.html
		// back to /x/ and we end up in a loop.
		serveEmbeddedFile := func(w http.ResponseWriter, path string) {
			data, err := fs.ReadFile(sub, path)
			if err != nil {
				http.Error(w, "not found", 404)
				return
			}
			ct := "application/octet-stream"
			switch {
			case strings.HasSuffix(path, ".html"):
				ct = "text/html; charset=utf-8"
			case strings.HasSuffix(path, ".css"):
				ct = "text/css; charset=utf-8"
			case strings.HasSuffix(path, ".js"):
				ct = "text/javascript; charset=utf-8"
			case strings.HasSuffix(path, ".json"), strings.HasSuffix(path, ".map"):
				ct = "application/json"
			case strings.HasSuffix(path, ".svg"):
				ct = "image/svg+xml"
			case strings.HasSuffix(path, ".png"):
				ct = "image/png"
			case strings.HasSuffix(path, ".jpg"), strings.HasSuffix(path, ".jpeg"):
				ct = "image/jpeg"
			case strings.HasSuffix(path, ".ico"):
				ct = "image/x-icon"
			case strings.HasSuffix(path, ".woff2"):
				ct = "font/woff2"
			case strings.HasSuffix(path, ".woff"):
				ct = "font/woff"
			case strings.HasSuffix(path, ".ttf"):
				ct = "font/ttf"
			}
			w.Header().Set("Content-Type", ct)
			// HTML shells must revalidate — they reference hashed chunk names that
			// change on every build. JS/CSS under /_next/static/ are content-hashed
			// and can be cached indefinitely.
			if strings.HasSuffix(path, ".html") {
				w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
				w.Header().Set("Pragma", "no-cache")
				w.Header().Set("Expires", "0")
			} else if strings.Contains(path, "_next/static/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			w.Write(data)
		}
		r.NotFound(func(w http.ResponseWriter, req *http.Request) {
			cleanPath := strings.TrimPrefix(filepath.Clean(req.URL.Path), "/")
			if cleanPath == "" {
				cleanPath = "index.html"
			}
			// Dynamic-route fallback FIRST. With trailingSlash:true
			// Next.js emits /<segment>/<id>/index.html shells; the
			// placeholder id is __dynamic__. When a request comes in
			// for /rooms/real-uuid/, serve the __dynamic__ shell
			// directly so the client can read the real id from
			// window.location without bouncing on FileServer's
			// index.html→dir redirect.
			parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
			if len(parts) >= 2 && parts[1] != "__dynamic__" {
				shellHTML := parts[0] + "/__dynamic__/index.html"
				realHTML := parts[0] + "/" + parts[1] + "/index.html"
				if _, err := fs.Stat(sub, shellHTML); err == nil {
					if _, err := fs.Stat(sub, realHTML); err != nil {
						serveEmbeddedFile(w, shellHTML)
						return
					}
				}
			}
			// Try exact file inside the embed — let FileServer do
			// the usual static MIME/etag handling.
			if info, err := fs.Stat(sub, cleanPath); err == nil && !info.IsDir() {
				if strings.Contains(cleanPath, "_next/static/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fsrv.ServeHTTP(w, req)
				return
			}
			// Directory request (e.g. /login/) — serve its
			// index.html directly. FileServer would 301-loop here.
			if info, err := fs.Stat(sub, cleanPath); err == nil && info.IsDir() {
				idx := strings.TrimSuffix(cleanPath, "/") + "/index.html"
				if _, err := fs.Stat(sub, idx); err == nil {
					serveEmbeddedFile(w, idx)
					return
				}
			}
			// SPA root fallback.
			serveEmbeddedFile(w, "index.html")
		})
		slog.Info("routes registered", "health", "/health", "api", "/v1/*", "ws", "/ws", "web", "embedded")
	default:
		slog.Info("routes registered", "health", "/health", "api", "/v1/*", "ws", "/ws", "web", "not found")
	}
}
