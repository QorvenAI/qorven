// Copyright 2026 Tekky AI Academy LLP. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ─── Token getter injection ───────────────────────────────────────────────────
// Each tool optionally holds a getToken func that is called at execution time,
// allowing vault changes to take effect without restart. If getToken returns "",
// the tool falls back to the context-injected token (WithGitHubToken).

type tokenGetter func() string

// WithToken constructors — gateway registers these so hot-reload works.
func NewGhRepoInfoToolWithToken(get tokenGetter) *GhRepoInfoTool {
	t := NewGhRepoInfoTool(); t.getToken = get; return t
}
func NewGhListIssuesToolWithToken(get tokenGetter) *GhListIssuesTool {
	t := NewGhListIssuesTool(); t.getToken = get; return t
}
func NewGhReadIssueToolWithToken(get tokenGetter) *GhReadIssueTool {
	t := NewGhReadIssueTool(); t.getToken = get; return t
}
func NewGhCreateIssueToolWithToken(get tokenGetter) *GhCreateIssueTool {
	t := NewGhCreateIssueTool(); t.getToken = get; return t
}
func NewGhCreateBranchToolWithToken(get tokenGetter) *GhCreateBranchTool {
	t := NewGhCreateBranchTool(); t.getToken = get; return t
}
func NewGhPushFileToolWithToken(get tokenGetter) *GhPushFileTool {
	t := NewGhPushFileTool(); t.getToken = get; return t
}
func NewGhOpenPRToolWithToken(get tokenGetter) *GhOpenPRTool {
	t := NewGhOpenPRTool(); t.getToken = get; return t
}
func NewGhPostCommentToolWithToken(get tokenGetter) *GhPostCommentTool {
	t := NewGhPostCommentTool(); t.getToken = get; return t
}
func NewGhListPRChecksToolWithToken(get tokenGetter) *GhListPRChecksTool {
	t := NewGhListPRChecksTool(); t.getToken = get; return t
}
func NewGhMergePRToolWithToken(get tokenGetter) *GhMergePRTool {
	t := NewGhMergePRTool(); t.getToken = get; return t
}

// ─── GitHub API client (shared, stateless) ────────────────────────────────────

const ghAPIBase = "https://api.github.com"

type ghClient struct {
	token string
}

func (c *ghClient) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, ghAPIBase+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	return data, resp.StatusCode, nil
}

// ghResolveToken returns the best available GitHub token.
// Priority: tool-level getter (vault/hot-reload) → context injection → empty.
func ghResolveToken(ctx context.Context, get tokenGetter) string {
	if get != nil {
		if tok := get(); tok != "" {
			return tok
		}
	}
	if v, ok := ctx.Value(ctxKey("gh_token")).(string); ok && v != "" {
		return v
	}
	return ""
}

// ghTokenFromCtx retrieves the GitHub PAT stored in context by gateway.
func ghTokenFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey("gh_token")).(string); ok && v != "" {
		return v
	}
	return ""
}

// WithGitHubToken injects the GitHub PAT into a context so all gh_* tools
// pick it up without needing per-instance state.
func WithGitHubToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, ctxKey("gh_token"), token)
}

// ghError formats a GitHub API error response for the LLM.
func ghError(status int, body []byte) *Result {
	var e struct {
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &e); err == nil && e.Message != "" {
		if len(e.Errors) > 0 {
			msgs := make([]string, len(e.Errors))
			for i, er := range e.Errors {
				msgs[i] = er.Message
			}
			return ErrorResult(fmt.Sprintf("GitHub API error %d: %s — %s", status, e.Message, strings.Join(msgs, "; ")))
		}
		return ErrorResult(fmt.Sprintf("GitHub API error %d: %s", status, e.Message))
	}
	return ErrorResult(fmt.Sprintf("GitHub API error %d", status))
}

// ─── gh_repo_info ─────────────────────────────────────────────────────────────

// GhRepoInfoTool fetches metadata about a GitHub repository.
type GhRepoInfoTool struct{ getToken tokenGetter }

func NewGhRepoInfoTool() *GhRepoInfoTool { return &GhRepoInfoTool{} }

func (t *GhRepoInfoTool) Name() string { return "gh_repo_info" }
func (t *GhRepoInfoTool) Description() string {
	return `Get metadata about a GitHub repository: default branch, open issues count, description, topics, last push.
Use this first before working on a repo to understand its structure.`
}
func (t *GhRepoInfoTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner": map[string]any{"type": "string", "description": "GitHub org or user, e.g. 'qorven-ai'"},
			"repo":  map[string]any{"type": "string", "description": "Repository name, e.g. 'qorven-mono'"},
		},
		"required": []string{"owner", "repo"},
	}
}

