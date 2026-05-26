// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	socialqor "github.com/qorvenai/qorven/internal/qor/social"
)

// socialOAuthProvider defines the OAuth 2.0 config for each social platform.
type socialOAuthProvider struct {
	Name        string
	AuthURL     string
	TokenURL    string
	Scopes      []string
	PKCE        bool   // use PKCE (no client_secret needed)
	ExtraParams map[string]string
	// Platform to store token under
	Platform socialqor.Platform
}

// socialOAuthProviders is populated at handler time from gateway config.
// Client IDs/secrets come from gw.cfg — not hardcoded here.
var socialOAuthDefs = map[string]socialOAuthProvider{
	"twitter": {
		Name:     "X / Twitter",
		AuthURL:  "https://twitter.com/i/oauth2/authorize",
		TokenURL: "https://api.twitter.com/2/oauth2/token",
		Scopes:   []string{"tweet.read", "tweet.write", "users.read", "offline.access"},
		PKCE:     true,
		Platform: socialqor.PlatformTwitter,
	},
	"linkedin": {
		Name:     "LinkedIn",
		AuthURL:  "https://www.linkedin.com/oauth/v2/authorization",
		TokenURL: "https://www.linkedin.com/oauth/v2/accessToken",
		Scopes:   []string{"openid", "profile", "w_member_social", "r_basicprofile"},
		Platform: socialqor.PlatformLinkedIn,
	},
	"facebook": {
		Name:     "Facebook",
		AuthURL:  "https://www.facebook.com/v20.0/dialog/oauth",
		TokenURL: "https://graph.facebook.com/v20.0/oauth/access_token",
		Scopes:   []string{"pages_show_list", "pages_manage_posts", "pages_manage_engagement", "pages_read_engagement"},
		Platform: socialqor.PlatformFacebook,
	},
	"instagram": {
		Name:     "Instagram",
		AuthURL:  "https://www.facebook.com/v20.0/dialog/oauth",
		TokenURL: "https://graph.facebook.com/v20.0/oauth/access_token",
		Scopes:   []string{"instagram_basic", "instagram_content_publish", "pages_show_list"},
		Platform: socialqor.PlatformInstagram,
	},
	"threads": {
		Name:     "Threads",
		AuthURL:  "https://www.threads.net/oauth/authorize",
		TokenURL: "https://graph.threads.net/oauth/access_token",
		Scopes:   []string{"threads_basic", "threads_content_publish"},
		Platform: socialqor.PlatformThreads,
	},
	"tiktok": {
		Name:     "TikTok",
		AuthURL:  "https://www.tiktok.com/v2/auth/authorize/",
		TokenURL: "https://open.tiktokapis.com/v2/oauth/token/",
		Scopes:   []string{"user.info.basic", "video.publish", "video.upload"},
		PKCE:     true,
		Platform: socialqor.PlatformTikTok,
	},
	"youtube": {
		Name:     "YouTube",
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
		Scopes:   []string{"https://www.googleapis.com/auth/youtube.upload", "https://www.googleapis.com/auth/youtube.force-ssl"},
		ExtraParams: map[string]string{"access_type": "offline", "prompt": "consent"},
		Platform: socialqor.PlatformYouTube,
	},
	"pinterest": {
		Name:     "Pinterest",
		AuthURL:  "https://www.pinterest.com/oauth/",
		TokenURL: "https://api.pinterest.com/v5/oauth/token",
		Scopes:   []string{"boards:read", "pins:write"},
		Platform: socialqor.PlatformPinterest,
	},
	"reddit": {
		Name:     "Reddit",
		AuthURL:  "https://www.reddit.com/api/v1/authorize",
		TokenURL: "https://www.reddit.com/api/v1/access_token",
		Scopes:   []string{"identity", "submit", "read"},
		ExtraParams: map[string]string{"duration": "permanent"},
		Platform: socialqor.PlatformReddit,
	},
	"discord": {
		Name:     "Discord",
		AuthURL:  "https://discord.com/api/oauth2/authorize",
		TokenURL: "https://discord.com/api/oauth2/token",
		Scopes:   []string{"identify", "bot", "webhook.incoming"},
		Platform: socialqor.PlatformDiscord,
	},
	"slack": {
		Name:     "Slack",
		AuthURL:  "https://slack.com/oauth/v2/authorize",
		TokenURL: "https://slack.com/api/oauth.v2.access",
		Scopes:   []string{"chat:write", "channels:read", "incoming-webhook"},
		Platform: socialqor.PlatformSlack,
	},
	"medium": {
		Name:     "Medium",
		AuthURL:  "https://medium.com/m/oauth/authorize",
		TokenURL: "https://api.medium.com/v1/tokens",
		Scopes:   []string{"basicProfile", "publishPost"},
		Platform: socialqor.PlatformMedium,
	},
	"googlemybusiness": {
		Name:     "Google My Business",
		AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL: "https://oauth2.googleapis.com/token",
		Scopes:   []string{"https://www.googleapis.com/auth/business.manage"},
		ExtraParams: map[string]string{"access_type": "offline", "prompt": "consent"},
		Platform: socialqor.PlatformGoogleMyBiz,
	},
}

