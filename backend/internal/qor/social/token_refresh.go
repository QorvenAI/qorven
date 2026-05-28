// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TokenRefreshWorker periodically refreshes expiring OAuth tokens.
// It runs as a background goroutine started at gateway startup.
type TokenRefreshWorker struct {
	store     *Store
	encryptFn func(plain string) (string, error)
	decryptFn func(enc string) (string, error)
	// credsFn returns client_id + client_secret for a platform
	credsFn func(platform Platform) (clientID, clientSecret string)
	client  *http.Client
}

func NewTokenRefreshWorker(
	store *Store,
	encrypt func(string) (string, error),
	decrypt func(string) (string, error),
	creds func(Platform) (string, string),
) *TokenRefreshWorker {
	return &TokenRefreshWorker{
		store:     store,
		encryptFn: encrypt,
		decryptFn: decrypt,
		credsFn:   creds,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Start runs the refresh loop until ctx is cancelled.
// Checks every 15 minutes; refreshes tokens expiring within 24 hours.
func (w *TokenRefreshWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	// Run once immediately at startup
	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *TokenRefreshWorker) runOnce(ctx context.Context) {
	integrations, err := w.store.ListExpiringSoon(ctx, 24*time.Hour)
	if err != nil {
		slog.Error("social.token_refresh.list_failed", "err", err)
		return
	}
	if len(integrations) == 0 {
		return
	}
	slog.Info("social.token_refresh.running", "count", len(integrations))
	for _, integ := range integrations {
		var lastErr error
		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 {
				backoff := time.Duration(1<<uint(attempt)) * 2 * time.Second
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
			}
			if lastErr = w.refresh(ctx, integ); lastErr == nil {
				break
			}
			slog.Warn("social.token_refresh.retry",
				"integration_id", integ.ID, "platform", integ.Platform,
				"attempt", attempt+1, "err", lastErr)
		}
		if lastErr != nil {
			slog.Error("social.token_refresh.permanently_failed",
				"integration_id", integ.ID, "platform", integ.Platform, "err", lastErr)
			w.store.MarkNeedsReconnect(ctx, integ.ID)
		}
	}
}

func (w *TokenRefreshWorker) refresh(ctx context.Context, integ Integration) error {
	if integ.RefreshToken == "" {
		return fmt.Errorf("no refresh token")
	}

	// Decrypt tokens for use
	refreshToken := integ.RefreshToken
	if w.decryptFn != nil {
		if plain, err := w.decryptFn(integ.RefreshToken); err == nil && plain != "" {
			refreshToken = plain
		}
	}

	clientID, clientSecret := w.credsFn(integ.Platform)

	tokenURL, ok := platformTokenURL[integ.Platform]
	if !ok {
		return fmt.Errorf("no token URL for platform %s", integ.Platform)
	}

	formData := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}
	if clientSecret != "" {
		formData.Set("client_secret", clientSecret)
	}

	resp, err := w.client.PostForm(tokenURL, formData)
	if err != nil {
		return fmt.Errorf("token refresh request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return fmt.Errorf("parse token response: %w", err)
	}
	if tok.Error != "" {
		return fmt.Errorf("token refresh error: %s", tok.Error)
	}
	if tok.AccessToken == "" {
		return fmt.Errorf("empty access token in response")
	}

	// Encrypt new tokens
	newAccess := tok.AccessToken
	if w.encryptFn != nil {
		if enc, err := w.encryptFn(tok.AccessToken); err == nil {
			newAccess = enc
		}
	}
	newRefresh := integ.RefreshToken // keep old if not rotated
	if tok.RefreshToken != "" {
		if w.encryptFn != nil {
			if enc, err := w.encryptFn(tok.RefreshToken); err == nil {
				newRefresh = enc
			}
		} else {
			newRefresh = tok.RefreshToken
		}
	}

	var expiry *time.Time
	if tok.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
		expiry = &t
	}

	if err := w.store.UpdateTokens(ctx, integ.ID, newAccess, newRefresh, expiry); err != nil {
		return fmt.Errorf("store tokens: %w", err)
	}

	slog.Info("social.token_refresh.ok",
		"integration_id", integ.ID, "platform", integ.Platform, "account", integ.AccountName)
	return nil
}

// platformTokenURL maps each OAuth platform to its token refresh endpoint.
var platformTokenURL = map[Platform]string{
	PlatformTwitter:     "https://api.twitter.com/2/oauth2/token",
	PlatformLinkedIn:    "https://www.linkedin.com/oauth/v2/accessToken",
	PlatformFacebook:    "https://graph.facebook.com/v20.0/oauth/access_token",
	PlatformInstagram:   "https://graph.facebook.com/v20.0/oauth/access_token",
	PlatformThreads:     "https://graph.threads.net/refresh_access_token",
	PlatformTikTok:      "https://open.tiktokapis.com/v2/oauth/token/",
	PlatformYouTube:     "https://oauth2.googleapis.com/token",
	PlatformPinterest:   "https://api.pinterest.com/v5/oauth/token",
	PlatformReddit:      "https://www.reddit.com/api/v1/access_token",
	PlatformDiscord:     "https://discord.com/api/oauth2/token",
	PlatformSlack:       "https://slack.com/api/oauth.v2.access",
	PlatformMedium:      "https://api.medium.com/v1/tokens",
	PlatformGoogleMyBiz: "https://oauth2.googleapis.com/token",
}

// threadsFallback handles Threads' non-standard refresh which uses GET not POST.
// Called automatically when the standard POST fails for Threads.
func refreshThreadsToken(client *http.Client, refreshToken string) (string, int, error) {
	u := fmt.Sprintf("https://graph.threads.net/refresh_access_token?grant_type=th_refresh_token&access_token=%s",
		url.QueryEscape(refreshToken))
	resp, err := client.Get(u)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var r struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", 0, err
	}
	return r.AccessToken, r.ExpiresIn, nil
}

// FacebookLongLivedTokenExchange converts a short-lived Facebook/Instagram token
// to a long-lived (~59 day) token. Called on first connect, not refresh.
func FacebookLongLivedTokenExchange(ctx context.Context, shortToken, clientID, clientSecret string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	params := url.Values{
		"grant_type":        {"fb_exchange_token"},
		"client_id":         {clientID},
		"client_secret":     {clientSecret},
		"fb_exchange_token": {shortToken},
	}
	resp, err := client.PostForm("https://graph.facebook.com/v20.0/oauth/access_token", params)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var r struct {
		AccessToken string `json:"access_token"`
		Error       struct{ Message string `json:"message"` } `json:"error"`
	}
	json.Unmarshal(body, &r)
	if r.Error.Message != "" {
		return "", fmt.Errorf("fb token exchange: %s", r.Error.Message)
	}
	if r.AccessToken == "" {
		return "", fmt.Errorf("empty token in fb exchange response: %s", strings.TrimSpace(string(body)))
	}
	return r.AccessToken, nil
}