func (t *GhRepoInfoTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	if owner == "" || repo == "" {
		return ErrorResult("owner and repo are required")
	}
	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured — add one in Settings → Provider Keys → GitHub")
	}

	c := &ghClient{token: tok}
	data, status, err := c.do(ctx, "GET", fmt.Sprintf("/repos/%s/%s", owner, repo), nil)
	if err != nil {
		return ErrorResult("request failed: " + err.Error())
	}
	if status != 200 {
		return ghError(status, data)
	}

	var r struct {
		FullName      string   `json:"full_name"`
		Description   string   `json:"description"`
		DefaultBranch string   `json:"default_branch"`
		OpenIssues    int      `json:"open_issues_count"`
		Stars         int      `json:"stargazers_count"`
		Language      string   `json:"language"`
		Topics        []string `json:"topics"`
		PushedAt      string   `json:"pushed_at"`
		Private       bool     `json:"private"`
		HTMLURL       string   `json:"html_url"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return ErrorResult("parse failed: " + err.Error())
	}

	sb := &strings.Builder{}
	fmt.Fprintf(sb, "**%s**\n", r.FullName)
	if r.Description != "" {
		fmt.Fprintf(sb, "Description: %s\n", r.Description)
	}
	fmt.Fprintf(sb, "Default branch: `%s`\n", r.DefaultBranch)
	fmt.Fprintf(sb, "Open issues: %d  |  Stars: %d  |  Language: %s\n", r.OpenIssues, r.Stars, r.Language)
	if len(r.Topics) > 0 {
		fmt.Fprintf(sb, "Topics: %s\n", strings.Join(r.Topics, ", "))
	}
	fmt.Fprintf(sb, "Last push: %s\n", r.PushedAt)
	fmt.Fprintf(sb, "URL: %s\n", r.HTMLURL)
	if r.Private {
		fmt.Fprintf(sb, "Visibility: private\n")
	}

	return TextResult(sb.String())
}

// ─── gh_list_issues ───────────────────────────────────────────────────────────

// GhListIssuesTool lists open issues, optionally filtered by label or state.
type GhListIssuesTool struct{ getToken tokenGetter }

func NewGhListIssuesTool() *GhListIssuesTool { return &GhListIssuesTool{} }

func (t *GhListIssuesTool) Name() string { return "gh_list_issues" }
func (t *GhListIssuesTool) Description() string {
	return `List issues in a GitHub repository. Supports filtering by state (open/closed/all), label, and assignee.
Returns issue number, title, labels, assignee, and creation date. Use gh_read_issue to get full body.`
}
func (t *GhListIssuesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner":    map[string]any{"type": "string"},
			"repo":     map[string]any{"type": "string"},
			"state":    map[string]any{"type": "string", "enum": []string{"open", "closed", "all"}, "description": "Default: open"},
			"label":    map[string]any{"type": "string", "description": "Filter by label name"},
			"assignee": map[string]any{"type": "string", "description": "Filter by GitHub username"},
			"limit":    map[string]any{"type": "integer", "description": "Max results (default 20, max 100)"},
		},
		"required": []string{"owner", "repo"},
	}
}

func (t *GhListIssuesTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	if owner == "" || repo == "" {
		return ErrorResult("owner and repo are required")
	}
	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured")
	}

	state := "open"
	if s, ok := args["state"].(string); ok && s != "" {
		state = s
	}
	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
		if limit > 100 {
			limit = 100
		}
	}

	q := url.Values{}
	q.Set("state", state)
	q.Set("per_page", fmt.Sprintf("%d", limit))
	q.Set("sort", "updated")
	if label, ok := args["label"].(string); ok && label != "" {
		q.Set("labels", label)
	}
	if assignee, ok := args["assignee"].(string); ok && assignee != "" {
		q.Set("assignee", assignee)
	}

	c := &ghClient{token: tok}
	path := fmt.Sprintf("/repos/%s/%s/issues?%s", owner, repo, q.Encode())
	data, status, err := c.do(ctx, "GET", path, nil)
	if err != nil {
		return ErrorResult("request failed: " + err.Error())
	}
	if status != 200 {
		return ghError(status, data)
	}

	var issues []struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		State     string `json:"state"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		HTMLURL   string `json:"html_url"`
		Assignee  *struct {
			Login string `json:"login"`
		} `json:"assignee"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		PullRequest *struct {
			URL string `json:"url"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(data, &issues); err != nil {
		return ErrorResult("parse failed: " + err.Error())
	}

	if len(issues) == 0 {
		return TextResult(fmt.Sprintf("No %s issues found in %s/%s", state, owner, repo))
	}

	sb := &strings.Builder{}
	fmt.Fprintf(sb, "**%d %s issue(s) in %s/%s:**\n\n", len(issues), state, owner, repo)
	for _, iss := range issues {
		// Skip PRs (GitHub API returns PRs in issues endpoint)
		if iss.PullRequest != nil {
			continue
		}
		labels := make([]string, len(iss.Labels))
		for i, l := range iss.Labels {
			labels[i] = l.Name
		}
		assignee := "unassigned"
		if iss.Assignee != nil {
			assignee = "@" + iss.Assignee.Login
		}
		updated := iss.UpdatedAt
		if len(updated) >= 10 {
			updated = updated[:10]
		}
		fmt.Fprintf(sb, "- #%d **%s** [%s] assignee:%s updated:%s\n",
			iss.Number, iss.Title, strings.Join(labels, ","), assignee, updated)
	}
	return TextResult(sb.String())
}