// socialOAuthCreds returns the client_id and client_secret for a platform.
// Uses the same vault storage as the existing OAuth apps system:
//   vault: tenant=default, platform=__oauth_app_social_{platform}__
// These are configured via Settings → Social → OAuth Apps in the UI.
func (gw *Gateway) socialOAuthCreds(platform string) (clientID, clientSecret string) {
	if gw.vault == nil {
		return "", ""
	}
	data, ok := readVaultOAuthApp(context.Background(), gw.vault, "default", "social_"+platform)
	if !ok {
		return "", ""
	}
	return data.ClientID, data.ClientSecret
}

func (gw *Gateway) socialOAuthRedirectURI(platform string) string {
	base := gw.cfg.Server.BaseURL
	if base == "" {
		base = "http://localhost:4200"
	}
	return strings.TrimRight(base, "/") + "/v1/social/oauth/" + platform + "/callback"
}

// handleSocialOAuthStart redirects the user to the platform's OAuth consent page.
// GET /v1/social/oauth/{platform}/start?agent_id=<id>
func (gw *Gateway) handleSocialOAuthStart(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	def, ok := socialOAuthDefs[platform]
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "unsupported platform: " + platform})
		return
	}

	clientID, _ := gw.socialOAuthCreds(platform)
	if clientID == "" {
		writeJSON(w, 400, map[string]string{
			"error": fmt.Sprintf("OAuth not configured for %s — add client_id via Settings", platform),
		})
		return
	}

	agentID := r.URL.Query().Get("agent_id")

	// Generate CSRF state: base64(random 16 bytes)
	stateBytes := make([]byte, 16)
	rand.Read(stateBytes)
	state := base64.RawURLEncoding.EncodeToString(stateBytes)

	// Store state + agent_id in a short-lived cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "social_oauth_state_" + platform,
		Value:    state + "|" + agentID,
		Path:     "/",
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	params := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {gw.socialOAuthRedirectURI(platform)},
		"response_type": {"code"},
		"scope":         {strings.Join(def.Scopes, " ")},
		"state":         {state},
	}
	for k, v := range def.ExtraParams {
		params.Set(k, v)
	}

	// PKCE: generate code_verifier + code_challenge
	if def.PKCE {
		verifier := make([]byte, 32)
		rand.Read(verifier)
		cv := base64.RawURLEncoding.EncodeToString(verifier)
		h := sha256.Sum256([]byte(cv))
		cc := base64.RawURLEncoding.EncodeToString(h[:])
		params.Set("code_challenge", cc)
		params.Set("code_challenge_method", "S256")
		// Store verifier in cookie for callback
		http.SetCookie(w, &http.Cookie{
			Name:     "social_oauth_cv_" + platform,
			Value:    cv,
			Path:     "/",
			MaxAge:   600,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	http.Redirect(w, r, def.AuthURL+"?"+params.Encode(), http.StatusFound)
}

// handleSocialOAuthCallback exchanges the code for tokens and stores them.
// GET /v1/social/oauth/{platform}/callback
func (gw *Gateway) handleSocialOAuthCallback(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	def, ok := socialOAuthDefs[platform]
	if !ok {
		http.Error(w, "unsupported platform", 400)
		return
	}

	// Validate state
	stateCookie, err := r.Cookie("social_oauth_state_" + platform)
	if err != nil {
		http.Error(w, "missing state cookie", 400)
		return
	}
	parts := strings.SplitN(stateCookie.Value, "|", 2)
	if len(parts) != 2 || parts[0] != r.URL.Query().Get("state") {
		http.Error(w, "state mismatch", 400)
		return
	}
	agentID := parts[1]

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{Name: "social_oauth_state_" + platform, MaxAge: -1, Path: "/"})

	code := r.URL.Query().Get("code")
	if code == "" {
		errMsg := r.URL.Query().Get("error_description")
		if errMsg == "" {
			errMsg = r.URL.Query().Get("error")
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<script>window.opener&&window.opener.postMessage({type:'social_oauth_error',platform:%q,error:%q},'*');window.close()</script>`,
			platform, errMsg)
		return
	}

	clientID, clientSecret := gw.socialOAuthCreds(platform)
	if clientID == "" {
		http.Error(w, "oauth not configured", 400)
		return
	}

	// Exchange code for token
	formData := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {gw.socialOAuthRedirectURI(platform)},
		"client_id":    {clientID},
	}
	if !def.PKCE && clientSecret != "" {
		formData.Set("client_secret", clientSecret)
	}
	if def.PKCE {
		if cvCookie, err := r.Cookie("social_oauth_cv_" + platform); err == nil {
			formData.Set("code_verifier", cvCookie.Value)
			http.SetCookie(w, &http.Cookie{Name: "social_oauth_cv_" + platform, MaxAge: -1, Path: "/"})
		}
	}

	resp, err := http.PostForm(def.TokenURL, formData)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), 500)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<script>window.opener&&window.opener.postMessage({type:'social_oauth_error',platform:%q,error:'token exchange failed'},'*');window.close()</script>`,
			platform)
		return
	}

	// Encrypt + store
	encAccess, encErr := gw.encryptIntegrationKey(tok.AccessToken)
	if encErr != nil {
		encAccess = tok.AccessToken // store plain if encrypt unavailable
	}
	encRefresh := ""
	if tok.RefreshToken != "" {
		encRefresh, _ = gw.encryptIntegrationKey(tok.RefreshToken)
		if encRefresh == "" {
			encRefresh = tok.RefreshToken
		}
	}

	var expiry *time.Time
	if tok.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
		expiry = &t
	}

	store := gw.socialStore()
	if store == nil {
		http.Error(w, "database not configured", http.StatusServiceUnavailable)
		return
	}

	// Fetch account name from platform API
	accountName, accountID := gw.socialFetchAccountInfo(platform, tok.AccessToken)

	integ := socialqor.Integration{
		Platform:     def.Platform,
		AccountName:  accountName,
		AccountID:    accountID,
		AccessToken:  encAccess,
		RefreshToken: encRefresh,
		TokenExpiry:  expiry,
		AgentID:      agentID,
		Active:       true,
	}
	id, saveErr := store.SaveIntegration(r.Context(), integ)
	if saveErr != nil {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<script>window.opener&&window.opener.postMessage({type:'social_oauth_error',platform:%q,error:'save failed'},'*');window.close()</script>`,
			platform)
		return
	}

	// Post message to opener and close popup
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w,
		`<script>window.opener&&window.opener.postMessage({type:'social_oauth_success',platform:%q,id:%q,account:%q},'*');window.close()</script>`,
		platform, id, accountName)
}

// handleSocialOAuthStatus returns whether a platform is connected for an agent.
// GET /v1/social/oauth/{platform}/status?agent_id=<id>
func (gw *Gateway) handleSocialOAuthStatus(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	def, ok := socialOAuthDefs[platform]
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "unsupported platform"})
		return
	}
	store := gw.socialStore()
	if store == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	access, _, err := store.GetIntegrationToken(r.Context(), agentID, def.Platform)
	connected := err == nil && access != ""
	writeJSON(w, 200, map[string]any{"platform": platform, "connected": connected})
}

// handleSocialOAuthRevoke deletes the stored token for a platform.
// POST /v1/social/oauth/{platform}/revoke?agent_id=<id>
func (gw *Gateway) handleSocialOAuthRevoke(w http.ResponseWriter, r *http.Request) {
	platform := chi.URLParam(r, "platform")
	def, ok := socialOAuthDefs[platform]
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "unsupported platform"})
		return
	}
	store := gw.socialStore()
	if store == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	agentID := r.URL.Query().Get("agent_id")
	// Find and delete the integration for this agent+platform
	integrations, err := store.ListIntegrations(r.Context(), agentID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	for _, i := range integrations {
		if i.Platform == def.Platform {
			store.DeleteIntegration(r.Context(), i.ID)
		}
	}
	writeJSON(w, 200, map[string]string{"status": "revoked"})
}

// socialFetchAccountInfo calls the platform API to get the display name and ID
// after a successful OAuth token exchange.
func (gw *Gateway) socialFetchAccountInfo(platform, token string) (name, id string) {
	client := &http.Client{Timeout: 10 * time.Second}
	switch platform {
	case "twitter":
		req, _ := http.NewRequest("GET", "https://api.twitter.com/2/users/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			var r struct {
				Data struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Username string `json:"username"`
				} `json:"data"`
			}
			json.NewDecoder(resp.Body).Decode(&r)
			return "@" + r.Data.Username, r.Data.ID
		}
	case "linkedin":
		req, _ := http.NewRequest("GET", "https://api.linkedin.com/v2/userinfo", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			var r struct {
				Sub  string `json:"sub"`
				Name string `json:"name"`
			}
			json.NewDecoder(resp.Body).Decode(&r)
			return r.Name, r.Sub
		}
	case "facebook", "instagram":
		req, _ := http.NewRequest("GET", "https://graph.facebook.com/me?fields=id,name&access_token="+token, nil)
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			var r struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			json.NewDecoder(resp.Body).Decode(&r)
			return r.Name, r.ID
		}
	case "youtube", "googlemybusiness":
		req, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			var r struct {
				ID    string `json:"id"`
				Email string `json:"email"`
				Name  string `json:"name"`
			}
			json.NewDecoder(resp.Body).Decode(&r)
			return r.Name, r.ID
		}
	case "reddit":
		req, _ := http.NewRequest("GET", "https://oauth.reddit.com/api/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "Qorven/1.0")
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			var r struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			json.NewDecoder(resp.Body).Decode(&r)
			return "u/" + r.Name, r.ID
		}
	case "discord":
		req, _ := http.NewRequest("GET", "https://discord.com/api/users/@me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			var r struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			}
			json.NewDecoder(resp.Body).Decode(&r)
			return r.Username, r.ID
		}
	case "slack":
		req, _ := http.NewRequest("GET", "https://slack.com/api/auth.test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			var r struct {
				UserID string `json:"user_id"`
				Team   string `json:"team"`
				User   string `json:"user"`
			}
			json.NewDecoder(resp.Body).Decode(&r)
			return r.User + " (" + r.Team + ")", r.UserID
		}
	case "medium":
		req, _ := http.NewRequest("GET", "https://api.medium.com/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			var r struct {
				Data struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Username string `json:"username"`
				} `json:"data"`
			}
			json.NewDecoder(resp.Body).Decode(&r)
			return "@" + r.Data.Username, r.Data.ID
		}
	}
	return platform + "-account", ""
}

// ─── Social OAuth App Credentials (operator configuration) ────────────────────
//
// Before users can click "Connect with Twitter", the operator must register
// a Twitter developer app and paste the client_id + client_secret here.
// Stored in the vault as __oauth_app_social_{platform}__.
//
// GET    /v1/social/oauth/apps                — list all platforms + status
// POST   /v1/social/oauth/apps/{platform}     — save client_id + client_secret
// DELETE /v1/social/oauth/apps/{platform}     — remove creds

// handleSocialOAuthAppsGet lists all platforms with whether credentials are configured.
func (gw *Gateway) handleSocialOAuthAppsGet(w http.ResponseWriter, r *http.Request) {
	type platformStatus struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		HasCreds    bool   `json:"has_creds"`
		PKCE        bool   `json:"pkce"`
		RedirectURI string `json:"redirect_uri"`
		DocsURL     string `json:"docs_url"`
	}
	out := make([]platformStatus, 0, len(socialOAuthDefs))
	for id, def := range socialOAuthDefs {
		clientID, _ := gw.socialOAuthCreds(id)
		out = append(out, platformStatus{
			ID:          id,
			Name:        def.Name,
			HasCreds:    clientID != "" || def.PKCE,
			PKCE:        def.PKCE,
			RedirectURI: gw.socialOAuthRedirectURI(id),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out})
}

// handleSocialOAuthAppSet saves client_id + client_secret for a social platform.
func (gw *Gateway) handleSocialOAuthAppSet(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil || gw.vault == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	platform := chi.URLParam(r, "platform")
	if _, ok := socialOAuthDefs[platform]; !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unsupported platform: " + platform})
		return
	}

	var body struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	body.ClientID = strings.TrimSpace(body.ClientID)
	body.ClientSecret = strings.TrimSpace(body.ClientSecret)
	if body.ClientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_id is required"})
		return
	}

	if err := writeVaultOAuthApp(r.Context(), gw.vault, "default", "social_"+platform, body.ClientID, body.ClientSecret); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("vault write: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "platform": platform})
}

// handleSocialOAuthAppDelete removes stored credentials for a social platform.
func (gw *Gateway) handleSocialOAuthAppDelete(w http.ResponseWriter, r *http.Request) {
	if gw.vault == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "vault not available"})
		return
	}
	platform := chi.URLParam(r, "platform")
	if _, ok := socialOAuthDefs[platform]; !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unsupported platform: " + platform})
		return
	}
	_ = deleteVaultOAuthApp(r.Context(), gw.vault, "default", "social_"+platform)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
