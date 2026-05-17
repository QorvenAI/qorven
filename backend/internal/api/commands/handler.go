// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/qorvenai/qorven/internal/api/events"
)

// SubmitFunc kicks off a prompt submission. Implementations own the actual
// LLM stream; the command handler only transforms the request and fires
// this. Returns the message id assigned to the user-side message, or an
// error. sessionID is guaranteed non-empty; text has already been composed
// from the draft + override. Context carries the request's auth/tenant.
type SubmitFunc func(ctx context.Context, sessionID, agentID, text string, meta map[string]string) (messageID string, err error)

// CommandRunner handles any ExecuteCommandRequest. Must return a
// Response-ready data map (or nil) plus an error. Unknown commands should
// return an error with Code="unknown_command" so the handler returns 400.
type CommandRunner func(ctx context.Context, cmd string, args json.RawMessage) (map[string]any, error)

// SessionResolver maps a session key to a canonical id. Used by
// AppendPromptRequest which accepts either the UUID or the session_key.
// Optional: when nil, the handler trusts the caller's session id as-is.
type SessionResolver func(ctx context.Context, sessionID string) (string, error)

// OwnerCheck authorizes the request's actor to operate on sessionID.
// Implementations typically look up the session's creator and compare
// to the authenticated user on the request context, admitting admins
// and allow-listed service accounts.
//
// Return an AuthzError (or wrap one) to produce a 403 response with
// a structured body; any other error returns 500.
// Return nil to permit the request.
type OwnerCheck func(ctx context.Context, sessionID string) error

// AuthzError signals "authenticated user is not authorized for this
// session". The handler returns 403 with {"ok":false,"error":Error,"code":"forbidden"}.
// Use errors.Is to match.
type AuthzError struct {
	Reason string
	Code   string
}

func (e *AuthzError) Error() string {
	if e.Reason == "" {
		return "forbidden"
	}
	return e.Reason
}

// Server wires the command endpoints against application-level services.
// Construct once at gateway boot and call Routes to obtain a sub-router.
type Server struct {
	Emitter   *events.Emitter
	Drafts    DraftBackend
	Submit    SubmitFunc
	Run       CommandRunner
	Resolve   SessionResolver
	// OwnerCheck is called for every session-bound command before the
	// handler proceeds. Returning a *AuthzError emits a 403; any other
	// error produces a 500. Pass nil to disable (single-tenant local dev).
	OwnerCheck OwnerCheck
	Logger     *slog.Logger
	MaxBodyKB  int64 // request body cap; 0 → 256 KiB

	reqID atomic.Int64
}

// maxBodyBytes returns the configured body cap in bytes.
func (s *Server) maxBodyBytes() int64 {
	if s.MaxBodyKB > 0 {
		return s.MaxBodyKB * 1024
	}
	return 256 * 1024
}

// Routes returns a sub-router that can be mounted under /v1/commands.
func (s *Server) Routes() chi.Router {
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	if s.Drafts == nil {
		s.Drafts = NewDraftStoreGuarded(30*time.Minute, 1)
	}
	r := chi.NewRouter()
	r.Use(s.middleware)

	r.Post("/append_prompt", s.handleAppendPrompt)
	r.Post("/clear_prompt", s.handleClearPrompt)
	r.Post("/submit_prompt", s.handleSubmitPrompt)
	r.Post("/execute", s.handleExecuteCommand)
	r.Post("/open_sessions", s.handleOpenSessions)
	r.Post("/open_models", s.handleOpenModels)
	r.Post("/open_themes", s.handleOpenThemes)
	r.Post("/toast", s.handleShowToast)
	r.Post("/resize", s.handleResize)
	return r
}

// middleware injects a request id header and enforces content-type /
// body size.
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := fmt.Sprintf("cmd-%d", s.reqID.Add(1))
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "no-store")

		if ct := r.Header.Get("Content-Type"); ct != "" {
			if !strings.HasPrefix(ct, "application/json") {
				writeErr(w, http.StatusUnsupportedMediaType, "content-type must be application/json", "bad_content_type")
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, s.maxBodyBytes())
		next.ServeHTTP(w, r)
	})
}

// decode reads the JSON body into dst with strict unknown-field detection.
func decode(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty body")
		}
		return err
	}
	// Reject additional JSON values.
	var tail json.RawMessage
	if err := dec.Decode(&tail); err != io.EOF {
		return errors.New("unexpected trailing data")
	}
	return nil
}

// writeOK is the success path.
func writeOK(w http.ResponseWriter, status int, data map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{OK: true, Data: data})
}

