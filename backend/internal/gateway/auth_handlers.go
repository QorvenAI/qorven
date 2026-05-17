// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/auth"
	"github.com/qorvenai/qorven/internal/providers"
)

// handleSetup creates the first admin account (only works when no users exist).
func (gw *Gateway) handleSetup(w http.ResponseWriter, r *http.Request) {
	if gw.authSvc == nil {
		writeJSON(w, 503, map[string]string{"error": "auth not initialized"})
		return
	}
	if !gw.authSvc.SetupRequired(r.Context()) {
		writeJSON(w, 400, map[string]string{"error": "setup already completed"})
		return
	}

	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Username == "" || req.Password == "" {
		writeJSON(w, 400, map[string]string{"error": "username and password required"})
		return
	}

	user, err := gw.authSvc.CreateUserWithDisplay(r.Context(), req.Username, req.Password, req.Email, req.DisplayName, "admin", "")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 201, map[string]any{"user": user, "message": "Admin account created. Please login."})
}

// handleLogin authenticates and returns JWT in cookie + body.
func (gw *Gateway) handleLogin(w http.ResponseWriter, r *http.Request) {
	if gw.authSvc == nil {
		writeJSON(w, 503, map[string]string{"error": "auth not initialized"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Username == "" || req.Password == "" {
		writeJSON(w, 400, map[string]string{"error": "username and password required"})
		return
	}

	token, user, err := gw.authSvc.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": err.Error()})
		return
	}

	// Mint a persisted refresh token so the frontend can renew the
	// short-lived 24h JWT without re-prompting the user. Best-effort —
	// if the DB write fails we still return a valid JWT; worst case
	// the user has to re-login in 24h instead of 7 days.
	refresh, _ := gw.authSvc.IssueRefreshToken(r.Context(), user.ID, r.UserAgent(), clientIP(r))

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

	// JWT cookie — 24h lifetime so a lost laptop on the floor doesn't
	// leak auth forever. Refresh cookie stretches the UX to 7 days.
	http.SetCookie(w, &http.Cookie{
		Name: "qorven_session", Value: token, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(auth.JWTLifetime.Seconds()),
	})
	if refresh != "" {
		http.SetCookie(w, &http.Cookie{
			Name: "qorven_refresh", Value: refresh, Path: "/",
			HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
			MaxAge: int(auth.RefreshTokenLifetime.Seconds()),
		})
	}

	writeJSON(w, 200, map[string]any{
		"token":         token,
		"refresh_token": refresh,
		"user":          user,
	})
}

// handleRefresh exchanges a refresh token for a fresh JWT. The refresh
// token is sent either in the qorven_refresh httpOnly cookie or in a
// JSON body (for CLI clients). Falls back to the session cookie for
// older clients that haven't upgraded to the refresh-token flow.
func (gw *Gateway) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if gw.authSvc == nil { writeJSON(w, 503, map[string]string{"error": "auth not initialized"}); return }

	// 1. Refresh-token path: qorven_refresh cookie or body.refresh_token
	var refreshTok string
	if cookie, err := r.Cookie("qorven_refresh"); err == nil {
		refreshTok = cookie.Value
	}
	if refreshTok == "" {
		var body struct{ RefreshToken string `json:"refresh_token"` }
		_ = json.NewDecoder(r.Body).Decode(&body)
		refreshTok = body.RefreshToken
	}
	if refreshTok != "" {
		user, err := gw.authSvc.ValidateRefreshToken(r.Context(), refreshTok)
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": err.Error()})
			return
		}
		newToken := gw.authSvc.IssueToken(user)
		secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
		http.SetCookie(w, &http.Cookie{
			Name: "qorven_session", Value: newToken, Path: "/",
			HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
			MaxAge: int(auth.JWTLifetime.Seconds()),
		})
		writeJSON(w, 200, map[string]any{"token": newToken})
		return
	}

	// 2. Legacy fallback — sliding-window reissue from a still-valid JWT.
	//    Kept so rolling deployments don't break clients that haven't
	//    got a refresh token yet.
	user := userFromContext(r.Context())
	if user == nil {
		if cookie, err := r.Cookie("qorven_session"); err == nil {
			user, _ = gw.authSvc.ValidateToken(cookie.Value)
		}
	}
	if user == nil { writeJSON(w, 401, map[string]string{"error": "not authenticated"}); return }

	newToken := gw.authSvc.IssueToken(user)
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name: "qorven_session", Value: newToken, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(auth.JWTLifetime.Seconds()),
	})
	writeJSON(w, 200, map[string]any{"token": newToken})
}