// ─── gh_read_issue ────────────────────────────────────────────────────────────

// GhReadIssueTool reads the full body, comments, and timeline of a specific issue.
type GhReadIssueTool struct{ getToken tokenGetter }

func NewGhReadIssueTool() *GhReadIssueTool { return &GhReadIssueTool{} }

func (t *GhReadIssueTool) Name() string { return "gh_read_issue" }
func (t *GhReadIssueTool) Description() string {
	return `Read the full details of a GitHub issue: title, body, all comments, labels, milestone, and assignees.
Call this before starting work on an issue so you fully understand requirements.`
}
func (t *GhReadIssueTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner":  map[string]any{"type": "string"},
			"repo":   map[string]any{"type": "string"},
			"number": map[string]any{"type": "integer", "description": "Issue number"},
		},
		"required": []string{"owner", "repo", "number"},
	}
}

func (t *GhReadIssueTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	number, ok := args["number"].(float64)
	if owner == "" || repo == "" || !ok {
		return ErrorResult("owner, repo, and number are required")
	}
	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured")
	}

	c := &ghClient{token: tok}

	// Fetch issue
	data, status, err := c.do(ctx, "GET", fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, int(number)), nil)
	if err != nil {
		return ErrorResult("request failed: " + err.Error())
	}
	if status != 200 {
		return ghError(status, data)
	}

	var iss struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		Body      string `json:"body"`
		State     string `json:"state"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		HTMLURL   string `json:"html_url"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
		Assignees []struct {
			Login string `json:"login"`
		} `json:"assignees"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		Milestone *struct {
			Title string `json:"title"`
		} `json:"milestone"`
		Comments int `json:"comments"`
	}
	if err := json.Unmarshal(data, &iss); err != nil {
		return ErrorResult("parse failed: " + err.Error())
	}

	sb := &strings.Builder{}
	fmt.Fprintf(sb, "## Issue #%d: %s\n\n", iss.Number, iss.Title)
	fmt.Fprintf(sb, "**State:** %s  |  **Author:** @%s  |  **Created:** %s\n", iss.State, iss.User.Login, iss.CreatedAt[:10])

	if len(iss.Labels) > 0 {
		labels := make([]string, len(iss.Labels))
		for i, l := range iss.Labels {
			labels[i] = l.Name
		}
		fmt.Fprintf(sb, "**Labels:** %s\n", strings.Join(labels, ", "))
	}
	if len(iss.Assignees) > 0 {
		assignees := make([]string, len(iss.Assignees))
		for i, a := range iss.Assignees {
			assignees[i] = "@" + a.Login
		}
		fmt.Fprintf(sb, "**Assignees:** %s\n", strings.Join(assignees, ", "))
	}
	if iss.Milestone != nil {
		fmt.Fprintf(sb, "**Milestone:** %s\n", iss.Milestone.Title)
	}
	fmt.Fprintf(sb, "\n### Description\n\n%s\n", iss.Body)

	// Fetch comments
	if iss.Comments > 0 {
		cData, cStatus, cErr := c.do(ctx, "GET",
			fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=30", owner, repo, int(number)), nil)
		if cErr == nil && cStatus == 200 {
			var comments []struct {
				User    struct{ Login string } `json:"user"`
				Body    string                `json:"body"`
				Created string                `json:"created_at"`
			}
			if json.Unmarshal(cData, &comments) == nil && len(comments) > 0 {
				fmt.Fprintf(sb, "\n### Comments (%d)\n\n", len(comments))
				for _, cm := range comments {
					fmt.Fprintf(sb, "**@%s** (%s):\n%s\n\n", cm.User.Login, cm.Created[:10], cm.Body)
				}
			}
		}
	}

	return TextResult(sb.String())
}

// ─── gh_create_issue ─────────────────────────────────────────────────────────

// GhCreateIssueTool creates a new GitHub issue.
type GhCreateIssueTool struct{ getToken tokenGetter }

func NewGhCreateIssueTool() *GhCreateIssueTool { return &GhCreateIssueTool{} }

func (t *GhCreateIssueTool) Name() string { return "gh_create_issue" }
func (t *GhCreateIssueTool) Description() string {
	return `Create a new GitHub issue. Use this to report bugs, request features, or track work items.