// writeErr is the canonical error response.
func writeErr(w http.ResponseWriter, status int, msg, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{OK: false, Error: msg, Code: code})
}

// resolveSession looks up (or passes through) the session id.
func (s *Server) resolveSession(ctx context.Context, sid string) (string, error) {
	if sid == "" {
		return "", errors.New("session_id required")
	}
	if s.Resolve == nil {
		return sid, nil
	}
	resolved, err := s.Resolve(ctx, sid)
	if err != nil {
		return "", err
	}
	if resolved == "" {
		return "", fmt.Errorf("session %q not found", sid)
	}
	return resolved, nil
}

// authorize enforces ownership. Writes the 403/500 response and returns
// false when the caller must stop; returns true when the handler may
// proceed. Safe to call when OwnerCheck is nil (no-op → true).
func (s *Server) authorize(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	if s.OwnerCheck == nil {
		return true
	}
	if err := s.OwnerCheck(r.Context(), sessionID); err != nil {
		var authz *AuthzError
		if errors.As(err, &authz) {
			code := authz.Code
			if code == "" {
				code = "forbidden"
			}
			writeErr(w, 403, authz.Error(), code)
			return false
		}
		s.Logger.Warn("commands.authorize error", "session_id", sessionID, "err", err)
		writeErr(w, 500, "authorization check failed", "authz_error")
		return false
	}
	return true
}

// ─────────────────────────── handlers ────────────────────────────

func (s *Server) handleAppendPrompt(w http.ResponseWriter, r *http.Request) {
	var req AppendPromptRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error(), "bad_request")
		return
	}
	sid, err := s.resolveSession(r.Context(), req.SessionID)
	if err != nil {
		writeErr(w, 400, err.Error(), "invalid_session")
		return
	}
	if !s.authorize(w, r, sid) {
		return
	}
	if req.Text == "" {
		writeErr(w, 400, "text required", "bad_request")
		return
	}
	// Cap per-append to protect against runaway clients.
	if len(req.Text) > 64*1024 {
		writeErr(w, 413, "text exceeds per-append limit (64 KiB)", "payload_too_large")
		return
	}
	size := s.Drafts.Append(sid, req.Text)
	writeOK(w, 200, map[string]any{
		"session_id":  sid,
		"draft_runes": size,
	})
}

func (s *Server) handleClearPrompt(w http.ResponseWriter, r *http.Request) {
	var req ClearPromptRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error(), "bad_request")
		return
	}
	sid, err := s.resolveSession(r.Context(), req.SessionID)
	if err != nil {
		writeErr(w, 400, err.Error(), "invalid_session")
		return
	}
	if !s.authorize(w, r, sid) {
		return
	}
	s.Drafts.Clear(sid)
	writeOK(w, 200, map[string]any{"session_id": sid})
}

func (s *Server) handleSubmitPrompt(w http.ResponseWriter, r *http.Request) {
	var req SubmitPromptRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error(), "bad_request")
		return
	}
	sid, err := s.resolveSession(r.Context(), req.SessionID)
	if err != nil {
		writeErr(w, 400, err.Error(), "invalid_session")
		return
	}
	if !s.authorize(w, r, sid) {
		return
	}
	text := req.Text
	if text == "" {
		text = s.Drafts.Take(sid)
	} else {
		s.Drafts.Clear(sid)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		writeErr(w, 400, "empty prompt", "bad_request")
		return
	}
	if s.Submit == nil {
		writeErr(w, 503, "submit handler not configured", "not_configured")
		return
	}
	msgID, err := s.Submit(r.Context(), sid, req.AgentID, text, req.Metadata)
	if err != nil {
		s.Logger.Warn("commands.submit_prompt failed",
			"session_id", sid, "agent_id", req.AgentID, "err", err)
		writeErr(w, 500, err.Error(), "submit_failed")
		return
	}
	writeOK(w, 202, map[string]any{
		"session_id": sid,
		"message_id": msgID,
	})
}

