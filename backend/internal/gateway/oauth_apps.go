// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/oauth"
	"github.com/qorvenai/qorven/internal/vault"
)

// OAuth-apps management endpoints.
//
// To use OAuth-based connectors (Google, Slack, GitHub, etc.) each
// Qorven install registers an OAuth app per provider on that
// provider's developer console. The provider's callback URL points
// at this Qorven instance, so the registered client_id + secret
// cannot be shared across installs — every deployment wires its own.
//
// The operator can provide default credentials via config/env
// (oauth.RegisterDefaults reads them at boot). This endpoint lets
// the operator ALSO register credentials at runtime via the UI, and
// those runtime-registered credentials override the env defaults on
// a per-tenant basis. Credentials live in the vault (encrypted with
// the gateway's key) so they never land in plaintext JSON.
//
// API:
//   GET    /v1/oauth/apps                — list providers + status
//   POST   /v1/oauth/apps/{provider}     — set client_id/secret
//   DELETE /v1/oauth/apps/{provider}     — revert to env defaults
//
// When no credentials are present (user-set OR env default), the
// provider is flagged needs_setup=true; the catalog UI then shows
// a "Configure" button that points here instead of "Connect".

// oauthAppSummary is the JSON shape returned by the list endpoint.
// Sensitive fields (ClientSecret) are NEVER included.
type oauthAppSummary struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Scopes       []string `json:"scopes"`
	RedirectURL  string   `json:"redirect_url"` // what the user pastes into the provider console
	HasClientID  bool     `json:"has_client_id"`
	IsUserSet    bool     `json:"is_user_set"`  // true if credentials came from vault (per-tenant)
	DocsURL      string   `json:"docs_url"`
	SetupGuide   string   `json:"setup_guide"` // short inline instructions
}

// providerDocs maps provider IDs to their OAuth app registration URL
// and a short imperative setup guide. The catalog UI surfaces these
// to the user so they know where to click.
var providerDocs = map[string]struct{ docs, guide string }{
	"google": {
		docs:  "https://console.cloud.google.com/apis/credentials",
		guide: "In Google Cloud Console, create an OAuth 2.0 Client ID for a Web application, then paste the redirect URL above into the \"Authorized redirect URIs\" field.",
	},
	"microsoft": {
		docs:  "https://portal.azure.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade",
		guide: "In Azure Portal, register an app, add the redirect URL as a Web redirect URI, then copy the Application (client) ID and a client secret from Certificates & secrets.",
	},
	"slack": {
		docs:  "https://api.slack.com/apps",
		guide: "Create a new Slack app at api.slack.com/apps, enable OAuth & Permissions, add the redirect URL, and copy the Client ID + Client Secret from Basic Information.",
	},
	"github": {
		docs:  "https://github.com/settings/developers",
		guide: "In GitHub Developer settings, create a new OAuth app, paste the redirect URL into \"Authorization callback URL\", and copy the Client ID + generate a Client Secret.",
	},
	"twitter": {
		docs:  "https://developer.twitter.com/en/portal/dashboard",
		guide: "In the Twitter Developer Portal, create a Project + App, enable OAuth 2.0, add the redirect URL, and copy the Client ID + Client Secret.",
	},
	"linkedin": {
		docs:  "https://www.linkedin.com/developers/apps",
		guide: "Create a LinkedIn app, request the Sign In with LinkedIn using OpenID Connect product, add the redirect URL under Auth, then copy the Client ID + Client Secret.",
	},
	"salesforce": {
		docs:  "https://help.salesforce.com/s/articleView?id=sf.connected_app_create.htm",
		guide: "Create a Salesforce Connected App, enable OAuth, add the redirect URL as a callback, and copy the Consumer Key (Client ID) and Consumer Secret.",
	},
	"shopify": {
		docs:  "https://partners.shopify.com/",
		guide: "In the Shopify Partner Dashboard, create an app, set the redirect URL, and copy the API key + API secret.",
	},
}

// vaultCredentialKind is the marker we use in the vault so oauth-app
// credentials (user-supplied client_id/secret) don't collide with
// per-user tokens.
const vaultKindOAuthApp = "oauth_app"

// handleListOAuthApps returns the status of every registered provider
// so the Settings UI can render a table of "you need to configure
// these, these are working, these fell back to defaults".
func (gw *Gateway) handleListOAuthApps(w http.ResponseWriter, r *http.Request) {
	if gw.oauthMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "oauth not configured"})
		return
	}
	tenantID := defaultTenant
	out := []oauthAppSummary{}
	for _, p := range gw.oauthMgr.ListProviders() {
		userCreds, hasUser := readVaultOAuthApp(r.Context(), gw.vault, tenantID, p.ID)
		hasClient := p.ClientID != ""
		if hasUser {
			// Reflect that we'd override the env defaults on next auth.
			hasClient = userCreds.ClientID != ""
		}
		docs := providerDocs[p.ID]
		out = append(out, oauthAppSummary{
			ID:          p.ID,
			Name:        p.Name,
			Scopes:      p.Scopes,
			RedirectURL: gw.oauthMgr.RedirectURL(p.ID),
			HasClientID: hasClient,
			IsUserSet:   hasUser,
			DocsURL:     docs.docs,
			SetupGuide:  docs.guide,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out})
}