After creating, post the issue URL to the room so the team knows about it.`
}
func (t *GhCreateIssueTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner":     map[string]any{"type": "string"},
			"repo":      map[string]any{"type": "string"},
			"title":     map[string]any{"type": "string", "description": "Issue title"},
			"body":      map[string]any{"type": "string", "description": "Issue body (markdown supported)"},
			"labels":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Label names"},
			"assignees": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "GitHub usernames to assign"},
		},
		"required": []string{"owner", "repo", "title"},
	}
}

func (t *GhCreateIssueTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	title, _ := args["title"].(string)
	if owner == "" || repo == "" || title == "" {
		return ErrorResult("owner, repo, and title are required")
	}
	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured")
	}

	payload := map[string]any{"title": title}
	if body, ok := args["body"].(string); ok && body != "" {
		payload["body"] = body
	}
	if labels, ok := args["labels"].([]any); ok && len(labels) > 0 {
		payload["labels"] = labels
	}
	if assignees, ok := args["assignees"].([]any); ok && len(assignees) > 0 {
		payload["assignees"] = assignees
	}

	c := &ghClient{token: tok}
	data, status, err := c.do(ctx, "POST", fmt.Sprintf("/repos/%s/%s/issues", owner, repo), payload)
	if err != nil {
		return ErrorResult("request failed: " + err.Error())
	}
	if status != 201 {
		return ghError(status, data)
	}

	var created struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		Title   string `json:"title"`
	}
	if err := json.Unmarshal(data, &created); err != nil {
		return ErrorResult("parse failed: " + err.Error())
	}

	return TextResult(fmt.Sprintf("✅ Created issue #%d: **%s**\nURL: %s", created.Number, created.Title, created.HTMLURL))
}

// ─── gh_create_branch ────────────────────────────────────────────────────────

// GhCreateBranchTool creates a new branch from a base (default: main).
type GhCreateBranchTool struct{ getToken tokenGetter }

func NewGhCreateBranchTool() *GhCreateBranchTool { return &GhCreateBranchTool{} }

func (t *GhCreateBranchTool) Name() string { return "gh_create_branch" }
func (t *GhCreateBranchTool) Description() string {
	return `Create a new branch in a GitHub repository from a base branch.
Always create a branch before making changes — never commit directly to main.
Naming convention: "feat/issue-{number}-{short-description}" or "fix/issue-{number}-{desc}".`
}
func (t *GhCreateBranchTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner":  map[string]any{"type": "string"},
			"repo":   map[string]any{"type": "string"},
			"branch": map[string]any{"type": "string", "description": "New branch name, e.g. 'feat/issue-42-add-login'"},
			"from":   map[string]any{"type": "string", "description": "Base branch (default: repo default branch)"},
		},
		"required": []string{"owner", "repo", "branch"},
	}
}

func (t *GhCreateBranchTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	branch, _ := args["branch"].(string)
	if owner == "" || repo == "" || branch == "" {
		return ErrorResult("owner, repo, and branch are required")
	}
	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured")
	}

	c := &ghClient{token: tok}

	// Determine base branch
	baseBranch := ""
	if from, ok := args["from"].(string); ok && from != "" {
		baseBranch = from
	} else {
		// Fetch repo default branch
		repoData, repoStatus, repoErr := c.do(ctx, "GET", fmt.Sprintf("/repos/%s/%s", owner, repo), nil)
		if repoErr != nil || repoStatus != 200 {
			return ErrorResult("failed to fetch repo info to determine default branch")
		}
		var repoInfo struct {
			DefaultBranch string `json:"default_branch"`
		}
		if json.Unmarshal(repoData, &repoInfo) == nil {
			baseBranch = repoInfo.DefaultBranch
		}
		if baseBranch == "" {
			baseBranch = "main"
		}
	}

	// Get SHA of base branch tip
	refData, refStatus, refErr := c.do(ctx, "GET",
		fmt.Sprintf("/repos/%s/%s/git/ref/heads/%s", owner, repo, baseBranch), nil)
	if refErr != nil {
		return ErrorResult("get base ref failed: " + refErr.Error())
	}
	if refStatus != 200 {
		return ghError(refStatus, refData)
	}

	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.Unmarshal(refData, &ref); err != nil || ref.Object.SHA == "" {
		return ErrorResult("failed to parse base branch SHA")
	}

	// Create new ref
	payload := map[string]any{
		"ref": "refs/heads/" + branch,
		"sha": ref.Object.SHA,
	}
	createData, createStatus, createErr := c.do(ctx, "POST",
		fmt.Sprintf("/repos/%s/%s/git/refs", owner, repo), payload)
	if createErr != nil {
		return ErrorResult("create branch failed: " + createErr.Error())
	}
	if createStatus != 201 {
		return ghError(createStatus, createData)
	}

	return TextResult(fmt.Sprintf("✅ Created branch `%s` from `%s` in %s/%s\nSHA: %s",
		branch, baseBranch, owner, repo, ref.Object.SHA[:8]))
}

// ─── gh_push_file ─────────────────────────────────────────────────────────────

// GhPushFileTool creates or updates a single file in a GitHub repository.
// For multi-file changes use exec + git push via the exec tool instead.
type GhPushFileTool struct{ getToken tokenGetter }

func NewGhPushFileTool() *GhPushFileTool { return &GhPushFileTool{} }

func (t *GhPushFileTool) Name() string { return "gh_push_file" }
func (t *GhPushFileTool) Description() string {
	return `Create or update a single file in a GitHub repository via the Contents API.
