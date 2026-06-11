// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

// Deploy-target: cloud (Vercel / Netlify)
//
// Vault platform IDs:
//   "deploy:vercel"  — API token stored as APIKey or AccessToken
//   "deploy:netlify" — API token stored as APIKey or AccessToken
//
// Vercel: POST /v13/deployments (git source — triggers a new deployment from
// the connected GitHub repo at the given ref).  The response `url` field is
// the deployment hostname; we prepend https://.
//
// Netlify: POST /api/v1/sites/{site_id}/builds (trigger a rebuild of an
// existing site) when spec.ProjectID is set as the Netlify site ID.
// If the project does not carry a Netlify site ID we fall back to creating a
// new site linked to the GitHub repo and return the admin URL as the URL with
// a Detail note that the first build may still be running.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/qorvenai/qorven/internal/gateway/deploy"
)

// cloudDeployTarget deploys to an external cloud provider (vercel|netlify)
// using a token from the vault.
type cloudDeployTarget struct {
	gw       *Gateway
	provider string // "vercel" or "netlify"
}

// newCloudTarget constructs a cloudDeployTarget for the given provider.
func newCloudTarget(gw *Gateway, provider string) deploy.Target {
	return &cloudDeployTarget{gw: gw, provider: provider}
}

// Deploy triggers a cloud deployment for the project described by spec.
func (t *cloudDeployTarget) Deploy(ctx context.Context, s deploy.Spec) (deploy.Result, error) {
	// Precondition 1: a connected GitHub repo is required.
	if s.RepoOwner == "" || s.RepoName == "" {
		return deploy.Result{}, fmt.Errorf("cloud deploy requires a connected GitHub repo")
	}

	// Precondition 2: provider token must exist in the vault.
	token, err := t.cloudProviderToken(ctx)
	if err != nil {
		return deploy.Result{}, err
	}

	switch t.provider {
	case "vercel":
		return t.deployVercel(ctx, s, token)
	case "netlify":
		return t.deployNetlify(ctx, s, token)
	default:
		return deploy.Result{}, fmt.Errorf("unsupported cloud provider: %s", t.provider)
	}
}

// cloudProviderToken retrieves the API token for this provider from the vault.
func (t *cloudDeployTarget) cloudProviderToken(ctx context.Context) (string, error) {
	if t.gw.vault == nil {
		return "", fmt.Errorf("connect a %s token in Settings to deploy there", t.provider)
	}
	platformID := "deploy:" + t.provider
	cred, err := t.gw.vault.Get(ctx, defaultTenant, platformID)
	if err != nil {
		return "", fmt.Errorf("connect a %s token in Settings to deploy there", t.provider)
	}
	token := cred.Data.APIKey
	if token == "" {
		token = cred.Data.AccessToken
	}
	if token == "" {
		return "", fmt.Errorf("connect a %s token in Settings to deploy there", t.provider)
	}
	return token, nil
}

// --- Vercel ---

// vercelDeployBody is the JSON body for POST /v13/deployments.
type vercelDeployBody struct {
	Name      string            `json:"name"`
	GitSource vercelGitSource   `json:"gitSource"`
	ProjectSettings *vercelProjectSettings `json:"projectSettings,omitempty"`
}

type vercelGitSource struct {
	Type string `json:"type"` // "github"
	Org  string `json:"org"`
	Repo string `json:"repo"`
	Ref  string `json:"ref"`
}

type vercelProjectSettings struct {
	Framework string `json:"framework,omitempty"`
}

type vercelDeployResponse struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (t *cloudDeployTarget) deployVercel(ctx context.Context, s deploy.Spec, token string) (deploy.Result, error) {
	ref := s.ReleaseTag
	if ref == "" {
		ref = "main"
	}

	slug := s.Slug
	if slug == "" {
		slug = sanitizeSlug(s.ProjectID)
	}

	body := vercelDeployBody{
		Name: slug,
		GitSource: vercelGitSource{
			Type: "github",
			Org:  s.RepoOwner,
			Repo: s.RepoName,
			Ref:  ref,
		},
	}
	if s.Framework != "" {
		body.ProjectSettings = &vercelProjectSettings{Framework: s.Framework}
	}

	respBody, err := cloudPost(ctx, "https://api.vercel.com/v13/deployments", token, body)
	if err != nil {
		return deploy.Result{}, fmt.Errorf("vercel deploy: %w", err)
	}

	var resp vercelDeployResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return deploy.Result{}, fmt.Errorf("vercel deploy: parse response: %w", err)
	}
	if resp.Error != nil && resp.Error.Message != "" {
		return deploy.Result{}, fmt.Errorf("vercel deploy: %s", resp.Error.Message)
	}
	if resp.URL == "" {
		return deploy.Result{}, fmt.Errorf("vercel deploy: empty URL in response (deployment id: %s)", resp.ID)
	}

	deployURL := resp.URL
	if len(deployURL) > 0 && deployURL[0] != 'h' {
		deployURL = "https://" + deployURL
	}

	return deploy.Result{
		URL:    deployURL,
		Target: "cloud:vercel",
		Detail: fmt.Sprintf("vercel deployment id: %s; repo: %s/%s@%s", resp.ID, s.RepoOwner, s.RepoName, ref),
	}, nil
}