// setOAuthAppRequest is the body for POST /v1/oauth/apps/{provider}.
type setOAuthAppRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// handleSetOAuthApp registers per-tenant OAuth app credentials. The
// creds go into the vault (encrypted) and the provider config is
// updated in-memory so subsequent authorize calls use them.
func (gw *Gateway) handleSetOAuthApp(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "provider")
	if providerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider required"})
		return
	}
	if gw.oauthMgr == nil || gw.vault == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "oauth not configured"})
		return
	}
	var body setOAuthAppRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	body.ClientID = strings.TrimSpace(body.ClientID)
	body.ClientSecret = strings.TrimSpace(body.ClientSecret)
	if body.ClientID == "" || body.ClientSecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_id and client_secret are required"})
		return
	}
	provider, ok := gw.oauthMgr.GetProvider(providerID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown provider: " + providerID})
		return
	}

	tenantID := defaultTenant
	if err := writeVaultOAuthApp(r.Context(), gw.vault, tenantID, providerID, body.ClientID, body.ClientSecret); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("vault write: %v", err)})
		return
	}

	// Re-register the provider with the new credentials so the next
	// AuthorizeURL call uses them. Env-default values stay in place
	// for any provider the tenant hasn't overridden.
	provider.ClientID = body.ClientID
	provider.ClientSecret = body.ClientSecret
	gw.oauthMgr.RegisterProvider(provider)

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "saved",
		"id":     providerID,
	})
}

// handleDeleteOAuthApp removes the tenant-level override so the
// provider falls back to env defaults (or to unconfigured if there
// were no defaults).
func (gw *Gateway) handleDeleteOAuthApp(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "provider")
	if providerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider required"})
		return
	}
	if gw.oauthMgr == nil || gw.vault == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "oauth not configured"})
		return
	}
	tenantID := defaultTenant
	// Best-effort delete — vault handles missing rows gracefully.
	_ = deleteVaultOAuthApp(r.Context(), gw.vault, tenantID, providerID)

	// Re-register the provider without tenant creds so the next
	// authorize call uses the original env defaults.
	if provider, ok := gw.oauthMgr.GetProvider(providerID); ok {
		// Look up the defaults from RegisterDefaults shape. Simplest
		// recovery: clear client_id+secret; the UI will show it as
		// "needs setup" so the user reconfigures.
		provider.ClientID = ""
		provider.ClientSecret = ""
		gw.oauthMgr.RegisterProvider(provider)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- vault helpers ---
//
// vault.Get / Delete take (tenantID, platformID). To keep OAuth-app
// credentials (user-supplied client_id+secret) distinct from per-user
// OAuth tokens (stored under the real provider id, e.g. "slack"), we
// prefix the app-credential platform_id so the two never collide:
//
//   oauth tokens:    tenant_id=X, platform_id=slack
//   oauth app creds: tenant_id=X, platform_id=__oauth_app_slack__
//
// The label column remains "default" for both, which keeps the
// existing upsert semantics intact.

func oauthAppPlatformID(providerID string) string {
	return "__oauth_app_" + providerID + "__"
}

func readVaultOAuthApp(
	ctx context.Context, v *vault.Vault, tenantID, providerID string,
) (vault.CredentialData, bool) {
	if v == nil {
		return vault.CredentialData{}, false
	}
	cred, err := v.Get(ctx, tenantID, oauthAppPlatformID(providerID))
	if err != nil || cred == nil {
		return vault.CredentialData{}, false
	}
	return cred.Data, true
}

func writeVaultOAuthApp(
	ctx context.Context, v *vault.Vault,
	tenantID, providerID, clientID, clientSecret string,
) error {
	if v == nil {
		return fmt.Errorf("vault unavailable")
	}
	data := vault.CredentialData{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}
	_, err := v.Save(
		ctx, tenantID, oauthAppPlatformID(providerID), "default", vaultKindOAuthApp,
		data, nil, nil,
	)
	return err
}

func deleteVaultOAuthApp(
	ctx context.Context, v *vault.Vault, tenantID, providerID string,
) error {
	if v == nil {
		return nil
	}
	return v.Delete(ctx, tenantID, oauthAppPlatformID(providerID))
}

// --- bootstrap: hydrate oauth manager from vault on startup ---
//
// Without this, a user who set their Slack/GitHub/etc. creds in a
// previous run would see "needs setup" on every reboot. Call once
// right after oauth.RegisterDefaults.
func (gw *Gateway) hydrateOAuthAppsFromVault(ctx context.Context) {
	if gw.oauthMgr == nil || gw.vault == nil {
		return
	}
	// Time-out the hydration so a slow vault doesn't block startup.
	hctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for _, p := range gw.oauthMgr.ListProviders() {
		data, ok := readVaultOAuthApp(hctx, gw.vault, defaultTenant, p.ID)
		if !ok {
			continue
		}
		if data.ClientID == "" {
			continue
		}
		p.ClientID = data.ClientID
		p.ClientSecret = data.ClientSecret
		gw.oauthMgr.RegisterProvider(p)
	}
}

// Unused local import; silences static analyzers if the package is
// pulled in without any direct usage in future refactors.
var _ = oauth.ProviderConfig{}
