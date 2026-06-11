// Copyright 2026 Tekky AI Academy LLP. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ─── gh_submit_review ─────────────────────────────────────────────────────────

// GhSubmitReviewTool submits a formal GitHub PR review.
type GhSubmitReviewTool struct{ getToken tokenGetter }

func NewGhSubmitReviewTool() *GhSubmitReviewTool { return &GhSubmitReviewTool{} }
func NewGhSubmitReviewToolWithToken(get tokenGetter) *GhSubmitReviewTool {
	t := NewGhSubmitReviewTool(); t.getToken = get; return t
}

func (t *GhSubmitReviewTool) Name() string { return "gh_submit_review" }
func (t *GhSubmitReviewTool) Description() string {
	return `Submit a formal GitHub pull request review (APPROVE, REQUEST_CHANGES, or COMMENT).
APPROVE and REQUEST_CHANGES gate whether the PR can be merged (subject to branch protection rules).
COMMENT posts an informational review without approving or blocking.
Optional line-level comments anchor to specific file locations:
  - path: repo-relative file path (e.g. "src/main.go")
  - line: the line number in the file (1-based)
  - side: "RIGHT" for the new version of the file, "LEFT" for the old version
  - body: the comment text
Use this after reviewing code with gh_repo_info or reading file contents. Post a summary to the
room with room_post after submitting.`
}

func (t *GhSubmitReviewTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"owner":       map[string]any{"type": "string", "description": "Repository owner (user or org)"},
			"repo":        map[string]any{"type": "string", "description": "Repository name"},
			"pull_number": map[string]any{"type": "integer", "description": "PR number to review"},
			"event": map[string]any{
				"type":        "string",
				"enum":        []string{"APPROVE", "REQUEST_CHANGES", "COMMENT"},
				"description": "Review decision: APPROVE merges, REQUEST_CHANGES blocks, COMMENT is informational",
			},
			"body": map[string]any{
				"type":        "string",
				"description": "Overall review summary (markdown). Required when event is REQUEST_CHANGES.",
			},
			"comments": map[string]any{
				"type":        "array",
				"description": "Optional inline line comments anchored to specific file locations",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "Repo-relative file path"},
						"line": map[string]any{"type": "integer", "description": "Line number (1-based)"},
						"side": map[string]any{
							"type":        "string",
							"enum":        []string{"RIGHT", "LEFT"},
							"description": "RIGHT = new version of file, LEFT = old version",
						},
						"body": map[string]any{"type": "string", "description": "Comment text"},
					},
					"required": []string{"path", "line", "body"},
				},
			},
		},
		"required": []string{"owner", "repo", "pull_number", "event"},
	}
}

func (t *GhSubmitReviewTool) Execute(ctx context.Context, args map[string]any) *Result {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	event, _ := args["event"].(string)

	if owner == "" || repo == "" {
		return ErrorResult("owner and repo are required")
	}

	// pull_number arrives as float64 from JSON unmarshalling.
	prFloat, ok := args["pull_number"].(float64)
	if !ok || prFloat <= 0 {
		return ErrorResult("pull_number is required and must be a positive integer")
	}
	pullNumber := int(prFloat)

	// Validate event.
	switch event {
	case "APPROVE", "REQUEST_CHANGES", "COMMENT":
		// valid
	case "":
		return ErrorResult("event is required: APPROVE, REQUEST_CHANGES, or COMMENT")
	default:
		return ErrorResult(fmt.Sprintf("invalid event %q: must be APPROVE, REQUEST_CHANGES, or COMMENT", event))
	}

	tok := ghResolveToken(ctx, t.getToken)
	if tok == "" {
		return ErrorResult("no GitHub token configured")
	}

	payload := map[string]any{
		"event": event,
	}
	if body, ok := args["body"].(string); ok && body != "" {
		payload["body"] = body
	}

	// Build inline comments array if provided and non-empty.
	if raw, ok := args["comments"].([]any); ok && len(raw) > 0 {
		comments := make([]map[string]any, 0, len(raw))
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			c := map[string]any{}
			if p, ok := m["path"].(string); ok && p != "" {
				c["path"] = p
			}
			if l, ok := m["line"].(float64); ok && l > 0 {
				c["line"] = int(l)
			}
			if s, ok := m["side"].(string); ok && (s == "RIGHT" || s == "LEFT") {
				c["side"] = s
			} else {
				c["side"] = "RIGHT" // sensible default
			}
			if b, ok := m["body"].(string); ok && b != "" {
				c["body"] = b
			}
			if _, hasPath := c["path"]; hasPath {
				if _, hasLine := c["line"]; hasLine {
					if _, hasBody := c["body"]; hasBody {
						comments = append(comments, c)
					}
				}
			}
		}
		if len(comments) > 0 {
			payload["comments"] = comments
		}
	}

	c := &ghClient{token: tok}
	data, status, err := c.do(ctx, "POST",
		fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, pullNumber), payload)
	if err != nil {
		return ErrorResult("request failed: " + err.Error())
	}
	if status >= 400 {
		return ghError(status, data)
	}

	var review struct {
		ID    int    `json:"id"`
		State string `json:"state"`
		Body  string `json:"body"`
		User  struct {
			Login string `json:"login"`
		} `json:"user"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(data, &review); err != nil {
		return ErrorResult("parse failed: " + err.Error())
	}

	stateLabel := review.State
	switch stateLabel {
	case "APPROVED":
		stateLabel = "APPROVED ✅"
	case "CHANGES_REQUESTED":
		stateLabel = "CHANGES REQUESTED ⚠️"
	case "COMMENTED":
		stateLabel = "COMMENT 💬"
	}

	return SuccessResult(fmt.Sprintf("Submitted review #%d on PR #%d — %s\nURL: %s",
		review.ID, pullNumber, stateLabel, review.HTMLURL))
}