For large multi-file changes, use exec to run git commands in the workspace instead.
Requires: owner, repo, path (file path in repo), content (file text), branch, message (commit message).`
}
func (t *GhPushFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner":   map[string]any{"type": "string"},
			"repo":    map[string]any{"type": "string"},
			"path":    map[string]any{"type": "string", "description": "File path in repo, e.g. 'src/main.go'"},
			"content": map[string]any{"type": "string", "description": "Full file content (UTF-8 text)"},
			"branch":  map[string]any{"type": "string", "description": "Branch to commit to"},
			"message": map[string]any{"type": "string", "description": "Commit message"},
		},
		"required": []string{"owner", "repo", "path", "content", "branch", "message"},
	}
}

func (t *GhPushFileTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	branch, _ := args["branch"].(string)
	message, _ := args["message"].(string)
	if owner == "" || repo == "" || path == "" || content == "" || branch == "" || message == "" {
		return ErrorResult("owner, repo, path, content, branch, and message are all required")
	}
	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured")
	}

	c := &ghClient{token: tok}

	// Check if file exists — need its SHA for updates
	existingData, existingStatus, _ := c.do(ctx, "GET",
		fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, branch), nil)

	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	payload := map[string]any{
		"message": message,
		"content": encoded,
		"branch":  branch,
	}

	// If file exists, include its SHA so GitHub accepts the update
	if existingStatus == 200 {
		var existing struct {
			SHA string `json:"sha"`
		}
		if json.Unmarshal(existingData, &existing) == nil && existing.SHA != "" {
			payload["sha"] = existing.SHA
		}
	}

	data, status, err := c.do(ctx, "PUT",
		fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, path), payload)
	if err != nil {
		return ErrorResult("request failed: " + err.Error())
	}
	if status != 200 && status != 201 {
		return ghError(status, data)
	}

	verb := "Updated"
	if existingStatus == 404 {
		verb = "Created"
	}

	var result struct {
		Content struct {
			HTMLURL string `json:"html_url"`
		} `json:"content"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	json.Unmarshal(data, &result)

	return TextResult(fmt.Sprintf("✅ %s `%s` on branch `%s`\nCommit: %s\nURL: %s",
		verb, path, branch, result.Commit.SHA[:8], result.Content.HTMLURL))
}

// ─── gh_open_pr ───────────────────────────────────────────────────────────────

// GhOpenPRTool opens a pull request.
type GhOpenPRTool struct{ getToken tokenGetter }

func NewGhOpenPRTool() *GhOpenPRTool { return &GhOpenPRTool{} }

func (t *GhOpenPRTool) Name() string { return "gh_open_pr" }
func (t *GhOpenPRTool) Description() string {
	return `Open a pull request. After pushing code to a branch, call this to request review.
Write a clear description explaining what changed and why. Reference the issue with "Closes #N".
Post the PR URL to the room using room_post after creation.`
}
func (t *GhOpenPRTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner":  map[string]any{"type": "string"},
			"repo":   map[string]any{"type": "string"},
			"title":  map[string]any{"type": "string", "description": "PR title"},
			"body":   map[string]any{"type": "string", "description": "PR description (markdown). Include 'Closes #N' to link issues."},
			"head":   map[string]any{"type": "string", "description": "Source branch (the one with your changes)"},
			"base":   map[string]any{"type": "string", "description": "Target branch (default: main)"},
			"draft":  map[string]any{"type": "boolean", "description": "Open as draft PR (default: false)"},
		},
		"required": []string{"owner", "repo", "title", "head"},
	}
}

func (t *GhOpenPRTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	title, _ := args["title"].(string)
	head, _ := args["head"].(string)
	if owner == "" || repo == "" || title == "" || head == "" {
		return ErrorResult("owner, repo, title, and head branch are required")
	}
	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured")
	}

	base := "main"
	if b, ok := args["base"].(string); ok && b != "" {
		base = b
	}
	draft := false
	if d, ok := args["draft"].(bool); ok {
		draft = d
	}

	payload := map[string]any{
		"title": title,
		"head":  head,
		"base":  base,
		"draft": draft,
	}
	if body, ok := args["body"].(string); ok && body != "" {
		payload["body"] = body
	}

	c := &ghClient{token: tok}
	data, status, err := c.do(ctx, "POST", fmt.Sprintf("/repos/%s/%s/pulls", owner, repo), payload)
	if err != nil {
		return ErrorResult("request failed: " + err.Error())
	}
	if status != 201 {
		return ghError(status, data)
	}

	var pr struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		Title   string `json:"title"`
		State   string `json:"state"`
		Draft   bool   `json:"draft"`
	}
	if err := json.Unmarshal(data, &pr); err != nil {
		return ErrorResult("parse failed: " + err.Error())
	}

	draftLabel := ""
	if pr.Draft {
		draftLabel = " (draft)"
	}
	return TextResult(fmt.Sprintf("✅ Opened PR #%d%s: **%s**\n`%s` → `%s`\nURL: %s",
		pr.Number, draftLabel, pr.Title, head, base, pr.HTMLURL))
}