func (gw *Gateway) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Revoke the persisted refresh token (if any) so a stolen cookie
	// value from before this logout can't keep minting JWTs.
	if gw.authSvc != nil {
		if cookie, err := r.Cookie("qorven_refresh"); err == nil && cookie.Value != "" {
			gw.authSvc.RevokeRefreshToken(r.Context(), cookie.Value)
		}
	}
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{Name: "qorven_session", Value: "", Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "qorven_refresh", Value: "", Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	writeJSON(w, 200, map[string]string{"message": "logged out"})
}

// handleForgotPassword generates a 6-digit OTP for password reset.
// Delivery: Telegram (if paired) → always log.
// Returns delivery channel so UI can show the right message.
func (gw *Gateway) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if gw.authSvc == nil { writeJSON(w, 503, map[string]string{"error": "auth not initialized"}); return }
	var req struct{ Username string `json:"username_or_email"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" {
		writeJSON(w, 400, map[string]string{"error": "username required"}); return
	}

	otp, user, err := gw.authSvc.CreateOTP(r.Context(), req.Username)
	if err != nil {
		// Generic 200 — don't leak whether user exists
		writeJSON(w, 200, map[string]any{"status": "ok", "delivery": "log"})
		return
	}

	delivery := "log"
	tenantID := user.TenantID
	if tenantID == "" { tenantID = defaultTenant }

	if gw.sendOTPViaTelegram(r.Context(), tenantID, otp) {
		delivery = "telegram"
	}

	slog.Info("auth.otp_created", "user", user.Username, "delivery", delivery, "otp", otp)

	writeJSON(w, 200, map[string]any{"status": "ok", "delivery": delivery})
}

// handleVerifyOTP validates the 6-digit OTP and returns a short-lived reset token.
func (gw *Gateway) handleVerifyOTP(w http.ResponseWriter, r *http.Request) {
	if gw.authSvc == nil { writeJSON(w, 503, map[string]string{"error": "auth not initialized"}); return }
	var req struct {
		Username string `json:"username_or_email"`
		OTP      string `json:"otp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.OTP == "" {
		writeJSON(w, 400, map[string]string{"error": "username and otp required"}); return
	}

	resetToken, err := gw.authSvc.VerifyOTP(r.Context(), req.Username, req.OTP)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid or expired code. Please try again."}); return
	}

	writeJSON(w, 200, map[string]string{"token": resetToken})
}

// handleResetPassword consumes a magic link + sets a new password.
// Rate-limited by the router (same 3/min bucket as forgot-password).
func (gw *Gateway) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if gw.authSvc == nil { writeJSON(w, 503, map[string]string{"error": "auth not initialized"}); return }
	var req struct{
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" || req.NewPassword == "" {
		writeJSON(w, 400, map[string]string{"error": "token and new_password required"}); return
	}

	user, err := gw.authSvc.ConsumeMagicLink(r.Context(), req.Token)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()}); return
	}
	if err := gw.authSvc.ResetPassword(r.Context(), user.ID, req.NewPassword); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()}); return
	}
	writeJSON(w, 200, map[string]string{"status": "password reset"})
}