// --- Netlify ---

type netlifyBuildBody struct {
	ClearCache bool `json:"clear_cache"`
}

type netlifyCreateSiteBody struct {
	Name string            `json:"name"`
	Repo *netlifyRepoInfo  `json:"repo,omitempty"`
}

type netlifyRepoInfo struct {
	Provider string `json:"provider"`
	RepoPath string `json:"repo_path"` // "owner/repo"
	Branch   string `json:"branch"`
}

type netlifySiteResponse struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	AdminURL string `json:"admin_url"`
	Name    string `json:"name"`
	Error   string `json:"errors,omitempty"`
}

func (t *cloudDeployTarget) deployNetlify(ctx context.Context, s deploy.Spec, token string) (deploy.Result, error) {
	ref := s.ReleaseTag
	if ref == "" {
		ref = "main"
	}

	slug := s.Slug
	if slug == "" {
		slug = sanitizeSlug(s.ProjectID)
	}

	// If ProjectID looks like a Netlify site ID (alphanumeric with dashes, UUID-ish),
	// trigger a rebuild on the existing site.  Otherwise create a new site.
	//
	// Heuristic: Netlify site IDs are short alphanumeric-dash strings (not UUIDs).
	// We attempt a rebuild first; if that returns 404 we fall through to create.
	rebuildURL := fmt.Sprintf("https://api.netlify.com/api/v1/sites/%s/builds", s.ProjectID)
	resp, err := cloudPost(ctx, rebuildURL, token, netlifyBuildBody{ClearCache: false})
	if err == nil {
		// Rebuild triggered successfully.
		var build struct {
			ID      string `json:"id"`
			DeployID string `json:"deploy_id"`
		}
		_ = json.Unmarshal(resp, &build)
		adminURL := fmt.Sprintf("https://app.netlify.com/sites/%s/deploys", s.ProjectID)
		return deploy.Result{
			URL:    adminURL,
			Target: "cloud:netlify",
			Detail: fmt.Sprintf("netlify rebuild triggered on site %s (build id: %s); repo: %s/%s@%s", s.ProjectID, build.ID, s.RepoOwner, s.RepoName, ref),
		}, nil
	}

	// No existing site found or ProjectID is not a Netlify site ID — create one.
	createBody := netlifyCreateSiteBody{
		Name: slug,
		Repo: &netlifyRepoInfo{
			Provider: "github",
			RepoPath: s.RepoOwner + "/" + s.RepoName,
			Branch:   ref,
		},
	}
	siteResp, createErr := cloudPost(ctx, "https://api.netlify.com/api/v1/sites", token, createBody)
	if createErr != nil {
		return deploy.Result{}, fmt.Errorf("netlify deploy: %w", createErr)
	}

	var site netlifySiteResponse
	if err := json.Unmarshal(siteResp, &site); err != nil {
		return deploy.Result{}, fmt.Errorf("netlify deploy: parse response: %w", err)
	}

	siteURL := site.URL
	if siteURL == "" {
		siteURL = site.AdminURL
	}
	detail := fmt.Sprintf("netlify site created (id: %s); first build may still be running; repo: %s/%s@%s — confirm in %s", site.ID, s.RepoOwner, s.RepoName, ref, site.AdminURL)

	return deploy.Result{
		URL:    siteURL,
		Target: "cloud:netlify",
		Detail: detail,
	}, nil
}

// --- Shared HTTP helper (mirrors ghPost's client/timeout/Bearer/error pattern) ---

// cloudPost sends a JSON POST to url with a Bearer token and returns the raw
// response body.  Error messages are extracted from the JSON response when
// the server returns a 4xx/5xx status.
func cloudPost(ctx context.Context, url, token string, body any) (json.RawMessage, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))

	if resp.StatusCode >= 400 {
		// Try to extract a human-readable message from the JSON response.
		var apiErr struct {
			Message string `json:"message"` // Vercel
			Error   struct {
				Message string `json:"message"` // Vercel nested
			} `json:"error"`
			Errors []string `json:"errors"` // Netlify
		}
		_ = json.Unmarshal(respBody, &apiErr)
		msg := apiErr.Message
		if msg == "" {
			msg = apiErr.Error.Message
		}
		if msg == "" && len(apiErr.Errors) > 0 {
			msg = apiErr.Errors[0]
		}
		if msg == "" {
			msg = fmt.Sprintf("API error %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("%s", msg)
	}

	return json.RawMessage(respBody), nil
}