func (s *Server) handleExecuteCommand(w http.ResponseWriter, r *http.Request) {
	var req ExecuteCommandRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error(), "bad_request")
		return
	}
	if req.Command == "" {
		writeErr(w, 400, "command required", "bad_request")
		return
	}
	if s.Run == nil {
		writeErr(w, 503, "runner not configured", "not_configured")
		return
	}
	// Session-bound commands must authorize the actor against the session's
	// owner before reaching the runner. Commands without a session_id
	// argument fall through to the runner unchanged.
	if sid := sessionIDFromArgs(req.Args); sid != "" {
		resolved, err := s.resolveSession(r.Context(), sid)
		if err != nil {
			writeErr(w, 400, err.Error(), "invalid_session")
			return
		}
		if !s.authorize(w, r, resolved) {
			return
		}
	}
	data, err := s.Run(r.Context(), req.Command, req.Args)
	if err != nil {
		// Convention: error messages starting with "unknown command" map to 400.
		if strings.HasPrefix(err.Error(), "unknown command") {
			writeErr(w, 400, err.Error(), "unknown_command")
			return
		}
		s.Logger.Warn("commands.execute failed", "command", req.Command, "err", err)
		writeErr(w, 500, err.Error(), "command_failed")
		return
	}
	writeOK(w, 200, data)
}

func (s *Server) handleOpenSessions(w http.ResponseWriter, r *http.Request) {
	var req OpenSessionsRequest
	_ = decode(r, &req) // empty body is OK
	// These picker commands are primarily UI hints — we emit a session
	// event so every attached client (web + TUI) can react. No server-side
	// state change.
	if s.Emitter != nil {
		_ = s.Emitter.Emit(r.Context(), events.SinkAll, events.TypeSessionUpdated, events.SessionUpdatedProps{
			SessionID: "",
			Changes: map[string]any{
				"open_picker": "sessions",
				"agent_id":    req.AgentID,
				"filter":      req.Filter,
			},
		})
	}
	writeOK(w, 200, map[string]any{"picker": "sessions"})
}

func (s *Server) handleOpenModels(w http.ResponseWriter, r *http.Request) {
	var req OpenModelsRequest
	_ = decode(r, &req)
	if s.Emitter != nil {
		_ = s.Emitter.Emit(r.Context(), events.SinkAll, events.TypeSessionUpdated, events.SessionUpdatedProps{
			SessionID: "",
			Changes:   map[string]any{"open_picker": "models", "scope": req.Scope},
		})
	}
	writeOK(w, 200, map[string]any{"picker": "models"})
}

func (s *Server) handleOpenThemes(w http.ResponseWriter, r *http.Request) {
	var req OpenThemesRequest
	_ = decode(r, &req)
	if s.Emitter != nil {
		_ = s.Emitter.Emit(r.Context(), events.SinkAll, events.TypeSessionUpdated, events.SessionUpdatedProps{
			SessionID: "",
			Changes:   map[string]any{"open_picker": "themes"},
		})
	}
	writeOK(w, 200, map[string]any{"picker": "themes"})
}

func (s *Server) handleShowToast(w http.ResponseWriter, r *http.Request) {
	var req ShowToastRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error(), "bad_request")
		return
	}
	if req.Message == "" {
		writeErr(w, 400, "message required", "bad_request")
		return
	}
	switch req.Level {
	case "", "info", "success", "warn", "error":
		// ok
	default:
		writeErr(w, 400, "invalid level", "bad_request")
		return
	}
	if req.Level == "" {
		req.Level = "info"
	}
	if req.DurationMS == 0 {
		req.DurationMS = 4000
	}
	if s.Emitter != nil {
		_ = s.Emitter.Emit(r.Context(), events.SinkAll, events.TypeSessionUpdated, events.SessionUpdatedProps{
			Changes: map[string]any{
				"toast": map[string]any{
					"level":       req.Level,
					"message":     req.Message,
					"duration_ms": req.DurationMS,
					"actor":       req.Actor,
				},
			},
		})
	}
	writeOK(w, 200, nil)
}

// sessionIDFromArgs extracts a session_id field from a command's args
// JSON. Returns "" when args is empty, malformed, or has no session_id.
// Used by handleExecuteCommand to authorize session-bound commands.
func sessionIDFromArgs(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var probe struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return ""
	}
	return probe.SessionID
}

func (s *Server) handleResize(w http.ResponseWriter, r *http.Request) {
	var req ResizeRequest
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error(), "bad_request")
		return
	}
	if req.Cols <= 0 || req.Rows <= 0 {
		writeErr(w, 400, "cols and rows must be positive", "bad_request")
		return
	}
	if req.SessionID != "" {
		sid, err := s.resolveSession(r.Context(), req.SessionID)
		if err != nil {
			writeErr(w, 400, err.Error(), "invalid_session")
			return
		}
		if !s.authorize(w, r, sid) {
			return
		}
	}
	// acknowledge only. Phase 3 wires this to PTY resize.
	writeOK(w, 200, map[string]any{"cols": req.Cols, "rows": req.Rows})
}