// handleMe returns the current authenticated user.
func (gw *Gateway) handleMe(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, 401, map[string]string{"error": "not authenticated"})
		return
	}
	// Re-fetch from DB to include display_name and avatar_url which aren't in the JWT.
	if gw.db != nil {
		var full auth.User
		var emailVal, displayNameVal, avatarVal *string
		err := gw.db.Pool.QueryRow(r.Context(),
			`SELECT id, tenant_id, username, email, display_name, COALESCE(avatar_url,''), role, is_active FROM users WHERE id = $1`, user.ID,
		).Scan(&full.ID, &full.TenantID, &full.Username, &emailVal, &displayNameVal, &avatarVal, &full.Role, &full.IsActive)
		if err == nil {
			if emailVal != nil { full.Email = *emailVal }
			if displayNameVal != nil { full.DisplayName = *displayNameVal }
			if avatarVal != nil { full.AvatarURL = *avatarVal }
			writeJSON(w, 200, full)
			return
		}
	}
	writeJSON(w, 200, user)
}

// handleListAPIKeys returns all non-revoked API keys for the current user.
func (gw *Gateway) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, 401, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, name, created_at, last_used_at FROM api_keys WHERE user_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC`,
		user.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()
	type apiKeyRow struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		CreatedAt  string  `json:"created_at"`
		LastUsedAt *string `json:"last_used_at"`
	}
	list := []apiKeyRow{}
	for rows.Next() {
		var k apiKeyRow
		var lastUsed *string
		if err := rows.Scan(&k.ID, &k.Name, &k.CreatedAt, &lastUsed); err == nil {
			k.LastUsedAt = lastUsed
			list = append(list, k)
		}
	}
	writeJSON(w, 200, list)
}

// handleRevokeAPIKey revokes a specific API key owned by the current user.
func (gw *Gateway) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, 401, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")
	ct, err := gw.db.Pool.Exec(r.Context(),
		`UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		id, user.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, 404, map[string]string{"error": "key not found"})
		return
	}
	w.WriteHeader(204)
}

// handleCreateAPIKey generates a new API key for the current user.
func (gw *Gateway) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, 401, map[string]string{"error": "not authenticated"})
		return
	}
	var req struct{ Name string `json:"name"` }
	json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		req.Name = "default"
	}

	key, err := gw.authSvc.CreateAPIKey(r.Context(), user.ID, req.Name)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 201, map[string]string{"key": key, "message": "Save this key — it won't be shown again"})
}

// handleChangePassword lets the authenticated user update their password.
func (gw *Gateway) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, 401, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.authSvc == nil {
		writeJSON(w, 503, map[string]string{"error": "auth not available"})
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.CurrentPassword == "" || req.NewPassword == "" {
		writeJSON(w, 400, map[string]string{"error": "current_password and new_password required"})
		return
	}
	if err := gw.authSvc.ChangePassword(r.Context(), user.ID, req.CurrentPassword, req.NewPassword); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"status": "password updated"})
}

// handleSetupCheck returns whether setup is required.
func (gw *Gateway) handleSetupCheck(w http.ResponseWriter, r *http.Request) {
	required := gw.authSvc != nil && gw.authSvc.SetupRequired(r.Context())
	writeJSON(w, 200, map[string]bool{"setup_required": required})
}