// ─── gh_post_comment ─────────────────────────────────────────────────────────

// GhPostCommentTool posts a comment on an issue or PR.
type GhPostCommentTool struct{ getToken tokenGetter }

func NewGhPostCommentTool() *GhPostCommentTool { return &GhPostCommentTool{} }

func (t *GhPostCommentTool) Name() string { return "gh_post_comment" }
func (t *GhPostCommentTool) Description() string {
	return `Post a comment on a GitHub issue or pull request.
Use this to report progress, ask questions, or provide review feedback.
The comment body supports GitHub Flavored Markdown.`
}
func (t *GhPostCommentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner":  map[string]any{"type": "string"},
			"repo":   map[string]any{"type": "string"},
			"number": map[string]any{"type": "integer", "description": "Issue or PR number"},
			"body":   map[string]any{"type": "string", "description": "Comment body (markdown supported)"},
		},
		"required": []string{"owner", "repo", "number", "body"},
	}
}

func (t *GhPostCommentTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	number, ok := args["number"].(float64)
	body, _ := args["body"].(string)
	if owner == "" || repo == "" || !ok || body == "" {
		return ErrorResult("owner, repo, number, and body are required")
	}
	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured")
	}

	c := &ghClient{token: tok}
	data, status, err := c.do(ctx, "POST",
		fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, int(number)),
		map[string]any{"body": body})
	if err != nil {
		return ErrorResult("request failed: " + err.Error())
	}
	if status != 201 {
		return ghError(status, data)
	}

	var comment struct {
		ID      int    `json:"id"`
		HTMLURL string `json:"html_url"`
	}
	json.Unmarshal(data, &comment)

	return TextResult(fmt.Sprintf("✅ Posted comment on #%d\nURL: %s", int(number), comment.HTMLURL))
}

// ─── gh_list_pr_checks ───────────────────────────────────────────────────────

// GhListPRChecksTool lists CI check results for a PR or commit SHA.
type GhListPRChecksTool struct{ getToken tokenGetter }

func NewGhListPRChecksTool() *GhListPRChecksTool { return &GhListPRChecksTool{} }

func (t *GhListPRChecksTool) Name() string { return "gh_list_pr_checks" }
func (t *GhListPRChecksTool) Description() string {
	return `List CI check run results for a pull request or commit SHA.
Shows check name, status (queued/in_progress/completed), and conclusion (success/failure/neutral/cancelled).
Use this to know whether CI is passing before merging.`
}
func (t *GhListPRChecksTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner":  map[string]any{"type": "string"},
			"repo":   map[string]any{"type": "string"},
			"pr":     map[string]any{"type": "integer", "description": "PR number (used to resolve the HEAD SHA)"},
			"sha":    map[string]any{"type": "string", "description": "Commit SHA (alternative to pr number)"},
		},
		"required": []string{"owner", "repo"},
	}
}

