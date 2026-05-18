// Copyright 2026 Tekky AI Academy LLP. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/tools"
)

// handleProjectGitHubConnect binds a GitHub owner/repo to a project and
// generates a per-project webhook HMAC secret.
//
// POST /v1/projects/:id/github/connect
// Body: { "owner": "acme", "repo": "backend", "default_branch": "main" }
// Response: { "webhook_url": "...", "webhook_secret": "..." }
func (gw *Gateway) handleProjectGitHubConnect(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil {
		writeJSON(w, 503, map[string]string{"error": "projects not initialized"})
		return
	}
	projectID := chi.URLParam(r, "id")
	p := gw.projectReg.Get(projectID)
	if p == nil {
		writeJSON(w, 404, map[string]string{"error": "project not found"})
		return
	}

	var req struct {
		Owner         string `json:"owner"`
		Repo          string `json:"repo"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Owner == "" || req.Repo == "" {
		writeJSON(w, 400, map[string]string{"error": "owner and repo are required"})
		return
	}
	if req.DefaultBranch == "" {
		req.DefaultBranch = "main"
	}

	// Generate a 32-byte (64-char hex) per-project webhook secret.
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to generate secret"})
		return
	}
	secret := hex.EncodeToString(secretBytes)

	gw.projectReg.UpdateBuild(projectID, func(proj *tools.CodeProject) {
		proj.GitHubOwner = req.Owner
		proj.GitHubRepo = req.Repo
		proj.GitHubSecret = secret
		proj.DefaultBranch = req.DefaultBranch
	})

	baseURL := "https://qorven.ai"
	if gw.cfg != nil && gw.cfg.Server.BaseURL != "" {
		baseURL = strings.TrimRight(gw.cfg.Server.BaseURL, "/")
	}
	webhookURL := fmt.Sprintf("%s/v1/webhooks/github?project=%s", baseURL, projectID)

	writeJSON(w, 200, map[string]string{
		"webhook_url":    webhookURL,
		"webhook_secret": secret,
		"owner":          req.Owner,
		"repo":           req.Repo,
		"default_branch": req.DefaultBranch,
	})
}

// handleProjectGitHubDisconnect removes the GitHub binding from a project.
//
// DELETE /v1/projects/:id/github/connect
func (gw *Gateway) handleProjectGitHubDisconnect(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil {
		writeJSON(w, 503, map[string]string{"error": "projects not initialized"})
		return
	}
	projectID := chi.URLParam(r, "id")
	if gw.projectReg.Get(projectID) == nil {
		writeJSON(w, 404, map[string]string{"error": "project not found"})
		return
	}
	gw.projectReg.UpdateBuild(projectID, func(proj *tools.CodeProject) {
		proj.GitHubOwner = ""
		proj.GitHubRepo = ""
		proj.GitHubSecret = ""
		proj.DefaultBranch = ""
	})
	w.WriteHeader(204)
}

// handleProjectGitHubStatus returns the GitHub connection status and live
// counts fetched from the GitHub API.
//
// GET /v1/projects/:id/github/status
func (gw *Gateway) handleProjectGitHubStatus(w http.ResponseWriter, r *http.Request) {
	if gw.projectReg == nil {
		writeJSON(w, 503, map[string]string{"error": "projects not initialized"})
		return
	}
	projectID := chi.URLParam(r, "id")
	p := gw.projectReg.Get(projectID)
	if p == nil {
		writeJSON(w, 404, map[string]string{"error": "project not found"})
		return
	}

	if p.GitHubOwner == "" || p.GitHubRepo == "" {
		writeJSON(w, 200, map[string]any{"connected": false})
		return
	}

	// Fetch open PR + issue counts from GitHub API (best-effort; errors return 0).
	openPRs, openIssues := 0, 0
	prData, _, err := gw.ghProxy(r.Context(), fmt.Sprintf("/repos/%s/%s/pulls", p.GitHubOwner, p.GitHubRepo),
		url.Values{"state": []string{"open"}, "per_page": []string{"1"}})
	if err == nil {
		prs := []json.RawMessage{}
		if json.Unmarshal(prData, &prs) == nil {
			// GitHub doesn't expose count in list — use Link header heuristic; fallback to len.
			openPRs = len(prs)
		}
	}
	issueData, _, err := gw.ghProxy(r.Context(), fmt.Sprintf("/repos/%s/%s/issues", p.GitHubOwner, p.GitHubRepo),
		url.Values{"state": []string{"open"}, "per_page": []string{"1"}})
	if err == nil {
		issues := []json.RawMessage{}
		if json.Unmarshal(issueData, &issues) == nil {
			openIssues = len(issues)
		}
	}

	writeJSON(w, 200, map[string]any{
		"connected":      true,
		"owner":          p.GitHubOwner,
		"repo":           p.GitHubRepo,
		"default_branch": p.DefaultBranch,
		"open_prs":       openPRs,
		"open_issues":    openIssues,
	})
}

// handleGitHubMergePR proxies a PR merge to the GitHub API.
//
// POST /v1/github/:owner/:repo/pulls/:pr/merge
// Body: { "merge_method": "squash|merge|rebase", "commit_title": "..." }
func (gw *Gateway) handleGitHubMergePR(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	prNumStr := chi.URLParam(r, "pr")
	prNum, err := strconv.Atoi(prNumStr)
	if err != nil || prNum <= 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid pr number"})
		return
	}

	var body struct {
		MergeMethod  string `json:"merge_method"`
		CommitTitle  string `json:"commit_title"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.MergeMethod == "" {
		body.MergeMethod = "squash"
	}

	tok := gw.ghVaultToken(r.Context())
	if tok == "" {
		writeJSON(w, 401, map[string]string{"error": "no GitHub token configured"})
		return
	}

	payload := map[string]any{"merge_method": body.MergeMethod}
	if body.CommitTitle != "" {
		payload["commit_title"] = body.CommitTitle
	}
	payloadBytes, _ := json.Marshal(payload)

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/merge", owner, repo, prNum)
	req, _ := http.NewRequestWithContext(r.Context(), "PUT", apiURL, strings.NewReader(string(payloadBytes)))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode >= 400 {
		var ghErr struct{ Message string `json:"message"` }
		json.Unmarshal(respBody, &ghErr)
		msg := ghErr.Message
		if msg == "" {
			msg = fmt.Sprintf("GitHub API error %d", resp.StatusCode)
		}
		writeJSON(w, resp.StatusCode, map[string]string{"error": msg})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(respBody)
}

// handleGitHubCloseIssue closes a GitHub issue via the API.
//
// POST /v1/github/:owner/:repo/issues/:number/close
func (gw *Gateway) handleGitHubCloseIssue(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	numStr := chi.URLParam(r, "number")
	num, err := strconv.Atoi(numStr)
	if err != nil || num <= 0 {
		writeJSON(w, 400, map[string]string{"error": "invalid issue number"})
		return
	}

	tok := gw.ghVaultToken(r.Context())
	if tok == "" {
		writeJSON(w, 401, map[string]string{"error": "no GitHub token configured"})
		return
	}

	payload := `{"state":"closed"}`
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d", owner, repo, num)
	req, _ := http.NewRequestWithContext(r.Context(), "PATCH", apiURL, strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var ghErr struct{ Message string `json:"message"` }
		json.Unmarshal(respBody, &ghErr)
		msg := ghErr.Message
		if msg == "" {
			msg = fmt.Sprintf("GitHub API error %d", resp.StatusCode)
		}
		writeJSON(w, resp.StatusCode, map[string]string{"error": msg})
		return
	}

	writeJSON(w, 200, map[string]any{"closed": true, "number": num})
}

// ghProxyPost is a helper for POST/PUT/PATCH calls to the GitHub API (not on ghProxy which is GET-only).
func (gw *Gateway) ghProxyMutate(ctx context.Context, method, path string, body []byte) (json.RawMessage, int, error) {
	tok := gw.ghVaultToken(ctx)
	if tok == "" {
		return nil, 401, fmt.Errorf("no GitHub token configured")
	}
	apiURL := "https://api.github.com" + path
	var reqBody io.Reader
	if body != nil {
		reqBody = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL, reqBody)
	if err != nil {
		return nil, 500, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 500, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode >= 400 {
		var ghErr struct{ Message string `json:"message"` }
		json.Unmarshal(respBody, &ghErr)
		msg := ghErr.Message
		if msg == "" {
			msg = fmt.Sprintf("GitHub API error %d", resp.StatusCode)
		}
		return nil, resp.StatusCode, fmt.Errorf("%s", msg)
	}
	return json.RawMessage(respBody), resp.StatusCode, nil
}