// AuthMiddlewareV2 checks JWT cookie → Bearer token → gateway_token → API key.
func (gw *Gateway) AuthMiddlewareV2(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for public endpoints only — /auth/me, /auth/change-password, /auth/api-keys
		// are authenticated and must NOT be skipped.
		publicPaths := map[string]bool{
			"/auth/login":           true,
			"/auth/logout":          true,
			"/auth/refresh":         true,
			"/auth/setup":           true,
			"/auth/setup-check":     true,
			"/auth/forgot-password": true,
			"/auth/reset-password":  true,
			"/v1/auth/":             true,
			"/v1/setup/finalize":    true,
			"/health":               true,
		}
		if publicPaths[r.URL.Path] || strings.HasPrefix(r.URL.Path, "/setup") {
			next.ServeHTTP(w, r)
			return
		}

		// Setup-required redirect: if no users exist, force setup
		if gw.authSvc != nil && gw.authSvc.SetupRequired(r.Context()) {
			accept := r.Header.Get("Accept")
			if strings.Contains(accept, "text/html") {
				http.Redirect(w, r, "/setup", http.StatusTemporaryRedirect)
				return
			}
			writeJSON(w, 403, map[string]any{"setup_required": true, "error": "setup not completed"})
			return
		}

		// 1. JWT cookie
		if cookie, err := r.Cookie("qorven_session"); err == nil && gw.authSvc != nil {
			if user, err := gw.authSvc.ValidateToken(cookie.Value); err == nil {
				r = r.WithContext(withUser(r.Context(), user))
				next.ServeHTTP(w, r)
				return
			}
		}

		// 2. Bearer token
		bearer := r.Header.Get("Authorization")
		bearer = strings.TrimPrefix(bearer, "Bearer ")
		if bearer == "" {
			bearer = r.URL.Query().Get("token")
		}

		if bearer != "" {
			// 2a. Check gateway_token (fast path)
			if gw.cfg.Auth.Token != "" && bearer == gw.cfg.Auth.Token {
				next.ServeHTTP(w, r)
				return
			}

			// 2b. Check JWT token
			if gw.authSvc != nil {
				if user, err := gw.authSvc.ValidateToken(bearer); err == nil {
					r = r.WithContext(withUser(r.Context(), user))
					next.ServeHTTP(w, r)
					return
				}
			}

			// 2c. Check API key (qk_ prefix)
			if strings.HasPrefix(bearer, "qk_") && gw.authSvc != nil {
				if user, err := gw.authSvc.ValidateAPIKey(r.Context(), bearer); err == nil {
					r = r.WithContext(withUser(r.Context(), user))
					next.ServeHTTP(w, r)
					return
				}
			}

			// 2d. Check DB api_keys (legacy)
			if gw.db != nil {
				var keyID string
				err := gw.db.Pool.QueryRow(r.Context(),
					`SELECT id FROM api_keys WHERE key_hash = encode(sha256($1::bytea), 'hex') AND revoked_at IS NULL`,
					bearer).Scan(&keyID)
				if err == nil {
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		// 3. No auth configured → allow (backward compat for local dev).
		//    Phase 4 hardening: refuse this bypass in multi-tenant mode.
		//    A multi-tenant deployment has no "local dev" — every request
		//    crosses a tenant boundary, so anonymous admit would leak
		//    data across tenants. Single-tenant behavior is byte-for-byte
		//    preserved: the bypass still fires exactly when gateway token
		//    is empty AND setup has not completed AND the deployment is
		//    not declared multi-tenant.
		if gw.cfg.Auth.Token == "" && (gw.authSvc == nil || gw.authSvc.SetupRequired(r.Context())) {
			if gw.deploymentConfig == nil || !gw.deploymentConfig.IsMultiTenant(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// 4. Check if browser request → redirect to login
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "text/html") {
			http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
			return
		}

		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
	})
}

// Context helpers
type ctxKeyUser struct{}

func withUser(ctx context.Context, u *auth.User) context.Context {
	return context.WithValue(ctx, ctxKeyUser{}, u)
}

func userFromContext(ctx context.Context) *auth.User {
	u, _ := ctx.Value(ctxKeyUser{}).(*auth.User)
	return u
}

// handleSetupFinalize completes the setup wizard — creates agents, channels, config.
func (gw *Gateway) handleSetupFinalize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InstanceName   string   `json:"instance_name"`
		PrimeName      string   `json:"prime_name"`
		PrimeIcon      string   `json:"prime_icon"`
		CallMe         string   `json:"call_me"`
		Style          string   `json:"style"`
		Language       string   `json:"language"`
		Goals          []string `json:"goals"`
		GoalsStr       string   `json:"goals_str"` // comma-separated fallback for CLI
		SearchProvider string   `json:"search_provider"`
		SearchKey      string   `json:"search_key"`
		EmailMode      string   `json:"email_mode"`
		EmailAddr      string   `json:"email_addr"`
		TgToken        string   `json:"tg_token"`
		TgChatID       string   `json:"tg_chat_id"`
		Bind           string   `json:"bind"`
		ApprovalMode   string   `json:"approval_mode"`

		// Phase-4 security step fields. Optional — absent values leave
		// the current config untouched. When set, the handler writes
		// them into ~/.qorven/config.toml so the next restart picks up
		// the new TLS mode / web listener port.
		TLSMode    string `json:"tls_mode"`
		TLSDomain  string `json:"tls_domain"`
		WebPort    string `json:"web_port"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid request"})
		return
	}

	ctx := r.Context()
	// Fallback: parse goals from comma-separated string
	if len(req.Goals) == 0 && req.GoalsStr != "" {
		for _, g := range strings.Split(req.GoalsStr, ",") {
			if t := strings.TrimSpace(g); t != "" { req.Goals = append(req.Goals, t) }
		}
	}

	if gw.agents == nil || gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not ready"})
		return
	}

	// 1. Update tenant name
	if req.InstanceName != "" {
		gw.db.Pool.Exec(ctx, "UPDATE tenants SET name = $1 WHERE id = $2", req.InstanceName, defaultTenant)
	}

	// 2. Update Prime (chief) agent — build a rich soul bundle from wizard inputs
	prime, _ := gw.agents.GetByKey(ctx, "chief")
	if prime == nil {
		prime, _ = gw.agents.GetByKey(ctx, "prime") // legacy fallback
	}
	if prime != nil {
		styleMap := map[string]string{
			"professional": "formal and structured — precise language, concise replies",
			"casual":       "friendly and conversational — warm, personable, uses contractions",
			"technical":    "direct and code-focused — minimal prose, show code and commands",
		}
		personality := styleMap[req.Style]
		if personality == "" { personality = "helpful and adaptable" }

		lang := req.Language
		if lang == "" { lang = "en" }

		callMeClause := ""
		if req.CallMe != "" {
			callMeClause = fmt.Sprintf("\nAlways address the user as %q — never use generic greetings like \"User\" or \"Human\".", req.CallMe)
		}

		goalLines := ""
		for _, g := range req.Goals {
			goalLines += "\n- " + g
		}
		goalsSection := ""
		if goalLines != "" {
			goalsSection = fmt.Sprintf("\n\n## User Goals\nThe user's primary focus areas are:%s\nAlign task prioritisation and suggestions to these goals.", goalLines)
		}

		soulContent := fmt.Sprintf(`You are %s — the user's primary AI assistant on the Qorven platform.

## Identity
- Communication style: %s
- Primary language: %s%s

## Role
You are the Chief of Staff. You have full access to all tools, agents, and services in this workspace.
You can delegate tasks to specialist agents, coordinate work, review outputs, and report back.
When delegating: "I'll have [Agent] handle that — I'll report back when it's done."

## Capabilities
- Run agentic tasks end-to-end (research, code, writing, automation)
- Access connected services via execute_action
- Search and browse the web
- Manage tasks, plans, and agent workflows
- Remember things across sessions via memory_save and memory_search%s

## Behaviour
- Be proactive — if you notice something worth the user's attention, surface it
- Never just describe what you would do; always take action using available tools
- Keep replies concise unless the user asks for detail
- When uncertain, ask one targeted clarifying question rather than proceeding on assumptions`,
			req.PrimeName, personality, lang, callMeClause, goalsSection)

		// Upsert the soul bundle — this is the primary identity injection
		if gw.bundleStore != nil {
			gw.bundleStore.Upsert(ctx, agent.Bundle{
				AgentID:    prime.ID,
				BundleType: "soul",
				Name:       "prime_soul",
				Content:    soulContent,
				Priority:   200,
				Enabled:    true,
			})
		}

		// Keep system_prompt in sync for backwards compat (session logging etc.)
		agentUpdate := map[string]any{"display_name": req.PrimeName, "system_prompt": soulContent}
		if req.PrimeIcon != "" {
			agentUpdate["avatar"] = req.PrimeIcon
		}
		gw.agents.Update(ctx, prime.ID, agentUpdate)
	}

	// 2b. Write user profile so the agent always knows the user's name and language
	if req.CallMe != "" || req.Language != "" {
		factsJSON, _ := json.Marshal(map[string]any{"name": req.CallMe})
		lang := req.Language
		if lang == "" { lang = "en" }
		gw.db.Pool.Exec(ctx,
			`INSERT INTO user_profiles (tenant_id, facts, language)
			 VALUES ($1, $2::jsonb, $3)
			 ON CONFLICT (tenant_id) DO UPDATE SET facts = $2::jsonb, language = $3, updated_at = NOW()`,
			defaultTenant, string(factsJSON), lang)
	}

	// 3. Create specialist agents based on goals
	goalAgents := map[string]struct{ key, name, role string }{
		"coding":     {"developer", "Developer", "developer"},
		"research":   {"researcher", "Researcher", "researcher"},
		"email":      {"email-agent", "Email Agent", "worker"},
		"creative":   {"writer", "Writer", "writer"},
		"social":     {"social-agent", "Social Agent", "worker"},
		"automation": {"automator", "Automator", "worker"},
	}
	model := ""
	if prime != nil && prime.Model != "" {
		model = prime.Model
	} else if gw.agentLoop != nil && gw.agentLoop.SmartRouter != nil {
		model = gw.agentLoop.SmartRouter.BestModelForTier(providers.TierStandard)
	}
	for _, g := range req.Goals {
		if spec, ok := goalAgents[g]; ok {
			existing, _ := gw.agents.GetByKey(ctx, spec.key)
			if existing == nil {
				gw.agents.Create(ctx, defaultTenant, agent.CreateAgentInput{
					AgentKey: spec.key, DisplayName: spec.name, Role: spec.role,
					Model: model, Temperature: 0.5, ContextWindow: 128000,
					MaxToolIterations: 20, ToolProfile: "full",
				})
			}
		}
	}

	// 4. Save search provider (use json.Marshal to prevent JSON injection)
	if req.SearchProvider != "" && req.SearchProvider != "none" && req.SearchKey != "" {
		searchCfg, _ := json.Marshal(map[string]string{"api_key": req.SearchKey})
		gw.db.Pool.Exec(ctx, `INSERT INTO provider_configs (tenant_id, provider_type, config) VALUES ($1, $2, $3) ON CONFLICT (tenant_id, provider_type) DO UPDATE SET config = $3`,
			defaultTenant, "search_"+req.SearchProvider, string(searchCfg))
	}

	// 5. Create Telegram channel if configured (use json.Marshal to prevent JSON injection)
	if req.TgToken != "" && prime != nil {
		tgCfg, _ := json.Marshal(map[string]string{"bot_token": req.TgToken, "owner_chat_id": req.TgChatID})
		gw.db.Pool.Exec(ctx, `INSERT INTO channel_instances (tenant_id, agent_id, channel_type, config, enabled, dm_policy) VALUES ($1, $2, 'telegram', $3, true, 'open') ON CONFLICT DO NOTHING`,
			defaultTenant, prime.ID, string(tgCfg))
	}

	// 6. Persist TLS + web-port choices from the wizard's security step
	// into the in-memory Config and log guidance for the admin to
	// mirror into ~/.qorven/config.toml. We do NOT rewrite the config
	// file from the gateway process — the file may be managed by
	// packaging (systemd unit, docker volume) and silent rewrites are
	// surprising. The log line gives the admin copy-pasteable TOML.
	if req.TLSMode != "" {
		gw.cfg.Server.TLS.Mode = req.TLSMode
	}
	if req.TLSDomain != "" {
		gw.cfg.Server.TLS.Domain = req.TLSDomain
	}
	if req.WebPort != "" {
		// Preserve whatever host is already in WebListen (default
		// 0.0.0.0) so setting only a port doesn't silently change
		// the bind interface.
		host := "0.0.0.0"
		if parts := strings.SplitN(gw.cfg.Server.WebListen, ":", 2); len(parts) == 2 && parts[0] != "" {
			host = parts[0]
		}
		gw.cfg.Server.WebListen = host + ":" + req.WebPort
	}
	if req.TLSMode != "" || req.TLSDomain != "" || req.WebPort != "" {
		slog.Info("setup.security_applied (restart to take effect; mirror into ~/.qorven/config.toml)",
			"tls_mode", gw.cfg.Server.TLS.Mode,
			"tls_domain", gw.cfg.Server.TLS.Domain,
			"web_listen", gw.cfg.Server.WebListen,
		)
	}

	writeJSON(w, 200, map[string]string{"status": "ok", "message": "Setup complete"})
}

// handlePatchProfile lets the authenticated user update their email, display name and avatar.
func (gw *Gateway) handlePatchProfile(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}
	var req struct {
		Email       *string `json:"email"`
		DisplayName *string `json:"display_name"`
		AvatarURL   *string `json:"avatar_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Email == nil && req.DisplayName == nil && req.AvatarURL == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nothing to update"})
		return
	}

	setClauses := []string{}
	args := []any{}
	i := 1
	if req.Email != nil {
		setClauses = append(setClauses, "email = $"+strconv.Itoa(i))
		args = append(args, *req.Email)
		i++
	}
	if req.DisplayName != nil {
		setClauses = append(setClauses, "display_name = $"+strconv.Itoa(i))
		args = append(args, *req.DisplayName)
		i++
	}
	if req.AvatarURL != nil {
		setClauses = append(setClauses, "avatar_url = $"+strconv.Itoa(i))
		args = append(args, *req.AvatarURL)
		i++
	}
	setClauses = append(setClauses, "updated_at = now()")
	args = append(args, user.ID)

	query := "UPDATE users SET " + strings.Join(setClauses, ", ") + " WHERE id = $" + strconv.Itoa(i)
	if _, err := gw.db.Pool.Exec(r.Context(), query, args...); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}

	var updated auth.User
	var emailVal, displayNameVal, avatarVal *string
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT id, tenant_id, username, email, display_name, role, COALESCE(avatar_url,'') FROM users WHERE id = $1`, user.ID,
	).Scan(&updated.ID, &updated.TenantID, &updated.Username, &emailVal, &displayNameVal, &updated.Role, &updated.AvatarURL)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if emailVal != nil { updated.Email = *emailVal }
	if displayNameVal != nil { updated.DisplayName = *displayNameVal }
	_ = avatarVal
	writeJSON(w, http.StatusOK, updated)
}

// handleListSessions returns active refresh-token sessions for the current user.
func (gw *Gateway) handleListUserSessions(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, 401, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, user_agent, ip_address, created_at, last_used_at, expires_at
		 FROM refresh_tokens
		 WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
		 ORDER BY COALESCE(last_used_at, created_at) DESC`,
		user.ID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()
	type sessionRow struct {
		ID          string  `json:"id"`
		UserAgent   string  `json:"user_agent"`
		IPAddress   string  `json:"ip_address"`
		CreatedAt   string  `json:"created_at"`
		LastUsedAt  *string `json:"last_used_at"`
		ExpiresAt   string  `json:"expires_at"`
	}
	list := []sessionRow{}
	for rows.Next() {
		var s sessionRow
		var ip, lastUsed *string
		rows.Scan(&s.ID, &s.UserAgent, &ip, &s.CreatedAt, &lastUsed, &s.ExpiresAt)
		if ip != nil { s.IPAddress = *ip }
		s.LastUsedAt = lastUsed
		list = append(list, s)
	}
	writeJSON(w, 200, list)
}

// handleRevokeSession revokes (kills) a specific session owned by the current user.
func (gw *Gateway) handleRevokeUserSession(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		writeJSON(w, 401, map[string]string{"error": "not authenticated"})
		return
	}
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := gw.authSvc.RevokeSessionByID(r.Context(), user.ID, id); err != nil {
		writeJSON(w, 404, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(204)
}

// clientIP extracts the real client IP from standard proxy headers.
func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return strings.TrimSpace(strings.SplitN(v, ",", 2)[0])
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return strings.TrimSpace(strings.SplitN(v, ",", 2)[0])
	}
	if h, _, found := strings.Cut(r.RemoteAddr, ":"); found {
		return h
	}
	return r.RemoteAddr
}