func (t *GhListPRChecksTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	if owner == "" || repo == "" {
		return ErrorResult("owner and repo are required")
	}
	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured")
	}

	c := &ghClient{token: tok}

	// Resolve SHA from PR number if needed
	sha := ""
	if s, ok := args["sha"].(string); ok && s != "" {
		sha = s
	} else if prNum, ok := args["pr"].(float64); ok {
		prData, prStatus, prErr := c.do(ctx, "GET",
			fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, int(prNum)), nil)
		if prErr != nil || prStatus != 200 {
			return ErrorResult(fmt.Sprintf("failed to fetch PR #%d", int(prNum)))
		}
		var pr struct {
			Head struct {
				SHA string `json:"sha"`
			} `json:"head"`
		}
		if json.Unmarshal(prData, &pr) == nil {
			sha = pr.Head.SHA
		}
	}
	if sha == "" {
		return ErrorResult("provide either pr number or sha")
	}

	data, status, err := c.do(ctx, "GET",
		fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs", owner, repo, sha), nil)
	if err != nil {
		return ErrorResult("request failed: " + err.Error())
	}
	if status != 200 {
		return ghError(status, data)
	}

	var result struct {
		TotalCount int `json:"total_count"`
		CheckRuns  []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HTMLURL    string `json:"html_url"`
			StartedAt  string `json:"started_at"`
		} `json:"check_runs"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return ErrorResult("parse failed: " + err.Error())
	}

	if result.TotalCount == 0 {
		return TextResult(fmt.Sprintf("No CI checks found for SHA %s (CI may not be configured)", sha[:8]))
	}

	sb := &strings.Builder{}
	fmt.Fprintf(sb, "**CI checks for %s (SHA: %s):**\n\n", owner+"/"+repo, sha[:8])

	allPassing := true
	for _, run := range result.CheckRuns {
		icon := "⏳"
		switch run.Conclusion {
		case "success":
			icon = "✅"
		case "failure":
			icon = "❌"
			allPassing = false
		case "cancelled":
			icon = "⚪"
		case "neutral":
			icon = "➖"
		default:
			if run.Status == "in_progress" {
				icon = "🔄"
				allPassing = false
			} else if run.Status == "queued" {
				icon = "⏳"
				allPassing = false
			}
		}
		conclusion := run.Conclusion
		if conclusion == "" {
			conclusion = run.Status
		}
		fmt.Fprintf(sb, "%s **%s** — %s\n", icon, run.Name, conclusion)
	}

	if allPassing && result.TotalCount > 0 {
		fmt.Fprintf(sb, "\n✅ All %d checks passing", result.TotalCount)
	} else {
		failing := 0
		for _, r := range result.CheckRuns {
			if r.Conclusion == "failure" {
				failing++
			}
		}
		if failing > 0 {
			fmt.Fprintf(sb, "\n❌ %d/%d checks failing — review logs before merging", failing, result.TotalCount)
		}
	}

	return TextResult(sb.String())
}

// ─── gh_task_register ────────────────────────────────────────────────────────

// GhTaskRegisterTool lets an agent formally commit to working on a GitHub issue.
// Once registered, the QOROS loop injects per-tick guidance until the task completes.
type GhTaskRegisterTool struct{}

func NewGhTaskRegisterTool() *GhTaskRegisterTool { return &GhTaskRegisterTool{} }

func (t *GhTaskRegisterTool) Name() string { return "gh_task_register" }
func (t *GhTaskRegisterTool) Description() string {
	return `Register a GitHub issue as an active development task.
Call this ONCE after reading an issue with gh_read_issue, before creating a branch.
The system will guide you through each phase: branch → code → test → PR → CI → merge.
You will receive context on every tick telling you exactly what to do next.

This is your formal commitment to autonomously complete the issue.`
}
func (t *GhTaskRegisterTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner":        map[string]any{"type": "string"},
			"repo":         map[string]any{"type": "string"},
			"issue_number": map[string]any{"type": "integer"},
			"branch":       map[string]any{"type": "string", "description": "Branch name to create, e.g. 'fix/issue-42-login-timeout'"},
			"room_id":      map[string]any{"type": "string", "description": "Optional room ID to post progress updates to"},
		},
		"required": []string{"owner", "repo", "issue_number", "branch"},
	}
}

func (t *GhTaskRegisterTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	branch, _ := args["branch"].(string)
	issueNum, ok := args["issue_number"].(float64)
	if owner == "" || repo == "" || branch == "" || !ok {
		return ErrorResult("owner, repo, branch, and issue_number are required")
	}
	roomID, _ := args["room_id"].(string)
	agentID := AgentIDFromCtx(ctx)
	if agentID == "" {
		agentID = "agent"
	}

	// Use the global task queue singleton
	import_agent_task_queue(owner, repo, branch, roomID, agentID, int(issueNum))

	return TextResult(fmt.Sprintf(
		"✅ Task registered: %s/%s#%d → branch `%s`\n"+
			"QOROS will guide you through each phase on every tick.\n"+
			"Next step: call gh_create_branch to create the branch.",
		owner, repo, int(issueNum), branch,
	))
}

// import_agent_task_queue is called by the tool to register in the global queue.
// It's a thin wrapper to avoid importing the agent package from tools (circular dep).
// The actual registration happens via the exported GlobalGitHubTaskQueue.
var githubTaskRegisterFn func(agentID, owner, repo, branch, roomID string, issueNum int) string

// SetGitHubTaskRegisterFn is called by gateway to inject the registration callback.
func SetGitHubTaskRegisterFn(fn func(agentID, owner, repo, branch, roomID string, issueNum int) string) {
	githubTaskRegisterFn = fn
}

func import_agent_task_queue(owner, repo, branch, roomID, agentID string, issueNum int) string {
	if githubTaskRegisterFn != nil {
		return githubTaskRegisterFn(agentID, owner, repo, branch, roomID, issueNum)
	}
	return ""
}

// ─── gh_merge_pr ─────────────────────────────────────────────────────────────

// GhMergePRTool merges a pull request once CI is green and reviews are approved.
type GhMergePRTool struct{ getToken tokenGetter }

func NewGhMergePRTool() *GhMergePRTool { return &GhMergePRTool{} }

func (t *GhMergePRTool) Name() string { return "gh_merge_pr" }
func (t *GhMergePRTool) Description() string {
	return `Merge a pull request. Only call this when:
1. All CI checks are passing (verify with gh_list_pr_checks first)
2. The PR has been reviewed (check gh_read_issue for review comments)
3. The coordinator agent has explicitly approved the merge in the room.
Merge method: squash (default), merge, or rebase.`
}
func (t *GhMergePRTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner":         map[string]any{"type": "string"},
			"repo":          map[string]any{"type": "string"},
			"pr":            map[string]any{"type": "integer", "description": "PR number"},
			"merge_method":  map[string]any{"type": "string", "enum": []string{"squash", "merge", "rebase"}, "description": "Default: squash"},
			"commit_title":  map[string]any{"type": "string", "description": "Custom commit title (squash/merge only)"},
		},
		"required": []string{"owner", "repo", "pr"},
	}
}

func (t *GhMergePRTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	pr, ok := args["pr"].(float64)
	if owner == "" || repo == "" || !ok {
		return ErrorResult("owner, repo, and pr number are required")
	}
	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured")
	}

	method := "squash"
	if m, ok := args["merge_method"].(string); ok && m != "" {
		method = m
	}

	payload := map[string]any{"merge_method": method}
	if title, ok := args["commit_title"].(string); ok && title != "" {
		payload["commit_title"] = title
	}

	c := &ghClient{token: tok}
	data, status, err := c.do(ctx, "PUT",
		fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, repo, int(pr)), payload)
	if err != nil {
		return ErrorResult("request failed: " + err.Error())
	}
	if status == 405 {
		return ErrorResult("PR cannot be merged — CI may still be running or there are merge conflicts")
	}
	if status != 200 {
		return ghError(status, data)
	}

	var result struct {
		SHA     string `json:"sha"`
		Merged  bool   `json:"merged"`
		Message string `json:"message"`
	}
	json.Unmarshal(data, &result)

	return TextResult(fmt.Sprintf("✅ Merged PR #%d into %s/%s\nCommit: %s\n%s",
		int(pr), owner, repo, result.SHA[:8], result.Message))
}

// ─── gh_create_repo ────────────────────────────────────────────────────────────

// GhCreateRepoTool creates a new GitHub repository.
type GhCreateRepoTool struct{ getToken tokenGetter }

func NewGhCreateRepoTool() *GhCreateRepoTool { return &GhCreateRepoTool{} }
func NewGhCreateRepoToolWithToken(get tokenGetter) *GhCreateRepoTool {
	t := NewGhCreateRepoTool(); t.getToken = get; return t
}

func (t *GhCreateRepoTool) Name() string { return "gh_create_repo" }
func (t *GhCreateRepoTool) Description() string {
	return `Create a new GitHub repository. Use this at the start of a project build to create the repo where code will be pushed.
Returns the full repo name (owner/repo) and clone URL.
After creating, use gh_create_branch, gh_push_file, and gh_open_pr to push code.`
}
func (t *GhCreateRepoTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":        map[string]any{"type": "string", "description": "Repository name (kebab-case, e.g. 'my-saas-app')"},
			"description": map[string]any{"type": "string", "description": "Short description of the project"},
			"private":     map[string]any{"type": "boolean", "description": "Create as private repo (default: true)"},
			"auto_init":   map[string]any{"type": "boolean", "description": "Initialize with README (default: true)"},
		},
		"required": []string{"name"},
	}
}

func (t *GhCreateRepoTool) Execute(ctx context.Context, args map[string]any) *Result {
	name, _ := args["name"].(string)
	if name == "" {
		return ErrorResult("name is required")
	}
	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured — add one in Settings → Provider Keys → GitHub")
	}

	private := true
	if v, ok := args["private"].(bool); ok {
		private = v
	}
	autoInit := true
	if v, ok := args["auto_init"].(bool); ok {
		autoInit = v
	}

	payload := map[string]any{
		"name":      name,
		"private":   private,
		"auto_init": autoInit,
	}
	if desc, ok := args["description"].(string); ok && desc != "" {
		payload["description"] = desc
	}

	c := &ghClient{token: tok}
	data, status, err := c.do(ctx, "POST", "/user/repos", payload)
	if err != nil {
		return ErrorResult("request failed: " + err.Error())
	}
	if status == 422 {
		return ErrorResult("repository name already exists or is invalid — try a different name")
	}
	if status != 201 {
		return ghError(status, data)
	}

	var repo struct {
		FullName  string `json:"full_name"`
		HTMLURL   string `json:"html_url"`
		CloneURL  string `json:"clone_url"`
		SSHURL    string `json:"ssh_url"`
		Private   bool   `json:"private"`
	}
	if err := json.Unmarshal(data, &repo); err != nil {
		return ErrorResult("parse failed: " + err.Error())
	}

	visibility := "public"
	if repo.Private {
		visibility = "private"
	}

	return TextResult(fmt.Sprintf("✅ Created %s repo: **%s**\nURL: %s\nClone: %s",
		visibility, repo.FullName, repo.HTMLURL, repo.CloneURL))
}
