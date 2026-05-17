// Copyright 2026 Tekky AI Academy LLP. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
)

// ghVaultToken returns the GitHub PAT from env or vault.
func (gw *Gateway) ghVaultToken(ctx context.Context) string {
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		return tok
	}
	if gw.vault != nil {
		if cred, err := gw.vault.Get(ctx, defaultTenant, "github"); err == nil {
			if cred.Data.APIKey != "" {
				return cred.Data.APIKey
			}
			if cred.Data.AccessToken != "" {
				return cred.Data.AccessToken
			}
		}
	}
	return ""
}

// ghProxy forwards a request to the GitHub REST API and returns the raw JSON.
func (gw *Gateway) ghProxy(ctx context.Context, path string, query url.Values) (json.RawMessage, int, error) {
	tok := gw.ghVaultToken(ctx)
	if tok == "" {
		return nil, 401, fmt.Errorf("no GitHub token configured — add one in Settings → Provider Keys → GitHub")
	}

	u := "https://api.github.com" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, 500, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 500, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))

	if resp.StatusCode >= 400 {
		var ghErr struct {
			Message string `json:"message"`
		}
		json.Unmarshal(body, &ghErr)
		msg := ghErr.Message
		if msg == "" {
			msg = fmt.Sprintf("GitHub API error %d", resp.StatusCode)
		}
		return nil, resp.StatusCode, fmt.Errorf("%s", msg)
	}

	return json.RawMessage(body), resp.StatusCode, nil
}

// handleGitHubRepoInfo proxies GET /repos/{owner}/{repo}.
func (gw *Gateway) handleGitHubRepoInfo(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	data, _, err := gw.ghProxy(r.Context(), fmt.Sprintf("/repos/%s/%s", owner, repo), nil)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(data)
}

// handleGitHubListIssues proxies GET /repos/{owner}/{repo}/issues.
// Query params forwarded: state, labels, assignee, per_page, sort.
func (gw *Gateway) handleGitHubListIssues(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	q := url.Values{}
	q.Set("state", firstOf(r.URL.Query().Get("state"), "open"))
	q.Set("per_page", firstOf(r.URL.Query().Get("limit"), "30"))
	q.Set("sort", "updated")
	q.Set("direction", "desc")
	if label := r.URL.Query().Get("labels"); label != "" {
		q.Set("labels", label)
	}
	if assignee := r.URL.Query().Get("assignee"); assignee != "" {
		q.Set("assignee", assignee)
	}

	data, _, err := gw.ghProxy(r.Context(), fmt.Sprintf("/repos/%s/%s/issues", owner, repo), q)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}

	// Filter out PRs (GitHub issues endpoint returns PRs too)
	items := []json.RawMessage{}
	json.Unmarshal(data, &items)
	filtered := []json.RawMessage{}
	for _, item := range items {
		var check struct {
			PullRequest *struct{} `json:"pull_request"`
		}
		if json.Unmarshal(item, &check) == nil && check.PullRequest == nil {
			filtered = append(filtered, item)
		}
	}
	if filtered == nil {
		filtered = []json.RawMessage{}
	}

	issuesJSON, _ := json.Marshal(filtered)
	writeJSON(w, 200, map[string]json.RawMessage{"issues": json.RawMessage(issuesJSON)})
}

// handleGitHubListPulls proxies GET /repos/{owner}/{repo}/pulls.
func (gw *Gateway) handleGitHubListPulls(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	q := url.Values{}
	q.Set("state", firstOf(r.URL.Query().Get("state"), "open"))
	q.Set("per_page", firstOf(r.URL.Query().Get("limit"), "20"))
	q.Set("sort", "updated")
	q.Set("direction", "desc")

	data, _, err := gw.ghProxy(r.Context(), fmt.Sprintf("/repos/%s/%s/pulls", owner, repo), q)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}

	pulls := []json.RawMessage{}
	json.Unmarshal(data, &pulls)
	if pulls == nil {
		pulls = []json.RawMessage{}
	}
	pullsJSON, _ := json.Marshal(pulls)
	writeJSON(w, 200, map[string]json.RawMessage{"pulls": json.RawMessage(pullsJSON)})
}

// handleGitHubPRChecks proxies GET /repos/{owner}/{repo}/commits/{ref}/check-runs
// for a given pull request number. First resolves the PR head SHA, then fetches checks.
func (gw *Gateway) handleGitHubPRChecks(w http.ResponseWriter, r *http.Request) {
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	prNum := chi.URLParam(r, "prNum")

	// Resolve PR head SHA.
	prData, status, err := gw.ghProxy(r.Context(), fmt.Sprintf("/repos/%s/%s/pulls/%s", owner, repo, prNum), nil)
	if err != nil || status != 200 {
		writeJSON(w, status, map[string]string{"error": fmt.Sprintf("pr lookup failed: %v", err)})
		return
	}
	var pr struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	if err := json.Unmarshal(prData, &pr); err != nil || pr.Head.SHA == "" {
		writeJSON(w, 502, map[string]string{"error": "could not parse PR head SHA"})
		return
	}

	data, _, err := gw.ghProxy(r.Context(), fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs", owner, repo, pr.Head.SHA), nil)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, json.RawMessage(data))
}

// firstOf returns the first non-empty string.
func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
