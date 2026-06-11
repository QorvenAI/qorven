// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ─── gh_create_release ────────────────────────────────────────────────────────

// GhCreateReleaseTool creates a GitHub release via the Releases API.
// It is the action executed by the release gate on human approval, and can
// also be called directly by a release-manager agent.
type GhCreateReleaseTool struct{ getToken tokenGetter }

func NewGhCreateReleaseTool() *GhCreateReleaseTool { return &GhCreateReleaseTool{} }
func NewGhCreateReleaseToolWithToken(get tokenGetter) *GhCreateReleaseTool {
	t := NewGhCreateReleaseTool(); t.getToken = get; return t
}

func (t *GhCreateReleaseTool) Name() string { return "gh_create_release" }
func (t *GhCreateReleaseTool) Description() string {
	return `Create a GitHub release. Tags the repo and publishes release notes.
Call this after all PRs for a version have been merged and the release gate has been approved.
Parameters:
  - owner, repo: target repository
  - tag_name: version tag to create (e.g. "v0.1.2")
  - name: release title (defaults to tag_name)
  - body: release notes in markdown
  - draft: publish as draft (default false)
  - prerelease: mark as pre-release (default false)`
}

func (t *GhCreateReleaseTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner":      map[string]any{"type": "string", "description": "GitHub org or user"},
			"repo":       map[string]any{"type": "string", "description": "Repository name"},
			"tag_name":   map[string]any{"type": "string", "description": "Git tag to create, e.g. 'v0.1.2'"},
			"name":       map[string]any{"type": "string", "description": "Release title (defaults to tag_name)"},
			"body":       map[string]any{"type": "string", "description": "Release notes (markdown)"},
			"draft":      map[string]any{"type": "boolean", "description": "Publish as draft (default false)"},
			"prerelease": map[string]any{"type": "boolean", "description": "Mark as pre-release (default false)"},
		},
		"required": []string{"owner", "repo", "tag_name"},
	}
}

func (t *GhCreateReleaseTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	tagName, _ := args["tag_name"].(string)
	if owner == "" || repo == "" || tagName == "" {
		return ErrorResult("owner, repo, and tag_name are required")
	}

	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured — add one in Settings → Provider Keys → GitHub")
	}

	releaseName := tagName
	if n, ok := args["name"].(string); ok && n != "" {
		releaseName = n
	}

	payload := map[string]any{
		"tag_name": tagName,
		"name":     releaseName,
	}
	if body, ok := args["body"].(string); ok && body != "" {
		payload["body"] = body
	}
	if draft, ok := args["draft"].(bool); ok {
		payload["draft"] = draft
	}
	if pre, ok := args["prerelease"].(bool); ok {
		payload["prerelease"] = pre
	}

	c := &ghClient{token: tok}
	data, status, err := c.do(ctx, "POST",
		fmt.Sprintf("/repos/%s/%s/releases", owner, repo), payload)
	if err != nil {
		return ErrorResult("request failed: " + err.Error())
	}
	if status != 201 {
		return ghError(status, data)
	}

	var release struct {
		ID      int    `json:"id"`
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
		HTMLURL string `json:"html_url"`
		Draft   bool   `json:"draft"`
	}
	if err := json.Unmarshal(data, &release); err != nil {
		return ErrorResult("parse failed: " + err.Error())
	}

	draftLabel := ""
	if release.Draft {
		draftLabel = " (draft)"
	}
	return TextResult(fmt.Sprintf("Created release%s **%s** (%s) in %s/%s\nURL: %s",
		draftLabel, release.Name, release.TagName, owner, repo, release.HTMLURL))
}
